package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/streaks/contracts"
	"levelup/internal/modules/streaks/internal/domain"
	"levelup/internal/modules/streaks/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

const (
	tenantA = "tenant-a"
	tenantB = "tenant-b"
	player1 = "player-1"
	player2 = "player-2" // inactive
)

var jobCredit = pointscontracts.Topic(pointscontracts.JobCredit)

type harness struct {
	svc   *Service
	repo  *fakeRepo
	ob    *fakeOutbox
	clock *clock.Fake
	tz    *fakeTenants

	// st and t serve the auto-record helpers (patch, deleteStreak).
	st domain.Streak
	t  *testing.T
}

func allPerms() allowKeys {
	a := allowKeys{}
	for _, p := range contracts.AllPermissions {
		a[p.Key()] = true
	}
	return a
}

func newHarness(t *testing.T, enf authz.Enforcer) *harness {
	t.Helper()
	h := &harness{
		repo:  newFakeRepo(),
		ob:    &fakeOutbox{},
		clock: clock.NewFake(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)),
		tz:    &fakeTenants{tz: map[string]string{tenantA: "UTC", tenantB: "UTC"}},
	}
	players := &fakePlayers{players: map[string]ports.PlayerSnapshot{
		player1: {ID: player1, TenantID: tenantA, ExternalID: "ext-1", Active: true},
		player2: {ID: player2, TenantID: tenantA, ExternalID: "ext-2", Active: false},
	}}
	h.svc = NewService(h.repo, players, h.tz, h.ob, enf, nil, h.clock)
	h.svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
	return h
}

func ctxFor(tenant string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "user-1", TenantID: tenant, RoleIDs: []int64{2}})
}

func (h *harness) seedStreak(t *testing.T, in domain.NewStreakInput) domain.Streak {
	t.Helper()
	if in.Name == "" {
		in.Name = "Daily Login"
	}
	if in.ActivityKey == "" {
		in.ActivityKey = "daily_login"
	}
	if in.Period == "" {
		in.Period = "daily"
	}
	in.Active = true
	st, err := domain.NewStreak(tenantA, in, h.clock.Now())
	require.NoError(t, err)
	require.NoError(t, h.repo.CreateStreak(context.Background(), nil, st))
	return st
}

func day(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }

func (h *harness) record(t *testing.T, st domain.Streak, key string, at time.Time) RecordResult {
	t.Helper()
	res, err := h.svc.HandleRecord(context.Background(), contracts.RecordCmdV1{
		IdempotencyKey: key,
		TenantID:       tenantA,
		PlayerID:       player1,
		StreakID:       st.ID,
		Source:         effect.Source{Kind: effect.SourceRule, ID: "eff-" + key, ActivityID: "act-" + key},
		OccurredAt:     at,
	})
	require.NoError(t, err)
	return res
}

func (h *harness) credits() []pointscontracts.CreditCmdV1 {
	var out []pointscontracts.CreditCmdV1
	for _, r := range h.ob.published {
		if r.topic == jobCredit {
			out = append(out, r.payload.(pointscontracts.CreditCmdV1))
		}
	}
	return out
}

// ---- CRUD ----

func TestCreateStampsTenantAndRequiresPermission(t *testing.T) {
	h := newHarness(t, allPerms())
	st, err := h.svc.Create(ctxFor(tenantA), domain.NewStreakInput{Name: "Daily Login", ActivityKey: "daily_login", Period: "daily", Active: true})
	require.NoError(t, err)
	require.Equal(t, tenantA, st.TenantID)
	require.Equal(t, "daily-login", st.Slug)

	_, err = h.svc.Create(ctxFor(tenantA), domain.NewStreakInput{Name: "Other", ActivityKey: "daily_login", Period: "daily"})
	require.ErrorIs(t, err, domain.ErrActivityKeyTaken)

	member := newHarness(t, allowKeys{contracts.PermViewAny.Key(): true})
	_, err = member.svc.Create(ctxFor(tenantA), domain.NewStreakInput{Name: "X", ActivityKey: "x", Period: "daily"})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	_, err = h.svc.Create(authz.Into(context.Background(), authz.Principal{UserID: "u"}), domain.NewStreakInput{})
	require.ErrorIs(t, err, authz.ErrNoTenant)
}

