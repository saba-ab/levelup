package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

const (
	badgeID   = "0192f0a4-0000-7000-8000-000000000001"
	missionID = "0192f0a4-0000-7000-8000-000000000002"
	rewardID  = "0192f0a4-0000-7000-8000-000000000003"
	unknownID = "0192f0a4-0000-7000-8000-0000000000ff"
)

func ctxFixture() Context {
	return Context{
		EventTypes: []EventTypeRef{{Slug: "purchase_completed"}, {Slug: "review_submitted"}},
		Badges:     []EntityRef{{ID: badgeID, Name: "First Review"}},
		Missions:   []EntityRef{{ID: missionID, Name: "Shop 3x"}},
		Rewards:    []EntityRef{{ID: rewardID, Name: "Mug"}},
		Levels:     []LevelRef{{LevelNumber: 1, XPRequired: 0}, {LevelNumber: 2, XPRequired: 100}},
	}
}

// decode mimics the service: model output decoded with UseNumber.
func decode(t *testing.T, s string) []any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	dec.UseNumber()
	var v []any
	require.NoError(t, dec.Decode(&v))
	return v
}

func normalize(t *testing.T, kind string, count int, raw string) DraftSet {
	t.Helper()
	req, err := NewDraftRequest(kind, "prompt", count, ctxFixture())
	require.NoError(t, err)
	return NormalizeDrafts(req, decode(t, raw))
}

func TestNewDraftRequest(t *testing.T) {
	cases := []struct {
		name   string
		kind   string
		prompt string
		count  int
		want   error
	}{
		{"default count", "badge", "x", 0, nil},
		{"kind is case-insensitive", " Rule ", "x", 5, nil},
		{"unknown kind", "streak", "x", 1, ErrUnknownKind},
		{"blank prompt", "badge", "   ", 1, ErrPromptRequired},
		{"prompt too long", "badge", strings.Repeat("é", MaxPromptLen+1), 1, ErrPromptTooLong},
		{"prompt at limit", "badge", strings.Repeat("é", MaxPromptLen), 1, nil},
		{"count too high", "badge", "x", 6, ErrBadCount},
		{"negative count", "badge", "x", -1, ErrBadCount},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := NewDraftRequest(tc.kind, tc.prompt, tc.count, Context{})
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			if tc.count == 0 {
				require.Equal(t, DefaultCount, req.Count)
			}
		})
	}
}

func TestContextValidation(t *testing.T) {
	_, err := NewDraftRequest("badge", "x", 1, Context{Badges: []EntityRef{{ID: "nope"}}})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	require.Equal(t, "invalid_context", errs.CodeOf(err))
	require.Contains(t, errs.FieldsOf(err), "context.badges[0].id")

	_, err = NewDraftRequest("badge", "x", 1, Context{EventTypes: []EventTypeRef{{Slug: "Bad Slug"}}})
	require.Contains(t, errs.FieldsOf(err), "context.event_types[0].slug")

	many := make([]EntityRef, MaxContextItems+1)
	for i := range many {
		many[i] = EntityRef{ID: badgeID}
	}
	_, err = NewDraftRequest("badge", "x", 1, Context{Rewards: many})
	require.Contains(t, errs.FieldsOf(err), "context.rewards")
}

func TestBadgeDrafts(t *testing.T) {
	set := normalize(t, "badge", 5, `[
		{"name":"First Review","description":"Leave a review.","tier":"bronze","category":"social",
		 "points_value":50,"is_stackable":false,"max_awards":3,"is_secret":false,
		 "requirements":{"all":[{"metric":"activity_count","event_type":"review_submitted","gte":1}],"any":null}},
		{"name":"Bad","description":"","tier":"mythic","category":"social","points_value":null,
		 "is_stackable":false,"max_awards":null,"is_secret":false,"requirements":null},
		{"name":"Unknown event","description":"","tier":"gold","category":"skill","points_value":null,
		 "is_stackable":true,"max_awards":10,"is_secret":true,"requirements":{"all":[
			{"metric":"activity_count","event_type":"login","gte":3},
			{"metric":"level","event_type":"purchase_completed","gte":0}],"any":null}},
		"not an object"
	]`)
	require.Len(t, set.Drafts, 1)
	require.Equal(t, map[string]any{
		"name": "First Review", "description": "Leave a review.", "tier": "bronze", "category": "social",
		"points_value": int64(50), "is_stackable": false, "is_secret": false,
		"requirements": map[string]any{"all": []map[string]any{
			{"metric": "activity_count", "event_type": "review_submitted", "gte": int64(1)},
		}},
	}, set.Drafts[0], "max_awards dropped for a non-stackable badge; nulls omitted")

	require.Len(t, set.Rejected, 3)
	require.Equal(t, 1, set.Rejected[0].Index)
	require.Contains(t, set.Rejected[0].Reasons[0], "tier: must be one of")
	require.Equal(t, 2, set.Rejected[1].Index)
	require.Equal(t, []string{
		"requirements.all[0].event_type: must be one of the event types listed in context",
		"requirements.all[1].event_type: is only allowed with metric activity_count",
		"requirements.all[1].gte: must be between 1 and 1000000000000",
	}, set.Rejected[1].Reasons)
	require.Equal(t, []string{"draft is not an object"}, set.Rejected[2].Reasons)
}

