package app

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	activitycontracts "levelup/internal/modules/activity/contracts"
	badgescontracts "levelup/internal/modules/badges/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/rules/contracts"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
)

func activity(activityID, playerID, eventType, props string) activitycontracts.ReceivedV1 {
	dec := json.NewDecoder(bytes.NewReader([]byte(props)))
	dec.UseNumber()
	var m map[string]any
	_ = dec.Decode(&m)
	return activitycontracts.ReceivedV1{
		ActivityID: activityID, TenantID: tenantA, EventID: "evt-" + activityID[len(activityID)-4:],
		EventType: eventType, PlayerID: playerID, Properties: m, OccurredAt: t0, ReceivedAt: t0,
	}
}

func amountOf(cmd any) int64 { return cmd.(pointscontracts.CreditCmdV1).Amount }

const act1 = "0198d000-0000-7000-8000-00000000a001"

func decisionFor(t *testing.T, h *harness, activityID string) domain.Decision {
	t.Helper()
	d, ok := h.repo.decisions[domain.DecisionID(activityID)]
	require.True(t, ok, "decision recorded")
	return d
}

func TestDecideHappyPathIssuesCommandsAndDecisionFact(t *testing.T) {
	h := newHarness(t, allPerms)
	r := h.liveRule(t, "Purchase Bonus", "purchase", 10,
		`[{"source":"trigger","field":"amount","operator":"gte","value":100}]`,
		`[{"type":"credit_points","amount":50},{"type":"award_badge","badge_id":"`+badgeID+`"},{"type":"grant_xp","amount":5,"description":"xp"}]`, ``)
	h.ob.events = nil

	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{"amount":100.00}`)))

	d := decisionFor(t, h, act1)
	require.Equal(t, contracts.OutcomeMatched, d.Outcome)
	require.Equal(t, playerP, d.PlayerID)
	require.Equal(t, int64(1), d.RulesetGeneration)
	require.Equal(t, []string{"job.points.credit", "job.badges.award", "job.progression.grant_xp", contracts.TopicDecisionMade}, h.ob.topics())

	key0 := domain.EffectKey(act1, r.Current.ID, 0)
	src := effect.Source{Kind: effect.SourceRule, ID: domain.EffectID(act1, r.Current.ID, 0), ActivityID: act1}
	require.Equal(t, pointscontracts.CreditCmdV1{IdempotencyKey: key0, TenantID: tenantA, PlayerID: playerP, Amount: 50,
		Kind: pointscontracts.KindEarn, Description: "Rule: Purchase Bonus", Source: src, OccurredAt: t0},
		h.ob.byTopic("job.points.credit")[0], "B7: the engine injects rule/activity provenance")
	award := h.ob.byTopic("job.badges.award")[0].(badgescontracts.AwardCmdV1)
	require.Equal(t, badgeID, award.BadgeID)
	require.Equal(t, domain.EffectKey(act1, r.Current.ID, 1), award.IdempotencyKey)
	xp := h.ob.byTopic("job.progression.grant_xp")[0].(progressioncontracts.GrantXPCmdV1)
	require.Equal(t, int64(5), xp.Amount)

	made := h.ob.byTopic(contracts.TopicDecisionMade)[0].(contracts.DecisionMadeV1)
	require.Equal(t, d.ID, made.DecisionID)
	require.Equal(t, []string{r.Rule.ID}, made.MatchedRules)
	require.Len(t, made.Effects, 3)
	require.Equal(t, key0, made.Effects[0].IdempotencyKey)

	require.Len(t, h.repo.executions, 1)
	ex := h.repo.executions[0]
	require.Equal(t, domain.ExecFired, ex.Status)
	require.Equal(t, 3, ex.EffectsCount)
	require.Len(t, ex.ConditionResults, 1)
	require.True(t, ex.ConditionResults[0].Result)
	require.Len(t, h.repo.effects, 3)
	for _, e := range h.repo.effects {
		require.Equal(t, domain.EffectRequested, e.Status)
		require.NotEmpty(t, e.Command)
	}
}

func TestDecideNoRulesStillRecordsDecision(t *testing.T) { // G01
	h := newHarness(t, allPerms)
	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{}`)))
	d := decisionFor(t, h, act1)
	require.Equal(t, contracts.OutcomeNoMatch, d.Outcome)
	require.Empty(t, h.repo.executions)
	require.Equal(t, []string{contracts.TopicDecisionMade}, h.ob.topics())
}

