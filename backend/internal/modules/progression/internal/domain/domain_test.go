package domain_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/progression/internal/domain"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func lvl(id string, number int, xp int64) domain.Level {
	return domain.Level{ID: id, TenantID: "t1", Number: number, Name: id, XPRequired: xp, Active: true}
}

func TestNewLevelInvariants(t *testing.T) {
	cases := []struct {
		name string
		spec domain.LevelSpec
		want error
	}{
		{"ok", domain.LevelSpec{Number: 1, XPRequired: 0}, nil},
		{"number zero", domain.LevelSpec{Number: 0}, domain.ErrInvalidLevelNumber},
		{"negative xp", domain.LevelSpec{Number: 2, XPRequired: -1}, domain.ErrNegativeXPRequired},
		{"negative reward", domain.LevelSpec{Number: 2, PointsReward: -5}, domain.ErrNegativePointsReward},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, err := domain.NewLevel("t1", tc.spec, now)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "Level 1", l.Name, "name defaults to Level N")
			require.NotEmpty(t, l.ID)
		})
	}

	_, err := domain.NewLevel("", domain.LevelSpec{Number: 1}, now)
	require.ErrorIs(t, err, domain.ErrMissingTenant)
}

func TestApplyPatchKeepsOmittedAndClearsBadge(t *testing.T) {
	l, err := domain.NewLevel("t1", domain.LevelSpec{Number: 2, Name: "Two", XPRequired: 100, BadgeRewardID: "b1", Description: "d"}, now)
	require.NoError(t, err)

	empty := ""
	xp := int64(150)
	require.NoError(t, l.Apply(domain.LevelPatch{XPRequired: &xp, BadgeRewardID: &empty}, now.Add(time.Hour)))
	require.Equal(t, int64(150), l.XPRequired)
	require.Equal(t, "", l.BadgeRewardID)
	require.Equal(t, "Two", l.Name)
	require.Equal(t, "d", l.Description)
	require.Equal(t, now.Add(time.Hour), l.UpdatedAt)

	bad := -1
	before := l
	require.ErrorIs(t, l.Apply(domain.LevelPatch{Number: &bad}, now), domain.ErrInvalidLevelNumber)
	require.Equal(t, before, l, "a rejected patch changes nothing")
}