func TestDraftsBeyondCountAreIgnored(t *testing.T) {
	set := normalize(t, "segment", 1, `[
		{"name":"A","description":"","conditions":{"all":[{"field":"level","operator":"gte","value":5}],"any":null}},
		{"name":"B","description":"","conditions":{"all":[{"field":"level","operator":"gte","value":6}],"any":null}}
	]`)
	require.Len(t, set.Drafts, 1)
	require.Empty(t, set.Rejected)
}

func TestLevelDraftsKeepTheLadderMonotonic(t *testing.T) {
	set := normalize(t, "level", 5, `[
		{"level_number":3,"name":"Gold","description":"","xp_required":500,"points_reward":0,
		 "badge_reward_id":null,"perks":{"benefits":["Free shipping"]}},
		{"level_number":2,"name":"Dup","description":"","xp_required":50,"points_reward":0,"badge_reward_id":null,"perks":{"benefits":[]}},
		{"level_number":4,"name":"Cheap","description":"","xp_required":400,"points_reward":0,"badge_reward_id":null,"perks":{"benefits":[]}},
		{"level_number":5,"name":"Hacked","description":"","xp_required":900,"points_reward":0,"badge_reward_id":"`+unknownID+`","perks":{"benefits":[]}},
		{"level_number":6,"name":"Diamond","description":"","xp_required":2000,"points_reward":100,"badge_reward_id":"`+badgeID+`","perks":{"benefits":[]}}
	]`)
	require.Len(t, set.Drafts, 2)
	require.Equal(t, map[string]any{
		"level_number": int64(3), "name": "Gold", "xp_required": int64(500), "points_reward": int64(0),
		"perks": map[string]any{"benefits": []string{"Free shipping"}},
	}, set.Drafts[0])
	require.Equal(t, badgeID, set.Drafts[1]["badge_reward_id"])

	require.Len(t, set.Rejected, 3)
	require.Equal(t, []string{"level_number: level 2 already exists"}, set.Rejected[0].Reasons)
	require.Contains(t, set.Rejected[1].Reasons[0], "xp_required: must increase with level_number", "level 4 needs more than draft level 3")
	require.Equal(t, []string{"badge_reward_id: must be the id of a badge listed in context"}, set.Rejected[2].Reasons)
}

func TestMissionDraftsAreCreatedAsDraftStatus(t *testing.T) {
	set := normalize(t, "mission", 2, `[
		{"name":"Shop","description":"Buy 3 times","type":"weekly","target":3,
		 "criteria":{"event_type":"purchase_completed","where":[{"field":"cart.total","operator":"gte","value":20},
			{"field":"coupon","operator":"exists","value":null}],"increment":{"by":"property","field":"quantity"}},
		 "points_reward":100,"xp_reward":50,"badge_reward_id":null,"max_completions_per_player":2},
		{"name":"Zero","description":"","type":"one_time","target":0,"criteria":{"event_type":null,"where":null,"increment":null},
		 "points_reward":10,"xp_reward":0,"badge_reward_id":null,"max_completions_per_player":null}
	]`)
	require.Len(t, set.Drafts, 1)
	require.Equal(t, map[string]any{
		"name": "Shop", "description": "Buy 3 times", "type": "weekly", "status": "draft", "target": int64(3),
		"criteria": map[string]any{
			"event_type": "purchase_completed",
			"where": []map[string]any{
				{"field": "cart.total", "operator": "gte", "value": json.Number("20")},
				{"field": "coupon", "operator": "exists"},
			},
			"increment": map[string]any{"by": "property", "field": "quantity"},
		},
		"points_reward": int64(100), "xp_reward": int64(50), "max_completions_per_player": int64(2),
	}, set.Drafts[0])
	require.Equal(t, []string{"target: must be between 1 and 1000000000"}, set.Rejected[0].Reasons)

	set = normalize(t, "mission", 2, `[
		{"name":"Count","description":"","type":"daily","target":2,
		 "criteria":{"event_type":"purchase_completed","where":null,"increment":{"by":"count","field":null}},
		 "points_reward":0,"xp_reward":0,"badge_reward_id":null,"max_completions_per_player":null},
		{"name":"Orphan where","description":"","type":"daily","target":2,
		 "criteria":{"event_type":null,"where":[{"field":"amount","operator":"gt","value":"10"}],"increment":null},
		 "points_reward":0,"xp_reward":0,"badge_reward_id":null,"max_completions_per_player":null}
	]`)
	require.Len(t, set.Drafts, 1)
	require.Equal(t, map[string]any{"event_type": "purchase_completed"}, set.Drafts[0]["criteria"], "increment by count is the default")
	require.Equal(t, []string{
		"criteria.event_type: is required when where or increment is set",
		"criteria.where[0].value: must be a number",
	}, set.Rejected[0].Reasons)
}

