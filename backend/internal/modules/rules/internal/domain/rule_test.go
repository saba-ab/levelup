package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/rules/internal/domain/eval"
)

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"First Purchase Bonus": "first-purchase-bonus",
		"  --Hello__World!! ":  "hello-world",
		"ÜBER deal 2026":       "ber-deal-2026",
	} {
		require.Equal(t, want, Slugify(in))
	}
}

func TestNewRuleValidation(t *testing.T) {
	ok := NewRuleParams{TenantID: "t", Name: "Bonus", TriggerEvent: "purchase.completed:v1"}
	r, err := NewRule(ok, now)
	require.NoError(t, err)
	require.Equal(t, StatusDraft, r.Status)
	require.Equal(t, "bonus", r.Slug)

	cases := map[string]func(p *NewRuleParams){
		"name_required":         func(p *NewRuleParams) { p.Name = " " },
		"invalid_slug":          func(p *NewRuleParams) { p.Slug = "a--b" },
		"invalid_trigger_event": func(p *NewRuleParams) { p.TriggerEvent = "Has Space" },
		"invalid_program_id":    func(p *NewRuleParams) { p.ProgramID = "nope" },
		"invalid_priority":      func(p *NewRuleParams) { p.Priority = -1 },
	}
	for name, mut := range cases {
		p := ok
		mut(&p)
		_, err := NewRule(p, now)
		require.Error(t, err, name)
	}
	p := ok
	p.Name = "!!!" // slug derives empty
	_, err = NewRule(p, now)
	require.ErrorIs(t, err, ErrBadSlug)
}

func TestStatusTransitions(t *testing.T) {
	cases := []struct {
		from, to   string
		hasVersion bool
		err        error
	}{
		{StatusDraft, StatusActive, false, ErrPublishRequired},
		{StatusDraft, StatusInactive, false, ErrInvalidTransition},
		{StatusDraft, StatusArchived, false, nil},
		{StatusActive, StatusInactive, true, nil},
		{StatusInactive, StatusActive, true, nil},
		{StatusActive, StatusArchived, true, nil},
		{StatusArchived, StatusActive, true, ErrRuleArchived},
		{StatusArchived, StatusArchived, true, nil},
		{StatusActive, StatusActive, true, nil},
		{StatusActive, "paused", true, ErrBadStatus},
	}
	for _, tc := range cases {
		r := Rule{Status: tc.from}
		if tc.hasVersion {
			r.CurrentVersionID = "v"
		}
		err := r.SetStatus(tc.to, now)
		if tc.err != nil {
			require.ErrorIs(t, err, tc.err, "%s→%s", tc.from, tc.to)
			require.Equal(t, tc.from, r.Status)
			continue
		}
		require.NoError(t, err, "%s→%s", tc.from, tc.to)
		require.Equal(t, tc.to, r.Status)
	}
}

func TestPublishSetsTheOneLiveVersion(t *testing.T) {
	r := Rule{ID: "r", Status: StatusDraft}
	require.ErrorIs(t, r.Publish(Version{ID: "v1", RuleID: "other"}, now), ErrVersionNotFound)
	require.NoError(t, r.Publish(Version{ID: "v1", RuleID: "r"}, now))
	require.True(t, r.Live())
	require.NoError(t, r.Publish(Version{ID: "v2", RuleID: "r"}, now))
	require.Equal(t, "v2", r.CurrentVersionID, "a rule has exactly one current version")
	r.Status = StatusArchived
	require.ErrorIs(t, r.Publish(Version{ID: "v3", RuleID: "r"}, now), ErrRuleArchived)
}

func TestAffectsRuleset(t *testing.T) {
	live := Rule{Status: StatusActive, CurrentVersionID: "v", TriggerEvent: "a", Priority: 1}
	draft := Rule{Status: StatusDraft, TriggerEvent: "a"}
	changed := draft
	changed.Priority = 9
	require.False(t, changed.AffectsRuleset(draft), "drafts never affect the ruleset")
	moved := live
	moved.Priority = 2
	require.True(t, moved.AffectsRuleset(live))
	desc := live
	desc.Description = "x"
	require.False(t, desc.AffectsRuleset(live), "description is not evaluated")
	deleted := live
	deleted.Delete(now)
	require.True(t, deleted.AffectsRuleset(live))
}

func TestVersionDraftEditAndImmutability(t *testing.T) {
	r := Rule{ID: "r", TenantID: "t"}
	v, err := NewVersion(r, 1, Definition{Actions: json.RawMessage(`[{"type":"grant_xp","amount":1}]`)}, "u", eval.Options{}, now)
	require.NoError(t, err)
	require.Equal(t, "null", string(v.Conditions))
	require.Equal(t, "null", string(v.Schedule))
	require.False(t, v.StopProcessing)

	require.NoError(t, v.Edit(DefinitionPatch{Actions: json.RawMessage(`[{"type":"grant_xp","amount":2}]`)}, eval.Options{}))
	require.JSONEq(t, `[{"type":"grant_xp","amount":2}]`, string(v.Actions))

	stop := true
	require.NoError(t, v.Edit(DefinitionPatch{StopProcessing: &stop,
		Schedule: json.RawMessage(`{"days_of_week":[1,2],"timezone":"Asia/Tbilisi"}`)}, eval.Options{}))
	require.True(t, v.StopProcessing)
	require.JSONEq(t, `{"days_of_week":[1,2],"timezone":"Asia/Tbilisi"}`, string(v.Schedule))
	require.JSONEq(t, `[{"type":"grant_xp","amount":2}]`, string(v.Actions), "untouched parts are kept")

	before := v
	require.Error(t, v.Edit(DefinitionPatch{Conditions: json.RawMessage(`[{"source":"x"}]`)}, eval.Options{}))
	require.Error(t, v.Edit(DefinitionPatch{Schedule: json.RawMessage(`{"timezone":"Mars/Base"}`)}, eval.Options{}))
	require.Equal(t, before, v, "a failed edit leaves the draft untouched")

	require.NoError(t, v.Edit(DefinitionPatch{Schedule: json.RawMessage(`null`)}, eval.Options{}))
	require.Equal(t, "null", string(v.Schedule), "an explicit null clears the schedule")

	v.MarkPublished(now)
	first := *v.PublishedAt
	v.MarkPublished(now.Add(time.Hour))
	require.Equal(t, first, *v.PublishedAt)
	require.ErrorIs(t, v.Edit(DefinitionPatch{Actions: json.RawMessage(`[{"type":"grant_xp","amount":3}]`)}, eval.Options{}), ErrVersionPublished)

	_, err = NewVersion(r, 2, Definition{Actions: json.RawMessage(`[{"type":"credit_points","amount":0}]`)}, "u", eval.Options{}, now)
	require.Error(t, err)
	require.True(t, DefinitionPatch{}.IsZero())
	require.False(t, DefinitionPatch{StopProcessing: &stop}.IsZero())
}

func TestDerivedIDsAreDeterministic(t *testing.T) {
	require.Equal(t, DecisionID("a"), DecisionID("a"))
	require.NotEqual(t, DecisionID("a"), DecisionID("b"))
	require.Equal(t, EffectKey("a", "v", 0), EffectKey("a", "v", 0))
	require.NotEqual(t, EffectKey("a", "v", 0), EffectKey("a", "v", 1))
	require.NotEqual(t, EffectKey("a", "v", 0), EffectID("a", "v", 0))
	require.Equal(t, "d:2026-10-05", DayWindow(now))
	require.Equal(t, "w:2026-W41", WeekWindow(now))
}
