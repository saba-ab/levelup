package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/badges/internal/domain"
	missionscontracts "levelup/internal/modules/missions/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	streakscontracts "levelup/internal/modules/streaks/contracts"
	"levelup/internal/platform/bus"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

func envelope(t *testing.T, topic string, payload any) bus.Envelope {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return bus.Envelope{EventID: id.NewID(), Topic: topic, OccurredAt: t0, Payload: raw}
}

func requirements(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

func (h *harness) seedAutoBadge(t *testing.T, raw string, mut func(*domain.NewBadgeParams)) domain.Badge {
	t.Helper()
	return h.seedBadge(t, func(p *domain.NewBadgeParams) {
		p.Requirements = requirements(t, raw)
		if mut != nil {
			mut(p)
		}
	})
}

func credited(playerID string, lifetime int64, at time.Time) pointscontracts.LedgerMovedV1 {
	return pointscontracts.LedgerMovedV1{
		EntryID: id.NewID(), IdempotencyKey: id.NewID(), TenantID: tenantA, PlayerID: playerID,
		Amount: 10, LifetimeEarned: lifetime, OccurredAt: at, At: at,
	}
}

func completed(playerID, attemptID string) missionscontracts.CompletedV1 {
	return missionscontracts.CompletedV1{AttemptID: attemptID, TenantID: tenantA, PlayerID: playerID, MissionID: id.NewID(), At: t0}
}

func activity(playerID, eventType string) activitycontracts.ReceivedV1 {
	return activitycontracts.ReceivedV1{
		ActivityID: id.NewID(), TenantID: tenantA, EventID: id.NewID(), EventType: eventType,
		PlayerID: playerID, OccurredAt: t0, ReceivedAt: t0,
	}
}

func (h *harness) holding(badgeID, playerID string) int {
	pb, found, _ := h.repo.PlayerBadge(context.Background(), tenantA, playerID, badgeID)
	if !found {
		return 0
	}
	return pb.EarnedCount
}

// ---- projection ----

func TestLifetimePointsNewestEventWins(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	newer := credited(player1, 900, t0.Add(time.Minute))
	older := credited(player1, 400, t0)

	require.NoError(t, h.svc.OnPointsCredited(ctx, envelope(t, pointscontracts.TopicCredited, newer)))
	require.NoError(t, h.svc.OnPointsCredited(ctx, envelope(t, pointscontracts.TopicCredited, older)), "reordered")
	require.EqualValues(t, 900, h.repo.playerStats(tenantA, player1).LifetimePoints)

	require.NoError(t, h.svc.OnPointsCredited(ctx, envelope(t, pointscontracts.TopicCredited, newer)), "redelivered")
	require.EqualValues(t, 900, h.repo.playerStats(tenantA, player1).LifetimePoints)

	latest := credited(player1, 1200, t0.Add(2*time.Minute))
	require.NoError(t, h.svc.OnPointsCredited(ctx, envelope(t, pointscontracts.TopicCredited, latest)))
	require.EqualValues(t, 1200, h.repo.playerStats(tenantA, player1).LifetimePoints)
}

func TestIncrementsAreDedupedAndMaximaOnlyRise(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()

	m1 := envelope(t, missionscontracts.TopicCompleted, completed(player1, id.NewID()))
	require.NoError(t, h.svc.OnMissionCompleted(ctx, m1))
	require.NoError(t, h.svc.OnMissionCompleted(ctx, m1))
	// Same attempt republished under a new envelope id: still one.
	m1again := m1
	m1again.EventID = id.NewID()
	require.NoError(t, h.svc.OnMissionCompleted(ctx, m1again))
	require.NoError(t, h.svc.OnMissionCompleted(ctx, envelope(t, missionscontracts.TopicCompleted, completed(player1, id.NewID()))))

	a := envelope(t, activitycontracts.TopicReceived, activity(player1, "purchase"))
	require.NoError(t, h.svc.OnActivityReceived(ctx, a))
	require.NoError(t, h.svc.OnActivityReceived(ctx, a))
	require.NoError(t, h.svc.OnActivityReceived(ctx, envelope(t, activitycontracts.TopicReceived, activity(player1, "purchase"))))
	require.NoError(t, h.svc.OnActivityReceived(ctx, envelope(t, activitycontracts.TopicReceived, activity(player1, "login"))))
	unresolved := activity("", "purchase")
	require.NoError(t, h.svc.OnActivityReceived(ctx, envelope(t, activitycontracts.TopicReceived, unresolved)), "no player: skipped")

	for _, n := range []int{5, 9, 3} {
		require.NoError(t, h.svc.OnStreakActivity(ctx, envelope(t, streakscontracts.TopicActivityRecorded,
			streakscontracts.ActivityRecordedV1{TenantID: tenantA, PlayerID: player1, StreakID: id.NewID(), CurrentCount: n})))
	}
	for _, n := range []int{4, 2} {
		require.NoError(t, h.svc.OnLevelReached(ctx, envelope(t, progressioncontracts.TopicLevelReached,
			progressioncontracts.LevelReachedV1{TenantID: tenantA, PlayerID: player1, LevelNumber: n})))
	}
	aw := envelope(t, contracts.TopicAwarded, contracts.AwardedV1{AwardID: id.NewID(), TenantID: tenantA, PlayerID: player1, BadgeID: id.NewID()})
	require.NoError(t, h.svc.OnBadgeAwarded(ctx, aw))
	require.NoError(t, h.svc.OnBadgeAwarded(ctx, aw))

	st := h.repo.playerStats(tenantA, player1)
	require.EqualValues(t, 2, st.MissionsCompleted)
	require.Equal(t, map[string]int64{"purchase": 2, "login": 1}, st.ActivityCounts)
	require.EqualValues(t, 9, st.MaxStreak)
	require.EqualValues(t, 4, st.Level)
	require.EqualValues(t, 1, st.BadgesEarned)
}

func TestProjectionRejectsMalformedFacts(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	bad := bus.Envelope{EventID: id.NewID(), Topic: pointscontracts.TopicCredited, Payload: []byte(`{`)}
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.OnPointsCredited(ctx, bad)))

	noTenant := credited(player1, 10, t0)
	noTenant.TenantID = ""
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.OnPointsCredited(ctx, envelope(t, pointscontracts.TopicCredited, noTenant))))

	noIDs := envelope(t, missionscontracts.TopicCompleted, completed(player1, ""))
	noIDs.EventID = ""
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.OnMissionCompleted(ctx, noIDs)))
}

