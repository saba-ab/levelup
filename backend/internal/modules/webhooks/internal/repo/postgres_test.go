package repo

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/webhooks/contracts"
	"levelup/internal/modules/webhooks/internal/app"
	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/modules/webhooks/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "webhooks", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "webhooks")
	return NewPostgres(moduleDB), moduleDB
}

func inTx(t *testing.T, db *gorm.DB, fn func(tx *gorm.DB) error) {
	t.Helper()
	require.NoError(t, postgres.InTx(context.Background(), db, fn))
}

func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func seedEndpoint(t *testing.T, r *Postgres, db *gorm.DB, tenantID string, types ...string) domain.Endpoint {
	t.Helper()
	e, err := domain.NewEndpoint(tenantID, domain.NewEndpointInput{
		URL: "https://hooks.example.com/x", Description: "d", EventTypes: types, Active: true,
	}, domain.URLPolicy{}, now())
	require.NoError(t, err)
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateEndpoint(context.Background(), tx, e) })
	return e
}

func TestEndpointRoundTripVersionAndTenancy(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	e := seedEndpoint(t, r, db, tenant, "points.credited", "badges.awarded")

	got, err := r.EndpointByID(ctx, tenant, e.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"badges.awarded", "points.credited"}, got.EventTypes, "text[] round trip")
	require.Equal(t, e.Secret, got.Secret)
	require.True(t, got.Active)

	_, err = r.EndpointByID(ctx, id.NewID(), e.ID)
	require.ErrorIs(t, err, domain.ErrEndpointNotFound, "another tenant's endpoint is not found")
	_, err = r.EndpointByID(ctx, tenant, "nope")
	require.ErrorIs(t, err, domain.ErrEndpointNotFound)

	// Version guard.
	got.Description = "changed"
	inTx(t, db, func(tx *gorm.DB) error { return r.SaveEndpoint(ctx, tx, got, false) })
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveEndpoint(ctx, tx, got, false) })
	require.ErrorIs(t, err, domain.ErrVersionConflict)

	n, err := r.CountEndpoints(ctx, tenant)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	inTx(t, db, func(tx *gorm.DB) error { return r.SoftDeleteEndpoint(ctx, tx, tenant, e.ID, now()) })
	_, err = r.EndpointByID(ctx, tenant, e.ID)
	require.ErrorIs(t, err, domain.ErrEndpointNotFound)
	n, err = r.CountEndpoints(ctx, tenant)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestActiveEndpointsForMatchesEventAndWildcard(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	badges := seedEndpoint(t, r, db, tenant, "badges.awarded")
	all := seedEndpoint(t, r, db, tenant, "*")
	points := seedEndpoint(t, r, db, tenant, "points.credited")
	off := seedEndpoint(t, r, db, tenant, "*")
	gone := seedEndpoint(t, r, db, tenant, "badges.awarded")
	seedEndpoint(t, r, db, id.NewID(), "*")
	inTx(t, db, func(tx *gorm.DB) error {
		_, err := r.DisableEndpoint(ctx, tx, tenant, off.ID, "manual", now())
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error { return r.SoftDeleteEndpoint(ctx, tx, tenant, gone.ID, now()) })

	got, err := r.ActiveEndpointsFor(ctx, tenant, "badges.awarded")
	require.NoError(t, err)
	ids := []string{}
	for _, e := range got {
		ids = append(ids, e.ID)
	}
	require.ElementsMatch(t, []string{badges.ID, all.ID}, ids)

	got, err = r.ActiveEndpointsFor(ctx, tenant, "points.credited")
	require.NoError(t, err)
	require.True(t, containsEndpoint(got, points.ID))
	require.True(t, containsEndpoint(got, all.ID))
	require.Len(t, got, 2)
}

func TestInsertDeliveryIsIdempotentPerEndpointAndEvent(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	e1 := seedEndpoint(t, r, db, tenant, "*")
	e2 := seedEndpoint(t, r, db, tenant, "*")

	insert := func(endpointID, eventID string) bool {
		var ok bool
		inTx(t, db, func(tx *gorm.DB) error {
			var err error
			ok, err = r.InsertDelivery(ctx, tx, domain.NewDelivery(tenant, endpointID, eventID, "badges.awarded", []byte(`{"a":1}`), now()))
			return err
		})
		return ok
	}
	require.True(t, insert(e1.ID, "ev-1"))
	require.False(t, insert(e1.ID, "ev-1"), "same endpoint+event: nothing inserted")
	require.True(t, insert(e2.ID, "ev-1"), "another endpoint gets its own delivery")
	require.True(t, insert(e1.ID, "ev-2"))

	// Concurrent redeliveries of the same fact: exactly one row wins.
	var wg sync.WaitGroup
	wins := make(chan bool, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = postgres.InTx(ctx, db, func(tx *gorm.DB) error {
				ok, err := r.InsertDelivery(ctx, tx, domain.NewDelivery(tenant, e1.ID, "ev-race", "badges.awarded", []byte(`{}`), now()))
				wins <- ok
				return err
			})
		}()
	}
	wg.Wait()
	close(wins)
	n := 0
	for ok := range wins {
		if ok {
			n++
		}
	}
	require.Equal(t, 1, n)

	rows, err := r.ListDeliveries(ctx, tenant, app.DeliveryFilter{EndpointID: e1.ID}, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 3)
}