func TestCrossTenantStreakIsNotFound(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{})

	_, err := h.svc.Get(ctxFor(tenantB), st.ID)
	require.ErrorIs(t, err, domain.ErrStreakNotFound)
	name := "hijack"
	_, err = h.svc.Update(ctxFor(tenantB), st.ID, domain.StreakPatch{Name: &name})
	require.ErrorIs(t, err, domain.ErrStreakNotFound)
	require.ErrorIs(t, h.svc.Delete(ctxFor(tenantB), st.ID), domain.ErrStreakNotFound)
	_, err = h.svc.Record(ctxFor(tenantB), RecordInput{StreakID: st.ID, PlayerID: player1})
	require.ErrorIs(t, err, domain.ErrStreakNotFound)
}

func TestUpdateIsPartialAndDeleteHides(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{PointsPerPeriod: 10})
	name := "Renamed"
	got, err := h.svc.Update(ctxFor(tenantA), st.ID, domain.StreakPatch{Name: &name})
	require.NoError(t, err)
	require.Equal(t, "Renamed", got.Name)
	require.Equal(t, int64(10), got.PointsPerPeriod, "omitted fields stay untouched")

	viewer := newHarness(t, allowKeys{contracts.PermView.Key(): true})
	viewer.repo = h.repo
	viewer.svc.repo = h.repo
	_, err = viewer.svc.Update(ctxFor(tenantA), st.ID, domain.StreakPatch{Name: &name})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(viewer.svc.Delete(ctxFor(tenantA), st.ID)))

	require.NoError(t, h.svc.Delete(ctxFor(tenantA), st.ID))
	_, err = h.svc.Get(ctxFor(tenantA), st.ID)
	require.ErrorIs(t, err, domain.ErrStreakNotFound)
}

func TestListPagesWithCursor(t *testing.T) {
	h := newHarness(t, allPerms())
	for i, k := range []string{"a", "b", "c"} {
		h.seedStreak(t, domain.NewStreakInput{Name: "S " + k, ActivityKey: k})
		h.clock.Advance(time.Duration(i+1) * time.Second)
	}
	page1, next, err := h.svc.List(ctxFor(tenantA), ListFilter{}, "", 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, next)
	page2, next, err := h.svc.List(ctxFor(tenantA), ListFilter{}, next, 2)
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.Empty(t, next)

	other, _, err := h.svc.List(ctxFor(tenantB), ListFilter{}, "", 10)
	require.NoError(t, err)
	require.Empty(t, other)
}

// ---- record: the S1 fix ----

func TestRecordNewPeriodPaysAndPublishes(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{PointsPerPeriod: 10})

	res := h.record(t, st, "k1", h.clock.Now())
	require.Equal(t, OutcomeRecorded, res.Outcome)
	require.Equal(t, 1, res.PlayerStreak.CurrentCount)
	require.Equal(t, []string{jobCredit, contracts.TopicActivityRecorded}, h.ob.topics())

	credit := h.credits()[0]
	require.Equal(t, id.Derive("streak_period", res.PlayerStreak.ID, "2026-10-05"), credit.IdempotencyKey)
	require.Equal(t, int64(10), credit.Amount)
	require.Equal(t, tenantA, credit.TenantID)
	require.Equal(t, effect.SourceStreak, credit.Source.Kind)
	require.Equal(t, "act-k1", credit.Source.ActivityID)

	ev := h.ob.published[1].payload.(contracts.ActivityRecordedV1)
	require.True(t, ev.NewPeriod)
	require.Equal(t, day(5), ev.PeriodStart)
	require.Equal(t, 1, ev.CurrentCount)
}

