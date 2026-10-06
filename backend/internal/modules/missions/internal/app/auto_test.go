package app

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/missions/internal/domain"
	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
)

const actID = "0198d000-0000-7000-8000-0000000a0001"

// activityEv builds an activity as the subscriber decodes it (UseNumber).
func activityEv(t *testing.T, activityID, eventType, playerID, properties string) activitycontracts.ReceivedV1 {
	t.Helper()
	var p map[string]any
	if properties != "" {
		dec := json.NewDecoder(bytes.NewReader([]byte(properties)))
		dec.UseNumber()
		require.NoError(t, dec.Decode(&p))
	}
	return activitycontracts.ReceivedV1{
		ActivityID: activityID, TenantID: tenantA, EventID: "evt-" + activityID, EventType: eventType,
		PlayerID: playerID, Properties: p, OccurredAt: start, ReceivedAt: start,
	}
}

func crit(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

func (h *harness) autoMission(t *testing.T, criteria string, mod func(*CreateMissionCmd)) domain.Mission {
	t.Helper()
	return h.mission(t, func(c *CreateMissionCmd) {
		c.Criteria = crit(t, criteria)
		if mod != nil {
			mod(c)
		}
	})
}

func TestActivityProgressesMatchingMissionAndCompletes(t *testing.T) {
	h := newHarness(t, nil)
	m := h.autoMission(t, `{"event_type": "purchase_completed"}`, nil) // target 3, rewards
	other := h.autoMission(t, `{"event_type": "login"}`, func(c *CreateMissionCmd) { c.Name = "Logins" })
	manual := h.mission(t, func(c *CreateMissionCmd) { c.Name = "Manual" })

	for i, a := range []string{"0198d000-0000-7000-8000-0000000a0011", "0198d000-0000-7000-8000-0000000a0012", "0198d000-0000-7000-8000-0000000a0013"} {
		res, err := h.svc.HandleActivity(context.Background(), activityEv(t, a, "purchase_completed", player1, `{"amount": 10}`))
		require.NoError(t, err)
		require.Equal(t, AutoResult{Matched: 1, Applied: 1, Complete: map[bool]int{true: 1}[i == 2]}, res)
	}
	attempts := h.attempts(m.ID, player1)
	require.Len(t, attempts, 1)
	require.Equal(t, contracts.AttemptCompleted, attempts[0].Status)
	require.EqualValues(t, 3, attempts[0].Progress)
	require.Empty(t, h.attempts(other.ID, player1))
	require.Empty(t, h.attempts(manual.ID, player1))

	require.Len(t, h.ob.byTopic(contracts.TopicStarted), 1)
	require.Len(t, h.ob.byTopic(contracts.TopicProgressUpdated), 3)
	completed := h.ob.byTopic(contracts.TopicCompleted)
	require.Len(t, completed, 1)
	require.Equal(t, "0198d000-0000-7000-8000-0000000a0013", completed[0].(contracts.CompletedV1).ActivityID)
	credits := h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobCredit))
	require.Len(t, credits, 1, "completion rewards ride the same path")

	updated := h.ob.byTopic(contracts.TopicProgressUpdated)[0].(contracts.AttemptV1)
	require.Equal(t, AutoKey("0198d000-0000-7000-8000-0000000a0011", m.ID, player1), updated.IdempotencyKey)
}

func TestActivityRedeliveryIsNoOp(t *testing.T) {
	h := newHarness(t, nil)
	m := h.autoMission(t, `{"event_type": "purchase_completed"}`, nil)
	ev := activityEv(t, actID, "purchase_completed", player1, "")

	_, err := h.svc.HandleActivity(context.Background(), ev)
	require.NoError(t, err)
	published := len(h.ob.published)

	res, err := h.svc.HandleActivity(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, AutoResult{Matched: 1}, res, "the duplicate key applies nothing")
	require.Len(t, h.ob.published, published)
	require.EqualValues(t, 1, h.attempts(m.ID, player1)[0].Progress)
}