func TestRewardDrafts(t *testing.T) {
	set := normalize(t, "reward", 4, `[
		{"name":"10% off","description":"","type":"discount","points_cost":500,"value":"10.00","value_type":"percentage",
		 "badge_reward_id":null,"max_redemptions":null,"max_per_player":1,"claim_ttl_days":30,"level_requirement":null},
		{"name":"Too much","description":"","type":"discount","points_cost":500,"value":"150","value_type":"percentage",
		 "badge_reward_id":null,"max_redemptions":null,"max_per_player":null,"claim_ttl_days":null,"level_requirement":null},
		{"name":"Badge","description":"","type":"badge","points_cost":100,"value":null,"value_type":null,
		 "badge_reward_id":null,"max_redemptions":null,"max_per_player":null,"claim_ttl_days":null,"level_requirement":null},
		{"name":"Float","description":"","type":"item","points_cost":10.5,"value":null,"value_type":null,
		 "badge_reward_id":null,"max_redemptions":null,"max_per_player":null,"claim_ttl_days":null,"level_requirement":null}
	]`)
	require.Len(t, set.Drafts, 1)
	require.Equal(t, map[string]any{
		"name": "10% off", "type": "discount", "status": "draft", "points_cost": int64(500), "value": "10.00",
		"value_type": "percentage", "max_per_player": int64(1), "claim_ttl_days": int64(30),
	}, set.Drafts[0])
	require.Len(t, set.Rejected, 3)
	require.Contains(t, set.Rejected[0].Reasons, "value: a percentage must be greater than 0 and at most 100")
	require.Contains(t, set.Rejected[1].Reasons, "badge_reward_id: badge rewards need a badge listed in context")
	require.Contains(t, set.Rejected[2].Reasons, "points_cost: must be an integer")
}

func TestRuleDraftsFollowTheRulesGrammar(t *testing.T) {
	set := normalize(t, "rule", 5, `[
		{"name":"Points per purchase","description":"","trigger_event":"purchase_completed","priority":10,
		 "conditions":{"all":[
			{"source":"trigger","field":"amount","operator":"gte","value":100},
			{"source":"player","field":"attributes.country","operator":"in","value":["DE","AT"]},
			{"source":"activity","field":"properties.channel","operator":"exists","value":null}],"any":null},
		 "actions":[
			{"type":"credit_points","amount":20,"description":"Big order","badge_id":null,"mission_id":null,"increment":null,"reward_id":null,"activity_key":null},
			{"type":"award_badge","amount":null,"description":null,"badge_id":"`+badgeID+`","mission_id":null,"increment":null,"reward_id":null,"activity_key":null},
			{"type":"progress_mission","amount":null,"description":null,"badge_id":null,"mission_id":"`+missionID+`","increment":null,"reward_id":null,"activity_key":null}],
		 "limits":{"max_per_player":null,"max_per_player_per_day":5,"max_per_player_per_week":null,"cooldown_seconds":null}},
		{"name":"Invented badge","description":"","trigger_event":"purchase_completed","priority":0,"conditions":null,
		 "actions":[{"type":"award_badge","amount":null,"description":null,"badge_id":"`+unknownID+`","mission_id":null,"increment":null,"reward_id":null,"activity_key":null}],
		 "limits":null},
		{"name":"No actions","description":"","trigger_event":"purchase_completed","priority":0,"conditions":null,"actions":[],"limits":null},
		{"name":"Bad field","description":"","trigger_event":"login","priority":0,
		 "conditions":{"all":[{"source":"player","field":"password","operator":"eq","value":"x"}],"any":[]},
		 "actions":[{"type":"grant_xp","amount":0,"description":null,"badge_id":null,"mission_id":null,"increment":null,"reward_id":null,"activity_key":null}],"limits":null}
	]`)
	require.Len(t, set.Drafts, 1)
	d := set.Drafts[0]
	require.Equal(t, "purchase_completed", d["trigger_event"])
	require.Equal(t, int64(10), d["priority"])
	require.Equal(t, map[string]any{"all": []map[string]any{
		{"source": "trigger", "field": "amount", "operator": "gte", "value": json.Number("100")},
		{"source": "player", "field": "attributes.country", "operator": "in", "value": []any{"DE", "AT"}},
		{"source": "activity", "field": "properties.channel", "operator": "exists"},
	}}, d["conditions"])
	require.Equal(t, []map[string]any{
		{"type": "credit_points", "amount": int64(20), "description": "Big order"},
		{"type": "award_badge", "badge_id": badgeID},
		{"type": "progress_mission", "mission_id": missionID},
	}, d["actions"], "only the keys each action type accepts")
	require.Equal(t, map[string]any{"max_per_player_per_day": int64(5)}, d["limits"])

	out, err := json.Marshal(d)
	require.NoError(t, err)
	require.Contains(t, string(out), `"value":100`)

	require.Len(t, set.Rejected, 3)
	require.Equal(t, []string{"actions[0].badge_id: must be the id of a badge listed in context"}, set.Rejected[0].Reasons)
	require.Equal(t, []string{"actions: must be a non-empty list"}, set.Rejected[1].Reasons)
	require.ElementsMatch(t, []string{
		"actions[0].amount: must be between 1 and 1000000000",
		"conditions: needs exactly one of all or any",
		"trigger_event: must be one of the event types listed in context",
	}, set.Rejected[2].Reasons)
}

