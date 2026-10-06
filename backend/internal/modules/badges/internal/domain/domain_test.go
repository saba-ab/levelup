package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/shared/effect"
)

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func intp(v int) *int       { return &v }
func i64p(v int64) *int64   { return &v }
func strp(v string) *string { return &v }

func validParams() domain.NewBadgeParams {
	return domain.NewBadgeParams{
		TenantID: "t1", Name: "First Steps", Tier: "gold", Category: "achievement", Active: true,
	}
}

func TestNewBadgeDefaultsAndInvariants(t *testing.T) {
	b, err := domain.NewBadge(validParams(), now)
	require.NoError(t, err)
	require.Equal(t, "first-steps", b.Slug)
	require.EqualValues(t, 50, b.PointsValue, "gold default points (parity)")
	require.NotEmpty(t, b.ID)

	cases := map[string]struct {
		mut  func(*domain.NewBadgeParams)
		want error
	}{
		"blank name":         {func(p *domain.NewBadgeParams) { p.Name = "  " }, domain.ErrNameRequired},
		"bad tier":           {func(p *domain.NewBadgeParams) { p.Tier = "mythic" }, domain.ErrInvalidTier},
		"bad category":       {func(p *domain.NewBadgeParams) { p.Category = "misc" }, domain.ErrInvalidCategory},
		"negative points":    {func(p *domain.NewBadgeParams) { p.PointsValue = i64p(-1) }, domain.ErrNegativePoints},
		"zero max awards":    {func(p *domain.NewBadgeParams) { p.MaxAwards = intp(0) }, domain.ErrInvalidMaxAwards},
		"bad explicit slug":  {func(p *domain.NewBadgeParams) { p.Slug = "Not A Slug" }, domain.ErrInvalidSlug},
		"zero points is ok":  {func(p *domain.NewBadgeParams) { p.PointsValue = i64p(0) }, nil},
		"explicit slug ok":   {func(p *domain.NewBadgeParams) { p.Slug = "custom-1" }, nil},
		"stackable with max": {func(p *domain.NewBadgeParams) { p.Stackable = true; p.MaxAwards = intp(3) }, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := validParams()
			tc.mut(&p)
			_, err := domain.NewBadge(p, now)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestTierRankAndDefaultPoints(t *testing.T) {
	cases := []struct {
		tier   domain.Tier
		rank   int
		points int64
	}{
		{domain.TierBronze, 1, 10}, {domain.TierSilver, 2, 25}, {domain.TierGold, 3, 50},
		{domain.TierPlatinum, 4, 100}, {domain.TierDiamond, 5, 250},
	}
	for _, tc := range cases {
		require.True(t, tc.tier.Valid())
		require.Equal(t, tc.rank, tc.tier.Rank())
		require.Equal(t, tc.points, tc.tier.DefaultPoints())
	}
	require.False(t, domain.Tier("mythic").Valid())
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"First Steps":        "first-steps",
		"  Hello,  World!! ": "hello-world",
		"100% Club":          "100-club",
		"!!!":                "badge",
		"Ünïcode Star":       "n-code-star",
	}
	for in, want := range cases {
		require.Equal(t, want, domain.Slugify(in), in)
	}
}

func TestApplyPatchKeepsSlugOnRenameAndClearsNullables(t *testing.T) {
	p := validParams()
	p.Stackable = true
	p.MaxAwards = intp(3)
	p.Requirements = map[string]any{"all": []any{map[string]any{"metric": "missions_completed", "gte": 1.0}}}
	b, err := domain.NewBadge(p, now)
	require.NoError(t, err)

	later := now.Add(time.Hour)
	require.NoError(t, b.Apply(domain.Patch{
		Name:            strp("Renamed"),
		MaxAwardsSet:    true,
		RequirementsSet: true,
	}, later))
	require.Equal(t, "Renamed", b.Name)
	require.Equal(t, "first-steps", b.Slug, "slug is stable on rename (B18)")
	require.Nil(t, b.MaxAwards, "explicit null clears max_awards (B20)")
	require.Nil(t, b.Requirements)
	require.Equal(t, later, b.UpdatedAt)
}

func TestApplyPatchRejectsInvalidAndLeavesBadgeUntouched(t *testing.T) {
	b, err := domain.NewBadge(validParams(), now)
	require.NoError(t, err)
	before := b
	err = b.Apply(domain.Patch{Name: strp("ok"), PointsValue: i64p(-5)}, now.Add(time.Hour))
	require.ErrorIs(t, err, domain.ErrNegativePoints)
	require.Equal(t, before, b)
}

func TestCanAward(t *testing.T) {
	deletedAt := now
	cases := map[string]struct {
		badge  domain.Badge
		earned int
		want   string
	}{
		"first award":                   {domain.Badge{Active: true}, 0, ""},
		"inactive":                      {domain.Badge{Active: false}, 0, effect.ReasonTargetInactive},
		"deleted":                       {domain.Badge{Active: true, DeletedAt: &deletedAt}, 0, effect.ReasonTargetInactive},
		"non-stackable twice":           {domain.Badge{Active: true}, 1, effect.ReasonAlreadyEarned},
		"non-stackable ignores max":     {domain.Badge{Active: true, MaxAwards: intp(5)}, 1, effect.ReasonAlreadyEarned},
		"stackable unlimited":           {domain.Badge{Active: true, Stackable: true}, 99, ""},
		"stackable under max":           {domain.Badge{Active: true, Stackable: true, MaxAwards: intp(2)}, 1, ""},
		"stackable at max":              {domain.Badge{Active: true, Stackable: true, MaxAwards: intp(2)}, 2, effect.ReasonLimitReached},
		"stackable max 1 behaves limit": {domain.Badge{Active: true, Stackable: true, MaxAwards: intp(1)}, 1, effect.ReasonLimitReached},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.badge.CanAward(tc.earned))
		})
	}
}

func TestPlayerBadgeAward(t *testing.T) {
	pb := domain.NewPlayerBadge("t1", "p1", "b1", now)
	require.True(t, pb.Award(now))
	require.Equal(t, 1, pb.EarnedCount)
	later := now.Add(time.Minute)
	require.False(t, pb.Award(later))
	require.Equal(t, 2, pb.EarnedCount)
	require.Equal(t, now, pb.FirstAwardedAt)
	require.Equal(t, later, pb.LastAwardedAt)
}

func TestDriftChecks(t *testing.T) {
	cases := map[string]struct {
		d    domain.Drift
		want []string
	}{
		"clean":               {domain.Drift{EarnedCount: 1, AppliedAwards: 1}, nil},
		"count mismatch":      {domain.Drift{EarnedCount: 2, AppliedAwards: 1, Stackable: true}, []string{domain.DriftCountMismatch}},
		"non-stackable over":  {domain.Drift{EarnedCount: 2, AppliedAwards: 2}, []string{domain.DriftOverMax}},
		"stackable over max":  {domain.Drift{EarnedCount: 3, AppliedAwards: 3, Stackable: true, MaxAwards: intp(2)}, []string{domain.DriftOverMax}},
		"stackable unlimited": {domain.Drift{EarnedCount: 30, AppliedAwards: 30, Stackable: true}, nil},
	}
	for name, tc := range cases {
		require.Equal(t, tc.want, tc.d.Checks(), name)
	}
}
