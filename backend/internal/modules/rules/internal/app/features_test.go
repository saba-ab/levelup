package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/rules/contracts"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/modules/rules/internal/ports"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
)

// liveSpec creates and publishes a rule with a schedule / stop flag.
func (h *harness) liveSpec(t *testing.T, name, trigger string, priority int, in CreateRuleInput) RuleView {
	t.Helper()
	ctx := asTenant(tenantA)
	in.Name, in.TriggerEvent, in.Priority = name, trigger, priority
	v, err := h.svc.CreateRule(ctx, in)
	require.NoError(t, err)
	pub, err := h.svc.Publish(ctx, v.Rule.ID, 1)
	require.NoError(t, err)
	return pub
}

func execOf(t *testing.T, h *harness, activityID, ruleID string) domain.Execution {
	t.Helper()
	d := domain.DecisionID(activityID)
	for _, e := range h.repo.executions {
		if e.DecisionID == d && e.RuleID == ruleID {
			return e
		}
	}
	t.Fatalf("no execution of rule %s for activity %s", ruleID, activityID)
	return domain.Execution{}
}

const (
	actA = "0198d000-0000-7000-8000-00000000f001"
	actB = "0198d000-0000-7000-8000-00000000f002"
	actC = "0198d000-0000-7000-8000-00000000f003"
)

func TestDecideStopProcessingSkipsLowerPriorityRules(t *testing.T) {
	h := newHarness(t, allPerms)
	stopper := h.liveSpec(t, "vip", "purchase", 10, CreateRuleInput{Actions: raw(credit50), StopProcessing: true,
		Limits: raw(`{"max_per_player":1}`)})
	lower := h.liveSpec(t, "standard", "purchase", 5, CreateRuleInput{Actions: raw(`[{"type":"credit_points","amount":5}]`),
		Limits: raw(`{"max_per_player":10}`)})
	h.ob.events = nil

	require.NoError(t, h.svc.Decide(context.Background(), activity(actA, playerP, "purchase", `{}`)))
	require.Equal(t, domain.ExecFired, execOf(t, h, actA, stopper.Rule.ID).Status)
	skipped := execOf(t, h, actA, lower.Rule.ID)
	require.Equal(t, domain.ExecSkippedByStop, skipped.Status)
	require.False(t, skipped.Matched)
	require.Empty(t, skipped.ConditionResults)
	require.Len(t, h.repo.effects, 1, "a skipped rule emits nothing")
	require.Len(t, h.ob.byTopic("job.points.credit"), 1)
	for k := range h.repo.counters {
		require.NotContains(t, k, lower.Rule.ID, "a skipped rule consumes no limit counter")
	}
	made := h.ob.byTopic(contracts.TopicDecisionMade)[0].(contracts.DecisionMadeV1)
	require.Equal(t, []string{stopper.Rule.ID}, made.MatchedRules)

	// The stopper is now limited: a limited rule did not fire, so it stops nothing.
	require.NoError(t, h.svc.Decide(context.Background(), activity(actB, playerP, "purchase", `{}`)))
	require.Equal(t, domain.ExecLimited, execOf(t, h, actB, stopper.Rule.ID).Status)
	require.Equal(t, domain.ExecFired, execOf(t, h, actB, lower.Rule.ID).Status)
	require.Equal(t, contracts.OutcomeMatched, decisionFor(t, h, actB).Outcome)
}

func TestDecideScheduleUsesOccurredAt(t *testing.T) {
	h := newHarness(t, allPerms)
	// t0 is Monday 12:00 UTC. The clock is irrelevant: only occurred_at counts.
	weekend := h.liveSpec(t, "weekend", "purchase", 10, CreateRuleInput{Actions: raw(credit50), StopProcessing: true,
		Schedule: raw(`{"days_of_week":[0,6]}`)})
	always := h.liveSpec(t, "always", "purchase", 1, CreateRuleInput{Actions: raw(credit50)})

	require.NoError(t, h.svc.Decide(context.Background(), activity(actA, playerP, "purchase", `{}`)))
	require.Equal(t, domain.ExecOutOfSchedule, execOf(t, h, actA, weekend.Rule.ID).Status)
	require.Equal(t, domain.ExecFired, execOf(t, h, actA, always.Rule.ID).Status, "out_of_schedule does not stop")

	sat := activity(actB, playerP, "purchase", `{}`)
	sat.OccurredAt = time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	require.NoError(t, h.svc.Decide(context.Background(), sat))
	require.Equal(t, domain.ExecFired, execOf(t, h, actB, weekend.Rule.ID).Status)
	require.Equal(t, domain.ExecSkippedByStop, execOf(t, h, actB, always.Rule.ID).Status)
}

