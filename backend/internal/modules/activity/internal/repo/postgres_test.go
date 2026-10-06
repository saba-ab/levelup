package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/activity/internal/app"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/modules/activity/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

const (
	tenantA = "0198d000-0000-7000-8000-00000000000a"
	tenantB = "0198d000-0000-7000-8000-00000000000b"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "activity", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "activity")
	return NewPostgres(moduleDB), moduleDB
}

func newActivity(t *testing.T, tenantID, eventID string, at time.Time) domain.Activity {
	t.Helper()
	a, err := domain.NewActivity(domain.NewInput{
		TenantID:         tenantID,
		EventID:          eventID,
		EventType:        "purchase_completed",
		PlayerExternalID: "ext-1",
		Properties:       map[string]any{"amount": 12.5, "sku": "A"},
		Context:          map[string]any{"ip": "1.2.3.4"},
	}, at, domain.Limits{})
	require.NoError(t, err)
	return a
}

func insert(t *testing.T, r *Postgres, db *gorm.DB, a domain.Activity) bool {
	t.Helper()
	var ok bool
	require.NoError(t, postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		var err error
		ok, err = r.Insert(context.Background(), tx, a)
		return err
	}))
	return ok
}

func TestInsertIsIdempotentPerTenantEvent(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	eventID := id.NewID()
	now := time.Now().UTC()

	first := newActivity(t, tenantA, eventID, now)
	require.True(t, insert(t, r, db, first))
	require.False(t, insert(t, r, db, newActivity(t, tenantA, eventID, now)), "same (tenant, event_id) writes nothing")
	require.True(t, insert(t, r, db, newActivity(t, tenantB, eventID, now)), "uniqueness is per tenant")

	got, err := r.ByID(ctx, tenantA, first.ID)
	require.NoError(t, err)
	require.Equal(t, first.EventID, got.EventID)
	require.Equal(t, 12.5, got.Properties["amount"])
	require.Equal(t, "1.2.3.4", got.Context["ip"])
	require.True(t, first.OccurredAt.Equal(got.OccurredAt))

	_, err = r.ByID(ctx, tenantB, first.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestByEventIDsSeesRowsOfTheSameTransaction(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	a := newActivity(t, tenantA, id.NewID(), time.Now().UTC())
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		ok, err := r.Insert(ctx, tx, a)
		require.True(t, ok)
		require.NoError(t, err)
		got, err := r.ByEventIDs(ctx, tx, tenantA, []string{a.EventID})
		require.NoError(t, err)
		require.Equal(t, a.ID, got[a.EventID].ID)
		return nil
	}))
}

func TestApplyDecisionOnlySettlesPending(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	a := newActivity(t, tenantA, id.NewID(), time.Now().UTC())
	require.True(t, insert(t, r, db, a))

	settle := func(decisionID, status string) bool {
		var ok bool
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			var err error
			ok, err = r.ApplyDecision(ctx, tx, domain.Activity{
				ID: a.ID, TenantID: tenantA, Status: status, DecisionID: decisionID,
				Outcome: "matched", PlayerID: id.NewID(), DecidedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			})
			return err
		}))
		return ok
	}
	decisionID := id.NewID()
	require.True(t, settle(decisionID, domain.StatusDecided))
	require.False(t, settle(decisionID, domain.StatusDecided), "redelivery is a no-op")
	require.False(t, settle(id.NewID(), domain.StatusRejected), "a later decision does not overwrite")

	got, err := r.ByID(ctx, tenantA, a.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusDecided, got.Status)
	require.Equal(t, decisionID, got.DecisionID)
	require.NotEmpty(t, got.PlayerID)
}