func TestDecideNoMatchRecordsSkippedExecution(t *testing.T) { // G03
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, `[{"type":"trigger","field":"amount","operator":"greater_than","value":200}]`, credit50, ``)
	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{"amount":100}`)))
	require.Equal(t, contracts.OutcomeNoMatch, decisionFor(t, h, act1).Outcome)
	require.Equal(t, domain.ExecNotMatched, h.repo.executions[0].Status)
	require.Empty(t, h.repo.effects)
}

// G22/G28: a redelivered activity decides nothing new.
func TestDecideIsIdempotentOnRedelivery(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, ``, credit50, `{"max_per_player":5}`)
	h.ob.events = nil
	ev := activity(act1, playerP, "purchase", `{}`)
	require.NoError(t, h.svc.Decide(context.Background(), ev))
	require.NoError(t, h.svc.Decide(context.Background(), ev))
	require.Len(t, h.repo.decisions, 1)
	require.Len(t, h.repo.effects, 1)
	require.Len(t, h.ob.byTopic("job.points.credit"), 1)
	require.Len(t, h.ob.byTopic(contracts.TopicDecisionMade), 1)
	require.EqualValues(t, 1, h.repo.counters[tenantA+"|"+h.repo.effects[0].RuleID+"|"+playerP+"|lifetime"].count,
		"limit counters are not double counted")
}

// G28: an attempt that crashed mid-publish rolls back; the redelivery
// re-issues exactly the same decision id and effect keys.
func TestDecideRetryAfterFailureReusesKeys(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, ``, `[{"type":"credit_points","amount":1},{"type":"grant_xp","amount":2}]`, `{"max_per_player":1}`)
	h.ob.events = nil
	ev := activity(act1, playerP, "purchase", `{}`)

	h.ob.failOn = contracts.TopicDecisionMade
	err := h.svc.Decide(context.Background(), ev)
	isKind(t, err, errs.Unavailable)
	require.Empty(t, h.repo.decisions, "rolled back")
	require.Empty(t, h.repo.counters, "limit counters rolled back too")
	require.Empty(t, h.ob.events)

	h.ob.failOn = ""
	require.NoError(t, h.svc.Decide(context.Background(), ev))
	d := decisionFor(t, h, act1)
	require.Equal(t, contracts.OutcomeMatched, d.Outcome, "the limit was not consumed by the failed attempt")
	var keys []string
	for _, e := range h.repo.effects {
		keys = append(keys, e.IdempotencyKey)
	}
	vid := h.repo.effects[0].RuleVersionID
	require.Equal(t, []string{domain.EffectKey(act1, vid, 0), domain.EffectKey(act1, vid, 1)}, keys)
}

func TestDecideRejectsUnknownOrInactivePlayer(t *testing.T) {
	cases := []struct {
		name   string
		ev     activitycontracts.ReceivedV1
		reason string
	}{
		{"G26 unknown external id", func() activitycontracts.ReceivedV1 {
			ev := activity(act1, "", "purchase", `{}`)
			ev.PlayerExternalID = "nobody"
			return ev
		}(), effect.ReasonPlayerNotFound},
		{"G25 player of another tenant", activity(act1, playerB, "purchase", `{}`), effect.ReasonPlayerNotFound},
		{"inactive player", activity(act1, playerQ, "purchase", `{}`), effect.ReasonPlayerInactive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, allPerms)
			h.liveRule(t, "r", "purchase", 0, ``, credit50, ``)
			h.ob.events = nil
			require.NoError(t, h.svc.Decide(context.Background(), tc.ev), "a rejection is a result, not an error")
			d := decisionFor(t, h, act1)
			require.Equal(t, contracts.OutcomeRejected, d.Outcome)
			require.Equal(t, tc.reason, d.Reason)
			require.Empty(t, h.repo.effects)
			require.Equal(t, []string{contracts.TopicDecisionMade}, h.ob.topics())
		})
	}
}

func TestDecideResolvesByExternalID(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, `[{"source":"player","field":"attributes.tier","operator":"eq","value":"gold"}]`, credit50, ``)
	ev := activity(act1, "", "purchase", `{}`)
	ev.PlayerExternalID = "ext-p"
	require.NoError(t, h.svc.Decide(context.Background(), ev))
	d := decisionFor(t, h, act1)
	require.Equal(t, playerP, d.PlayerID)
	require.Equal(t, contracts.OutcomeMatched, d.Outcome)
}

func TestDecidePortFailureIsRetried(t *testing.T) {
	h := newHarness(t, allPerms)
	h.players.err = errs.New(errs.Unavailable, "player module down")
	err := h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{}`))
	isKind(t, err, errs.Unavailable)
	require.Empty(t, h.repo.decisions)
}