func TestDecideOutOfScheduleOnlyIsNoMatch(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveSpec(t, "october", "purchase", 0, CreateRuleInput{Actions: raw(credit50),
		Schedule: raw(`{"ends_at":"2026-10-01T00:00:00Z"}`)})
	require.NoError(t, h.svc.Decide(context.Background(), activity(actA, playerP, "purchase", `{}`)))
	require.Equal(t, contracts.OutcomeNoMatch, decisionFor(t, h, actA).Outcome)
	require.Empty(t, h.repo.effects)
}

func TestDecideHistoryFirstTimeAndCounts(t *testing.T) {
	h := newHarness(t, allPerms)
	first := h.liveRule(t, "first purchase", "purchase", 10, `[{"source":"history","field":"first_time"}]`, credit50, ``)
	third := h.liveRule(t, "third login this week", "purchase", 5,
		`[{"source":"history","field":"count","event_type":"login","window":"7d","operator":"gte","value":2}]`, credit50, ``)

	require.NoError(t, h.svc.Decide(context.Background(), activity(actA, playerP, "purchase", `{}`)))
	require.Equal(t, domain.ExecFired, execOf(t, h, actA, first.Rule.ID).Status)
	require.Equal(t, domain.ExecNotMatched, execOf(t, h, actA, third.Rule.ID).Status)
	require.Equal(t, 1, h.repo.historyHits, "one history query per decision")

	// Redelivery: no new decision, the projection is not bumped twice.
	require.NoError(t, h.svc.Decide(context.Background(), activity(actA, playerP, "purchase", `{}`)))
	require.Equal(t, int64(1), h.repo.eventDays[dayKey(tenantA, playerP, "purchase", t0)])

	// Two logins (no login rules: no history query, but still projected).
	for i, id := range []string{"0198d000-0000-7000-8000-00000000e001", "0198d000-0000-7000-8000-00000000e002"} {
		ev := activity(id, playerP, "login", `{}`)
		ev.OccurredAt = t0.Add(-time.Duration(i+1) * 24 * time.Hour)
		require.NoError(t, h.svc.Decide(context.Background(), ev))
	}
	require.Equal(t, 1, h.repo.historyHits, "rulesets without history leaves skip the query")

	require.NoError(t, h.svc.Decide(context.Background(), activity(actB, playerP, "purchase", `{}`)))
	require.Equal(t, domain.ExecNotMatched, execOf(t, h, actB, first.Rule.ID).Status, "the second purchase is not the first")
	ex := execOf(t, h, actB, third.Rule.ID)
	require.Equal(t, domain.ExecFired, ex.Status)
	require.Equal(t, int64(2), ex.ConditionResults[0].Actual)

	// An old login 8 days back is outside 7d.
	eight := activity(actC, playerP, "purchase", `{}`)
	eight.OccurredAt = t0.Add(-8 * 24 * time.Hour)
	require.NoError(t, h.svc.Decide(context.Background(), eight))
	require.Equal(t, domain.ExecNotMatched, execOf(t, h, actC, third.Rule.ID).Status)
	require.Equal(t, domain.ExecFired, execOf(t, h, actC, first.Rule.ID).Status,
		"first_time is event-time: nothing was recorded on or before that day")
}

func TestDecideRejectedActivitiesAreNotProjected(t *testing.T) {
	h := newHarness(t, allPerms)
	require.NoError(t, h.svc.Decide(context.Background(), activity(actA, playerQ, "purchase", `{}`))) // inactive
	deep := activity(actB, playerP, "purchase", `{}`)
	deep.CausationDepth = 9
	require.NoError(t, h.svc.Decide(context.Background(), deep))
	require.Empty(t, h.repo.eventDays)
}

func TestDecideAutoCreatePlayerRetriesInsteadOfRejecting(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, ``, credit50, ``)
	ev := activity(actA, "", "purchase", `{}`)
	ev.PlayerExternalID, ev.AutoCreatePlayer = "new-player", true
	h.ob.events = nil

	err := h.svc.Decide(context.Background(), ev)
	isKind(t, err, errs.Unavailable)
	require.Empty(t, h.repo.decisions, "nothing recorded: the delivery stays replayable")
	require.Empty(t, h.ob.events)

	// The player module created it meanwhile: the retried delivery decides.
	newID := "0198d000-0000-7000-8000-000000000103"
	h.players.players[newID] = playerSnapshot(newID, "new-player")
	require.NoError(t, h.svc.Decide(context.Background(), ev))
	d := decisionFor(t, h, actA)
	require.Equal(t, contracts.OutcomeMatched, d.Outcome)
	require.Equal(t, newID, d.PlayerID)

	// Without the flag an unknown player is still a recorded rejection.
	plain := activity(actB, "", "purchase", `{}`)
	plain.PlayerExternalID = "ghost"
	require.NoError(t, h.svc.Decide(context.Background(), plain))
	require.Equal(t, effect.ReasonPlayerNotFound, decisionFor(t, h, actB).Reason)
}