// Laravel: "every call increments and pays" (S1). Go: a second record in the
// same period is a no-op — no counter change, no points, no event.
func TestDuplicateSameDayRecordIsNoop(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{PointsPerPeriod: 10, Milestones: []domain.Milestone{{Count: 1, BonusPoints: 5}}})

	h.record(t, st, "k1", h.clock.Now())
	h.ob.reset()
	for _, k := range []string{"k2", "k3", "k4"} {
		res := h.record(t, st, k, h.clock.Now().Add(-time.Hour))
		require.Equal(t, OutcomeNoop, res.Outcome)
		require.Equal(t, 1, res.PlayerStreak.CurrentCount)
	}
	require.Empty(t, h.ob.published, "same period: no points, no events, no milestone")
}

func TestRedeliveredCommandIsNoop(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{PointsPerPeriod: 10})

	h.record(t, st, "k1", h.clock.Now())
	h.ob.reset()
	res := h.record(t, st, "k1", h.clock.Now())
	require.Equal(t, OutcomeDuplicate, res.Outcome)
	// Even for a later period, a reused key changes nothing.
	res = h.record(t, st, "k1", h.clock.Now().Add(48*time.Hour))
	require.Equal(t, OutcomeDuplicate, res.Outcome)
	require.Empty(t, h.ob.published)
	require.Len(t, h.repo.periods, 1)
}

func TestTimezoneBucketingAroundMidnight(t *testing.T) {
	h := newHarness(t, allPerms())
	h.tz.tz[tenantA] = "Asia/Tbilisi" // UTC+4: local midnight is 20:00Z
	st := h.seedStreak(t, domain.NewStreakInput{})
	h.clock.Advance(12 * time.Hour) // now 2026-10-06 00:00Z = 04:00 local on the 6th

	res := h.record(t, st, "k1", time.Date(2026, 10, 5, 19, 30, 0, 0, time.UTC)) // 23:30 local on the 5th
	require.Equal(t, day(5), res.PeriodStart)
	res = h.record(t, st, "k2", time.Date(2026, 10, 5, 20, 30, 0, 0, time.UTC)) // 00:30 local on the 6th
	require.Equal(t, OutcomeRecorded, res.Outcome, "same UTC day, different local day")
	require.Equal(t, day(6), res.PeriodStart)
	require.Equal(t, 2, res.PlayerStreak.CurrentCount)

	res = h.record(t, st, "k3", time.Date(2026, 10, 5, 23, 59, 0, 0, time.UTC)) // 03:59 local on the 6th
	require.Equal(t, OutcomeNoop, res.Outcome)

	// The same instants in UTC would have been ONE bucket.
	h2 := newHarness(t, allPerms())
	st2 := h2.seedStreak(t, domain.NewStreakInput{})
	h2.clock.Advance(12 * time.Hour)
	h2.record(t, st2, "k1", time.Date(2026, 10, 5, 19, 30, 0, 0, time.UTC))
	require.Equal(t, OutcomeNoop, h2.record(t, st2, "k2", time.Date(2026, 10, 5, 20, 30, 0, 0, time.UTC)).Outcome)
}

func TestUnknownOrInvalidTimezoneFallsBackToUTC(t *testing.T) {
	h := newHarness(t, allPerms())
	h.tz.tz[tenantA] = "Mars/Olympus"
	st := h.seedStreak(t, domain.NewStreakInput{})
	res := h.record(t, st, "k1", time.Date(2026, 10, 5, 23, 0, 0, 0, time.UTC))
	require.Equal(t, day(5), res.PeriodStart)

	delete(h.tz.tz, tenantA)
	res = h.record(t, st, "k2", time.Date(2026, 10, 4, 23, 0, 0, 0, time.UTC))
	require.Equal(t, day(4), res.PeriodStart)
}

func TestFutureOccurredAtIsClampedToNow(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{})
	res := h.record(t, st, "k1", h.clock.Now().Add(72*time.Hour))
	require.Equal(t, day(5), res.PeriodStart)
}

