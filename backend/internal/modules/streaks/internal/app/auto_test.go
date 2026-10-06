package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/streaks/contracts"
	"levelup/internal/modules/streaks/internal/domain"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
)

func activity(activityID, eventType string, at time.Time) activitycontracts.ReceivedV1 {
	return activitycontracts.ReceivedV1{
		ActivityID: activityID,
		TenantID:   tenantA,
		EventID:    "evt-" + activityID,
		EventType:  eventType,
		PlayerID:   player1,
		OccurredAt: at,
		ReceivedAt: at,
	}
}

func TestActivityRecordsMatchingStreak(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{PointsPerPeriod: 10, Milestones: []domain.Milestone{{Count: 1, BonusPoints: 5}}})
	require.True(t, st.AutoRecord, "auto_record defaults to true")

	res, err := h.svc.HandleActivity(context.Background(), activity("act-1", "daily_login", h.clock.Now()))
	require.NoError(t, err)
	require.Equal(t, OutcomeRecorded, res.Outcome)
	require.Equal(t, 1, res.PlayerStreak.CurrentCount)
	require.Equal(t, []int{1}, res.MilestonesReached)
	require.Equal(t, 1, h.ob.count(contracts.TopicActivityRecorded))
	require.Equal(t, 1, h.ob.count(contracts.TopicMilestoneReached))
	credits := h.credits()
	require.Len(t, credits, 2, "period points and milestone bonus")
	require.Equal(t, "act-1", credits[0].Source.ActivityID)

	ev := h.ob.published[0].payload
	for _, r := range h.ob.published {
		if r.topic == contracts.TopicActivityRecorded {
			ev = r.payload
		}
	}
	rec := ev.(contracts.ActivityRecordedV1)
	require.Equal(t, AutoKey("act-1", st.ID), rec.IdempotencyKey)
	require.Equal(t, effect.Source{Kind: effect.SourceStreak, ID: st.ID, ActivityID: "act-1"}, rec.Source)
}

func TestActivityRedeliveryIsNoop(t *testing.T) {
	h := newHarness(t, allPerms())
	h.seedStreak(t, domain.NewStreakInput{PointsPerPeriod: 10})
	ev := activity("act-1", "daily_login", h.clock.Now())

	_, err := h.svc.HandleActivity(context.Background(), ev)
	require.NoError(t, err)
	published := len(h.ob.published)

	res, err := h.svc.HandleActivity(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, OutcomeDuplicate, res.Outcome)
	require.Len(t, h.ob.published, published, "a redelivered activity publishes nothing")
	require.Len(t, h.credits(), 1)
}

func TestSecondActivitySamePeriodIsNoop(t *testing.T) {
	h := newHarness(t, allPerms())
	h.seedStreak(t, domain.NewStreakInput{PointsPerPeriod: 10})
	_, err := h.svc.HandleActivity(context.Background(), activity("act-1", "daily_login", h.clock.Now()))
	require.NoError(t, err)
	res, err := h.svc.HandleActivity(context.Background(), activity("act-2", "daily_login", h.clock.Now()))
	require.NoError(t, err)
	require.Equal(t, OutcomeNoop, res.Outcome)
	require.Len(t, h.credits(), 1, "a period pays once")
}

func TestActivityBucketsByOccurredAt(t *testing.T) {
	h := newHarness(t, allPerms())
	h.seedStreak(t, domain.NewStreakInput{})
	for i, d := range []int{3, 4, 5} {
		_, err := h.svc.HandleActivity(context.Background(), activity("act-"+string(rune('a'+i)), "daily_login", day(d).Add(9*time.Hour)))
		require.NoError(t, err)
	}
	rows, err := h.repo.PlayerStreaksByPlayers(context.Background(), tenantA, []string{player1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 3, rows[0].CurrentCount)
}

func TestActivityResolvesPlayerByExternalID(t *testing.T) {
	h := newHarness(t, allPerms())
	h.seedStreak(t, domain.NewStreakInput{})
	ev := activity("act-1", "daily_login", h.clock.Now())
	ev.PlayerID, ev.PlayerExternalID = "", "ext-1"
	res, err := h.svc.HandleActivity(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, OutcomeRecorded, res.Outcome)
	require.Equal(t, player1, res.PlayerStreak.PlayerID)
}

func TestActivitySkipsSilently(t *testing.T) {
	cases := map[string]func(h *harness, ev *activitycontracts.ReceivedV1){
		"no matching streak": func(_ *harness, ev *activitycontracts.ReceivedV1) { ev.EventType = "purchase" },
		"unknown player":     func(_ *harness, ev *activitycontracts.ReceivedV1) { ev.PlayerID = "ghost" },
		"unknown external id": func(_ *harness, ev *activitycontracts.ReceivedV1) {
			ev.PlayerID, ev.PlayerExternalID = "", "nobody"
		},
		"inactive player": func(_ *harness, ev *activitycontracts.ReceivedV1) { ev.PlayerID = player2 },
		"other tenant":    func(_ *harness, ev *activitycontracts.ReceivedV1) { ev.TenantID = tenantB },
		"opted out streak": func(h *harness, _ *activitycontracts.ReceivedV1) {
			h.patch(&domain.StreakPatch{AutoRecord: ptr(false)})
		},
		"inactive streak": func(h *harness, _ *activitycontracts.ReceivedV1) { h.patch(&domain.StreakPatch{Active: ptr(false)}) },
		"deleted streak":  func(h *harness, _ *activitycontracts.ReceivedV1) { h.deleteStreak() },
		"case differs":    func(_ *harness, ev *activitycontracts.ReceivedV1) { ev.EventType = "Daily_Login" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, allPerms())
			h.st = h.seedStreak(t, domain.NewStreakInput{PointsPerPeriod: 10})
			h.t = t
			ev := activity("act-1", "daily_login", h.clock.Now())
			mutate(h, &ev)
			h.ob.reset()
			res, err := h.svc.HandleActivity(context.Background(), ev)
			require.NoError(t, err)
			require.Equal(t, OutcomeNoop, res.Outcome)
			require.Empty(t, h.ob.published, "skips publish nothing, not even a rejection")
			require.Empty(t, h.repo.requests, "skips record nothing")
		})
	}
}

