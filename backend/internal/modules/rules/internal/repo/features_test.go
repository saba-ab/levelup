package repo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/rules/internal/app"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/shared/id"
)

func TestScheduleAndStopProcessingRoundTrip(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	rule, vs := seedLiveRule(t, r, db, tenant, "sched", 2, 1)
	got, err := r.LatestVersion(ctx, tenant, rule.ID)
	require.NoError(t, err)
	require.Equal(t, "null", string(got.Schedule), "default column value")
	require.False(t, got.StopProcessing)

	draft := vs[1]
	stop := true
	require.NoError(t, draft.Edit(domain.DefinitionPatch{StopProcessing: &stop,
		Schedule: json.RawMessage(`{"days_of_week":[1],"timezone":"Asia/Tbilisi"}`)}, eval.Options{}))
	inTx(t, db, func(tx *gorm.DB) error { return r.SaveDraftVersion(ctx, tx, draft) })
	got, err = r.LatestVersion(ctx, tenant, rule.ID)
	require.NoError(t, err)
	require.True(t, got.StopProcessing)
	require.JSONEq(t, `{"days_of_week":[1],"timezone":"Asia/Tbilisi"}`, string(got.Schedule))

	inTx(t, db, func(tx *gorm.DB) error {
		require.NoError(t, rule.Publish(draft, t0))
		draft.MarkPublished(t0)
		require.NoError(t, r.PublishVersion(ctx, tx, draft))
		return r.SaveRule(ctx, tx, rule)
	})
	srcs, err := r.LiveRuleSources(ctx, tenant, "sched")
	require.NoError(t, err)
	require.Len(t, srcs, 1)
	require.True(t, srcs[0].StopProcessing)
	require.JSONEq(t, `{"days_of_week":[1],"timezone":"Asia/Tbilisi"}`, string(srcs[0].Schedule))
	prog := eval.Compile(srcs, eval.Options{})
	res := eval.Evaluate(prog, eval.Facts{Activity: eval.Activity{EventType: "sched",
		Properties: map[string]any{"amount": json.Number("5")}, OccurredAt: t0}}) // Monday 16:00 in Tbilisi
	require.Equal(t, eval.StatusMatched, res.Rules[0].Status)
	require.True(t, res.Rules[0].StopProcessing)
}

