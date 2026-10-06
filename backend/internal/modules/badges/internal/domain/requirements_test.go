package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/shared/errs"
)

func reqJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

func TestParseRequirementsGrammar(t *testing.T) {
	valid := []string{
		`{"all":[{"metric":"lifetime_points","gte":1000}]}`,
		`{"any":[{"metric":"level","gte":5},{"metric":"streak_days","gte":7}]}`,
		`{"all":[{"metric":"missions_completed","gte":3}],"any":[{"metric":"badges_earned","gte":2}]}`,
		`{"all":[{"metric":"activity_count","event_type":"purchase","gte":10}]}`,
		`{"all":[{"metric":"activity_count","gte":10}]}`,
		`{"all":[],"any":[{"metric":"level","gte":1}]}`,
	}
	for _, raw := range valid {
		_, ok, err := domain.ParseRequirements(reqJSON(t, raw))
		require.NoError(t, err, raw)
		require.True(t, ok, raw)
	}

	invalid := []string{
		`{"missions":3}`,
		`{"all":{}}`,
		`{"all":[]}`,
		`{"all":[],"any":[]}`,
		`{"all":[1]}`,
		`{"all":[{"metric":"xp","gte":1}]}`,
		`{"all":[{"metric":"level"}]}`,
		`{"all":[{"gte":1}]}`,
		`{"all":[{"metric":"level","gte":0}]}`,
		`{"all":[{"metric":"level","gte":1.5}]}`,
		`{"all":[{"metric":"level","gte":"3"}]}`,
		`{"all":[{"metric":"level","event_type":"x","gte":3}]}`,
		`{"all":[{"metric":"activity_count","event_type":"","gte":3}]}`,
		`{"all":[{"metric":"level","gte":3,"lte":9}]}`,
	}
	for _, raw := range invalid {
		_, _, err := domain.ParseRequirements(reqJSON(t, raw))
		require.Error(t, err, raw)
		require.Equal(t, domain.CodeInvalidRequirements, errs.CodeOf(err), raw)
		require.Equal(t, errs.Invalid, errs.KindOf(err), raw)
		require.NotEmpty(t, errs.FieldsOf(err)["requirements"], raw)
	}

	_, ok, err := domain.ParseRequirements(nil)
	require.NoError(t, err)
	require.False(t, ok)
	_, ok, err = domain.ParseRequirements(map[string]any{})
	require.NoError(t, err)
	require.False(t, ok)
}

func TestRequirementsSatisfiedAllAndAny(t *testing.T) {
	stats := domain.PlayerStats{
		LifetimePoints: 1500, MissionsCompleted: 3, MaxStreak: 6, Level: 4, BadgesEarned: 2,
		ActivityCounts: map[string]int64{"purchase": 9, "login": 4},
	}
	cases := []struct {
		raw  string
		want bool
	}{
		{`{"all":[{"metric":"lifetime_points","gte":1500}]}`, true},
		{`{"all":[{"metric":"lifetime_points","gte":1501}]}`, false},
		{`{"all":[{"metric":"lifetime_points","gte":1000},{"metric":"missions_completed","gte":3}]}`, true},
		{`{"all":[{"metric":"lifetime_points","gte":1000},{"metric":"missions_completed","gte":4}]}`, false},
		{`{"any":[{"metric":"streak_days","gte":7},{"metric":"level","gte":4}]}`, true},
		{`{"any":[{"metric":"streak_days","gte":7},{"metric":"level","gte":5}]}`, false},
		{`{"all":[{"metric":"badges_earned","gte":2}],"any":[{"metric":"level","gte":9},{"metric":"streak_days","gte":6}]}`, true},
		{`{"all":[{"metric":"badges_earned","gte":3}],"any":[{"metric":"streak_days","gte":6}]}`, false},
		{`{"all":[{"metric":"activity_count","event_type":"purchase","gte":9}]}`, true},
		{`{"all":[{"metric":"activity_count","event_type":"purchase","gte":10}]}`, false},
		{`{"all":[{"metric":"activity_count","event_type":"refund","gte":1}]}`, false},
		{`{"all":[{"metric":"activity_count","gte":13}]}`, true}, // every type
		{`{"all":[{"metric":"activity_count","gte":14}]}`, false},
	}
	for _, tc := range cases {
		req, ok, err := domain.ParseRequirements(reqJSON(t, tc.raw))
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, tc.want, req.Satisfied(stats), tc.raw)
	}
	require.False(t, domain.Requirements{}.Satisfied(stats), "no conditions never match")
}

func TestRequirementsUses(t *testing.T) {
	req, _, err := domain.ParseRequirements(reqJSON(t,
		`{"all":[{"metric":"level","gte":2}],"any":[{"metric":"activity_count","event_type":"purchase","gte":1}]}`))
	require.NoError(t, err)
	require.True(t, req.Uses(domain.MetricLevel, ""))
	require.True(t, req.Uses(domain.MetricActivityCount, "purchase"))
	require.False(t, req.Uses(domain.MetricActivityCount, "login"))
	require.False(t, req.Uses(domain.MetricLifetimePoints, ""))

	anyType, _, err := domain.ParseRequirements(reqJSON(t, `{"all":[{"metric":"activity_count","gte":1}]}`))
	require.NoError(t, err)
	require.True(t, anyType.Uses(domain.MetricActivityCount, "login"))
}

func TestBadgeRequirementsValidatedOnCreateAndPatch(t *testing.T) {
	p := validParams()
	p.Requirements = map[string]any{"missions": 3.0}
	_, err := domain.NewBadge(p, now)
	require.Equal(t, domain.CodeInvalidRequirements, errs.CodeOf(err))

	p.Requirements = map[string]any{}
	b, err := domain.NewBadge(p, now)
	require.NoError(t, err)
	require.Nil(t, b.Requirements, "an empty object means no requirements")

	err = b.Apply(domain.Patch{RequirementsSet: true, Requirements: map[string]any{"all": "x"}}, now)
	require.Equal(t, domain.CodeInvalidRequirements, errs.CodeOf(err))
	require.Nil(t, b.Requirements, "a refused patch changes nothing")

	good := reqJSON(t, `{"all":[{"metric":"level","gte":3}]}`)
	require.NoError(t, b.Apply(domain.Patch{RequirementsSet: true, Requirements: good}, now))
	req, ok := b.AutoRequirements()
	require.True(t, ok)
	require.Equal(t, []domain.Condition{{Metric: domain.MetricLevel, Gte: 3}}, req.All)

	// A pre-grammar value already stored is kept on unrelated patches and
	// never evaluated.
	legacy := b
	legacy.Requirements = map[string]any{"missions": 3.0}
	require.NoError(t, legacy.Apply(domain.Patch{Name: strp("Renamed")}, now))
	_, ok = legacy.AutoRequirements()
	require.False(t, ok)
}