func TestOptOutCanBeReverted(t *testing.T) {
	h := newHarness(t, allPerms())
	h.t = t
	h.st = h.seedStreak(t, domain.NewStreakInput{AutoRecord: ptr(false)})
	require.False(t, h.st.AutoRecord)

	res, err := h.svc.HandleActivity(context.Background(), activity("act-1", "daily_login", h.clock.Now()))
	require.NoError(t, err)
	require.Equal(t, OutcomeNoop, res.Outcome)

	h.patch(&domain.StreakPatch{AutoRecord: ptr(true)})
	res, err = h.svc.HandleActivity(context.Background(), activity("act-1", "daily_login", h.clock.Now()))
	require.NoError(t, err)
	require.Equal(t, OutcomeRecorded, res.Outcome, "the skipped delivery wrote nothing, so a later one applies")

	// Explicit record commands still work while opted out.
	h.patch(&domain.StreakPatch{AutoRecord: ptr(false)})
	rec := h.record(t, h.st, "k-explicit", day(4))
	require.Equal(t, OutcomeRecorded, rec.Outcome)
}

func TestMalformedActivityIsInvalid(t *testing.T) {
	h := newHarness(t, allPerms())
	h.seedStreak(t, domain.NewStreakInput{})
	for name, ev := range map[string]activitycontracts.ReceivedV1{
		"no tenant":      {ActivityID: "a", EventType: "daily_login", PlayerID: player1},
		"no activity id": {TenantID: tenantA, EventType: "daily_login", PlayerID: player1},
		"no event type":  {TenantID: tenantA, ActivityID: "a", PlayerID: player1},
		"no player":      {TenantID: tenantA, ActivityID: "a", EventType: "daily_login"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.svc.HandleActivity(context.Background(), ev)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
		})
	}
}

func TestUpdateTogglesAutoRecord(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{})
	got, err := h.svc.Update(ctxFor(tenantA), st.ID, domain.StreakPatch{AutoRecord: ptr(false)})
	require.NoError(t, err)
	require.False(t, got.AutoRecord)
	got, err = h.svc.Update(ctxFor(tenantA), st.ID, domain.StreakPatch{Name: ptr("Renamed")})
	require.NoError(t, err)
	require.False(t, got.AutoRecord, "an omitted field stays untouched")
}

func (h *harness) patch(p *domain.StreakPatch) {
	h.t.Helper()
	st, err := h.svc.Update(ctxFor(tenantA), h.st.ID, *p)
	require.NoError(h.t, err)
	h.st = st
}

func (h *harness) deleteStreak() {
	h.t.Helper()
	require.NoError(h.t, h.svc.Delete(ctxFor(tenantA), h.st.ID))
}

func ptr[T any](v T) *T { return &v }

func TestActivityWaitsForAutoCreatedPlayer(t *testing.T) {
	h := newHarness(t, allPerms())
	h.seedStreak(t, domain.NewStreakInput{})
	ev := activity("act-1", "daily_login", h.clock.Now())
	ev.PlayerID, ev.PlayerExternalID, ev.AutoCreatePlayer = "", "not-yet", true
	_, err := h.svc.HandleActivity(context.Background(), ev)
	require.Equal(t, errs.Unavailable, errs.KindOf(err), "retried until the player module creates the player")
	require.Empty(t, h.repo.requests)

	ev.PlayerExternalID = "ext-1"
	res, err := h.svc.HandleActivity(context.Background(), ev)
	require.NoError(t, err)
	require.Equal(t, OutcomeRecorded, res.Outcome)
}