func TestSimulateDraftDefinition(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "live", "purchase", 0, ``, credit50, ``)
	ctx := asTenant(tenantA)
	occurred := time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC) // Saturday

	res, err := h.svc.Simulate(ctx, SimulateInput{PlayerID: playerP, OccurredAt: &occurred,
		Properties: raw(`{"amount":120}`),
		Definition: &DraftDefinition{TriggerEvent: "purchase", Definition: domain.Definition{
			Conditions: raw(`[{"source":"trigger","field":"amount","operator":"gte","value":100},{"source":"history","field":"first_time"}]`),
			Actions:    raw(`[{"type":"grant_xp","amount":7}]`),
			Schedule:   raw(`{"days_of_week":[6]}`),
		}}})
	require.NoError(t, err)
	require.True(t, res.Draft)
	require.True(t, res.HistoryLoaded)
	require.Equal(t, int64(0), res.RulesetGeneration)
	require.Equal(t, occurred, res.OccurredAt)
	require.Len(t, res.Rules, 1, "only the draft is evaluated, not the live ruleset")
	require.Equal(t, DraftRuleID, res.Rules[0].RuleID)
	require.Equal(t, contracts.OutcomeMatched, res.Outcome)
	require.Equal(t, int64(7), res.Rules[0].Actions[0].Amount)

	// Same draft on a Monday (default occurred_at = clock = t0): out of schedule.
	res, err = h.svc.Simulate(ctx, SimulateInput{EventType: "purchase", Player: &SimPlayer{},
		Definition: &DraftDefinition{TriggerEvent: "purchase", Definition: domain.Definition{
			Actions: raw(credit50), Schedule: raw(`{"days_of_week":[6]}`)}}})
	require.NoError(t, err)
	require.Equal(t, eval.StatusOutOfSchedule, res.Rules[0].Status)
	require.Equal(t, contracts.OutcomeNoMatch, res.Outcome)
	require.False(t, res.HistoryLoaded, "inline players have no history")

	// Compile errors are 422 with fields under definition.*.
	_, err = h.svc.Simulate(ctx, SimulateInput{Definition: &DraftDefinition{TriggerEvent: "Bad Trigger",
		Definition: domain.Definition{Actions: raw(`[{"type":"credit_points","amount":0}]`), Schedule: raw(`{"timezone":"Nowhere"}`)}}})
	isKind(t, err, errs.Invalid)
	require.Equal(t, contracts.CodeInvalidRuleDefinition, errs.CodeOf(err))
	fields := errs.FieldsOf(err)
	require.Contains(t, fields, "definition.trigger_event")
	require.Contains(t, fields, "definition.actions[0].amount")
	require.Contains(t, fields, "definition.schedule.timezone")

	_, err = h.svc.Simulate(ctx, SimulateInput{EventType: "login",
		Definition: &DraftDefinition{TriggerEvent: "purchase", Definition: domain.Definition{Actions: raw(credit50)}}})
	require.ErrorIs(t, err, domain.ErrDraftTriggerMismatch)

	_, err = h.svc.Simulate(ctx, SimulateInput{})
	isKind(t, err, errs.Invalid)
	require.Contains(t, errs.FieldsOf(err), "event_type")
	require.Empty(t, h.repo.eventDays, "simulation writes no history")
}

func TestSimulateLiveAppliesStopAndHistory(t *testing.T) {
	h := newHarness(t, allPerms)
	stop := h.liveSpec(t, "stop", "purchase", 10, CreateRuleInput{Actions: raw(credit50), StopProcessing: true,
		Limits: raw(`{"max_per_player":1}`)})
	lower := h.liveRule(t, "lower", "purchase", 1, `[{"source":"history","field":"first_time"}]`, credit50, ``)
	res, err := h.svc.Simulate(asTenant(tenantA), SimulateInput{EventType: "purchase", PlayerID: playerP})
	require.NoError(t, err)
	require.Equal(t, stop.Rule.ID, res.Rules[0].RuleID)
	require.True(t, res.Rules[0].StopProcessing)
	require.Equal(t, eval.StatusSkippedByStop, res.Rules[1].Status)
	require.Equal(t, stop.Rule.ID, res.Rules[1].StoppedBy)
	require.Equal(t, lower.Rule.ID, res.Rules[1].RuleID)
	require.True(t, res.HistoryLoaded)
}