func TestDecideInvalidPayloadIsDeadLettered(t *testing.T) {
	h := newHarness(t, allPerms)
	bad := []activitycontracts.ReceivedV1{
		{TenantID: tenantA, EventType: "e", PlayerID: playerP},
		{ActivityID: act1, EventType: "e", PlayerID: playerP},
		{ActivityID: act1, TenantID: tenantA, PlayerID: playerP},
		{ActivityID: act1, TenantID: tenantA, EventType: "e"},
	}
	for _, ev := range bad {
		isKind(t, h.svc.Decide(context.Background(), ev), errs.Invalid)
	}
}

func TestDecideCausationDepthCap(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "badge_earned", 0, ``, credit50, ``)
	h.ob.events = nil

	deep := activity(act1, playerP, "badge_earned", `{}`)
	deep.CausationDepth = 4
	require.NoError(t, h.svc.Decide(context.Background(), deep))
	d := decisionFor(t, h, act1)
	require.Equal(t, contracts.OutcomeRejected, d.Outcome)
	require.Equal(t, contracts.ReasonCausationDepthExceeded, d.Reason)
	require.Empty(t, h.repo.effects)

	atCap := activity("0198d000-0000-7000-8000-00000000a002", playerP, "badge_earned", `{}`)
	atCap.CausationDepth = 3
	require.NoError(t, h.svc.Decide(context.Background(), atCap))
	require.Len(t, h.repo.effects, 1)
}

