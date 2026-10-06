package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/analytics/migrations"
	badgescontracts "levelup/internal/modules/badges/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	missionscontracts "levelup/internal/modules/missions/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	rewardscontracts "levelup/internal/modules/rewards/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/modkit"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

// End to end through the subscription handlers: every consumed topic lands
// in the projection the dashboards read, and redelivery counts once.
func TestSubscriptionsFeedTheDashboards(t *testing.T) {
	dsn := pgtest.DSN(t)
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "analytics", migrations.FS))
	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)

	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	clk := clock.NewFake(now)
	m := New(modkit.Deps{DB: postgres.NewModuleDB(base, "analytics"), Authz: authz.AllowAll{}, Clock: clk}, Config{
		PruneSchedule: "0 4 * * *", Retention: 17520 * time.Hour, AppliedEventsRetention: 720 * time.Hour,
	})
	handlers := map[string]bus.Handler{}
	for _, s := range m.Subscriptions() {
		require.Equal(t, "analytics", s.Group)
		handlers[s.Topic] = s.Handler
	}

	tenant, player := id.NewID(), id.NewID()
	deliver := func(topic string, payload any) bus.Envelope {
		t.Helper()
		e, err := bus.NewEnvelope(ctx, clk, topic, payload)
		require.NoError(t, err)
		require.NoError(t, handlers[topic](ctx, e))
		return e
	}

	act := deliver(activitycontracts.TopicReceived, activitycontracts.ReceivedV1{
		TenantID: tenant, PlayerID: player, EventType: "login", EventID: "x1", OccurredAt: now,
	})
	require.NoError(t, handlers[activitycontracts.TopicReceived](ctx, act), "redelivery")
	deliver(pointscontracts.TopicCredited, pointscontracts.LedgerMovedV1{TenantID: tenant, PlayerID: player, Kind: "earn", Amount: 70, At: now})
	deliver(pointscontracts.TopicDebited, pointscontracts.LedgerMovedV1{TenantID: tenant, PlayerID: player, Kind: "spend", Amount: 20, At: now})
	deliver(badgescontracts.TopicAwarded, badgescontracts.AwardedV1{TenantID: tenant, PlayerID: player, BadgeID: "b1", At: now})
	deliver(missionscontracts.TopicStarted, missionscontracts.AttemptV1{TenantID: tenant, PlayerID: player, MissionID: "m1", At: now})
	deliver(missionscontracts.TopicCompleted, missionscontracts.CompletedV1{TenantID: tenant, PlayerID: player, MissionID: "m1", At: now})
	deliver(progressioncontracts.TopicLevelReached, progressioncontracts.LevelReachedV1{TenantID: tenant, PlayerID: player, LevelNumber: 3, At: now})
	deliver(rewardscontracts.TopicClaimed, rewardscontracts.ClaimV1{TenantID: tenant, PlayerID: player, RewardID: "r1", At: now})
	deliver(playercontracts.TopicPlayerCreated, playercontracts.PlayerCreatedV1{TenantID: tenant, PlayerID: player, At: now})

	viewer := authz.Into(ctx, authz.Principal{UserID: "u", TenantID: tenant})
	o, err := m.svc.Overview(viewer, nil, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, o.Totals.Activities)
	require.EqualValues(t, 1, o.Totals.ActivePlayers)
	require.EqualValues(t, 70, o.Totals.PointsCredited)
	require.EqualValues(t, 20, o.Totals.PointsDebited)
	require.EqualValues(t, 1, o.Totals.NewPlayers)
	require.EqualValues(t, 1, o.Totals.BadgesAwarded)
	require.EqualValues(t, 1, o.Totals.MissionsStarted)
	require.EqualValues(t, 1, o.Totals.MissionsCompleted)
	require.EqualValues(t, 1, o.Totals.LevelsReached)
	require.EqualValues(t, 1, o.Totals.RewardsClaimed)
	require.EqualValues(t, 1, o.DAU)

	r, err := m.svc.Retention(viewer, "week", 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, r.Cohorts[0].Size)
	require.Equal(t, []float64{100}, r.Cohorts[0].Retained)

	deliver(identitycontracts.TopicTenantDeleted, identitycontracts.TenantDeletedV1{TenantID: tenant, At: now})
	o, err = m.svc.Overview(viewer, nil, nil)
	require.NoError(t, err)
	require.Zero(t, o.Totals.Activities)
	require.Zero(t, o.Totals.PointsCredited)
}
