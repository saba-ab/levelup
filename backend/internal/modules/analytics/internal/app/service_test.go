package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/analytics/contracts"
	"levelup/internal/modules/analytics/internal/domain"
	"levelup/internal/shared/errs"
)

var viewer = allowKeys{contracts.PermView.Key(): true}

func d(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr(t time.Time) *time.Time { return &t }

func activity(eventID, tenant, player, eventType string, at time.Time) domain.Fact {
	return domain.Fact{
		EventID: eventID, TenantID: tenant, Day: at,
		Counters:  []domain.Counter{{Metric: contracts.MetricActivities, Dimension: eventType, Value: 1}},
		PlayerID:  player,
		EventType: eventType,
	}
}

func counter(eventID, tenant, metric, dim string, v int64, at time.Time) domain.Fact {
	return domain.Fact{EventID: eventID, TenantID: tenant, Day: at,
		Counters: []domain.Counter{{Metric: metric, Dimension: dim, Value: v}}}
}

func TestRecordIsIdempotentPerEventID(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, viewer)
	ctx := context.Background()
	f := counter("evt-1", tenantA, contracts.MetricPointsCredited, "earn", 50, now)

	require.NoError(t, svc.Record(ctx, f))
	require.NoError(t, svc.Record(ctx, f), "redelivery is not an error")

	o, err := svc.Overview(asTenant(tenantA), nil, nil)
	require.NoError(t, err)
	require.EqualValues(t, 50, o.Totals.PointsCredited, "a redelivered event must count once")
}

func TestRecordTruncatesToUTCDay(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, viewer)
	require.NoError(t, svc.Record(context.Background(),
		activity("e1", tenantA, "p1", "login", time.Date(2026, 10, 6, 23, 59, 0, 0, time.UTC))))
	require.True(t, repo.days[dayKey{tenantA, "p1", d("2026-10-06")}])
}

func TestRecordRejectsMalformedFactsAsInvalid(t *testing.T) {
	svc := newTestService(newFakeRepo(), viewer)
	for _, f := range []domain.Fact{
		{TenantID: tenantA, Day: now},
		{EventID: "e", Day: now},
		{EventID: "e", TenantID: tenantA},
	} {
		require.Equal(t, errs.Invalid, errs.KindOf(svc.Record(context.Background(), f)))
	}
}

func TestOverviewAggregatesAndScopesToTenant(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, viewer)
	ctx := context.Background()
	facts := []domain.Fact{
		activity("a1", tenantA, "p1", "login", d("2026-10-07")),
		activity("a2", tenantA, "p1", "purchase", d("2026-10-07")),
		activity("a3", tenantA, "p2", "login", d("2026-10-05")),
		activity("a4", tenantA, "p3", "login", d("2026-09-20")), // inside MAU, outside WAU
		activity("b1", tenantB, "p9", "login", d("2026-10-07")),
		counter("c1", tenantA, contracts.MetricPointsCredited, "earn", 100, d("2026-10-07")),
		counter("c2", tenantA, contracts.MetricPointsCredited, "bonus", 20, d("2026-10-06")),
		counter("c3", tenantA, contracts.MetricPointsDebited, "spend", 30, d("2026-10-06")),
		counter("c4", tenantA, contracts.MetricPlayersCreated, "", 1, d("2026-10-05")),
		counter("c5", tenantA, contracts.MetricBadgesAwarded, "badge-1", 1, d("2026-10-05")),
		counter("c6", tenantA, contracts.MetricRewardsClaimed, "reward-1", 1, d("2026-10-05")),
		counter("c7", tenantA, contracts.MetricPointsCredited, "earn", 999, d("2026-09-01")), // before the range
	}
	for _, f := range facts {
		require.NoError(t, svc.Record(ctx, f))
	}

	o, err := svc.Overview(asTenant(tenantA), ptr(d("2026-10-05")), ptr(d("2026-10-07")))
	require.NoError(t, err)
	require.Equal(t, d("2026-10-05"), o.Range.From)
	require.Len(t, o.Daily, 3)
	require.Equal(t, Totals{
		Activities: 3, ActivePlayers: 2, NewPlayers: 1, PointsCredited: 120, PointsDebited: 30,
		BadgesAwarded: 1, RewardsClaimed: 1,
	}, o.Totals)
	require.Equal(t, DailyOverview{Day: d("2026-10-07"), Activities: 2, ActivePlayers: 1, PointsCredited: 100}, o.Daily[2])
	require.Equal(t, DailyOverview{Day: d("2026-10-06"), PointsCredited: 20, PointsDebited: 30}, o.Daily[1])
	require.Equal(t, DailyOverview{Day: d("2026-10-05"), Activities: 1, ActivePlayers: 1, NewPlayers: 1}, o.Daily[0])
	require.EqualValues(t, 1, o.DAU)
	require.EqualValues(t, 2, o.WAU)
	require.EqualValues(t, 3, o.MAU)
}