func TestLockStuckAndRepublishCap(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	old := time.Now().UTC().Add(-time.Hour)
	stuck := newActivity(t, tenant, id.NewID(), old)
	fresh := newActivity(t, tenant, id.NewID(), time.Now().UTC())
	require.True(t, insert(t, r, db, stuck))
	require.True(t, insert(t, r, db, fresh))

	q := app.StuckQuery{ReceivedBefore: time.Now().UTC().Add(-5 * time.Minute), MaxRepublishes: 1, Limit: 1000}
	lock := func() []domain.Activity {
		var rows []domain.Activity
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			var err error
			rows, err = r.LockStuck(ctx, tx, q)
			if err != nil {
				return err
			}
			ids := make([]string, 0, len(rows))
			for _, a := range rows {
				if a.TenantID == tenant {
					ids = append(ids, a.ID)
				}
			}
			return r.MarkRepublished(ctx, tx, ids, time.Now().UTC())
		}))
		var mine []domain.Activity
		for _, a := range rows {
			if a.TenantID == tenant {
				mine = append(mine, a)
			}
		}
		return mine
	}
	got := lock()
	require.Len(t, got, 1)
	require.Equal(t, stuck.ID, got[0].ID)
	require.Empty(t, lock(), "at the cap: left alone")

	n, err := r.CountExhausted(ctx, q.ReceivedBefore, q.MaxRepublishes)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, int64(1))
}

func TestListKeysetAndFilters(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	base := time.Now().UTC()
	for i := range 3 {
		require.True(t, insert(t, r, db, newActivity(t, tenant, id.NewID(), base.Add(time.Duration(i)*time.Second))))
	}
	page, err := r.List(ctx, tenant, app.ListFilter{}, time.Time{}, "", 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	require.True(t, page[0].CreatedAt.After(page[1].CreatedAt))

	rest, err := r.List(ctx, tenant, app.ListFilter{}, page[1].CreatedAt, page[1].ID, 2)
	require.NoError(t, err)
	require.Len(t, rest, 1)

	none, err := r.List(ctx, tenant, app.ListFilter{Status: domain.StatusDecided}, time.Time{}, "", 10)
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestDeleteTenantAndMarkers(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	require.True(t, insert(t, r, db, newActivity(t, tenant, id.NewID(), time.Now().UTC())))

	purge := func() int64 {
		var n int64
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			var err error
			n, err = r.DeleteTenant(ctx, tx, tenant)
			return err
		}))
		return n
	}
	require.EqualValues(t, 1, purge())
	require.EqualValues(t, 0, purge())

	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, r.MarkRun(ctx, "activity.stuck_sweep", at))
	require.NoError(t, r.MarkRun(ctx, "activity.stuck_sweep", at.Add(time.Minute)))
	last, err := r.LastRun(ctx, "activity.stuck_sweep")
	require.NoError(t, err)
	require.True(t, at.Add(time.Minute).Equal(last))
}

func TestLastSeenPicksNewestPerPlayerWithinTenant(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, other := id.NewID(), id.NewID()
	p1, p2, p3 := id.NewID(), id.NewID(), id.NewID()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	add := func(tenantID, playerID, eventType string, occurred time.Time) {
		a := newActivity(t, tenantID, id.NewID(), at)
		a.PlayerID = playerID
		a.EventType = eventType
		a.OccurredAt = occurred
		require.True(t, insert(t, r, db, a))
	}
	add(tenant, p1, "login", at)
	add(tenant, p1, "purchase_completed", at.Add(2*time.Hour))
	add(tenant, p1, "login", at.Add(time.Hour)) // inserted later, occurred earlier
	add(tenant, p2, "login", at.Add(3*time.Hour))
	add(other, p3, "login", at.Add(5*time.Hour))
	add(tenant, "", "login", at.Add(6*time.Hour)) // unresolved

	got, err := r.LastSeen(ctx, tenant, []string{p1, p2, p3})
	require.NoError(t, err)
	byID := map[string]domain.LastSeen{}
	for _, g := range got {
		byID[g.PlayerID] = g
	}
	require.Len(t, byID, 2, "p3 belongs to another tenant")
	require.Equal(t, domain.LastSeen{PlayerID: p1, At: at.Add(2 * time.Hour), EventType: "purchase_completed"}, byID[p1])
	require.Equal(t, domain.LastSeen{PlayerID: p2, At: at.Add(3 * time.Hour), EventType: "login"}, byID[p2])

	none, err := r.LastSeen(ctx, tenant, nil)
	require.NoError(t, err)
	require.Empty(t, none)

	var idx int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM pg_indexes WHERE indexname = 'ix_activities_tenant_player_occurred'`).Scan(&idx).Error)
	require.EqualValues(t, 1, idx, "migration 0004 created the index")
}
