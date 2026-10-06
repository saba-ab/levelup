package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/modules/leaderboards/internal/app"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/id"
)

func TestActivityBoardConfigRoundTripsAndIsLookedUpByEventType(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	sum := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypeActivity,
		Activity: &domain.ActivityConfig{EventType: "purchase", Value: contracts.ActivityValueProperty, Property: "amount"}})
	mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypeActivity,
		Activity: &domain.ActivityConfig{EventType: "login", Value: contracts.ActivityValueCount}})
	points := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypePoints})

	got, err := r.ByID(ctx, tenant, sum.ID)
	require.NoError(t, err)
	require.Equal(t, sum.Activity, got.Activity)
	require.Equal(t, contracts.MetricEarned, got.Metric)

	pts, err := r.ByID(ctx, tenant, points.ID)
	require.NoError(t, err)
	require.Nil(t, pts.Activity, "other types carry no config")

	boards, err := r.ActiveForActivity(ctx, tenant, "purchase")
	require.NoError(t, err)
	require.Len(t, boards, 1)
	require.Equal(t, sum.ID, boards[0].ID)

	none, err := r.ActiveForActivity(ctx, id.NewID(), "purchase")
	require.NoError(t, err)
	require.Empty(t, none, "another tenant's boards are invisible")

	// The CHECK constraint backs the domain: an activity row needs a config.
	err = db.Exec(`INSERT INTO leaderboards_svc.leaderboards
        (id, tenant_id, slug, name, type, metric, reset_frequency, created_at, updated_at)
        VALUES (?, ?, 'raw', 'raw', 'activity', 'count', 'never', now(), now())`, id.NewID(), tenant).Error
	require.Error(t, err)
}

func TestActivityFactsScoreOnceAgainstPostgres(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	count := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypeActivity, ResetFrequency: contracts.ResetDaily,
		Activity: &domain.ActivityConfig{EventType: "purchase", Value: contracts.ActivityValueCount}})
	sum := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypeActivity,
		Activity: &domain.ActivityConfig{EventType: "purchase", Value: contracts.ActivityValueProperty, Property: "amount"}})
	svc := app.NewService(r, Disabled{}, nil, nopOutbox{}, allowAll{}, db, clock.NewFake(t0), nil, app.Settings{
		ClosedRetention: time.Hour, AllTimeTTL: time.Hour, CloseGrace: time.Minute, SnapshotLimit: 100, AppliedRetention: time.Hour,
	})

	fact := func(key string, amount float64, at time.Time) domain.Fact {
		return domain.Fact{EventID: key, TenantID: tenant, PlayerID: player, Kind: domain.FactActivity,
			EventType: "purchase", Properties: map[string]any{"amount": amount}, At: at}
	}
	key1, key2 := "activity:"+tenant+":e1", "activity:"+tenant+":e2"
	for range 2 {
		require.NoError(t, svc.ApplyActivity(ctx, fact(key1, 12, t0)))
	}
	require.NoError(t, svc.ApplyActivity(ctx, fact(key2, 8, t0.AddDate(0, 0, -1))))

	score := func(lb domain.Leaderboard, at time.Time) int64 {
		t.Helper()
		st, ok, err := r.PositionOf(ctx, lb, domain.PeriodOf(lb, at).Start, player)
		require.NoError(t, err)
		require.True(t, ok)
		return st.Score
	}
	require.Equal(t, int64(1), score(count, t0), "redelivery counts once")
	require.Equal(t, int64(1), score(count, t0.AddDate(0, 0, -1)), "period comes from occurred_at")
	require.Equal(t, int64(20), score(sum, t0))

	var applied int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM leaderboards_svc.applied_events WHERE event_id = ?`, key1).Scan(&applied).Error)
	require.Equal(t, int64(2), applied, "one row per (event, board)")
}