func TestRuleConditionOperatorValues(t *testing.T) {
	leaf := func(op, value string) string {
		return `[{"name":"R","description":"","trigger_event":"purchase_completed","priority":0,
			"conditions":{"any":[{"source":"trigger","field":"tags","operator":"` + op + `","value":` + value + `}],"all":null},
			"actions":[{"type":"credit_points","amount":1,"description":null,"badge_id":null,"mission_id":null,"increment":null,"reward_id":null,"activity_key":null}],"limits":null}]`
	}
	cases := []struct {
		op, value string
		ok        bool
	}{
		{"in", `"vip"`, true}, // scalar wrapped like the rules grammar
		{"in", `[]`, false},
		{"contains", `""`, false},
		{"contains", `"vip"`, true},
		{"eq", `null`, false},
		{"eq", `{"a":1}`, false},
		{"not_exists", `null`, true},
		{"between", `1`, false},
	}
	for _, tc := range cases {
		set := normalize(t, "rule", 1, leaf(tc.op, tc.value))
		require.Equal(t, tc.ok, len(set.Drafts) == 1, "%s %s: %v", tc.op, tc.value, set.Rejected)
	}
}

func TestSegmentDrafts(t *testing.T) {
	set := normalize(t, "segment", 3, `[
		{"name":"Engaged DACH","description":"High level","conditions":{"all":null,"any":[
			{"field":"attributes.country","operator":"in","value":["DE","AT"]},
			{"field":"level","operator":"gte","value":5}]}},
		{"name":"Bad path","description":"","conditions":{"all":[{"field":"a..b","operator":"eq","value":1}],"any":null}},
		{"name":"Missing","description":"","conditions":null}
	]`)
	require.Len(t, set.Drafts, 1)
	require.Equal(t, map[string]any{
		"name": "Engaged DACH", "description": "High level",
		"conditions": map[string]any{"any": []map[string]any{
			{"field": "attributes.country", "operator": "in", "value": []any{"DE", "AT"}},
			{"field": "level", "operator": "gte", "value": json.Number("5")},
		}},
	}, set.Drafts[0])
	require.Equal(t, []string{"conditions.all[0].field: must be a dot path"}, set.Rejected[0].Reasons)
	require.Equal(t, []string{"conditions: is required"}, set.Rejected[1].Reasons)
}

func TestTemplateCatalogue(t *testing.T) {
	names := map[string]bool{}
	ids := map[string]bool{}
	for _, tpl := range Templates {
		require.False(t, ids[tpl.ID], "duplicate id %s", tpl.ID)
		ids[tpl.ID] = true
		names[tpl.Name] = true
		_, err := NewDraftRequest(tpl.Kind, tpl.ExamplePrompt, tpl.DefaultCount, Context{})
		require.NoError(t, err, tpl.ID)
	}
	for _, n := range []string{"Badge Concept", "Badge Collection", "Level Description", "Tier Benefits",
		"Onboarding Mission", "Reward Ideas", "Point Rules", "Automation Rules", "Segment Definition", "Segmentation Strategy"} {
		require.True(t, names[n], n)
	}
}

func TestRemaining(t *testing.T) {
	require.Equal(t, int64(-1), Remaining(0, 10))
	require.Equal(t, int64(5), Remaining(10, 5))
	require.Equal(t, int64(0), Remaining(10, 12))
}
