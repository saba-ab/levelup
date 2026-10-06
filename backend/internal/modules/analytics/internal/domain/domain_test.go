package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr(t time.Time) *time.Time { return &t }

func TestDayOfTruncatesInUTC(t *testing.T) {
	tbilisi := time.FixedZone("GET", 4*3600)
	// 02:00 in UTC+4 on the 7th is 22:00 UTC on the 6th.
	require.Equal(t, day("2026-10-06"), DayOf(time.Date(2026, 10, 7, 2, 0, 0, 0, tbilisi)))
}

func TestWeekStartIsMonday(t *testing.T) {
	cases := map[string]string{
		"2026-10-05": "2026-10-05", // Monday
		"2026-10-07": "2026-10-05",
		"2026-10-11": "2026-10-05", // Sunday
		"2026-10-12": "2026-10-12",
	}
	for in, want := range cases {
		require.Equal(t, day(want), WeekStart(day(in)), in)
	}
}

func TestNewRange(t *testing.T) {
	today := time.Date(2026, 10, 6, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		from, to *time.Time
		want     Range
		err      error
	}{
		{name: "defaults to the 30 days ending today", want: Range{From: day("2026-09-07"), To: day("2026-10-06")}},
		{name: "explicit", from: ptr(day("2026-01-01")), to: ptr(day("2026-01-31")), want: Range{From: day("2026-01-01"), To: day("2026-01-31")}},
		{name: "single day", from: ptr(day("2026-01-01")), to: ptr(day("2026-01-01")), want: Range{From: day("2026-01-01"), To: day("2026-01-01")}},
		{name: "366 days allowed", from: ptr(day("2025-10-06")), to: ptr(day("2026-10-06")), want: Range{From: day("2025-10-06"), To: day("2026-10-06")}},
		{name: "367 days refused", from: ptr(day("2025-10-05")), to: ptr(day("2026-10-06")), err: ErrRangeTooLarge},
		{name: "inverted", from: ptr(day("2026-02-01")), to: ptr(day("2026-01-01")), err: ErrRangeInverted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := NewRange(c.from, c.to, today)
			if c.err != nil {
				require.ErrorIs(t, err, c.err)
				require.Equal(t, errs.Invalid, errs.KindOf(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, r)
			require.Len(t, r.EachDay(), r.Days())
		})
	}
}

func TestParseSteps(t *testing.T) {
	steps, err := ParseSteps(" signup, purchase ,repeat")
	require.NoError(t, err)
	require.Equal(t, []string{"signup", "purchase", "repeat"}, steps)

	for _, bad := range []string{"", "one", "a,,b", "a,b,c,d,e,f,g,h,i,j,k"} {
		_, err := ParseSteps(bad)
		require.ErrorIs(t, err, ErrBadSteps, bad)
	}
}

func TestParseCohort(t *testing.T) {
	require.NoError(t, ParseCohort("week", 8))
	require.NoError(t, ParseCohort("", 52))
	require.ErrorIs(t, ParseCohort("month", 8), ErrBadCohort)
	require.ErrorIs(t, ParseCohort("week", 0), ErrBadWeeks)
	require.ErrorIs(t, ParseCohort("week", 53), ErrBadWeeks)
}

func TestPct(t *testing.T) {
	require.InDelta(t, 33.33, Pct(1, 3), 0.001)
	require.InDelta(t, 100.0, Pct(5, 5), 0.001)
	require.Zero(t, Pct(5, 0))
}

func TestBuildRetention(t *testing.T) {
	today := day("2026-10-07") // Wednesday of the week of 10-05
	starts := CohortStarts(today, 3)
	require.Equal(t, []time.Time{day("2026-09-21"), day("2026-09-28"), day("2026-10-05")}, starts)

	sizes := map[time.Time]int64{day("2026-09-21"): 10, day("2026-09-28"): 4}
	cells := []CohortCell{
		{Cohort: day("2026-09-21"), Offset: 0, Players: 10},
		{Cohort: day("2026-09-21"), Offset: 1, Players: 5},
		{Cohort: day("2026-09-21"), Offset: 2, Players: 2},
		{Cohort: day("2026-09-28"), Offset: 0, Players: 4},
		{Cohort: day("2026-09-28"), Offset: 1, Players: 1},
	}
	got := BuildRetention(starts, sizes, cells)

	require.Len(t, got.Cohorts, 3)
	require.Equal(t, []float64{100, 50, 20}, got.Cohorts[0].Retained)
	require.Equal(t, []float64{100, 25}, got.Cohorts[1].Retained)
	require.Equal(t, []float64{0}, got.Cohorts[2].Retained, "an empty current cohort has one observable week")
	require.Zero(t, got.Cohorts[2].Size)

	// Curve: week 1 is (5+1)/(10+4); the empty cohort weighs nothing.
	require.Equal(t, CurvePoint{Week: 0, Pct: 100, Cohorts: 2}, got.Curve[0])
	require.Equal(t, CurvePoint{Week: 1, Pct: Pct(6, 14), Cohorts: 2}, got.Curve[1])
	require.Equal(t, CurvePoint{Week: 2, Pct: 20, Cohorts: 1}, got.Curve[2])
}

func TestFactValidate(t *testing.T) {
	ok := Fact{EventID: "e", TenantID: "t", Day: day("2026-01-01")}
	require.NoError(t, ok.Validate())
	require.False(t, ok.IsActivity())

	noEvent := ok
	noEvent.EventID = ""
	require.ErrorIs(t, noEvent.Validate(), ErrFactNoEvent)
	noTenant := ok
	noTenant.TenantID = ""
	require.ErrorIs(t, noTenant.Validate(), ErrFactNoTenant)
	noDay := ok
	noDay.Day = time.Time{}
	require.ErrorIs(t, noDay.Validate(), ErrFactNoDay)

	act := ok
	act.PlayerID, act.EventType = "p", "login"
	require.True(t, act.IsActivity())
}