func TestActivityPropertyIncrementAndWhere(t *testing.T) {
	h := newHarness(t, nil)
	m := h.autoMission(t, `{"event_type": "purchase_completed",
		"where": [{"field": "currency", "operator": "eq", "value": "USD"}, {"field": "cart.total", "operator": "gte", "value": 50}],
		"increment": {"by": "property", "field": "cart.total"}}`,
		func(c *CreateMissionCmd) { c.Target = 500 })

	cases := []struct {
		props string
		want  int64 // cumulative progress after the activity
	}{
		{`{"currency": "USD", "cart": {"total": 120.75}}`, 120},
		{`{"currency": "EUR", "cart": {"total": 300}}`, 120},  // where fails
		{`{"currency": "USD", "cart": {"total": 20}}`, 120},   // below gte
		{`{"currency": "USD"}`, 120},                          // missing field
		{`{"currency": "USD", "cart": {"total": "80"}}`, 200}, // numeric string
		{`{"currency": "USD", "cart": {"total": 1000}}`, 500}, // capped at target, completes
		{`{"currency": "USD", "cart": {"total": 60}}`, 60},    // repeating: fresh attempt
	}
	for i, c := range cases {
		activityID := "0198d000-0000-7000-8000-0000000b00" + string(rune('a'+i)) + "0"
		_, err := h.svc.HandleActivity(context.Background(), activityEv(t, activityID, "purchase_completed", player1, c.props))
		require.NoError(t, err)
		var latest domain.Attempt
		for _, a := range h.attempts(m.ID, player1) {
			if latest.ID == "" || a.CreatedAt.After(latest.CreatedAt) || a.Status == contracts.AttemptInProgress {
				latest = a
			}
		}
		require.Equal(t, c.want, latest.Progress, c.props)
	}
}

func TestActivityStartsAndRespectsLimitsQuietly(t *testing.T) {
	h := newHarness(t, nil)
	m := h.autoMission(t, `{"event_type": "login"}`, func(c *CreateMissionCmd) {
		c.Type = contracts.TypeOneTime
		c.Target = 1
	})
	_, err := h.svc.HandleActivity(context.Background(), activityEv(t, "0198d000-0000-7000-8000-0000000c0001", "login", player1, ""))
	require.NoError(t, err)
	require.Len(t, h.ob.byTopic(contracts.TopicCompleted), 1)
	h.ob.published = nil

	res, err := h.svc.HandleActivity(context.Background(), activityEv(t, "0198d000-0000-7000-8000-0000000c0002", "login", player1, ""))
	require.NoError(t, err)
	require.Equal(t, AutoResult{Matched: 1}, res)
	require.Empty(t, h.ob.published, "limit reached is silent: no progress_rejected for activities")
	require.Len(t, h.attempts(m.ID, player1), 1)
}

func TestActivitySkipsSilently(t *testing.T) {
	future := start.Add(24 * time.Hour)
	past := start.Add(-time.Hour)
	cases := map[string]struct {
		mod    func(*CreateMissionCmd)
		mutate func(*activitycontracts.ReceivedV1)
	}{
		"other event type":   {nil, func(ev *activitycontracts.ReceivedV1) { ev.EventType = "refund" }},
		"unknown player":     {nil, func(ev *activitycontracts.ReceivedV1) { ev.PlayerID = unknown }},
		"inactive player":    {nil, func(ev *activitycontracts.ReceivedV1) { ev.PlayerID = inactive }},
		"unknown external":   {nil, func(ev *activitycontracts.ReceivedV1) { ev.PlayerID, ev.PlayerExternalID = "", "nobody" }},
		"other tenant":       {nil, func(ev *activitycontracts.ReceivedV1) { ev.TenantID = tenantB }},
		"draft mission":      {func(c *CreateMissionCmd) { c.Status = contracts.MissionDraft }, nil},
		"not yet started":    {func(c *CreateMissionCmd) { c.StartsAt = &future }, nil},
		"window ended":       {func(c *CreateMissionCmd) { c.EndsAt = &past; s := past.Add(-time.Hour); c.StartsAt = &s }, nil},
		"causation too deep": {nil, func(ev *activitycontracts.ReceivedV1) { ev.CausationDepth = DefaultMaxCausationDepth + 1 }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, nil)
			h.autoMission(t, `{"event_type": "purchase_completed"}`, c.mod)
			ev := activityEv(t, actID, "purchase_completed", player1, "")
			if c.mutate != nil {
				c.mutate(&ev)
			}
			res, err := h.svc.HandleActivity(context.Background(), ev)
			require.NoError(t, err)
			require.Zero(t, res.Applied)
			require.Empty(t, h.ob.published)
			require.Empty(t, h.repo.attempts)
		})
	}
}

func TestActivityResolvesPlayerByExternalID(t *testing.T) {
	h := newHarness(t, nil)
	m := h.autoMission(t, `{"event_type": "purchase_completed"}`, nil)
	ev := activityEv(t, actID, "purchase_completed", "", "")
	ev.PlayerExternalID = "ext-2"
	res, err := h.svc.HandleActivity(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, 1, res.Applied)
	require.Len(t, h.attempts(m.ID, player2), 1)
}