// ---- evaluation and auto award ----

func TestAllRequirementsAwardOnceWhenLastConditionIsMet(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	b := h.seedAutoBadge(t, `{"all":[{"metric":"lifetime_points","gte":1000},{"metric":"missions_completed","gte":2}]}`, nil)

	require.NoError(t, h.svc.OnPointsCredited(ctx, envelope(t, pointscontracts.TopicCredited, credited(player1, 1500, t0))))
	require.NoError(t, h.svc.OnMissionCompleted(ctx, envelope(t, missionscontracts.TopicCompleted, completed(player1, id.NewID()))))
	require.Zero(t, h.holding(b.ID, player1), "only one of two missions")

	last := envelope(t, missionscontracts.TopicCompleted, completed(player1, id.NewID()))
	require.NoError(t, h.svc.OnMissionCompleted(ctx, last))
	require.Equal(t, 1, h.holding(b.ID, player1))

	awarded := h.ob.byTopic(contracts.TopicAwarded)
	require.Len(t, awarded, 1)
	ev := awarded[0].(contracts.AwardedV1)
	require.Equal(t, contracts.AutoAwardKey(player1, b.ID), ev.IdempotencyKey)
	require.Equal(t, id.Derive("badge_auto", player1, b.ID), ev.IdempotencyKey)
	require.Equal(t, effect.Source{Kind: contracts.SourceRequirements, ID: b.ID}, ev.Source)
	require.Len(t, h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobCredit)), 1, "badge points credited as usual")

	// Redelivery of the deciding fact, further facts and our own awarded
	// fact never award again.
	require.NoError(t, h.svc.OnMissionCompleted(ctx, last))
	require.NoError(t, h.svc.OnMissionCompleted(ctx, envelope(t, missionscontracts.TopicCompleted, completed(player1, id.NewID()))))
	require.NoError(t, h.svc.OnBadgeAwarded(ctx, envelope(t, contracts.TopicAwarded, ev)))
	require.Len(t, h.ob.byTopic(contracts.TopicAwarded), 1)
	require.Empty(t, h.ob.byTopic(contracts.TopicAwardRejected), "an earned badge is skipped, not rejected")
	require.Len(t, h.repo.appliedAwards(), 1)
}