func TestDecideLimitsMaxPerPlayer(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "first purchase", "purchase", 0, ``, credit50, `{"max_per_player":1}`)
	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{}`)))
	second := "0198d000-0000-7000-8000-00000000a002"
	require.NoError(t, h.svc.Decide(context.Background(), activity(second, playerP, "purchase", `{}`)))

	require.Equal(t, contracts.OutcomeMatched, decisionFor(t, h, act1).Outcome)
	require.Equal(t, contracts.OutcomeLimitReached, decisionFor(t, h, second).Outcome)
	require.Len(t, h.repo.effects, 1)
	require.Equal(t, domain.ExecLimited, h.repo.executions[1].Status)
	require.True(t, h.repo.executions[1].Matched)
}

func TestDecideLimitsPerDayAndCooldownUseActivityTime(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "daily", "login", 0, ``, credit50, `{"max_per_player_per_day":1,"cooldown_seconds":3600}`)
	mk := func(id string, hoursLater int) activitycontracts.ReceivedV1 {
		ev := activity(id, playerP, "login", `{}`)
		ev.OccurredAt = t0.Add(timeHours(hoursLater))
		return ev
	}
	ids := []string{"0198d000-0000-7000-8000-00000000b001", "0198d000-0000-7000-8000-00000000b002", "0198d000-0000-7000-8000-00000000b003"}
	require.NoError(t, h.svc.Decide(context.Background(), mk(ids[0], 0)))  // fires
	require.NoError(t, h.svc.Decide(context.Background(), mk(ids[1], 2)))  // same day → limited
	require.NoError(t, h.svc.Decide(context.Background(), mk(ids[2], 24))) // next day, outside cooldown → fires
	require.Equal(t, contracts.OutcomeMatched, decisionFor(t, h, ids[0]).Outcome)
	require.Equal(t, contracts.OutcomeLimitReached, decisionFor(t, h, ids[1]).Outcome)
	require.Equal(t, contracts.OutcomeMatched, decisionFor(t, h, ids[2]).Outcome)
}

func TestDecideReadsProgressAndPointsOnlyWhenReferenced(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, ``, credit50, ``)
	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{}`)))
	require.Zero(t, h.progress.calls)
	require.Zero(t, h.points.calls)

	// G13 + G15: real level and balance (Laravel: level always 0)
	h.liveRule(t, "lvl", "level_check", 0, `{"all":[{"source":"player","field":"level","operator":"gte","value":2},{"source":"player","field":"points","operator":"gte","value":100}]}`, credit50, ``)
	require.NoError(t, h.svc.Decide(context.Background(), activity("0198d000-0000-7000-8000-00000000a002", playerP, "level_check", `{}`)))
	require.Equal(t, 1, h.progress.calls)
	require.Equal(t, 1, h.points.calls)
	require.Equal(t, contracts.OutcomeMatched, decisionFor(t, h, "0198d000-0000-7000-8000-00000000a002").Outcome)
}

func TestDecideProgramScoping(t *testing.T) {
	prog := "0198d000-0000-7000-8000-0000000000f1"
	h := newHarness(t, allPerms)
	ctx := asTenant(tenantA)
	v, err := h.svc.CreateRule(ctx, CreateRuleInput{Name: "p", TriggerEvent: "purchase", ProgramID: prog, Actions: raw(credit50)})
	require.NoError(t, err)
	_, err = h.svc.Publish(ctx, v.Rule.ID, 1)
	require.NoError(t, err)

	// no programs port: program_id ignored (tenant-wide)
	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{}`)))
	require.Equal(t, contracts.OutcomeMatched, decisionFor(t, h, act1).Outcome)

	h.svc.SetPrograms(&fakePrograms{enrolled: map[string][]string{}})
	a2 := "0198d000-0000-7000-8000-00000000a002"
	require.NoError(t, h.svc.Decide(context.Background(), activity(a2, playerP, "purchase", `{}`)))
	require.Equal(t, contracts.OutcomeNoMatch, decisionFor(t, h, a2).Outcome)
	require.Equal(t, domain.ExecOutOfScope, h.repo.executions[len(h.repo.executions)-1].Status)

	h.svc.SetPrograms(&fakePrograms{enrolled: map[string][]string{playerP: {prog}}})
	a3 := "0198d000-0000-7000-8000-00000000a003"
	require.NoError(t, h.svc.Decide(context.Background(), activity(a3, playerP, "purchase", `{}`)))
	require.Equal(t, contracts.OutcomeMatched, decisionFor(t, h, a3).Outcome)
}

// G05: effects are issued in rule priority order.
func TestDecideOrdersEffectsByPriority(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "low", "purchase", 10, ``, `[{"type":"credit_points","amount":10}]`, ``)
	h.liveRule(t, "high", "purchase", 20, ``, `[{"type":"credit_points","amount":20}]`, ``)
	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{}`)))
	credits := h.ob.byTopic("job.points.credit")
	require.Len(t, credits, 2)
	require.EqualValues(t, 20, amountOf(credits[0]))
	require.EqualValues(t, 10, amountOf(credits[1]))
}