func TestActivityProgressesEveryMatchingMission(t *testing.T) {
	h := newHarness(t, nil)
	a := h.autoMission(t, `{"event_type": "purchase_completed"}`, func(c *CreateMissionCmd) { c.Name = "A" })
	b := h.autoMission(t, `{"event_type": "purchase_completed", "where": [{"field": "vip", "operator": "eq", "value": true}]}`,
		func(c *CreateMissionCmd) { c.Name = "B" })
	cc := h.autoMission(t, `{"event_type": "purchase_completed", "where": [{"field": "vip", "operator": "exists", "value": false}]}`,
		func(c *CreateMissionCmd) { c.Name = "C" })

	res, err := h.svc.HandleActivity(context.Background(), activityEv(t, actID, "purchase_completed", player1, `{"vip": true}`))
	require.NoError(t, err)
	require.Equal(t, 2, res.Applied)
	require.Len(t, h.attempts(a.ID, player1), 1)
	require.Len(t, h.attempts(b.ID, player1), 1)
	require.Empty(t, h.attempts(cc.ID, player1))
}

func TestMissionCompletedDoesNotTriggerItself(t *testing.T) {
	h := newHarness(t, nil)
	self := h.autoMission(t, `{"event_type": "mission_completed"}`, func(c *CreateMissionCmd) { c.Name = "Self"; c.Target = 1 })
	meta := h.autoMission(t, `{"event_type": "mission_completed"}`, func(c *CreateMissionCmd) { c.Name = "Meta"; c.Target = 5 })

	ev := activityEv(t, actID, activitycontracts.EventTypeMissionCompleted, player1, `{"mission_id": "`+self.ID+`"}`)
	ev.CausationDepth = 1
	res, err := h.svc.HandleActivity(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, 1, res.Applied)
	require.Empty(t, h.attempts(self.ID, player1), "a mission never progresses on its own completion")
	require.Len(t, h.attempts(meta.ID, player1), 1)
}

func TestLegacyCriteriaStayManual(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)
	// A row written before the grammar existed (bypassing validation).
	legacy := h.repo.missions[m.ID]
	legacy.Criteria = map[string]any{"event_type": "purchase_completed", "conditions": []any{}}
	h.repo.missions[m.ID] = legacy

	res, err := h.svc.HandleActivity(context.Background(), activityEv(t, actID, "purchase_completed", player1, ""))
	require.NoError(t, err)
	require.Zero(t, res.Matched)
	require.Empty(t, h.repo.attempts)
}

func TestMalformedActivityIsInvalid(t *testing.T) {
	h := newHarness(t, nil)
	for name, ev := range map[string]activitycontracts.ReceivedV1{
		"no tenant":       {ActivityID: actID, EventType: "x", PlayerID: player1},
		"tenant not uuid": {TenantID: "t", ActivityID: actID, EventType: "x", PlayerID: player1},
		"no activity id":  {TenantID: tenantA, EventType: "x", PlayerID: player1},
		"no event type":   {TenantID: tenantA, ActivityID: actID, PlayerID: player1},
		"no player":       {TenantID: tenantA, ActivityID: actID, EventType: "x"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.svc.HandleActivity(context.Background(), ev)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
		})
	}
}

func TestActivityTransientPlayerErrorRetries(t *testing.T) {
	h := newHarness(t, nil)
	h.autoMission(t, `{"event_type": "purchase_completed"}`, nil)
	h.svc.players.(*fakePlayers).err = errs.New(errs.Unavailable, "player down")
	_, err := h.svc.HandleActivity(context.Background(), activityEv(t, actID, "purchase_completed", player1, ""))
	require.Equal(t, errs.Unavailable, errs.KindOf(err))
	require.Empty(t, h.ob.published)
}

func TestAutoSourceCarriesActivity(t *testing.T) {
	h := newHarness(t, nil)
	m := h.autoMission(t, `{"event_type": "purchase_completed"}`, nil)
	_, err := h.svc.HandleActivity(context.Background(), activityEv(t, actID, "purchase_completed", player1, ""))
	require.NoError(t, err)
	ev, ok, err := h.repo.ProgressEventByKey(context.Background(), tenantA, AutoKey(actID, m.ID, player1))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, effect.SourceMission, ev.SourceKind)
	require.Equal(t, m.ID, ev.SourceID)
}

// ---- criteria validation through the service ----