func TestOutOfOrderRecordsConverge(t *testing.T) {
	orders := [][]int{{1, 2, 3, 4, 5}, {5, 3, 1, 4, 2}, {2, 5, 4, 1, 3}}
	for _, order := range orders {
		h := newHarness(t, allPerms())
		st := h.seedStreak(t, domain.NewStreakInput{PointsPerPeriod: 1})
		var last RecordResult
		for _, d := range order {
			last = h.record(t, st, "k"+string(rune('0'+d)), day(d).Add(9*time.Hour))
		}
		ps := last.PlayerStreak
		require.Equal(t, 5, ps.CurrentCount, "order %v", order)
		require.Equal(t, 5, ps.LongestCount, "order %v", order)
		require.Equal(t, day(1), *ps.RunStartedAt)
		require.Len(t, h.credits(), 5, "one credit per period whatever the order")
	}
}

func TestGracePeriodsBridgeMissedDays(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{GracePeriods: 1})
	h.record(t, st, "k1", day(2))
	res := h.record(t, st, "k2", day(4)) // day 3 missed: tolerated
	require.Equal(t, 2, res.PlayerStreak.CurrentCount)

	h0 := newHarness(t, allPerms())
	st0 := h0.seedStreak(t, domain.NewStreakInput{ActivityKey: "strict"})
	h0.record(t, st0, "k1", day(2))
	res = h0.record(t, st0, "k2", day(4))
	require.Equal(t, 1, res.PlayerStreak.CurrentCount, "no grace: the gap starts a new run")
	require.Equal(t, 1, res.PlayerStreak.LongestCount)
}

// ---- milestones (S5 fix) ----

func TestMilestonePaidOncePerRun(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{Milestones: []domain.Milestone{{Count: 2, BonusPoints: 100}}})

	h.clock.Advance(-3 * 24 * time.Hour) // now = Oct 2
	h.record(t, st, "k1", day(1))
	res := h.record(t, st, "k2", day(2))
	require.Equal(t, []int{2}, res.MilestonesReached)
	require.Equal(t, 1, h.ob.count(contracts.TopicMilestoneReached))
	credits := h.credits()
	require.Len(t, credits, 1)
	ev := h.ob.published[len(h.ob.published)-2].payload.(contracts.MilestoneReachedV1)
	require.Equal(t, id.Derive("streak_milestone", ev.AwardID), credits[0].IdempotencyKey)
	require.Equal(t, int64(100), credits[0].Amount)
	require.Equal(t, pointscontracts.KindBonus, credits[0].Kind)

	// Extending the same run does not pay again.
	h.clock.Advance(24 * time.Hour)
	res = h.record(t, st, "k3", day(3))
	require.Empty(t, res.MilestonesReached)
	require.Equal(t, 1, h.ob.count(contracts.TopicMilestoneReached))

	// A new run (after a gap) pays again.
	h.clock.Advance(3 * 24 * time.Hour) // Oct 6
	h.record(t, st, "k4", day(5))
	res = h.record(t, st, "k5", day(6))
	require.Equal(t, []int{2}, res.MilestonesReached)
	require.Equal(t, 2, h.ob.count(contracts.TopicMilestoneReached))
	require.Len(t, h.repo.awards, 2)
}

func TestLateRecordMergingRunsDoesNotRepayMilestone(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{Milestones: []domain.Milestone{{Count: 2, BonusPoints: 100}, {Count: 5, BonusPoints: 500}}})
	h.record(t, st, "k1", day(1))
	h.record(t, st, "k2", day(2)) // run 1-2 pays 2
	h.record(t, st, "k4", day(4))
	h.record(t, st, "k5", day(5)) // run 4-5 pays 2
	require.Equal(t, 2, h.ob.count(contracts.TopicMilestoneReached))

	res := h.record(t, st, "k3", day(3)) // merges into 1..5
	require.Equal(t, []int{5}, res.MilestonesReached, "only the newly reached milestone pays")
	require.Equal(t, 5, res.PlayerStreak.CurrentCount)
	require.Equal(t, 3, h.ob.count(contracts.TopicMilestoneReached))
}