func TestSettleEffects(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, ``,
		`[{"type":"credit_points","amount":5},{"type":"award_badge","badge_id":"`+badgeID+`"}]`, ``)
	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{}`)))
	creditKey, badgeKey := h.repo.effects[0].IdempotencyKey, h.repo.effects[1].IdempotencyKey

	applied, _ := json.Marshal(pointscontracts.LedgerMovedV1{IdempotencyKey: creditKey, TenantID: tenantA, Amount: 5})
	require.NoError(t, h.svc.Settle(context.Background(), pointscontracts.TopicCredited, applied))
	require.NoError(t, h.svc.Settle(context.Background(), pointscontracts.TopicCredited, applied), "redelivery is a no-op")
	require.Equal(t, domain.EffectApplied, h.repo.effects[0].Status)

	// G16: a badge already earned settles as a rejection; nothing fails
	rejected, _ := json.Marshal(badgescontracts.AwardRejectedV1{IdempotencyKey: badgeKey, TenantID: tenantA, Reason: effect.ReasonAlreadyEarned})
	require.NoError(t, h.svc.Settle(context.Background(), badgescontracts.TopicAwardRejected, rejected))
	require.Equal(t, domain.EffectRejected, h.repo.effects[1].Status)
	require.Equal(t, effect.ReasonAlreadyEarned, h.repo.effects[1].Reason)

	// a late, reordered "applied" for an already-rejected effect changes nothing
	late, _ := json.Marshal(badgescontracts.AwardedV1{IdempotencyKey: badgeKey, TenantID: tenantA})
	require.NoError(t, h.svc.Settle(context.Background(), badgescontracts.TopicAwarded, late))
	require.Equal(t, domain.EffectRejected, h.repo.effects[1].Status)

	// unknown keys (credits not issued by rules) and keyless payloads are ignored
	other, _ := json.Marshal(pointscontracts.LedgerMovedV1{IdempotencyKey: "level_reward:x", TenantID: tenantA})
	require.NoError(t, h.svc.Settle(context.Background(), pointscontracts.TopicCredited, other))
	require.NoError(t, h.svc.Settle(context.Background(), "missions.completed.v1", []byte(`{"attempt_id":"a","tenant_id":"t"}`)))
	// another tenant cannot settle tenant A's effect
	foreign, _ := json.Marshal(pointscontracts.LedgerMovedV1{IdempotencyKey: creditKey, TenantID: tenantB})
	require.NoError(t, h.svc.Settle(context.Background(), pointscontracts.TopicCreditRejected, foreign))
	require.Equal(t, domain.EffectApplied, h.repo.effects[0].Status)

	isKind(t, h.svc.Settle(context.Background(), pointscontracts.TopicCredited, []byte(`{`)), errs.Invalid)
	isKind(t, h.svc.Settle(context.Background(), "points.unknown.v1", applied), errs.Invalid)
}

func TestReconcileRepublishesPendingEffects(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, ``, credit50, ``)
	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{}`)))
	original := h.repo.effects[0].Command
	h.ob.events = nil

	require.NoError(t, h.svc.ReconcileEffects(context.Background()))
	require.Empty(t, h.ob.events, "not pending long enough yet")

	h.clock.Advance(6 * timeMinute)
	require.NoError(t, h.svc.ReconcileEffects(context.Background()))
	require.Equal(t, []string{"job.points.credit"}, h.ob.topics())
	require.JSONEq(t, string(original), string(h.ob.events[0].payload.(json.RawMessage)), "the exact command, same key")
	require.Equal(t, 1, h.repo.effects[0].Attempts)

	require.NoError(t, h.svc.ReconcileEffects(context.Background()))
	require.Len(t, h.ob.events, 1, "just retried: waits another PendingAfter")

	settled, _ := json.Marshal(pointscontracts.LedgerMovedV1{IdempotencyKey: h.repo.effects[0].IdempotencyKey, TenantID: tenantA})
	require.NoError(t, h.svc.Settle(context.Background(), pointscontracts.TopicCredited, settled))
	h.clock.Advance(6 * timeMinute)
	require.NoError(t, h.svc.ReconcileEffects(context.Background()))
	require.Len(t, h.ob.events, 1, "settled effects are never re-published")
}