func TestCreateAndPatchValidateCriteria(t *testing.T) {
	h := newHarness(t, nil)
	_, err := h.svc.CreateMission(h.ctx, CreateMissionCmd{
		Name: "Bad", Type: contracts.TypeRepeating, Target: 1,
		Criteria: crit(t, `{"event_type": "purchase", "where": [{"field": "amount", "operator": "between", "value": 1}]}`),
	})
	requireCode(t, err, errs.Invalid, domain.CodeInvalidCriteria)
	require.Equal(t, map[string]string{"criteria.where[0].operator": "must be one of eq, neq, gt, gte, lt, lte, in, contains, exists"},
		errs.FieldsOf(err))
	require.Empty(t, h.ob.published)

	m := h.mission(t, nil)
	_, err = h.svc.UpdateMission(h.ctx, m.ID, UpdateMissionCmd{Criteria: crit(t, `{"increment": {"by": "property"}}`)})
	requireCode(t, err, errs.Invalid, domain.CodeInvalidCriteria)
	require.Contains(t, errs.FieldsOf(err), "criteria.event_type")

	got, err := h.svc.UpdateMission(h.ctx, m.ID, UpdateMissionCmd{
		Criteria: crit(t, `{"event_type": "purchase", "increment": {"by": "property", "field": "qty"}}`),
	})
	require.NoError(t, err)
	require.Equal(t, "purchase", got.Criteria["event_type"])

	// A legacy stored criteria does not block unrelated updates.
	legacy := h.repo.missions[m.ID]
	legacy.Criteria = map[string]any{"min_amount": 3.0}
	h.repo.missions[m.ID] = legacy
	name := "Renamed"
	_, err = h.svc.UpdateMission(h.ctx, m.ID, UpdateMissionCmd{Name: &name})
	require.NoError(t, err)
}

// ---- stats ----

func TestMissionStats(t *testing.T) {
	h := newHarness(t, nil)
	m := h.autoMission(t, `{"event_type": "purchase_completed"}`, func(c *CreateMissionCmd) { c.Target = 2 })
	empty := h.mission(t, func(c *CreateMissionCmd) { c.Name = "Empty" })

	// player1 completes after 3 hours; player2 stays in progress.
	_, err := h.svc.HandleActivity(context.Background(), activityEv(t, "0198d000-0000-7000-8000-0000000d0001", "purchase_completed", player1, ""))
	require.NoError(t, err)
	_, err = h.svc.HandleActivity(context.Background(), activityEv(t, "0198d000-0000-7000-8000-0000000d0002", "purchase_completed", player2, ""))
	require.NoError(t, err)
	h.clock.Advance(3 * time.Hour)
	_, err = h.svc.HandleActivity(context.Background(), activityEv(t, "0198d000-0000-7000-8000-0000000d0003", "purchase_completed", player1, ""))
	require.NoError(t, err)

	st, err := h.svc.GetMissionStats(h.ctx, m.ID)
	require.NoError(t, err)
	require.EqualValues(t, 2, st.Started)
	require.EqualValues(t, 1, st.InProgress)
	require.EqualValues(t, 1, st.Completed)
	require.InDelta(t, 0.5, st.CompletionRate, 1e-9)
	require.NotNil(t, st.AvgHoursToComplete)
	require.InDelta(t, 3.0, *st.AvgHoursToComplete, 1e-9)

	zero, err := h.svc.GetMissionStats(h.ctx, empty.ID)
	require.NoError(t, err)
	require.Zero(t, zero.Started)
	require.Zero(t, zero.CompletionRate)
	require.Nil(t, zero.AvgHoursToComplete)

	page, err := h.svc.ListMissionStats(h.ctx, MissionFilter{}, "", 10)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	byID := map[string]MissionStats{}
	for _, it := range page.Items {
		byID[it.MissionID] = it
	}
	require.EqualValues(t, 1, byID[m.ID].Completed)
	require.Equal(t, "Empty", byID[empty.ID].Name)

	// Authorization and tenancy.
	_, err = h.svc.GetMissionStats(h.ctx, "0198d000-0000-7000-8000-0000000fffff")
	requireCode(t, err, errs.NotFound, "mission_not_found")
	denied := newHarness(t, allowKeys{})
	denied.svc.repo = h.repo
	_, err = denied.svc.GetMissionStats(denied.ctx, m.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = denied.svc.ListMissionStats(denied.ctx, MissionFilter{}, "", 10)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestMissionStatsCrossTenantIsNotFound(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)
	ctxB := authz.Into(context.Background(), authz.Principal{UserID: "b", TenantID: tenantB})
	_, err := h.svc.GetMissionStats(ctxB, m.ID)
	requireCode(t, err, errs.NotFound, "mission_not_found")
	page, err := h.svc.ListMissionStats(ctxB, MissionFilter{}, "", 10)
	require.NoError(t, err)
	require.Empty(t, page.Items)
}

func TestActivityWaitsForAutoCreatedPlayer(t *testing.T) {
	h := newHarness(t, nil)
	m := h.autoMission(t, `{"event_type": "purchase_completed"}`, nil)
	ev := activityEv(t, actID, "purchase_completed", "", "")
	ev.PlayerExternalID, ev.AutoCreatePlayer = "not-yet", true
	_, err := h.svc.HandleActivity(context.Background(), ev)
	require.Equal(t, errs.Unavailable, errs.KindOf(err), "retried until the player module creates the player")
	require.Empty(t, h.repo.attempts)

	ev.PlayerExternalID = "ext-2" // the player now exists
	res, err := h.svc.HandleActivity(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, 1, res.Applied)
	require.Len(t, h.attempts(m.ID, player2), 1)
}