func TestQueriesRequirePermissionAndTenant(t *testing.T) {
	svc := newTestService(newFakeRepo(), allowKeys{})
	ctx := asTenant(tenantA)
	_, err := svc.Overview(ctx, nil, nil)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = svc.Engagement(ctx, nil, nil)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = svc.Retention(ctx, "week", 8)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = svc.Funnel(ctx, "a,b", nil, nil)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	_, err = newTestService(newFakeRepo(), viewer).Overview(context.Background(), nil, nil)
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
}

func TestQueriesRefuseUnboundedRanges(t *testing.T) {
	svc := newTestService(newFakeRepo(), viewer)
	_, err := svc.Overview(asTenant(tenantA), ptr(d("2024-01-01")), ptr(d("2026-01-01")))
	require.ErrorIs(t, err, domain.ErrRangeTooLarge)
	_, err = svc.Engagement(asTenant(tenantA), ptr(d("2026-02-01")), ptr(d("2026-01-01")))
	require.ErrorIs(t, err, domain.ErrRangeInverted)
	_, err = svc.Retention(asTenant(tenantA), "week", 53)
	require.ErrorIs(t, err, domain.ErrBadWeeks)
	_, err = svc.Funnel(asTenant(tenantA), "only_one", nil, nil)
	require.ErrorIs(t, err, domain.ErrBadSteps)
}

func TestEngagementBreaksDownByDimension(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, viewer)
	ctx := context.Background()
	for i, f := range []domain.Fact{
		activity("", tenantA, "p1", "login", d("2026-10-07")),
		activity("", tenantA, "p2", "login", d("2026-10-07")),
		activity("", tenantA, "p1", "purchase", d("2026-10-06")),
		counter("", tenantA, contracts.MetricBadgesAwarded, "b1", 1, d("2026-10-07")),
		counter("", tenantA, contracts.MetricBadgesAwarded, "b2", 1, d("2026-10-07")),
		counter("", tenantA, contracts.MetricBadgesAwarded, "b2", 1, d("2026-10-06")),
		counter("", tenantA, contracts.MetricMissionsStarted, "m1", 1, d("2026-10-06")),
		counter("", tenantA, contracts.MetricMissionsStarted, "m1", 1, d("2026-10-06")),
		counter("", tenantA, contracts.MetricMissionsCompleted, "m1", 1, d("2026-10-07")),
		counter("", tenantA, contracts.MetricLevelsReached, "2", 1, d("2026-10-07")),
		counter("", tenantA, contracts.MetricRewardsClaimed, "r1", 1, d("2026-10-07")),
	} {
		f.EventID = "e" + string(rune('a'+i))
		require.NoError(t, svc.Record(ctx, f))
	}
	e, err := svc.Engagement(asTenant(tenantA), ptr(d("2026-10-06")), ptr(d("2026-10-07")))
	require.NoError(t, err)
	require.Equal(t, []DayCount{{Day: d("2026-10-06"), Count: 1}, {Day: d("2026-10-07"), Count: 2}}, e.BadgesPerDay)
	require.EqualValues(t, 2, e.MissionsStarted)
	require.EqualValues(t, 1, e.MissionsCompleted)
	require.EqualValues(t, 1, e.LevelsReached)
	require.EqualValues(t, 1, e.RewardsClaimed)
	require.Equal(t, []KeyCount{{Key: "login", Count: 2}, {Key: "purchase", Count: 1}}, e.TopEventTypes)
	require.Equal(t, []KeyCount{{Key: "b2", Count: 2}, {Key: "b1", Count: 1}}, e.TopBadges)
	require.Equal(t, []KeyCount{{Key: "2", Count: 1}}, e.LevelsByNumber)
	require.Len(t, e.Daily, 2)
}