func TestDeliveryLifecycleAndStaleSweepQuery(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	e := seedEndpoint(t, r, db, tenant, "*")
	t0 := now().Add(-time.Hour)

	d := domain.NewDelivery(tenant, e.ID, "ev-1", "badges.awarded", []byte(`{"x":"é"}`), t0)
	inTx(t, db, func(tx *gorm.DB) error { _, err := r.InsertDelivery(ctx, tx, d); return err })

	stale, err := r.StalePending(ctx, now().Add(-10*time.Minute), now(), 100)
	require.NoError(t, err)
	require.True(t, containsDelivery(stale, d.ID))

	// Claim under lock, settle, read back.
	inTx(t, db, func(tx *gorm.DB) error {
		locked, err := r.DeliveryForUpdate(ctx, tx, tenant, d.ID)
		if err != nil {
			return err
		}
		if err := locked.Begin(now(), time.Minute); err != nil {
			return err
		}
		return r.SaveDelivery(ctx, tx, locked)
	})
	stale, err = r.StalePending(ctx, now().Add(-10*time.Minute), now(), 100)
	require.NoError(t, err)
	require.False(t, containsDelivery(stale, d.ID), "leased and just attempted: not stale")

	got, err := r.DeliveryByID(ctx, tenant, d.ID)
	require.NoError(t, err)
	got.Settle(domain.AttemptResult{StatusCode: 502, LatencyMS: 120, Body: "bad gateway\x00"}, true, now())
	inTx(t, db, func(tx *gorm.DB) error { return r.SaveDelivery(ctx, tx, got) })

	got, err = r.DeliveryByID(ctx, tenant, d.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusFailed, got.Status)
	require.Equal(t, 502, *got.ResponseStatus)
	require.Equal(t, int64(120), *got.LatencyMS)
	require.Equal(t, "bad gateway", got.ResponseBody)
	require.Equal(t, 1, got.Attempts)
	require.JSONEq(t, `{"x":"é"}`, string(got.Payload))
	require.Nil(t, got.LeaseUntil)

	_, err = r.DeliveryByID(ctx, id.NewID(), d.ID)
	require.ErrorIs(t, err, domain.ErrDeliveryNotFound, "cross-tenant")

	failed, err := r.ListDeliveries(ctx, tenant, app.DeliveryFilter{Status: domain.StatusFailed, Event: "badges.awarded"}, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, failed, 1)
	pending, err := r.ListDeliveries(ctx, tenant, app.DeliveryFilter{Status: domain.StatusPending}, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, pending)
}

