package domain

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestPeriodStartInTenantTimezoneAroundMidnight(t *testing.T) {
	tbilisi, err := time.LoadLocation("Asia/Tbilisi") // UTC+4
	require.NoError(t, err)
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	cases := []struct {
		name string
		at   time.Time
		loc  *time.Location
		p    Period
		want time.Time
	}{
		{"utc just before midnight", time.Date(2026, 10, 5, 23, 59, 59, 0, time.UTC), time.UTC, Daily, day(2026, 10, 5)},
		{"utc midnight", time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), time.UTC, Daily, day(2026, 10, 6)},
		{"20:00Z is already tomorrow in Tbilisi", time.Date(2026, 10, 5, 20, 0, 0, 0, time.UTC), tbilisi, Daily, day(2026, 10, 6)},
		{"19:59Z is still today in Tbilisi", time.Date(2026, 10, 5, 19, 59, 59, 0, time.UTC), tbilisi, Daily, day(2026, 10, 5)},
		{"03:00Z is still yesterday in New York", time.Date(2026, 10, 6, 3, 0, 0, 0, time.UTC), ny, Daily, day(2026, 10, 5)},
		{"nil location is UTC", time.Date(2026, 10, 5, 23, 0, 0, 0, time.UTC), nil, Daily, day(2026, 10, 5)},
		{"weekly starts monday", time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC), time.UTC, Weekly, day(2026, 10, 5)},
		{"sunday belongs to the week before", time.Date(2026, 10, 11, 23, 0, 0, 0, time.UTC), time.UTC, Weekly, day(2026, 10, 5)},
		{"sunday 20:00Z is next monday's week in Tbilisi", time.Date(2026, 10, 11, 20, 0, 0, 0, time.UTC), tbilisi, Weekly, day(2026, 10, 12)},
		{"monthly calendar month", time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC), time.UTC, Monthly, day(2026, 10, 1)},
		{"month rolls in Tbilisi before UTC", time.Date(2026, 10, 31, 21, 0, 0, 0, time.UTC), tbilisi, Monthly, day(2026, 11, 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, c.p.Start(c.at, c.loc))
		})
	}
}

func TestPeriodIndexIsConsecutive(t *testing.T) {
	for _, p := range []Period{Daily, Weekly, Monthly} {
		start := p.Start(time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC), time.UTC)
		for range 60 {
			next := p.Shift(start, 1)
			require.Equal(t, p.Index(start)+1, p.Index(next), "%s %s→%s", p, start, next)
			require.Equal(t, next, p.Start(next, time.UTC), "shift keeps bucket starts aligned")
			start = next
		}
	}
}

func TestRunsWithGracePeriods(t *testing.T) {
	d := func(n int) time.Time { return day(2026, 10, n) }
	cases := []struct {
		name    string
		grace   int
		buckets []time.Time
		want    []int // run lengths
	}{
		{"empty", 0, nil, nil},
		{"consecutive", 0, []time.Time{d(1), d(2), d(3)}, []int{3}},
		{"gap breaks without grace", 0, []time.Time{d(1), d(2), d(4)}, []int{2, 1}},
		{"one missed day tolerated with grace 1", 1, []time.Time{d(1), d(2), d(4)}, []int{3}},
		{"two missed days break grace 1", 1, []time.Time{d(1), d(4)}, []int{1, 1}},
		{"unsorted and duplicated input", 0, []time.Time{d(3), d(1), d(2), d(2)}, []int{3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			runs := Runs(Daily, c.grace, c.buckets)
			var got []int
			for _, r := range runs {
				got = append(got, r.Length)
			}
			require.Equal(t, c.want, got)
		})
	}
}

func dailyStreak(grace int, milestones ...Milestone) Streak {
	return Streak{ID: "s1", TenantID: "t1", Period: Daily, GracePeriods: grace, Milestones: milestones, Active: true}
}