func TestAnyRequirementsAwardOnFirstMatchingCondition(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	b := h.seedAutoBadge(t, `{"any":[{"metric":"level","gte":5},{"metric":"streak_days","gte":7}]}`, nil)

	require.NoError(t, h.svc.OnLevelReached(ctx, envelope(t, progressioncontracts.TopicLevelReached,
		progressioncontracts.LevelReachedV1{TenantID: tenantA, PlayerID: player1, LevelNumber: 4})))
	require.Zero(t, h.holding(b.ID, player1))
	require.NoError(t, h.svc.OnStreakActivity(ctx, envelope(t, streakscontracts.TopicActivityRecorded,
		streakscontracts.ActivityRecordedV1{TenantID: tenantA, PlayerID: player1, CurrentCount: 7})))
	require.Equal(t, 1, h.holding(b.ID, player1))
	require.NoError(t, h.svc.OnLevelReached(ctx, envelope(t, progressioncontracts.TopicLevelReached,
		progressioncontracts.LevelReachedV1{TenantID: tenantA, PlayerID: player1, LevelNumber: 5})))
	require.Len(t, h.ob.byTopic(contracts.TopicAwarded), 1)
}

func TestActivityCountPerEventTypeAndChainedBadgesEarned(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	buyer := h.seedAutoBadge(t, `{"all":[{"metric":"activity_count","event_type":"purchase","gte":2}]}`, nil)
	collector := h.seedAutoBadge(t, `{"all":[{"metric":"badges_earned","gte":1}]}`, nil)

	require.NoError(t, h.svc.OnActivityReceived(ctx, envelope(t, activitycontracts.TopicReceived, activity(player1, "purchase"))))
	require.NoError(t, h.svc.OnActivityReceived(ctx, envelope(t, activitycontracts.TopicReceived, activity(player1, "login"))))
	require.Zero(t, h.holding(buyer.ID, player1), "logins do not count as purchases")
	second := envelope(t, activitycontracts.TopicReceived, activity(player1, "purchase"))
	require.NoError(t, h.svc.OnActivityReceived(ctx, second))
	require.Equal(t, 1, h.holding(buyer.ID, player1))
	require.Zero(t, h.holding(collector.ID, player1), "badges_earned moves only with badges.awarded.v1")

	// The worker delivers our own awarded fact back to us.
	ev := h.ob.byTopic(contracts.TopicAwarded)[0].(contracts.AwardedV1)
	require.NoError(t, h.svc.OnBadgeAwarded(ctx, envelope(t, contracts.TopicAwarded, ev)))
	require.Equal(t, 1, h.holding(collector.ID, player1))
	require.NoError(t, h.svc.OnActivityReceived(ctx, second), "redelivery")
	require.Len(t, h.ob.byTopic(contracts.TopicAwarded), 2)
}