// ---- rejections are results ----

func TestRecordRejectionsArePublishedNotErrors(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{})
	inactive := h.seedStreak(t, domain.NewStreakInput{Name: "Old", ActivityKey: "old"})
	off := false
	_, err := h.svc.Update(ctxFor(tenantA), inactive.ID, domain.StreakPatch{Active: &off})
	require.NoError(t, err)

	cases := []struct {
		name string
		cmd  contracts.RecordCmdV1
		want string
	}{
		{"unknown activity key", contracts.RecordCmdV1{ActivityKey: "nope", PlayerID: player1}, contracts.ReasonStreakNotFound},
		{"unknown streak id", contracts.RecordCmdV1{StreakID: "missing", PlayerID: player1}, contracts.ReasonStreakNotFound},
		{"other tenant's key", contracts.RecordCmdV1{ActivityKey: "daily_login", PlayerID: player1, TenantID: tenantB}, contracts.ReasonStreakNotFound},
		{"inactive streak", contracts.RecordCmdV1{ActivityKey: "old", PlayerID: player1}, contracts.ReasonStreakInactive},
		{"player not found", contracts.RecordCmdV1{StreakID: st.ID, PlayerID: "ghost"}, effect.ReasonPlayerNotFound},
		{"player inactive", contracts.RecordCmdV1{StreakID: st.ID, PlayerID: player2}, effect.ReasonPlayerInactive},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h.ob.reset()
			c.cmd.IdempotencyKey = "rej-" + string(rune('a'+i))
			if c.cmd.TenantID == "" {
				c.cmd.TenantID = tenantA
			}
			res, err := h.svc.HandleRecord(context.Background(), c.cmd)
			require.NoError(t, err)
			require.Equal(t, OutcomeRejected, res.Outcome)
			require.Equal(t, []string{contracts.TopicRecordRejected}, h.ob.topics())
			require.Equal(t, c.want, h.ob.published[0].payload.(contracts.RecordRejectedV1).Reason)

			// Redelivery: no second rejection.
			res, err = h.svc.HandleRecord(context.Background(), c.cmd)
			require.NoError(t, err)
			require.Equal(t, OutcomeDuplicate, res.Outcome)
			require.Len(t, h.ob.published, 1)
		})
	}
	require.Empty(t, h.repo.ps, "rejections never create player streaks")
}

func TestRecordByActivityKey(t *testing.T) {
	h := newHarness(t, allPerms())
	h.seedStreak(t, domain.NewStreakInput{})
	res, err := h.svc.HandleRecord(context.Background(), contracts.RecordCmdV1{
		IdempotencyKey: "k1", TenantID: tenantA, PlayerID: player1, ActivityKey: "daily_login", OccurredAt: h.clock.Now(),
	})
	require.NoError(t, err)
	require.Equal(t, OutcomeRecorded, res.Outcome)
}

func TestMalformedCommandIsInvalid(t *testing.T) {
	h := newHarness(t, allPerms())
	for _, cmd := range []contracts.RecordCmdV1{
		{TenantID: tenantA, PlayerID: player1, StreakID: "s"},       // no key
		{IdempotencyKey: "k", PlayerID: player1, StreakID: "s"},     // no tenant
		{IdempotencyKey: "k", TenantID: tenantA, StreakID: "s"},     // no player
		{IdempotencyKey: "k", TenantID: tenantA, PlayerID: player1}, // no target
	} {
		_, err := h.svc.HandleRecord(context.Background(), cmd)
		require.Equal(t, errs.Invalid, errs.KindOf(err))
	}
	require.Empty(t, h.ob.published)
}

// ---- HTTP record ----