// Out-of-order arrival converges: every permutation of the same buckets
// yields the same counters (R59, doc 06 §11.8).
func TestRecomputeConvergesUnderAnyOrder(t *testing.T) {
	st := dailyStreak(0)
	all := []time.Time{day(2026, 10, 1), day(2026, 10, 2), day(2026, 10, 4), day(2026, 10, 5), day(2026, 10, 6)}
	nowP := day(2026, 10, 6)
	now := nowP.Add(10 * time.Hour)

	var want *PlayerStreak
	rng := rand.New(rand.NewPCG(1, 2))
	for range 25 {
		order := append([]time.Time(nil), all...)
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

		ps := PlayerStreak{ID: "ps"}
		var seen []time.Time
		for _, b := range order {
			seen = append(seen, b)
			ps.Recompute(st, seen, nowP, now)
		}
		if want == nil {
			want = &ps
			require.Equal(t, 3, ps.CurrentCount)
			require.Equal(t, 3, ps.LongestCount)
			require.Equal(t, day(2026, 10, 6), *ps.LastPeriodStart)
			require.Equal(t, day(2026, 10, 4), *ps.RunStartedAt)
			continue
		}
		require.Equal(t, want.CurrentCount, ps.CurrentCount)
		require.Equal(t, want.LongestCount, ps.LongestCount)
		require.Equal(t, *want.RunStartedAt, *ps.RunStartedAt)
	}
}

func TestRecomputeLateRecordFillsGap(t *testing.T) {
	st := dailyStreak(0)
	nowP := day(2026, 10, 3)
	ps := PlayerStreak{}
	ps.Recompute(st, []time.Time{day(2026, 10, 1), day(2026, 10, 3)}, nowP, nowP)
	require.Equal(t, 1, ps.CurrentCount)

	ps.Recompute(st, []time.Time{day(2026, 10, 1), day(2026, 10, 3), day(2026, 10, 2)}, nowP, nowP)
	require.Equal(t, 3, ps.CurrentCount, "the late day-2 record joins both runs deterministically")
	require.Equal(t, day(2026, 10, 1), *ps.RunStartedAt)
}

func TestRecomputeLapsedRunPublishesOnceThenStaysZero(t *testing.T) {
	st := dailyStreak(0)
	ps := PlayerStreak{}
	ps.Recompute(st, []time.Time{day(2026, 10, 1), day(2026, 10, 2)}, day(2026, 10, 2), day(2026, 10, 2))
	require.Equal(t, 2, ps.CurrentCount)

	// A late record for day 1 (already counted) arrives on day 5: the run is dead.
	out := ps.Recompute(st, []time.Time{day(2026, 9, 30), day(2026, 10, 1), day(2026, 10, 2)}, day(2026, 10, 5), day(2026, 10, 5))
	require.True(t, out.Lapsed)
	require.Equal(t, 2, out.BrokenCount)
	require.Equal(t, 0, ps.CurrentCount)
	require.Equal(t, 3, ps.LongestCount)
	require.NotNil(t, ps.BrokenAt)

	out = ps.Recompute(st, []time.Time{day(2026, 9, 29), day(2026, 9, 30), day(2026, 10, 1), day(2026, 10, 2)}, day(2026, 10, 5), day(2026, 10, 5))
	require.False(t, out.Lapsed, "a dead run is announced broken once")
}

func TestBreakAnnouncesOncePerRun(t *testing.T) {
	run := day(2026, 10, 1)
	ps := PlayerStreak{CurrentCount: 4, RunStartedAt: &run}
	prev, announce := ps.Break(day(2026, 10, 5))
	require.Equal(t, 4, prev)
	require.True(t, announce)
	require.Equal(t, 0, ps.CurrentCount)

	prev, announce = ps.Break(day(2026, 10, 5))
	require.Zero(t, prev)
	require.False(t, announce)
}