func TestAutoAwardRespectsStackableMaxInactiveAndPlayer(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	raw := `{"all":[{"metric":"level","gte":2}]}`
	held := h.seedAutoBadge(t, raw, nil)
	stackFull := h.seedAutoBadge(t, raw, func(p *domain.NewBadgeParams) { p.Stackable = true; p.MaxAwards = intp(1) })
	stackOpen := h.seedAutoBadge(t, raw, func(p *domain.NewBadgeParams) { p.Stackable = true; p.MaxAwards = intp(3) })
	inactive := h.seedAutoBadge(t, raw, func(p *domain.NewBadgeParams) { p.Active = false })
	legacy := h.seedBadge(t, nil)
	legacy.Requirements = map[string]any{"level": 2.0} // pre-grammar row
	h.repo.badges[legacy.ID] = legacy

	for _, b := range []domain.Badge{held, stackFull, stackOpen} {
		require.NoError(t, h.svc.HandleAwardJob(ctx, jobBody(t, awardCmd(b.ID, player1, "manual-"+b.ID))))
	}
	before := len(h.ob.byTopic(contracts.TopicAwarded))

	level := envelope(t, progressioncontracts.TopicLevelReached,
		progressioncontracts.LevelReachedV1{TenantID: tenantA, PlayerID: player1, LevelNumber: 2})
	require.NoError(t, h.svc.OnLevelReached(ctx, level))
	require.NoError(t, h.svc.OnLevelReached(ctx, level))

	require.Equal(t, 1, h.holding(held.ID, player1), "non-stackable already earned")
	require.Equal(t, 1, h.holding(stackFull.ID, player1), "max_awards reached")
	require.Equal(t, 2, h.holding(stackOpen.ID, player1), "one automatic stack, once")
	require.Zero(t, h.holding(inactive.ID, player1))
	require.Zero(t, h.holding(legacy.ID, player1), "pre-grammar requirements are never evaluated")
	require.Len(t, h.ob.byTopic(contracts.TopicAwarded), before+1)
	require.Empty(t, h.ob.byTopic(contracts.TopicAwardRejected))

	// An inactive player is skipped without recording anything, so the
	// badge is still awarded once the player is active again.
	inactiveBadge := h.seedAutoBadge(t, `{"all":[{"metric":"level","gte":1}]}`, nil)
	lv := envelope(t, progressioncontracts.TopicLevelReached,
		progressioncontracts.LevelReachedV1{TenantID: tenantA, PlayerID: player2, LevelNumber: 1})
	require.NoError(t, h.svc.OnLevelReached(ctx, lv))
	require.Zero(t, h.holding(inactiveBadge.ID, player2))
	_, found, _ := h.repo.AwardByKey(ctx, tenantA, contracts.AutoAwardKey(player2, inactiveBadge.ID))
	require.False(t, found)
	snap := h.players.players[player2]
	snap.Active = true
	h.players.players[player2] = snap
	require.NoError(t, h.svc.OnLevelReached(ctx, lv))
	require.Equal(t, 1, h.holding(inactiveBadge.ID, player2))
}

func TestRevokedAutoBadgeIsNotReawarded(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	b := h.seedAutoBadge(t, `{"all":[{"metric":"level","gte":1}]}`, nil)
	lv := func(n int) bus.Envelope {
		return envelope(t, progressioncontracts.TopicLevelReached,
			progressioncontracts.LevelReachedV1{TenantID: tenantA, PlayerID: player1, LevelNumber: n})
	}
	require.NoError(t, h.svc.OnLevelReached(ctx, lv(1)))
	require.Equal(t, 1, h.holding(b.ID, player1))
	require.NoError(t, h.svc.Revoke(ctxAs(tenantA), b.ID, player1))
	require.NoError(t, h.svc.OnLevelReached(ctx, lv(2)))
	require.Zero(t, h.holding(b.ID, player1), "exactly once: the auto key is spent")
	require.Len(t, h.ob.byTopic(contracts.TopicAwarded), 1)
}

func TestEvaluationRetriesAfterTransientFailure(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	b := h.seedAutoBadge(t, `{"all":[{"metric":"missions_completed","gte":1}]}`, nil)
	e := envelope(t, missionscontracts.TopicCompleted, completed(player1, id.NewID()))

	h.players.err = errors.New("player provider down")
	require.Error(t, h.svc.OnMissionCompleted(ctx, e), "the subscription is retried")
	require.EqualValues(t, 1, h.repo.playerStats(tenantA, player1).MissionsCompleted)
	require.Zero(t, h.holding(b.ID, player1))

	h.players.err = nil
	require.NoError(t, h.svc.OnMissionCompleted(ctx, e))
	require.EqualValues(t, 1, h.repo.playerStats(tenantA, player1).MissionsCompleted, "not counted twice")
	require.Equal(t, 1, h.holding(b.ID, player1), "the redelivery finishes the award")
}