func TestHTTPRecordChecks(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{})

	_, err := h.svc.Record(ctxFor(tenantA), RecordInput{StreakID: st.ID, PlayerID: "ghost"})
	require.ErrorIs(t, err, domain.ErrPlayerNotFound)
	_, err = h.svc.Record(ctxFor(tenantA), RecordInput{StreakID: st.ID, PlayerID: player2})
	require.ErrorIs(t, err, domain.ErrPlayerInactive)

	denied := newHarness(t, allowKeys{contracts.PermView.Key(): true})
	denied.svc.repo = h.repo
	_, err = denied.svc.Record(ctxFor(tenantA), RecordInput{StreakID: st.ID, PlayerID: player1})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Empty(t, h.ob.published)

	res, err := h.svc.Record(ctxFor(tenantA), RecordInput{StreakID: st.ID, PlayerID: player1, IdempotencyKey: "hdr"})
	require.NoError(t, err)
	require.Equal(t, OutcomeRecorded, res.Outcome)
	again, err := h.svc.Record(ctxFor(tenantA), RecordInput{StreakID: st.ID, PlayerID: player1, IdempotencyKey: "hdr"})
	require.NoError(t, err)
	require.Equal(t, OutcomeDuplicate, again.Outcome)
	require.Equal(t, 1, again.PlayerStreak.CurrentCount, "a replay reports the current state")
}

// ---- reset ----

func TestResetPublishesBrokenOnlyWhenRunning(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{})
	h.record(t, st, "k1", day(4))
	h.record(t, st, "k2", day(5))
	h.ob.reset()

	ps, err := h.svc.Reset(ctxFor(tenantA), st.ID, player1)
	require.NoError(t, err)
	require.Equal(t, 0, ps.CurrentCount)
	require.Equal(t, 2, ps.LongestCount)
	require.Equal(t, []string{contracts.TopicBroken}, h.ob.topics())
	ev := h.ob.published[0].payload.(contracts.BrokenV1)
	require.Equal(t, 2, ev.FinalCount)
	require.Equal(t, contracts.BrokenReasonReset, ev.Reason)

	h.ob.reset()
	_, err = h.svc.Reset(ctxFor(tenantA), st.ID, player1)
	require.NoError(t, err)
	require.Empty(t, h.ob.published, "resetting a zero streak announces nothing (S6)")

	// The next period starts a fresh run at 1.
	h.clock.Advance(24 * time.Hour)
	res := h.record(t, st, "k3", day(6))
	require.Equal(t, 1, res.PlayerStreak.CurrentCount)

	member := newHarness(t, allowKeys{contracts.PermRecord.Key(): true})
	member.svc.repo = h.repo
	_, err = member.svc.Reset(ctxFor(tenantA), st.ID, player1)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	_, err = h.svc.Reset(ctxFor(tenantB), st.ID, player1)
	require.ErrorIs(t, err, domain.ErrStreakNotFound)
}

// ---- break sweep (S4 fix) ----