func TestRetentionBuildsCohortsEndingThisWeek(t *testing.T) {
	repo := newFakeRepo()
	repo.cohortSizes = map[time.Time]int64{d("2026-09-28"): 4}
	repo.cohortCells = []domain.CohortCell{
		{Cohort: d("2026-09-28"), Offset: 0, Players: 4},
		{Cohort: d("2026-09-28"), Offset: 1, Players: 2},
	}
	svc := newTestService(repo, viewer)
	res, err := svc.Retention(asTenant(tenantA), "week", 2)
	require.NoError(t, err)
	require.Len(t, res.Cohorts, 2)
	require.Equal(t, d("2026-09-28"), res.Cohorts[0].Start)
	require.Equal(t, []float64{100, 50}, res.Cohorts[0].Retained)
	require.Equal(t, d("2026-10-05"), res.Cohorts[1].Start)
	require.Len(t, res.Curve, 2)
}

func TestFunnelPercentages(t *testing.T) {
	repo := newFakeRepo()
	repo.funnel = []int64{200, 50, 10}
	svc := newTestService(repo, viewer)
	f, err := svc.Funnel(asTenant(tenantA), "visit,signup,purchase", nil, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"visit", "signup", "purchase"}, repo.funnelSteps)
	require.Equal(t, FunnelStep{EventType: "visit", Players: 200, PctOfPrevious: 100, PctOfFirstStep: 100}, f.Steps[0])
	require.Equal(t, FunnelStep{EventType: "signup", Players: 50, PctOfPrevious: 25, PctOfFirstStep: 25}, f.Steps[1])
	require.Equal(t, FunnelStep{EventType: "purchase", Players: 10, PctOfPrevious: 20, PctOfFirstStep: 5}, f.Steps[2])
}

func TestFunnelWithNobodyAtTheFirstStep(t *testing.T) {
	repo := newFakeRepo()
	repo.funnel = []int64{0, 0}
	f, err := newTestService(repo, viewer).Funnel(asTenant(tenantA), "a,b", nil, nil)
	require.NoError(t, err)
	require.Zero(t, f.Steps[1].PctOfPrevious)
}

func TestPruneUsesRetentionsAndMarksRun(t *testing.T) {
	repo := newFakeRepo()
	n, err := newTestService(repo, viewer).Prune(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 3, n)
	require.Equal(t, domain.DayOf(now.Add(-2*365*24*time.Hour)), repo.pruneBefore)
	require.Equal(t, now.Add(-30*24*time.Hour), repo.pruneApplied)
	require.Equal(t, []string{contracts.JobPrune}, repo.marked)
}

func TestPurgeTenantIsIdempotentAndScoped(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, viewer)
	ctx := context.Background()
	require.NoError(t, svc.Record(ctx, activity("a1", tenantA, "p1", "login", now)))
	require.NoError(t, svc.Record(ctx, activity("b1", tenantB, "p2", "login", now)))

	require.NoError(t, svc.PurgeTenant(ctx, tenantA))
	require.NoError(t, svc.PurgeTenant(ctx, tenantA))
	require.Equal(t, errs.Invalid, errs.KindOf(svc.PurgeTenant(ctx, "")))

	a, err := svc.Overview(asTenant(tenantA), nil, nil)
	require.NoError(t, err)
	require.Zero(t, a.Totals.Activities)
	b, err := svc.Overview(asTenant(tenantB), nil, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, b.Totals.Activities)
}
