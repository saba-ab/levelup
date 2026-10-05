package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/shared/errs"
)

func TestPeriodStartBucketsInUTC(t *testing.T) {
	// Sunday 2026-10-04 23:30 in UTC+2 is Sunday 21:30 UTC.
	cest := time.FixedZone("CEST", 2*3600)
	sundayLocal := time.Date(2026, 10, 4, 23, 30, 0, 0, cest)
	tests := []struct {
		name string
		freq string
		at   time.Time
		want time.Time
	}{
		{"never", contracts.ResetNever, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), AllTimeStart},
		{"daily", contracts.ResetDaily, time.Date(2026, 10, 5, 23, 59, 59, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		{"daily from offset zone", contracts.ResetDaily, time.Date(2026, 10, 6, 1, 0, 0, 0, cest), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		{"weekly monday", contracts.ResetWeekly, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		{"weekly sunday belongs to previous monday", contracts.ResetWeekly, sundayLocal, time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)},
		{"weekly midweek", contracts.ResetWeekly, time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC), time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		{"monthly", contracts.ResetMonthly, time.Date(2026, 2, 28, 23, 0, 0, 0, time.UTC), time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, tt.want.Equal(PeriodStart(tt.freq, tt.at)), "got %s", PeriodStart(tt.freq, tt.at))
		})
	}
}

func TestPeriodEnd(t *testing.T) {
	start := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	end, ok := PeriodEnd(contracts.ResetDaily, start)
	require.True(t, ok)
	require.Equal(t, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), end)

	end, ok = PeriodEnd(contracts.ResetMonthly, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	require.True(t, ok)
	require.Equal(t, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), end)

	_, ok = PeriodEnd(contracts.ResetNever, AllTimeStart)
	require.False(t, ok)
}

func TestNewLeaderboardInvariants(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	base := NewLeaderboardInput{TenantID: "t1", Name: "Weekly Points Race", Type: contracts.TypePoints, ResetFrequency: contracts.ResetWeekly, Active: true}
	tests := []struct {
		name   string
		mutate func(*NewLeaderboardInput)
		err    error
	}{
		{"ok", func(*NewLeaderboardInput) {}, nil},
		{"no tenant", func(in *NewLeaderboardInput) { in.TenantID = "" }, ErrTenantRequired},
		{"blank name", func(in *NewLeaderboardInput) { in.Name = "  " }, ErrNameRequired},
		{"bad type", func(in *NewLeaderboardInput) { in.Type = "custom" }, ErrInvalidType},
		{"bad reset", func(in *NewLeaderboardInput) { in.ResetFrequency = "hourly" }, ErrInvalidReset},
		{"count on points", func(in *NewLeaderboardInput) { in.Metric = contracts.MetricCount }, ErrInvalidMetric},
		{"net on badges", func(in *NewLeaderboardInput) {
			in.Type = contracts.TypeBadges
			in.Metric = contracts.MetricNet
		}, ErrInvalidMetric},
		{"balance periodic", func(in *NewLeaderboardInput) { in.Metric = contracts.MetricBalance }, ErrBalancePeriodic},
		{"bad slug", func(in *NewLeaderboardInput) { in.Slug = "Not A Slug" }, ErrSlugInvalid},
		{"max entries too high", func(in *NewLeaderboardInput) { in.MaxEntries = 5000 }, ErrMaxEntries},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			tt.mutate(&in)
			lb, err := NewLeaderboard(in, now)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "weekly-points-race", lb.Slug)
			require.Equal(t, contracts.MetricEarned, lb.Metric)
			require.Equal(t, DefaultMaxEntries, lb.MaxEntries)
			require.NotEmpty(t, lb.ID)
		})
	}
}

func TestDefaultMetricPerType(t *testing.T) {
	now := time.Now()
	lb, err := NewLeaderboard(NewLeaderboardInput{TenantID: "t", Name: "Badges", Type: contracts.TypeBadges}, now)
	require.NoError(t, err)
	require.Equal(t, contracts.MetricCount, lb.Metric)
	require.Equal(t, contracts.ResetNever, lb.ResetFrequency)
}