func TestPlayerHistoryProjection(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player, other := id.NewID(), id.NewID(), id.NewID()
	day := func(n int) time.Time { return t0.AddDate(0, 0, n) }
	record := func(tenantID, playerID, eventType string, at time.Time) {
		inTx(t, db, func(tx *gorm.DB) error {
			return r.RecordPlayerEvent(ctx, tx, app.PlayerEvent{TenantID: tenantID, PlayerID: playerID, EventType: eventType, At: at})
		})
	}
	record(tenant, player, "login", day(0))
	record(tenant, player, "login", day(0).Add(-11*time.Hour)) // same UTC day → same bucket
	record(tenant, player, "login", day(-1))
	record(tenant, player, "login", day(-6))
	record(tenant, player, "login", day(-7))
	record(tenant, player, "login", day(-29))
	record(tenant, player, "login", day(-89))
	record(tenant, player, "login", day(-400))
	record(tenant, player, "login", day(+1)) // later day: never counted for t0
	record(tenant, player, "purchase", day(-3))
	record(tenant, other, "login", day(0))      // other player
	record(id.NewID(), player, "login", day(0)) // other tenant

	var rows int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM `+r.table("rulePlayerEventDay")+` WHERE tenant_id = ? AND player_id = ?`,
		tenant, player).Scan(&rows).Error)
	require.EqualValues(t, 9, rows, "one row per (event type, day)")

	got, err := r.PlayerHistory(ctx, app.HistoryQuery{TenantID: tenant, PlayerID: player,
		EventTypes: []string{"login", "purchase", "never"}, At: t0})
	require.NoError(t, err)
	require.Equal(t, map[string]map[string]int64{
		"login":    {eval.Window1d: 2, eval.Window7d: 4, eval.Window30d: 6, eval.Window90d: 7, eval.WindowAll: 8},
		"purchase": {eval.Window1d: 0, eval.Window7d: 1, eval.Window30d: 1, eval.Window90d: 1, eval.WindowAll: 1},
	}, got)

	// Windows end on the activity's day: an activity 2 days ago sees fewer.
	got, err = r.PlayerHistory(ctx, app.HistoryQuery{TenantID: tenant, PlayerID: player, EventTypes: []string{"login"}, At: day(-2)})
	require.NoError(t, err)
	require.Equal(t, map[string]int64{eval.Window1d: 0, eval.Window7d: 2, eval.Window30d: 3, eval.Window90d: 4, eval.WindowAll: 5}, got["login"])

	empty, err := r.PlayerHistory(ctx, app.HistoryQuery{TenantID: tenant, PlayerID: "not-a-uuid", EventTypes: []string{"login"}, At: t0})
	require.NoError(t, err)
	require.Empty(t, empty)
	empty, err = r.PlayerHistory(ctx, app.HistoryQuery{TenantID: tenant, PlayerID: player, At: t0})
	require.NoError(t, err)
	require.Empty(t, empty)

	inTx(t, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) })
	got, err = r.PlayerHistory(ctx, app.HistoryQuery{TenantID: tenant, PlayerID: player, EventTypes: []string{"login"}, At: t0})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestRuleStatsGroupedQuery(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	ruleA, _ := seedLiveRule(t, r, db, tenant, "stats", 1, 1)
	ruleB, _ := seedLiveRule(t, r, db, tenant, "stats", 1, 1)
	ghost := id.NewID() // executions of a rule id with no rule row (name "")

	type exSpec struct {
		rule    string
		status  string
		at      time.Time
		effects []domain.Effect
	}
	eff := func(typ string, amount int64, status string) domain.Effect {
		return domain.Effect{Type: typ, Params: map[string]any{"amount": amount}, Status: status}
	}
	specs := []exSpec{
		{ruleA.ID, domain.ExecFired, t0, []domain.Effect{
			eff(eval.ActionCreditPoints, 50, domain.EffectApplied), eff(eval.ActionGrantXP, 10, domain.EffectApplied)}},
		{ruleA.ID, domain.ExecFired, t0.Add(time.Hour), []domain.Effect{
			eff(eval.ActionCreditPoints, 25, domain.EffectRequested), eff(eval.ActionAwardBadge, 0, domain.EffectRejected)}},
		{ruleA.ID, domain.ExecNotMatched, t0, nil},
		{ruleA.ID, domain.ExecLimited, t0, nil},
		{ruleB.ID, domain.ExecOutOfSchedule, t0, nil},
		{ruleB.ID, domain.ExecSkippedByStop, t0, nil},
		{ruleB.ID, domain.ExecFired, t0, []domain.Effect{eff(eval.ActionCreditPoints, 7, domain.EffectRejected)}},
		{ghost, domain.ExecNotMatched, t0, nil},
		{ruleA.ID, domain.ExecFired, t0.Add(-48 * time.Hour), []domain.Effect{eff(eval.ActionCreditPoints, 1000, domain.EffectApplied)}}, // before from
		{ruleA.ID, domain.ExecFired, t0.Add(24 * time.Hour), []domain.Effect{eff(eval.ActionCreditPoints, 1000, domain.EffectApplied)}},  // at to: excluded
	}
	inTx(t, db, func(tx *gorm.DB) error {
		for _, sp := range specs {
			d := newDecision(tenant, id.NewID())
			d.EvaluatedAt = sp.at
			if _, err := r.InsertDecision(ctx, tx, d); err != nil {
				return err
			}
			ex := domain.Execution{ID: id.NewID(), TenantID: tenant, DecisionID: d.ID, RuleID: sp.rule, RuleVersionID: id.NewID(),
				Status: sp.status, Matched: sp.status == domain.ExecFired, CreatedAt: sp.at}
			if err := r.InsertExecutions(ctx, tx, []domain.Execution{ex}); err != nil {
				return err
			}
			for i, e := range sp.effects {
				e.ID, e.TenantID, e.DecisionID, e.ExecutionID = id.NewID(), tenant, d.ID, ex.ID
				e.RuleID, e.RuleVersionID, e.ActionIndex, e.IdempotencyKey = sp.rule, ex.RuleVersionID, i, id.NewID()
				e.Target, e.Command, e.RequestedAt = "job.x", []byte(`{}`), sp.at
				if err := r.InsertEffects(ctx, tx, []domain.Effect{e}); err != nil {
					return err
				}
			}
		}
		// another tenant's rows are invisible
		d := newDecision(id.NewID(), id.NewID())
		if _, err := r.InsertDecision(ctx, tx, d); err != nil {
			return err
		}
		return r.InsertExecutions(ctx, tx, []domain.Execution{{ID: id.NewID(), TenantID: d.TenantID, DecisionID: d.ID,
			RuleID: ruleA.ID, RuleVersionID: id.NewID(), Status: domain.ExecFired, CreatedAt: t0}})
	})

	stats, err := r.RuleStats(ctx, tenant, t0.Add(-24*time.Hour), t0.Add(24*time.Hour))
	require.NoError(t, err)
	require.Len(t, stats, 3)
	require.Equal(t, app.RuleStat{RuleID: ruleA.ID, Name: ruleA.Name, Fired: 2, NotMatched: 1, Limited: 1,
		EffectsApplied: 2, EffectsRejected: 1, PointsAwarded: 75, XPAwarded: 10}, stats[0], "fired DESC first")
	require.Equal(t, app.RuleStat{RuleID: ruleB.ID, Name: ruleB.Name, Fired: 1, OutOfSchedule: 1, SkippedByStop: 1,
		EffectsRejected: 1, PointsAwarded: 7}, stats[1])
	require.Equal(t, app.RuleStat{RuleID: ghost, NotMatched: 1}, stats[2])

	none, err := r.RuleStats(ctx, tenant, t0.Add(10*24*time.Hour), t0.Add(11*24*time.Hour))
	require.NoError(t, err)
	require.Empty(t, none)
	none, err = r.RuleStats(ctx, "nope", t0, t0.Add(time.Hour))
	require.NoError(t, err)
	require.Empty(t, none)
}