func TestSimulateWritesNothing(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, `[{"source":"trigger","field":"amount","operator":"gte","value":100}]`, credit50, `{"max_per_player":1}`)
	h.ob.events = nil
	res, err := h.svc.Simulate(asTenant(tenantA), SimulateInput{EventType: "purchase", PlayerExternalID: "ext-p",
		Properties: raw(`{"amount":120}`)})
	require.NoError(t, err)
	require.Equal(t, contracts.OutcomeMatched, res.Outcome)
	require.Equal(t, playerP, res.PlayerID)
	require.Len(t, res.Rules, 1)
	require.Len(t, res.Rules[0].Actions, 1)
	require.EqualValues(t, 1, res.Rules[0].Limits.MaxPerPlayer)
	require.Empty(t, h.repo.decisions)
	require.Empty(t, h.repo.counters)
	require.Empty(t, h.ob.events)

	// inline player, no lookup
	h.players.calls = 0
	res, err = h.svc.Simulate(asTenant(tenantA), SimulateInput{EventType: "purchase", Player: &SimPlayer{Level: ptrTo(2)},
		Properties: raw(`{"amount":99}`)})
	require.NoError(t, err)
	require.Equal(t, contracts.OutcomeNoMatch, res.Outcome)
	require.Zero(t, h.players.calls)

	_, err = h.svc.Simulate(asTenant(tenantA), SimulateInput{EventType: "purchase", PlayerExternalID: "nobody"})
	isKind(t, err, errs.NotFound)
	_, err = h.svc.Simulate(asTenant(tenantA), SimulateInput{EventType: "purchase", Properties: raw(`[1]`)})
	isKind(t, err, errs.Invalid)
	_, err = newHarness(t, allowKeys{}).svc.Simulate(asTenant(tenantA), SimulateInput{EventType: "purchase"})
	isKind(t, err, errs.PermissionDenied)
}

func TestDecisionQueriesAreTenantScoped(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, ``, credit50, ``)
	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{}`)))
	id := domain.DecisionID(act1)

	detail, err := h.svc.GetDecision(asTenant(tenantA), id)
	require.NoError(t, err)
	require.Len(t, detail.Executions, 1)
	require.Len(t, detail.Effects, 1)

	_, err = h.svc.GetDecision(asTenant(tenantB), id)
	isKind(t, err, errs.NotFound)
	rows, _, err := h.svc.ListDecisions(asTenant(tenantA), DecisionFilter{ActivityID: act1}, "", 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	rows, _, err = h.svc.ListDecisions(asTenant(tenantB), DecisionFilter{}, "", 0)
	require.NoError(t, err)
	require.Empty(t, rows)
	_, _, err = newHarness(t, allowKeys{"rules:view": true}).svc.ListDecisions(asTenant(tenantA), DecisionFilter{}, "", 0)
	isKind(t, err, errs.PermissionDenied)
}

func TestPurgeTenantIsIdempotent(t *testing.T) {
	h := newHarness(t, allPerms)
	h.liveRule(t, "r", "purchase", 0, ``, credit50, `{"max_per_player":3}`)
	require.NoError(t, h.svc.Decide(context.Background(), activity(act1, playerP, "purchase", `{}`)))
	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA, t0))
	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA, t0))
	require.Empty(t, h.repo.rules)
	require.Empty(t, h.repo.versions)
	require.Empty(t, h.repo.decisions)
	require.Empty(t, h.repo.effects)
	require.Empty(t, h.repo.counters)
	isKind(t, h.svc.PurgeTenant(context.Background(), "", t0), errs.Invalid)
}

const timeMinute = 60 * 1e9

func timeHours(h int) time.Duration { return time.Duration(h) * time.Hour }