func TestPatchValidatesAndKeepsOmittedFields(t *testing.T) {
	lb, err := NewLeaderboard(NewLeaderboardInput{TenantID: "t", Name: "A", Description: "keep", Type: contracts.TypeXP}, time.Now())
	require.NoError(t, err)

	empty := " "
	require.Equal(t, errs.Invalid, errs.KindOf(lb.Apply(Patch{Name: &empty}, time.Now())))
	require.Equal(t, "A", lb.Name, "a rejected patch changes nothing")

	name, maxE := "B", 10
	require.NoError(t, lb.Apply(Patch{Name: &name, MaxEntries: &maxE}, time.Now()))
	require.Equal(t, "B", lb.Name)
	require.Equal(t, 10, lb.MaxEntries)
	require.Equal(t, "keep", lb.Description)

	zero := 0
	require.ErrorIs(t, lb.Apply(Patch{MaxEntries: &zero}, time.Now()), ErrMaxEntries)
}

func TestContributionMatrix(t *testing.T) {
	board := func(typ, metric string) Leaderboard { return Leaderboard{Type: typ, Metric: metric} }
	fact := func(k FactKind, amount, abs int64) Fact { return Fact{Kind: k, Amount: amount, Absolute: abs} }
	tests := []struct {
		name string
		b    Leaderboard
		f    Fact
		want Op
		ok   bool
	}{
		{"earned credit", board(contracts.TypePoints, contracts.MetricEarned), fact(FactPointsCredited, 50, 500), Op{OpIncrement, 50}, true},
		{"earned ignores debit", board(contracts.TypePoints, contracts.MetricEarned), fact(FactPointsDebited, 50, 450), Op{}, false},
		{"net debit", board(contracts.TypePoints, contracts.MetricNet), fact(FactPointsDebited, 30, 470), Op{OpIncrement, -30}, true},
		{"net refund", board(contracts.TypePoints, contracts.MetricNet), fact(FactPointsRefunded, 30, 500), Op{OpIncrement, 30}, true},
		{"balance credit", board(contracts.TypePoints, contracts.MetricBalance), fact(FactPointsCredited, 50, 500), Op{OpSet, 500}, true},
		{"balance debit", board(contracts.TypePoints, contracts.MetricBalance), fact(FactPointsDebited, 50, 450), Op{OpSet, 450}, true},
		{"badge count", board(contracts.TypeBadges, contracts.MetricCount), fact(FactBadgeAwarded, 1, 0), Op{OpIncrement, 1}, true},
		{"mission count", board(contracts.TypeMissions, contracts.MetricCount), fact(FactMissionCompleted, 1, 0), Op{OpIncrement, 1}, true},
		{"xp earned", board(contracts.TypeXP, contracts.MetricEarned), fact(FactXPGained, 20, 220), Op{OpIncrement, 20}, true},
		{"xp balance", board(contracts.TypeXP, contracts.MetricBalance), fact(FactXPGained, 20, 220), Op{OpSet, 220}, true},
		{"type mismatch", board(contracts.TypeBadges, contracts.MetricCount), fact(FactPointsCredited, 5, 5), Op{}, false},
		{"non-positive amount", board(contracts.TypePoints, contracts.MetricEarned), fact(FactPointsCredited, 0, 0), Op{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op, ok := Contribution(tt.b, tt.f)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, op)
		})
	}
}

func TestAssignRanksCompetition(t *testing.T) {
	rows := []Standing{{PlayerID: "a", Score: 10}, {PlayerID: "b", Score: 8}, {PlayerID: "c", Score: 8}, {PlayerID: "d", Score: 5}}
	AssignRanks(rows, 0, 1)
	require.Equal(t, []int64{1, 2, 2, 4}, []int64{rows[0].Rank, rows[1].Rank, rows[2].Rank, rows[3].Rank})

	// A page starting mid-tie inherits the tie's rank.
	page := []Standing{{PlayerID: "c", Score: 8}, {PlayerID: "d", Score: 5}}
	AssignRanks(page, 2, 2)
	require.Equal(t, int64(2), page[0].Rank)
	require.Equal(t, int64(4), page[1].Rank)
	require.Equal(t, int64(3), page[1].Position)
}

func TestSlugify(t *testing.T) {
	require.Equal(t, "badge-collectors", Slugify("  Badge   Collectors!! "))
}