func TestCheckPlacement(t *testing.T) {
	ladder := domain.NewLadder([]domain.Level{lvl("l3", 3, 300), lvl("l1", 1, 0), lvl("l2", 2, 100)})
	cases := []struct {
		name string
		c    domain.Level
		want error
	}{
		{"appends above", lvl("new", 4, 500), nil},
		{"fits between gaps", lvl("new", 5, 301), nil},
		{"duplicate number", lvl("new", 2, 150), domain.ErrLevelNumberTaken},
		{"xp equal to lower level", lvl("new", 4, 300), domain.ErrXPNotIncreasing},
		{"xp below lower level", lvl("new", 4, 200), domain.ErrXPNotIncreasing},
		{"xp above higher level", lvl("l2", 2, 300), domain.ErrXPNotIncreasing},
		{"self update keeps its number", lvl("l2", 2, 150), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ladder.CheckPlacement(tc.c)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestLadderDerivation(t *testing.T) {
	inactive := lvl("l3", 3, 250)
	inactive.Active = false
	ladder := domain.NewLadder([]domain.Level{lvl("l1", 1, 0), lvl("l2", 2, 100), inactive, lvl("l4", 4, 500)})

	cases := []struct {
		total    int64
		current  string
		next     string
		crossed0 []string // crossed from 0
	}{
		{0, "l1", "l2", nil},
		{99, "l1", "l2", nil},
		{100, "l2", "l4", []string{"l2"}},
		{499, "l2", "l4", []string{"l2"}},
		{500, "l4", "", []string{"l2", "l4"}},
		{10_000, "l4", "", []string{"l2", "l4"}},
	}
	for _, tc := range cases {
		cur := ladder.LevelFor(tc.total)
		require.NotNil(t, cur)
		require.Equal(t, tc.current, cur.ID, "total %d", tc.total)
		next := ladder.NextAfter(tc.total)
		if tc.next == "" {
			require.Nil(t, next)
		} else {
			require.Equal(t, tc.next, next.ID)
		}
		var ids []string
		for _, l := range ladder.Crossed(0, tc.total) {
			ids = append(ids, l.ID)
		}
		require.Equal(t, tc.crossed0, ids, "inactive levels and the 0-threshold start level are never crossed")
	}

	require.Nil(t, domain.Ladder(nil).LevelFor(1000), "no ladder: no level")
	require.Empty(t, domain.Ladder(nil).Crossed(0, 1000))
}

func TestGainMultiLevelJumpAndNoLadder(t *testing.T) {
	ladder := domain.NewLadder([]domain.Level{lvl("l1", 1, 0), lvl("l2", 2, 100), lvl("l3", 3, 250)})
	p := domain.NewProgress("t1", "p1", now)

	crossed, err := p.Gain(300, ladder, now)
	require.NoError(t, err)
	require.Len(t, crossed, 2)
	require.Equal(t, "l2", crossed[0].ID)
	require.Equal(t, "l3", crossed[1].ID)
	require.Equal(t, int64(300), p.TotalXP)
	require.Equal(t, 3, p.LevelNumber)

	empty := domain.NewProgress("t1", "p2", now)
	crossed, err = empty.Gain(50, nil, now)
	require.NoError(t, err)
	require.Empty(t, crossed)
	require.Equal(t, 0, empty.LevelNumber)
	require.Equal(t, "", empty.LevelID)
}

func TestGainRejectsBadAmounts(t *testing.T) {
	p := domain.NewProgress("t1", "p1", now)
	_, err := p.Gain(0, nil, now)
	require.ErrorIs(t, err, domain.ErrNonPositiveAmount)

	p.TotalXP = math.MaxInt64 - 1
	_, err = p.Gain(2, nil, now)
	require.ErrorIs(t, err, domain.ErrXPOverflow)
	require.Equal(t, int64(math.MaxInt64-1), p.TotalXP)
}

func TestViewOf(t *testing.T) {
	ladder := domain.NewLadder([]domain.Level{lvl("l1", 1, 0), lvl("l2", 2, 100), lvl("l3", 3, 300)})

	v := domain.ViewOf("p1", 200, ladder)
	require.Equal(t, "l2", v.Current.ID)
	require.Equal(t, "l3", v.Next.ID)
	require.Equal(t, int64(100), *v.XPToNext)
	require.InDelta(t, 50.0, v.ProgressPercent, 0.001)

	v = domain.ViewOf("p1", 999, ladder)
	require.Equal(t, "l3", v.Current.ID)
	require.Nil(t, v.Next)
	require.Nil(t, v.XPToNext)
	require.InDelta(t, 100.0, v.ProgressPercent, 0.001)

	v = domain.ViewOf("p1", 40, nil)
	require.Nil(t, v.Current)
	require.Nil(t, v.Next)
	require.Zero(t, v.ProgressPercent, "no ladder is not max level")

	below := domain.NewLadder([]domain.Level{lvl("l1", 1, 100)})
	v = domain.ViewOf("p1", 25, below)
	require.Nil(t, v.Current, "below the first level")
	require.Equal(t, int64(75), *v.XPToNext)
	require.InDelta(t, 25.0, v.ProgressPercent, 0.001)
}

func TestNewXPGrantValidation(t *testing.T) {
	ok := domain.GrantSpec{TenantID: "t1", PlayerID: "p1", IdempotencyKey: "k", Amount: 5}
	cases := []struct {
		name string
		mut  func(*domain.GrantSpec)
		want error
	}{
		{"ok", func(*domain.GrantSpec) {}, nil},
		{"no tenant", func(s *domain.GrantSpec) { s.TenantID = "" }, domain.ErrMissingTenant},
		{"no player", func(s *domain.GrantSpec) { s.PlayerID = "" }, domain.ErrMissingPlayer},
		{"no key", func(s *domain.GrantSpec) { s.IdempotencyKey = "" }, domain.ErrMissingIdempotencyKey},
		{"zero amount", func(s *domain.GrantSpec) { s.Amount = 0 }, domain.ErrNonPositiveAmount},
		{"negative amount", func(s *domain.GrantSpec) { s.Amount = -3 }, domain.ErrNonPositiveAmount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := ok
			tc.mut(&spec)
			g, err := domain.NewXPGrant(spec, now)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, now, g.OccurredAt, "occurred_at defaults to now")
		})
	}
}
