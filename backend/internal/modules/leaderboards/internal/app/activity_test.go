package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/shared/errs"
)

func activity(eventID, player, eventType string, props map[string]any, at time.Time) domain.Fact {
	return domain.Fact{
		EventID: eventID, TenantID: tenantA, PlayerID: player, Kind: domain.FactActivity,
		EventType: eventType, Properties: props, At: at,
	}
}

func countCfg(eventType string) *domain.ActivityConfig {
	return &domain.ActivityConfig{EventType: eventType, Value: contracts.ActivityValueCount}
}

func sumCfg(eventType, prop string) *domain.ActivityConfig {
	return &domain.ActivityConfig{EventType: eventType, Value: contracts.ActivityValueProperty, Property: prop}
}

func TestCreateActivityBoardDerivesMetricAndValidatesConfig(t *testing.T) {
	h := newHarness(t, allPerms)
	count := h.board(t, CreateInput{Name: "act1", Type: contracts.TypeActivity, Activity: countCfg(" purchase ")})
	require.Equal(t, contracts.MetricCount, count.Metric)
	require.Equal(t, "purchase", count.Activity.EventType)

	sum := h.board(t, CreateInput{Name: "act2", Type: contracts.TypeActivity, Activity: sumCfg("purchase", "amount")})
	require.Equal(t, contracts.MetricEarned, sum.Metric)
	require.Equal(t, "amount", sum.Activity.Property)

	bad := []CreateInput{
		{Type: contracts.TypeActivity},
		{Type: contracts.TypeActivity, Activity: &domain.ActivityConfig{EventType: "", Value: "count"}},
		{Type: contracts.TypeActivity, Activity: &domain.ActivityConfig{EventType: "has space", Value: "count"}},
		{Type: contracts.TypeActivity, Activity: &domain.ActivityConfig{EventType: "x", Value: "max"}},
		{Type: contracts.TypeActivity, Activity: &domain.ActivityConfig{EventType: "x", Value: "property"}},
		{Type: contracts.TypeActivity, Activity: &domain.ActivityConfig{EventType: "x", Value: "count", Property: "amount"}},
		{Type: contracts.TypePoints, Activity: countCfg("x")},
	}
	for i, in := range bad {
		in.Name = "bad"
		_, err := h.svc.Create(ctxFor(tenantA), in)
		require.Equal(t, errs.Invalid, errs.KindOf(err), "case %d", i)
		require.Equal(t, domain.CodeInvalidConfig, errs.CodeOf(err), "case %d", i)
	}

	mismatched := []CreateInput{
		{Type: contracts.TypeActivity, Metric: contracts.MetricEarned, Activity: countCfg("x")},
		{Type: contracts.TypeActivity, Metric: contracts.MetricCount, Activity: sumCfg("x", "amount")},
		{Type: contracts.TypeActivity, Metric: contracts.MetricNet, Activity: sumCfg("x", "amount")},
		{Type: contracts.TypeActivity, Metric: contracts.MetricBalance, Activity: sumCfg("x", "amount")},
	}
	for i, in := range mismatched {
		in.Name = "mismatch"
		_, err := h.svc.Create(ctxFor(tenantA), in)
		require.Equal(t, domain.CodeInvalidMetric, errs.CodeOf(err), "case %d", i)
	}
}

func TestActivityCountAndSumBoards(t *testing.T) {
	h := newHarness(t, allPerms)
	count := h.board(t, CreateInput{Name: "act3", Type: contracts.TypeActivity, Activity: countCfg("purchase")})
	sum := h.board(t, CreateInput{Name: "act4", Type: contracts.TypeActivity, Activity: sumCfg("purchase", "amount")})
	other := h.board(t, CreateInput{Name: "act5", Type: contracts.TypeActivity, Activity: countCfg("login")})
	points := h.board(t, CreateInput{Name: "act6", Type: contracts.TypePoints})
	ctx := context.Background()
	period := domain.PeriodOf(count, t0)
	require.NoError(t, h.ranks.Replace(ctx, period, nil, t0.Add(time.Hour)))

	require.NoError(t, h.svc.ApplyActivity(ctx, activity("a1", p1, "purchase", map[string]any{"amount": 19.6}, t0)))
	require.NoError(t, h.svc.ApplyActivity(ctx, activity("a2", p1, "purchase", map[string]any{"amount": "5"}, t0)))
	// Not a number, negative, or missing: counts but adds nothing to the sum.
	require.NoError(t, h.svc.ApplyActivity(ctx, activity("a3", p1, "purchase", map[string]any{"amount": "lots"}, t0)))
	require.NoError(t, h.svc.ApplyActivity(ctx, activity("a4", p1, "purchase", map[string]any{"amount": -3.0}, t0)))
	require.NoError(t, h.svc.ApplyActivity(ctx, activity("a5", p1, "purchase", nil, t0)))

	c, _ := h.repo.score(count.ID, domain.AllTimeStart, p1)
	require.Equal(t, int64(5), c)
	s, _ := h.repo.score(sum.ID, domain.AllTimeStart, p1)
	require.Equal(t, int64(25), s, "19.6 rounds to 20, plus 5")
	_, ok := h.repo.score(other.ID, domain.AllTimeStart, p1)
	require.False(t, ok, "another event type does not move the board")
	_, ok = h.repo.score(points.ID, domain.AllTimeStart, p1)
	require.False(t, ok, "existing types ignore activities")

	require.Equal(t, []domain.Standing{{PlayerID: p1, Score: 5}}, h.ranks.snapshot(period))
}