func TestOnlyBadgesReadingTheChangedMetricAreEvaluated(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	b := h.seedAutoBadge(t, `{"all":[{"metric":"lifetime_points","gte":1}]}`, nil)
	// Stats already satisfy the badge, but a mission fact does not touch
	// lifetime_points, so nothing is looked up or awarded.
	h.repo.stats[ak(tenantA, player1)] = &fakeStats{PlayerStats: domain.PlayerStats{LifetimePoints: 5, ActivityCounts: map[string]int64{}}}
	calls := h.players.calls
	require.NoError(t, h.svc.OnMissionCompleted(ctx, envelope(t, missionscontracts.TopicCompleted, completed(player1, id.NewID()))))
	require.Zero(t, h.holding(b.ID, player1))
	require.Equal(t, calls, h.players.calls)
}

func TestTenantIsolationOfTheProjection(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	b := h.seedAutoBadge(t, `{"all":[{"metric":"level","gte":1}]}`, nil)
	// Same player id reported under another tenant: different projection,
	// and tenant A's badge is not evaluated for it.
	require.NoError(t, h.svc.OnLevelReached(ctx, envelope(t, progressioncontracts.TopicLevelReached,
		progressioncontracts.LevelReachedV1{TenantID: tenantB, PlayerID: player1, LevelNumber: 3})))
	require.Zero(t, h.holding(b.ID, player1))
	require.Zero(t, h.repo.playerStats(tenantA, player1).Level)
	require.EqualValues(t, 3, h.repo.playerStats(tenantB, player1).Level)
}

func TestPruneAppliedEventsHonoursRetention(t *testing.T) {
	h := newHarness(t, allPerms)
	h.repo.applied[ak(tenantA, "old")] = t0.Add(-40 * 24 * time.Hour)
	h.repo.applied[ak(tenantA, "new")] = t0.Add(-time.Hour)
	require.NoError(t, h.svc.PruneAppliedEvents(context.Background(), 30*24*time.Hour))
	require.Len(t, h.repo.applied, 1)
	_, kept := h.repo.applied[ak(tenantA, "new")]
	require.True(t, kept)
	require.NoError(t, h.svc.PruneAppliedEvents(context.Background(), 0), "disabled")
}

// ---- stats ----

func TestStatsZeroFillsThirtyDays(t *testing.T) {
	h := newHarness(t, allPerms)
	last := t0.Add(-time.Hour)
	h.repo.awardStats = AwardStats{
		Badges:        []BadgeStat{{BadgeID: "b1", AwardedCount: 3, UniquePlayers: 2, LastAwardedAt: &last}},
		PerDay:        map[string]int64{"2026-10-05": 2, "2026-09-06": 1, "2026-09-05": 7},
		TotalAwarded:  3,
		UniquePlayers: 2,
	}
	rep, err := h.svc.Stats(ctxAs(tenantA))
	require.NoError(t, err)
	require.Len(t, rep.AwardsPerDay, StatsDays)
	require.Equal(t, DayCount{Day: "2026-09-06", Count: 1}, rep.AwardsPerDay[0], "oldest first, 30 days incl. today")
	require.Equal(t, DayCount{Day: "2026-10-05", Count: 2}, rep.AwardsPerDay[StatsDays-1])
	require.Equal(t, DayCount{Day: "2026-09-20", Count: 0}, rep.AwardsPerDay[14])
	require.Equal(t, time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC), h.repo.statsSince)
	require.Equal(t, h.repo.awardStats.Badges, rep.Badges)
	require.EqualValues(t, 3, rep.TotalAwarded)

	_, err = newHarness(t, allowKeys{}).svc.Stats(ctxAs(tenantA))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = h.svc.Stats(context.Background())
	require.Error(t, err, "no tenant")
}

func TestAutoCreatedPlayersFirstActivityWaitsThenCounts(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := context.Background()
	fp := h.players

	ev := activity("", "purchase")
	ev.PlayerExternalID, ev.AutoCreatePlayer = "ext-new", true
	env := envelope(t, activitycontracts.TopicReceived, ev)
	require.Equal(t, errs.Unavailable, errs.KindOf(h.svc.OnActivityReceived(ctx, env)), "retried until the player exists")

	fp.external = map[string]string{"ext-new": player1}
	require.NoError(t, h.svc.OnActivityReceived(ctx, env))
	require.EqualValues(t, 1, h.repo.playerStats(tenantA, player1).ActivityCounts["purchase"])
}