func TestFailureStreakDisableAndPurge(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	e := seedEndpoint(t, r, db, tenant, "*")

	var n int
	for range 3 {
		inTx(t, db, func(tx *gorm.DB) error {
			var err error
			n, err = r.IncrementFailures(ctx, tx, tenant, e.ID)
			return err
		})
	}
	require.Equal(t, 3, n)
	over, err := r.ActiveEndpointsOverThreshold(ctx, 3, 100)
	require.NoError(t, err)
	require.True(t, containsEndpoint(over, e.ID))

	inTx(t, db, func(tx *gorm.DB) error { return r.ResetFailures(ctx, tx, tenant, e.ID) })
	got, _ := r.EndpointByID(ctx, tenant, e.ID)
	require.Zero(t, got.ConsecutiveFailures)

	var first, second bool
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		first, err = r.DisableEndpoint(ctx, tx, tenant, e.ID, contracts.DisabledReasonConsecutiveFailures, now())
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		second, err = r.DisableEndpoint(ctx, tx, tenant, e.ID, contracts.DisabledReasonConsecutiveFailures, now())
		return err
	})
	require.True(t, first)
	require.False(t, second, "already disabled: the caller publishes once")
	got, _ = r.EndpointByID(ctx, tenant, e.ID)
	require.False(t, got.Active)
	require.Equal(t, contracts.DisabledReasonConsecutiveFailures, got.DisabledReason)
	require.NotNil(t, got.DisabledAt)

	// Re-activation through SaveEndpoint resets the streak.
	inTx(t, db, func(tx *gorm.DB) error { _, err := r.IncrementFailures(ctx, tx, tenant, e.ID); return err })
	got, _ = r.EndpointByID(ctx, tenant, e.ID)
	on := true
	require.NoError(t, got.Apply(domain.EndpointPatch{Active: &on}, domain.URLPolicy{}, now()))
	inTx(t, db, func(tx *gorm.DB) error { return r.SaveEndpoint(ctx, tx, got, true) })
	got, _ = r.EndpointByID(ctx, tenant, e.ID)
	require.True(t, got.Active)
	require.Zero(t, got.ConsecutiveFailures)

	inTx(t, db, func(tx *gorm.DB) error {
		_, err := r.InsertDelivery(ctx, tx, domain.NewDelivery(tenant, e.ID, "ev", "badges.awarded", []byte(`{}`), now()))
		return err
	})
	other := seedEndpoint(t, r, db, id.NewID(), "*")
	inTx(t, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) })
	inTx(t, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) })
	_, err = r.EndpointByID(ctx, tenant, e.ID)
	require.ErrorIs(t, err, domain.ErrEndpointNotFound)
	rows, err := r.ListDeliveries(ctx, tenant, app.DeliveryFilter{}, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, rows)
	_, err = r.EndpointByID(ctx, other.TenantID, other.ID)
	require.NoError(t, err, "other tenants untouched")
}

func TestListEndpointsKeysetPaging(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	for range 3 {
		seedEndpoint(t, r, db, tenant, "*")
	}
	page, err := r.ListEndpoints(ctx, tenant, app.Page{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page, 2)
	last := page[1]
	rest, err := r.ListEndpoints(ctx, tenant, app.Page{Limit: 2, Before: last.CreatedAt, BeforeID: last.ID})
	require.NoError(t, err)
	require.Len(t, rest, 1)
	require.NotEqual(t, page[0].ID, rest[0].ID)
	require.NotEqual(t, page[1].ID, rest[0].ID)
}

func TestReconcileMarker(t *testing.T) {
	r, _ := setupRepo(t)
	ctx := context.Background()
	at := now()
	require.NoError(t, r.MarkRun(ctx, contracts.JobRetrySweep, at))
	require.NoError(t, r.MarkRun(ctx, contracts.JobRetrySweep, at.Add(time.Minute)))
	got, err := r.LastRun(ctx, contracts.JobRetrySweep)
	require.NoError(t, err)
	require.True(t, got.Equal(at.Add(time.Minute)))
}

func TestParseTextArray(t *testing.T) {
	for raw, want := range map[string][]string{
		`{}`:                    {},
		`{a}`:                   {"a"},
		`{badges.awarded,"*"}`:  {"badges.awarded", "*"},
		`{"a,b","c\"d","e\\f"}`: {"a,b", `c"d`, `e\f`},
	} {
		got, err := parseTextArray(raw)
		require.NoError(t, err, raw)
		require.Equal(t, textArray(want), got, raw)
	}
	_, err := parseTextArray(`nope`)
	require.Error(t, err)
	_, err = parseTextArray(`{"open}`)
	require.Error(t, err)

	v, err := textArray{"a", `b"c`}.Value()
	require.NoError(t, err)
	require.Equal(t, `{"a","b\"c"}`, v)
}

func containsDelivery(rows []domain.Delivery, id string) bool {
	for _, d := range rows {
		if d.ID == id {
			return true
		}
	}
	return false
}

func containsEndpoint(rows []domain.Endpoint, id string) bool {
	for _, e := range rows {
		if e.ID == id {
			return true
		}
	}
	return false
}
