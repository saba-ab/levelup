package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

func doc(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m))
	return m
}

func TestParseConditionsRejects(t *testing.T) {
	cases := map[string]string{
		"empty object":        `{}`,
		"two keys":            `{"all":[{"field":"is_active","op":"eq","value":true}],"any":[]}`,
		"unknown match":       `{"some":[{"field":"is_active","op":"eq","value":true}]}`,
		"empty list":          `{"all":[]}`,
		"not a list":          `{"all":{"field":"is_active"}}`,
		"item not object":     `{"all":[1]}`,
		"unknown field":       `{"all":[{"field":"email","op":"eq","value":"x"}]}`,
		"unknown key":         `{"all":[{"field":"is_active","op":"eq","value":true,"extra":1}]}`,
		"bad attr path":       `{"all":[{"field":"attributes.","op":"eq","value":"x"}]}`,
		"bad attr op":         `{"all":[{"field":"attributes.x","op":"before","value":"x"}]}`,
		"exists with value":   `{"all":[{"field":"attributes.x","op":"exists","value":1}]}`,
		"in without list":     `{"all":[{"field":"attributes.x","op":"in","value":"a"}]}`,
		"eq object value":     `{"all":[{"field":"attributes.x","op":"eq","value":{"a":1}}]}`,
		"is_active gt":        `{"all":[{"field":"is_active","op":"gt","value":true}]}`,
		"is_active string":    `{"all":[{"field":"is_active","op":"eq","value":"yes"}]}`,
		"created_at bad time": `{"all":[{"field":"created_at","op":"before","value":"soon"}]}`,
		"created_at eq":       `{"all":[{"field":"created_at","op":"eq","value":"2026-01-01"}]}`,
		"level string":        `{"all":[{"field":"level","op":"gte","value":"5"}]}`,
		"balance in":          `{"all":[{"field":"balance","op":"in","value":[1]}]}`,
		"has without id":      `{"all":[{"field":"badges_earned","op":"has","value":""}]}`,
		"badges contains":     `{"all":[{"field":"badges_earned","op":"contains","value":"b"}]}`,
		"too deep":            `{"all":[{"any":[{"all":[{"any":[{"field":"is_active","op":"eq","value":true}]}]}]}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseConditions(doc(t, raw))
			require.Error(t, err)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
			require.Equal(t, CodeInvalidConditions, errs.CodeOf(err))
		})
	}
}

func TestParseConditionsCapsCount(t *testing.T) {
	items := make([]any, MaxConditions+1)
	for i := range items {
		items[i] = map[string]any{"field": "is_active", "op": "eq", "value": true}
	}
	_, err := ParseConditions(map[string]any{"any": items})
	require.Error(t, err)
	_, err = ParseConditions(map[string]any{"any": items[:MaxConditions]})
	require.NoError(t, err)
}

func TestToMapRoundTrips(t *testing.T) {
	raw := doc(t, `{"all":[
		{"field":"attributes.country","op":"in","value":["GE","AM"]},
		{"field":"attributes.vip","op":"exists"},
		{"any":[{"field":"level","op":"gte","value":5},{"field":"badges_earned","op":"has","value":"b1"}]}
	]}`)
	g, err := ParseConditions(raw)
	require.NoError(t, err)
	b, err := json.Marshal(g.ToMap())
	require.NoError(t, err)
	var back map[string]any
	require.NoError(t, json.Unmarshal(b, &back))
	require.Equal(t, raw, back)
	g2, err := ParseConditions(back)
	require.NoError(t, err)
	require.Equal(t, g, g2)
	require.Equal(t, Needs{Level: true, Badges: true}, g.Needs())
}

func TestNeeds(t *testing.T) {
	g, err := ParseConditions(doc(t, `{"any":[
		{"field":"balance","op":"gt","value":1},
		{"field":"last_seen_days","op":"lte","value":7},
		{"field":"attributes.a","op":"eq","value":1}]}`))
	require.NoError(t, err)
	require.Equal(t, Needs{Wallet: true, LastSeen: true}, g.Needs())

	g, err = ParseConditions(doc(t, `{"all":[{"field":"is_active","op":"eq","value":true}]}`))
	require.NoError(t, err)
	require.Equal(t, Needs{}, g.Needs(), "player-only conditions call no other reader")
}

func TestMatches(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	seen := now.Add(-3*24*time.Hour - time.Hour)
	f := PlayerFacts{
		Active:    true,
		CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Attributes: map[string]any{
			"country": "GE", "age": float64(31), "tags": []any{"beta", "vip"},
			"profile": map[string]any{"plan": "pro", "signup": "2026-02-15"},
			"empty":   nil,
		},
		Level: 4, Balance: 250, LifetimeEarned: 1000,
		Badges:   map[string]bool{"b1": true, "b2": true},
		LastSeen: &seen,
	}
	never := f
	never.LastSeen = nil

	cases := []struct {
		cond  string
		facts PlayerFacts
		want  bool
	}{
		{`{"field":"attributes.country","op":"eq","value":"GE"}`, f, true},
		{`{"field":"attributes.country","op":"neq","value":"GE"}`, f, false},
		{`{"field":"attributes.missing","op":"neq","value":"GE"}`, f, true},
		{`{"field":"attributes.missing","op":"eq","value":"GE"}`, f, false},
		{`{"field":"attributes.age","op":"gte","value":31}`, f, true},
		{`{"field":"attributes.age","op":"gt","value":31}`, f, false},
		{`{"field":"attributes.age","op":"eq","value":31}`, f, true},
		{`{"field":"attributes.age","op":"eq","value":"31"}`, f, false},
		{`{"field":"attributes.country","op":"in","value":["AM","GE"]}`, f, true},
		{`{"field":"attributes.country","op":"not_in","value":["AM","GE"]}`, f, false},
		{`{"field":"attributes.missing","op":"not_in","value":["AM"]}`, f, true},
		{`{"field":"attributes.tags","op":"contains","value":"vip"}`, f, true},
		{`{"field":"attributes.country","op":"contains","value":"G"}`, f, true},
		{`{"field":"attributes.profile.plan","op":"eq","value":"pro"}`, f, true},
		{`{"field":"attributes.profile.signup","op":"lt","value":"2026-03-01"}`, f, true},
		{`{"field":"attributes.profile.plan.deeper","op":"exists"}`, f, false},
		{`{"field":"attributes.profile","op":"exists"}`, f, true},
		{`{"field":"attributes.empty","op":"exists"}`, f, false},
		{`{"field":"attributes.empty","op":"not_exists"}`, f, true},
		{`{"field":"attributes.country","op":"gt","value":5}`, f, false},
		{`{"field":"is_active","op":"eq","value":true}`, f, true},
		{`{"field":"is_active","op":"neq","value":true}`, f, false},
		{`{"field":"created_at","op":"before","value":"2026-04-01"}`, f, true},
		{`{"field":"created_at","op":"after","value":"2026-04-01T00:00:00Z"}`, f, false},
		{`{"field":"level","op":"gte","value":4}`, f, true},
		{`{"field":"level","op":"lte","value":3}`, f, false},
		{`{"field":"balance","op":"gte","value":250}`, f, true},
		{`{"field":"lifetime_earned","op":"lt","value":1000}`, f, false},
		{`{"field":"badges_earned","op":"gte","value":2}`, f, true},
		{`{"field":"badges_earned","op":"has","value":"b2"}`, f, true},
		{`{"field":"badges_earned","op":"not_has","value":"b2"}`, f, false},
		{`{"field":"badges_earned","op":"not_has","value":"b9"}`, f, true},
		{`{"field":"last_seen_days","op":"lte","value":3}`, f, true},
		{`{"field":"last_seen_days","op":"lte","value":2}`, f, false},
		{`{"field":"last_seen_days","op":"gte","value":30}`, never, true},
		{`{"field":"last_seen_days","op":"lte","value":30}`, never, false},
	}
	for _, c := range cases {
		g, err := ParseConditions(doc(t, `{"all":[`+c.cond+`]}`))
		require.NoError(t, err, c.cond)
		require.Equal(t, c.want, g.Matches(c.facts, now), c.cond)
	}
}

func TestGroupsCombine(t *testing.T) {
	now := time.Now()
	g, err := ParseConditions(doc(t, `{"all":[
		{"field":"is_active","op":"eq","value":true},
		{"any":[{"field":"level","op":"gte","value":10},{"field":"balance","op":"gte","value":100}]}
	]}`))
	require.NoError(t, err)
	require.True(t, g.Matches(PlayerFacts{Active: true, Balance: 100}, now))
	require.True(t, g.Matches(PlayerFacts{Active: true, Level: 10}, now))
	require.False(t, g.Matches(PlayerFacts{Active: true}, now))
	require.False(t, g.Matches(PlayerFacts{Active: false, Level: 10}, now))
}

func TestNewSegmentAndApply(t *testing.T) {
	now := time.Now().UTC()
	cond := map[string]any{"all": []any{map[string]any{"field": "is_active", "op": "eq", "value": true}}}

	_, err := NewSegment("", "u", NewSegmentInput{Name: "x", Conditions: cond}, now)
	require.ErrorIs(t, err, ErrNoTenant)
	_, err = NewSegment("t", "u", NewSegmentInput{Name: "  ", Conditions: cond}, now)
	require.ErrorIs(t, err, ErrNameRequired)
	_, err = NewSegment("t", "u", NewSegmentInput{Name: "x"}, now)
	require.Equal(t, CodeInvalidConditions, errs.CodeOf(err))

	s, err := NewSegment("t", "u", NewSegmentInput{Name: " VIPs ", Conditions: cond}, now)
	require.NoError(t, err)
	require.Equal(t, "VIPs", s.Name)
	require.NotEmpty(t, s.ID)

	name := "Whales"
	changed, err := s.Apply(SegmentPatch{Name: &name}, now)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, "Whales", s.Name)

	changed, err = s.Apply(SegmentPatch{Conditions: map[string]any{"any": []any{
		map[string]any{"field": "balance", "op": "gte", "value": 1000.0}}}}, now)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, MatchAny, s.Conditions.Match)

	_, err = s.Apply(SegmentPatch{Conditions: map[string]any{"bad": 1}}, now)
	require.Error(t, err)

	require.False(t, s.Refreshing(now))
	until := now.Add(time.Minute)
	s.RefreshLeaseUntil = &until
	require.True(t, s.Refreshing(now))
}