func TestActivityRedeliveryCountsOnce(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Name: "act7", Type: contracts.TypeActivity, Activity: sumCfg("purchase", "amount")})
	f := activity("activity:t:e1", p1, "purchase", map[string]any{"amount": 10.0}, t0)
	for range 3 {
		require.NoError(t, h.svc.ApplyActivity(context.Background(), f))
	}
	s, _ := h.repo.score(lb.ID, domain.AllTimeStart, p1)
	require.Equal(t, int64(10), s)
}

func TestActivityPeriodFromOccurredAt(t *testing.T) {
	h := newHarness(t, allPerms)
	daily := h.board(t, CreateInput{Name: "act8", Type: contracts.TypeActivity, ResetFrequency: contracts.ResetDaily, Activity: countCfg("login")})
	yesterday := t0.AddDate(0, 0, -1)
	require.NoError(t, h.svc.ApplyActivity(context.Background(), activity("l1", p1, "login", nil, yesterday)))
	require.NoError(t, h.svc.ApplyActivity(context.Background(), activity("l2", p1, "login", nil, t0)))
	require.NoError(t, h.svc.ApplyActivity(context.Background(), activity("l3", p1, "login", nil, t0)))

	prev, _ := h.repo.score(daily.ID, domain.PeriodStart(contracts.ResetDaily, yesterday), p1)
	cur, _ := h.repo.score(daily.ID, domain.PeriodStart(contracts.ResetDaily, t0), p1)
	require.Equal(t, int64(1), prev)
	require.Equal(t, int64(2), cur)
}

func TestActivityProgramBoardCountsMembersOnly(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Name: "act9", Type: contracts.TypeActivity, ProgramID: prog, Activity: countCfg("login")})
	ctx := context.Background()
	require.NoError(t, h.svc.SetMembership(ctx, tenantA, prog, p1, true, t0))
	require.NoError(t, h.svc.ApplyActivity(ctx, activity("l1", p1, "login", nil, t0)))
	require.NoError(t, h.svc.ApplyActivity(ctx, activity("l2", p2, "login", nil, t0)))
	_, ok := h.repo.score(lb.ID, domain.AllTimeStart, p1)
	require.True(t, ok)
	_, ok = h.repo.score(lb.ID, domain.AllTimeStart, p2)
	require.False(t, ok)
}

func TestActivityUnresolvedPlayerIsAckedMalformedIsInvalid(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Name: "act10", Type: contracts.TypeActivity, Activity: countCfg("login")})
	ctx := context.Background()

	require.NoError(t, h.svc.ApplyActivity(ctx, activity("l1", "", "login", nil, t0)), "unresolved player: ack, never DLQ")
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.ApplyActivity(ctx, activity("l2", "not-a-uuid", "login", nil, t0))))
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.ApplyActivity(ctx, activity("l3", p1, "", nil, t0))))
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.ApplyActivity(ctx, credit("c1", p1, 1, 1, t0))))
	_, ok := h.repo.score(lb.ID, domain.AllTimeStart, p1)
	require.False(t, ok)
}

func TestNumericProperty(t *testing.T) {
	for _, c := range []struct {
		in   any
		want int64
		ok   bool
	}{
		{float64(3), 3, true}, {2.5, 3, true}, {-2.5, -3, true}, {" 7.2 ", 7, true}, {int64(4), 4, true},
		{"x", 0, false}, {true, 0, false}, {nil, 0, false}, {map[string]any{}, 0, false}, {1e16, 0, false},
	} {
		got, ok := domain.NumericProperty(c.in)
		require.Equal(t, c.ok, ok, "%v", c.in)
		require.Equal(t, c.want, got, "%v", c.in)
	}
}

func TestActivityForAutoCreatedPlayerWaitsThenScores(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Name: "act11", Type: contracts.TypeActivity, Activity: countCfg("login")})
	ctx := context.Background()

	waiting := activity("l1", "", "login", nil, t0)
	waiting.PlayerExternalID, waiting.AutoCreatePlayer = "not-yet", true
	require.Equal(t, errs.Unavailable, errs.KindOf(h.svc.ApplyActivity(ctx, waiting)))

	ready := activity("l2", "", "login", nil, t0)
	ready.PlayerExternalID, ready.AutoCreatePlayer = "ext-1", true
	require.NoError(t, h.svc.ApplyActivity(ctx, ready))
	_, ok := h.repo.score(lb.ID, domain.AllTimeStart, p1)
	require.True(t, ok)
}