func TestBreakSweepBreaksLapsedRunsOnce(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{})
	graced := h.seedStreak(t, domain.NewStreakInput{Name: "Graced", ActivityKey: "graced", GracePeriods: 1})

	h.clock.Advance(-2 * 24 * time.Hour) // record while it is Oct 3
	h.record(t, st, "k1", day(2))
	h.record(t, st, "k2", day(3))
	_, err := h.svc.HandleRecord(context.Background(), contracts.RecordCmdV1{
		IdempotencyKey: "g1", TenantID: tenantA, PlayerID: player1, StreakID: graced.ID, OccurredAt: day(3),
	})
	require.NoError(t, err)
	h.ob.reset()
	h.clock.Advance(2 * 24 * time.Hour)

	// Now Oct 5: last bucket Oct 3 is 2 days old → daily lapsed; grace 1 keeps the other alive.
	n, err := h.svc.BreakSweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, []string{contracts.TopicBroken}, h.ob.topics())
	ev := h.ob.published[0].payload.(contracts.BrokenV1)
	require.Equal(t, st.ID, ev.StreakID)
	require.Equal(t, 2, ev.FinalCount)
	require.Equal(t, day(3), ev.LastPeriod)
	require.Equal(t, contracts.BrokenReasonLapsed, ev.Reason)
	require.Equal(t, h.clock.Now(), h.repo.markers[contracts.JobBreakSweep])

	h.ob.reset()
	n, err = h.svc.BreakSweep(context.Background())
	require.NoError(t, err)
	require.Zero(t, n)
	require.Empty(t, h.ob.published, "a second sweep announces nothing")

	// A day later the graced one lapses too.
	h.clock.Advance(24 * time.Hour)
	n, err = h.svc.BreakSweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestBreakSweepUsesTenantTimezone(t *testing.T) {
	h := newHarness(t, allPerms())
	h.tz.tz[tenantA] = "America/New_York"
	st := h.seedStreak(t, domain.NewStreakInput{})
	h.clock.Advance(-24 * time.Hour) // Oct 4 12:00Z
	h.record(t, st, "k1", h.clock.Now())

	// Oct 6 03:00Z is still Oct 5 in New York: the Oct 4 bucket is 1 day old.
	h.clock.Advance(39 * time.Hour)
	n, err := h.svc.BreakSweep(context.Background())
	require.NoError(t, err)
	require.Zero(t, n)

	h.clock.Advance(2 * time.Hour) // Oct 6 05:00Z = Oct 6 01:00 local
	n, err = h.svc.BreakSweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestLapseThreshold(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) // Wednesday
	cases := []struct {
		period domain.Period
		grace  int
		want   time.Time
	}{
		{domain.Daily, 0, day(6)},
		{domain.Daily, 2, day(4)},
		{domain.Weekly, 0, time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)},
		{domain.Monthly, 1, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		got := LapseThreshold(domain.Streak{Period: c.period, GracePeriods: c.grace}, time.UTC, now)
		require.Equal(t, c.want, got, "%s grace %d", c.period, c.grace)
	}
}

// ---- subscriptions, prune, reader ----

func TestPurgeTenantAndDeletePlayerAreIdempotent(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{Milestones: []domain.Milestone{{Count: 1}}})
	h.record(t, st, "k1", h.clock.Now())

	require.NoError(t, h.svc.DeletePlayer(context.Background(), tenantA, player1))
	require.NoError(t, h.svc.DeletePlayer(context.Background(), tenantA, player1))
	require.Empty(t, h.repo.ps)
	require.Empty(t, h.repo.awards)

	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))
	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))
	require.Empty(t, h.repo.streaks)
	require.Empty(t, h.repo.requests)

	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.PurgeTenant(context.Background(), "")))
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.DeletePlayer(context.Background(), tenantA, "")))
}

func TestPruneRequestsDeletesOldRows(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{})
	h.record(t, st, "old", h.clock.Now())
	h.clock.Advance(31 * 24 * time.Hour)
	h.record(t, st, "new", h.clock.Now())

	n, err := h.svc.PruneRequests(context.Background(), 30*24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.Len(t, h.repo.requests, 1)
}

func TestReaderAndPlayerStreaksList(t *testing.T) {
	h := newHarness(t, allPerms())
	st := h.seedStreak(t, domain.NewStreakInput{})
	h.record(t, st, "k1", h.clock.Now())

	snaps, err := NewReader(h.repo).PlayerStreaks(context.Background(), tenantA, []string{player1, "ghost"})
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	require.Equal(t, 1, snaps[0].CurrentCount)
	require.Equal(t, day(5), *snaps[0].LastPeriod)

	none, err := NewReader(h.repo).PlayerStreaks(context.Background(), tenantB, []string{player1})
	require.NoError(t, err)
	require.Empty(t, none)

	views, err := h.svc.ListPlayerStreaks(ctxFor(tenantA), player1)
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, st.ID, views[0].Streak.ID)

	_, err = h.svc.ListPlayerStreaks(ctxFor(tenantB), player1)
	require.ErrorIs(t, err, domain.ErrPlayerNotFound)
	_, err = h.svc.ListPlayerStreaks(ctxFor(tenantA), player2)
	require.NoError(t, err, "a read never creates rows and works for inactive players")
	require.Len(t, h.repo.ps, 1)
}