func TestResetExcludesEarlierBuckets(t *testing.T) {
	st := dailyStreak(0)
	nowP := day(2026, 10, 3)
	ps := PlayerStreak{}
	buckets := []time.Time{day(2026, 10, 1), day(2026, 10, 2), day(2026, 10, 3)}
	ps.Recompute(st, buckets, nowP, nowP)
	require.Equal(t, 3, ps.CurrentCount)

	require.Equal(t, 3, ps.Reset(nowP))
	require.Equal(t, 0, ps.CurrentCount)
	require.Equal(t, 3, ps.LongestCount, "reset keeps the longest")

	ps.Recompute(st, append(buckets, day(2026, 10, 4)), day(2026, 10, 4), day(2026, 10, 4))
	require.Equal(t, 1, ps.CurrentCount, "the run restarts after the reset")

	fresh := PlayerStreak{}
	require.Zero(t, fresh.Reset(nowP), "resetting nothing reports 0")
}

func TestNewStreakInvariants(t *testing.T) {
	ok := NewStreakInput{Name: "Daily Login", ActivityKey: "daily_login", Period: "daily", Active: true,
		Milestones: []Milestone{{Count: 30, BonusPoints: 500}, {Count: 7, BonusPoints: 100}}}
	st, err := NewStreak("t1", ok, time.Now())
	require.NoError(t, err)
	require.Equal(t, "daily-login", st.Slug)
	require.Equal(t, 7, st.Milestones[0].Count, "milestones are sorted")

	cases := []struct {
		name string
		mut  func(*NewStreakInput)
		want error
	}{
		{"no name", func(i *NewStreakInput) { i.Name = " " }, ErrNameRequired},
		{"bad period", func(i *NewStreakInput) { i.Period = "yearly" }, ErrBadPeriod},
		{"bad key", func(i *NewStreakInput) { i.ActivityKey = "Daily Login" }, ErrBadActivityKey},
		{"bad slug", func(i *NewStreakInput) { i.Slug = "-x" }, ErrBadSlug},
		{"negative grace", func(i *NewStreakInput) { i.GracePeriods = -1 }, ErrBadGracePeriods},
		{"negative points", func(i *NewStreakInput) { i.PointsPerPeriod = -1 }, ErrNegativePoints},
		{"zero milestone", func(i *NewStreakInput) { i.Milestones = []Milestone{{Count: 0}} }, ErrBadMilestoneCount},
		{"negative bonus", func(i *NewStreakInput) { i.Milestones = []Milestone{{Count: 3, BonusPoints: -1}} }, ErrNegativeBonus},
		{"duplicate milestone", func(i *NewStreakInput) { i.Milestones = []Milestone{{Count: 3}, {Count: 3}} }, ErrDuplicateMilestone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := ok
			c.mut(&in)
			_, err := NewStreak("t1", in, time.Now())
			require.ErrorIs(t, err, c.want)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
		})
	}

	_, err = NewStreak("", ok, time.Now())
	require.ErrorIs(t, err, ErrNoTenant)
}

func TestApplyPatchIsAtomic(t *testing.T) {
	st, err := NewStreak("t1", NewStreakInput{Name: "A", ActivityKey: "a", Period: "daily"}, time.Now())
	require.NoError(t, err)
	name := "B"
	bad := -5
	err = st.Apply(StreakPatch{Name: &name, GracePeriods: &bad}, time.Now())
	require.ErrorIs(t, err, ErrBadGracePeriods)
	require.Equal(t, "A", st.Name, "a rejected patch changes nothing")

	require.NoError(t, st.Apply(StreakPatch{Name: &name}, time.Now()))
	require.Equal(t, "B", st.Name)
}

func TestMilestoneAwardIDIsDeterministic(t *testing.T) {
	a := MilestoneAwardID("ps", 7, day(2026, 10, 1))
	require.Equal(t, a, MilestoneAwardID("ps", 7, day(2026, 10, 1)))
	require.NotEqual(t, a, MilestoneAwardID("ps", 7, day(2026, 10, 2)), "a new run is a new award")
}
