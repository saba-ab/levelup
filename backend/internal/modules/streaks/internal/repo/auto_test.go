package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/streaks/internal/app"
	"levelup/internal/modules/streaks/internal/domain"
	"levelup/internal/modules/streaks/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/id"
)

type pgOutbox struct{ topics []string }

func (o *pgOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, _ any) error {
	o.topics = append(o.topics, topic)
	return nil
}

type pgPlayers struct{ p ports.PlayerSnapshot }

func (f pgPlayers) PlayersByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, i := range ids {
		if i == f.p.ID && tenantID == f.p.TenantID {
			out[i] = f.p
		}
	}
	return out, nil
}

func (f pgPlayers) PlayersByExternalIDs(_ context.Context, tenantID string, ext []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, e := range ext {
		if e == f.p.ExternalID && tenantID == f.p.TenantID {
			out[e] = f.p
		}
	}
	return out, nil
}

type pgTenants struct{}

func (pgTenants) TenantsByIDs(context.Context, []string) (map[string]ports.TenantSnapshot, error) {
	return map[string]ports.TenantSnapshot{}, nil
}

type allowAll struct{}

func (allowAll) Authorize(context.Context, authz.Principal, authz.Permission, any) error { return nil }

func TestAutoRecordColumnDefaultsAndToggles(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()

	st := seed(t, r, db, tenant, "auto_default")
	got, err := r.StreakByID(ctx, tenant, st.ID)
	require.NoError(t, err)
	require.True(t, got.AutoRecord)

	off := false
	require.NoError(t, got.Apply(domain.StreakPatch{AutoRecord: &off}, time.Now().UTC()))
	inTx(t, db, func(tx *gorm.DB) error { return r.SaveStreak(ctx, tx, got) })
	got, err = r.StreakByID(ctx, tenant, st.ID)
	require.NoError(t, err)
	require.False(t, got.AutoRecord)

	optedOut, err := domain.NewStreak(tenant, domain.NewStreakInput{
		Name: "Opted out", ActivityKey: "opted_out", Period: "daily", Active: true, AutoRecord: &off,
	}, time.Now().UTC())
	require.NoError(t, err)
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateStreak(ctx, tx, optedOut) })
	got, err = r.StreakByID(ctx, tenant, optedOut.ID)
	require.NoError(t, err)
	require.False(t, got.AutoRecord, "an explicit false survives the insert")

	// Rows that predate the column read as auto-recording.
	require.NoError(t, db.Exec("UPDATE streaks_svc.streaks SET auto_record = DEFAULT WHERE id = ?", optedOut.ID).Error)
	got, err = r.StreakByID(ctx, tenant, optedOut.ID)
	require.NoError(t, err)
	require.True(t, got.AutoRecord)
}

func TestHandleActivityAgainstPostgresIsIdempotent(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	player := ports.PlayerSnapshot{ID: id.NewID(), TenantID: tenant, ExternalID: "ext-pg", Active: true}
	st := seed(t, r, db, tenant, "pg_login")

	ob := &pgOutbox{}
	now := time.Now().UTC()
	svc := app.NewService(r, pgPlayers{p: player}, pgTenants{}, ob, allowAll{}, db, clock.NewFake(now))

	ev := activitycontracts.ReceivedV1{
		ActivityID: id.NewID(), TenantID: tenant, EventID: "e1", EventType: "pg_login",
		PlayerExternalID: "ext-pg", OccurredAt: now, ReceivedAt: now,
	}
	res, err := svc.HandleActivity(ctx, ev)
	require.NoError(t, err)
	require.Equal(t, app.OutcomeRecorded, res.Outcome)
	published := len(ob.topics)

	res, err = svc.HandleActivity(ctx, ev)
	require.NoError(t, err)
	require.Equal(t, app.OutcomeDuplicate, res.Outcome)
	require.Len(t, ob.topics, published)

	var periods, requests int64
	require.NoError(t, db.Model(&streakPeriod{}).Where("tenant_id = ?", tenant).Count(&periods).Error)
	require.NoError(t, db.Model(&recordRequest{}).
		Where("tenant_id = ? AND idempotency_key = ?", tenant, app.AutoKey(ev.ActivityID, st.ID)).Count(&requests).Error)
	require.EqualValues(t, 1, periods)
	require.EqualValues(t, 1, requests)
}
