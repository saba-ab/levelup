package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/analytics/contracts"
	"levelup/internal/modules/analytics/internal/domain"
	"levelup/internal/modules/analytics/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "analytics", migrations.FS))
	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "analytics")
	return NewPostgres(moduleDB), moduleDB
}

func d(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func apply(t *testing.T, r *Postgres, db *gorm.DB, f domain.Fact) bool {
	t.Helper()
	if f.EventID == "" {
		f.EventID = id.NewID()
	}
	var applied bool
	require.NoError(t, postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		var err error
		applied, err = r.Apply(context.Background(), tx, f, time.Now().UTC())
		return err
	}))
	return applied
}

func act(tenant, player, eventType, day string) domain.Fact {
	return domain.Fact{
		TenantID: tenant, Day: d(day), PlayerID: player, EventType: eventType,
		Counters: []domain.Counter{{Metric: contracts.MetricActivities, Dimension: eventType, Value: 1}},
	}
}

func TestApplyIsIdempotentPerEventID(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	f := act(tenant, player, "login", "2026-10-06")
	f.EventID = "evt-" + id.NewID()
	f.Counters = append(f.Counters, domain.Counter{Metric: contracts.MetricPointsCredited, Dimension: "earn", Value: 40})

	require.True(t, apply(t, r, db, f))
	require.False(t, apply(t, r, db, f), "redelivery must change nothing")

	rg := domain.Range{From: d("2026-10-06"), To: d("2026-10-06")}
	rows, err := r.Counters(ctx, tenant, rg, []string{contracts.MetricActivities, contracts.MetricPointsCredited})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, d("2026-10-06"), row.Day)
		switch row.Metric {
		case contracts.MetricActivities:
			require.EqualValues(t, 1, row.Value)
			require.Equal(t, "login", row.Dimension)
		case contracts.MetricPointsCredited:
			require.EqualValues(t, 40, row.Value)
		}
	}
	n, err := r.DistinctActivePlayers(ctx, tenant, rg)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
}

func TestCountersSumAcrossEventsAndStayInTenant(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, other := id.NewID(), id.NewID()
	for range 3 {
		apply(t, r, db, act(tenant, id.NewID(), "login", "2026-10-06"))
	}
	apply(t, r, db, act(other, id.NewID(), "login", "2026-10-06"))
	apply(t, r, db, act(tenant, id.NewID(), "login", "2026-10-08")) // outside the range

	rg := domain.Range{From: d("2026-10-05"), To: d("2026-10-07")}
	rows, err := r.Counters(ctx, tenant, rg, []string{contracts.MetricActivities})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 3, rows[0].Value)

	byDay, err := r.ActivePlayersByDay(ctx, tenant, rg)
	require.NoError(t, err)
	require.Equal(t, map[time.Time]int64{d("2026-10-06"): 3}, byDay)
}

func TestFirstSeenKeepsEarliestDayUnderReordering(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	apply(t, r, db, act(tenant, player, "login", "2026-09-30"))
	apply(t, r, db, act(tenant, player, "login", "2026-09-22")) // late, earlier delivery
	apply(t, r, db, act(tenant, player, "login", "2026-10-06"))

	sizes, err := r.CohortSizes(ctx, tenant, d("2026-09-14"), d("2026-10-12"))
	require.NoError(t, err)
	require.Equal(t, map[time.Time]int64{d("2026-09-21"): 1}, sizes, "the player belongs to the week of 09-22")

	cells, err := r.CohortActivity(ctx, tenant, d("2026-09-14"), d("2026-10-12"))
	require.NoError(t, err)
	require.ElementsMatch(t, []domain.CohortCell{
		{Cohort: d("2026-09-21"), Offset: 0, Players: 1},
		{Cohort: d("2026-09-21"), Offset: 1, Players: 1},
		{Cohort: d("2026-09-21"), Offset: 2, Players: 1},
	}, cells)
}

func TestFunnelCountsStepsInDayOrder(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	inOrder, sameDay, reversed, partial := id.NewID(), id.NewID(), id.NewID(), id.NewID()

	apply(t, r, db, act(tenant, inOrder, "visit", "2026-10-01"))
	apply(t, r, db, act(tenant, inOrder, "signup", "2026-10-02"))
	apply(t, r, db, act(tenant, inOrder, "purchase", "2026-10-03"))

	apply(t, r, db, act(tenant, sameDay, "visit", "2026-10-04"))
	apply(t, r, db, act(tenant, sameDay, "signup", "2026-10-04"))
	apply(t, r, db, act(tenant, sameDay, "purchase", "2026-10-04"))

	apply(t, r, db, act(tenant, reversed, "signup", "2026-10-01"))
	apply(t, r, db, act(tenant, reversed, "visit", "2026-10-02"))

	apply(t, r, db, act(tenant, partial, "visit", "2026-10-01"))
	apply(t, r, db, act(tenant, partial, "signup", "2026-10-02"))
	apply(t, r, db, act(tenant, partial, "purchase", "2026-10-20")) // after the range

	got, err := r.Funnel(ctx, tenant, []string{"visit", "signup", "purchase"},
		domain.Range{From: d("2026-10-01"), To: d("2026-10-10")})
	require.NoError(t, err)
	require.Equal(t, []int64{4, 3, 2}, got)
}

func TestPruneAndPurge(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, other := id.NewID(), id.NewID()
	apply(t, r, db, act(tenant, id.NewID(), "login", "2020-01-01"))
	apply(t, r, db, act(tenant, id.NewID(), "login", "2026-10-06"))
	apply(t, r, db, act(other, id.NewID(), "login", "2026-10-06"))

	_, err := r.Prune(ctx, d("2024-10-06"), time.Now().UTC().Add(-time.Hour))
	require.NoError(t, err)
	old, err := r.DistinctActivePlayers(ctx, tenant, domain.Range{From: d("2020-01-01"), To: d("2020-01-01")})
	require.NoError(t, err)
	require.Zero(t, old)
	recent, err := r.DistinctActivePlayers(ctx, tenant, domain.Range{From: d("2026-10-06"), To: d("2026-10-06")})
	require.NoError(t, err)
	require.EqualValues(t, 1, recent)
	require.NoError(t, r.MarkRun(ctx, contracts.JobPrune, time.Now().UTC()))
	require.NoError(t, r.MarkRun(ctx, contracts.JobPrune, time.Now().UTC()))

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) }))
	gone, err := r.DistinctActivePlayers(ctx, tenant, domain.Range{From: d("2026-10-06"), To: d("2026-10-06")})
	require.NoError(t, err)
	require.Zero(t, gone)
	kept, err := r.DistinctActivePlayers(ctx, other, domain.Range{From: d("2026-10-06"), To: d("2026-10-06")})
	require.NoError(t, err)
	require.EqualValues(t, 1, kept)
}