func TestStats(t *testing.T) {
	h := newHarness(t, allPerms)
	h.repo.stats = []RuleStat{
		{RuleID: "a", Name: "A", Fired: 3, NotMatched: 1, EffectsApplied: 2, PointsAwarded: 150, XPAwarded: 10},
		{RuleID: "b", Name: "B", Fired: 1, Limited: 2, OutOfSchedule: 4, SkippedByStop: 5, EffectsRejected: 1, PointsAwarded: 5},
	}
	ctx := asTenant(tenantA)
	res, err := h.svc.Stats(ctx, nil, nil)
	require.NoError(t, err)
	require.Equal(t, t0, res.To)
	require.Equal(t, t0.Add(-30*24*time.Hour), res.From)
	require.Len(t, res.Rules, 2)
	require.Equal(t, RuleStat{Fired: 4, NotMatched: 1, Limited: 2, OutOfSchedule: 4, SkippedByStop: 5,
		EffectsApplied: 2, EffectsRejected: 1, PointsAwarded: 155, XPAwarded: 10}, res.Totals)

	from := t0.Add(-time.Hour)
	res, err = h.svc.Stats(ctx, &from, nil)
	require.NoError(t, err)
	require.Equal(t, from, res.From)

	h.repo.stats = nil
	res, err = h.svc.Stats(ctx, &from, nil)
	require.NoError(t, err)
	require.NotNil(t, res.Rules)
	require.Empty(t, res.Rules)

	for _, bad := range [][2]time.Time{{t0, t0}, {t0, t0.Add(-time.Hour)}, {t0.Add(-400 * 24 * time.Hour), t0}} {
		_, err = h.svc.Stats(ctx, &bad[0], &bad[1])
		require.ErrorIs(t, err, domain.ErrBadStatsRange)
	}
	_, err = newHarness(t, allowKeys{"rules:view_any": true}).svc.Stats(ctx, nil, nil)
	isKind(t, err, errs.PermissionDenied)
}

func TestLifecycleCarriesScheduleAndStopProcessing(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := asTenant(tenantA)
	v, err := h.svc.CreateRule(ctx, CreateRuleInput{Name: "s", TriggerEvent: "purchase", Actions: raw(credit50),
		Schedule: raw(`{"hours":{"from":"09:00","to":"17:00"}}`), StopProcessing: true})
	require.NoError(t, err)
	require.True(t, v.Latest.StopProcessing)
	require.JSONEq(t, `{"hours":{"from":"09:00","to":"17:00"}}`, string(v.Latest.Schedule))

	_, err = h.svc.CreateRule(ctx, CreateRuleInput{Name: "bad", TriggerEvent: "purchase", Actions: raw(credit50),
		Schedule: raw(`{"days_of_week":[9]}`)})
	isKind(t, err, errs.Invalid)
	require.Contains(t, errs.FieldsOf(err), "schedule.days_of_week[0]")

	off := false
	upd, err := h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{StopProcessing: &off, Schedule: raw(`null`)})
	require.NoError(t, err)
	require.False(t, upd.Latest.StopProcessing)
	require.Equal(t, "null", string(upd.Latest.Schedule))

	_, err = h.svc.Publish(ctx, v.Rule.ID, 1)
	require.NoError(t, err)
	on := true
	_, err = h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{StopProcessing: &on})
	require.ErrorIs(t, err, domain.ErrNoDraft, "published versions are immutable")

	v2, err := h.svc.CreateVersion(ctx, v.Rule.ID, CreateVersionInput{StopProcessing: &on,
		Schedule: raw(`{"timezone":"Asia/Tbilisi","days_of_week":[1]}`)})
	require.NoError(t, err)
	require.True(t, v2.StopProcessing)
	require.JSONEq(t, string(raw(credit50)), string(v2.Actions), "other parts are copied")
	v3, err := h.svc.CreateVersion(ctx, v.Rule.ID, CreateVersionInput{})
	require.NoError(t, err)
	require.True(t, v3.StopProcessing, "copied from the latest version")
	require.Equal(t, v2.Schedule, v3.Schedule)

	pub, err := h.svc.Publish(ctx, v.Rule.ID, 3)
	require.NoError(t, err)
	srcs, err := h.repo.LiveRuleSources(ctx, tenantA, "purchase")
	require.NoError(t, err)
	require.Len(t, srcs, 1)
	require.True(t, srcs[0].StopProcessing)
	require.Equal(t, pub.Current.ID, srcs[0].RuleVersionID)
}

func playerSnapshot(id, externalID string) ports.PlayerSnapshot {
	return ports.PlayerSnapshot{ID: id, TenantID: tenantA, ExternalID: externalID, Active: true}
}
