package eval

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

func matches(t testing.TB, conditions string, f Facts) bool {
	t.Helper()
	prog := Compile([]RuleSource{{RuleID: ruleA, RuleVersionID: verA,
		Conditions: json.RawMessage(conditions), Actions: json.RawMessage(credit(1))}}, Options{})
	res := Evaluate(prog, f)
	require.Len(t, res.Rules, 1)
	require.NotEqual(t, StatusInvalid, res.Rules[0].Status, res.Rules[0].Error)
	return res.Rules[0].Matched
}

func cond(src, field, op string, v any) string {
	m := map[string]any{"source": src, "field": field, "operator": op}
	if v != nil {
		m["value"] = v
	}
	b, _ := json.Marshal([]any{m})
	return string(b)
}

func TestOperatorsTypedSemantics(t *testing.T) {
	f := Facts{Activity: Activity{
		EventType: "purchase", EventID: "e1", CausationDepth: 1,
		Properties: map[string]any{
			"amount": json.Number("100"), "price": json.Number("9.5"), "big": json.Number("9007199254740993"),
			"sku": "PRO-123", "num_str": "42", "flag": true, "nothing": nil,
			"tags":   []any{"a", "b", json.Number("3")},
			"nested": map[string]any{"deep": map[string]any{"x": "y"}, "list": []any{map[string]any{"id": "z"}}},
			"date":   "2026-10-05",
		},
		Context: map[string]any{"channel": "web"},
	}}
	cases := []struct {
		name string
		cond string
		want bool
	}{
		{"eq number", cond("trigger", "amount", "eq", 100), true},
		{"eq float literal vs int", cond("trigger", "amount", "eq", 100.0), true},
		{"eq numeric string coerces", cond("trigger", "amount", "eq", "100"), true},
		{"eq non-numeric string vs number false", cond("trigger", "amount", "eq", "abc"), false},
		{"eq bool is not 1", cond("trigger", "flag", "eq", 1), false},
		{"eq bool", cond("trigger", "flag", "eq", true), true},
		{"neq mismatch true", cond("trigger", "flag", "neq", "true"), true},
		{"neq missing true", cond("trigger", "absent", "neq", 1), true},
		{"eq missing false", cond("trigger", "absent", "eq", 1), false},
		{"eq null false", cond("trigger", "nothing", "eq", 0), false},
		{"exact int64 beyond float precision", cond("trigger", "big", "eq", json.Number("9007199254740993")), true},
		{"exact int64 neighbour differs", cond("trigger", "big", "eq", json.Number("9007199254740992")), false},
		{"gt float", cond("trigger", "price", "gt", 9), true},
		{"lte float", cond("trigger", "price", "lte", 9.5), true},
		{"lt numeric string fact vs number", cond("trigger", "num_str", "lt", 50), true},
		{"gt string vs number non-numeric false (PHP true)", cond("trigger", "sku", "gt", 5), false},
		{"gt bool false", cond("trigger", "flag", "gt", 0), false},
		{"lt missing false (G10)", cond("trigger", "absent", "lt", 5), false},
		{"string ordering ISO dates", cond("trigger", "date", "gte", "2026-01-01"), true},
		{"two numeric strings compare numerically", cond("trigger", "num_str", "gt", "9"), true},
		{"in scalar wrapped", cond("trigger", "sku", "in", "PRO-123"), true},
		{"in list numbers", cond("trigger", "amount", "in", []any{1, 100}), true},
		{"in false", cond("trigger", "sku", "in", []any{"x"}), false},
		{"not_in true", cond("trigger", "sku", "not_in", []any{"x"}), true},
		{"not_in missing true", cond("trigger", "absent", "not_in", []any{"x"}), true},
		{"in missing false", cond("trigger", "absent", "in", []any{"x"}), false},
		{"contains substring", cond("trigger", "sku", "contains", "PRO-"), true},
		{"contains array element", cond("trigger", "tags", "contains", "b"), true},
		{"contains array number", cond("trigger", "tags", "contains", 3), true},
		{"contains number never stringified", cond("trigger", "amount", "contains", "10"), false},
		{"exists", cond("trigger", "sku", "exists", nil), true},
		{"exists null false", cond("trigger", "nothing", "exists", nil), false},
		{"not_exists missing", cond("trigger", "absent", "not_exists", nil), true},
		{"dot path", cond("trigger", "nested.deep.x", "eq", "y"), true},
		{"array index path", cond("trigger", "nested.list.0.id", "eq", "z"), true},
		{"array index out of range", cond("trigger", "nested.list.5.id", "exists", nil), false},
		{"activity.properties path", cond("activity", "properties.amount", "gte", 100), true},
		{"activity.context path", cond("activity", "context.channel", "eq", "web"), true},
		{"activity.event_type", cond("activity", "event_type", "eq", "purchase"), true},
		{"activity.causation_depth", cond("activity", "causation_depth", "lt", 2), true},
		{"player missing → false", cond("player", "level", "gte", 0), false},
		{"legacy operator alias", cond("trigger", "amount", "greater_than_or_equal", 100), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, matches(t, tc.cond, f))
		})
	}
}

func TestPlayerFacts(t *testing.T) {
	p := &Player{
		ID: "p1", ExternalID: "ext-1", Email: "a@b.c", Active: true,
		Attributes: map[string]any{"tier": "gold", "age": float64(30)},
		Level:      4, XP: 1200, HasProgress: true, Points: 0, HasPoints: true,
	}
	f := Facts{Player: p}
	require.True(t, matches(t, cond("player", "external_id", "eq", "ext-1"), f))
	require.True(t, matches(t, cond("player", "is_active", "eq", true), f))
	require.True(t, matches(t, cond("player", "attributes.tier", "eq", "gold"), f))
	require.True(t, matches(t, cond("player", "metadata.age", "gte", 18), f))
	require.True(t, matches(t, cond("player", "xp", "gt", 1000), f))
	require.True(t, matches(t, cond("player", "points_balance", "eq", 0), f))
	require.True(t, matches(t, cond("player", "email", "exists", nil), f))

	// level not loaded is missing, not 0 (Laravel B6 read 0 forever)
	p2 := *p
	p2.HasProgress = false
	require.False(t, matches(t, cond("player", "level", "gte", 0), Facts{Player: &p2}))
}

func TestCombinators(t *testing.T) {
	f := Facts{Activity: Activity{Properties: map[string]any{"amount": json.Number("150"), "coupon": "X"}}}
	all := `{"all":[{"source":"trigger","field":"amount","operator":"gte","value":100},
	               {"any":[{"source":"trigger","field":"coupon","operator":"eq","value":"Y"},
	                       {"not":{"source":"trigger","field":"vip","operator":"exists"}}]}]}`
	require.True(t, matches(t, all, f))

	notCoupon := `{"not":{"source":"trigger","field":"coupon","operator":"exists"}}`
	require.False(t, matches(t, notCoupon, f))

	anyNone := `{"any":[{"source":"trigger","field":"amount","operator":"lt","value":1},{"source":"trigger","field":"coupon","operator":"eq","value":"Z"}]}`
	require.False(t, matches(t, anyNone, f))

	// empty list and null conditions match (Laravel parity)
	require.True(t, matches(t, `[]`, f))
	require.True(t, matches(t, `null`, f))
}

func TestCompileRejectsBadGrammar(t *testing.T) {
	deep := `{"source":"trigger","field":"a","operator":"exists"}`
	for range MaxNestingDepth + 1 {
		deep = `{"not":` + deep + `}`
	}
	many := make([]string, DefaultMaxConditions+1)
	for i := range many {
		many[i] = `{"source":"trigger","field":"a","operator":"exists"}`
	}
	cases := []struct {
		name, conditions, actions, limits, field string
	}{
		{"unknown source", cond("session", "a", "eq", 1), credit(1), ``, "conditions[0].source"},
		{"unknown player field", cond("player", "password", "eq", 1), credit(1), ``, "conditions[0].field"},
		{"player attributes without path", cond("player", "attributes", "eq", 1), credit(1), ``, "conditions[0].field"},
		{"unknown activity field", cond("activity", "received_at", "eq", 1), credit(1), ``, "conditions[0].field"},
		{"bad path chars", cond("trigger", "a..b", "eq", 1), credit(1), ``, "conditions[0].field"},
		{"missing field", `[{"source":"trigger","operator":"eq","value":1}]`, credit(1), ``, "conditions[0].field"},
		{"missing operator", `[{"source":"trigger","field":"a","value":1}]`, credit(1), ``, "conditions[0].operator"},
		{"missing value", `[{"source":"trigger","field":"a","operator":"eq"}]`, credit(1), ``, "conditions[0].value"},
		{"null value", cond("trigger", "a", "eq", nil), credit(1), ``, "conditions[0].value"},
		{"object value", `[{"source":"trigger","field":"a","operator":"eq","value":{"x":1}}]`, credit(1), ``, "conditions[0].value"},
		{"value on exists", cond("trigger", "a", "exists", 1), credit(1), ``, "conditions[0].value"},
		{"empty in list", cond("trigger", "a", "in", []any{}), credit(1), ``, "conditions[0].value"},
		{"bool ordering", cond("trigger", "a", "gt", true), credit(1), ``, "conditions[0].value"},
		{"unknown leaf key", `[{"source":"trigger","field":"a","operator":"eq","value":1,"weight":2}]`, credit(1), ``, "conditions[0].weight"},
		{"both type and source", `[{"source":"trigger","type":"player","field":"a","operator":"eq","value":1}]`, credit(1), ``, "conditions[0].type"},
		{"combinator with siblings", `{"all":[{"source":"trigger","field":"a","operator":"exists"}],"any":[]}`, credit(1), ``, "conditions"},
		{"empty all", `{"all":[]}`, credit(1), ``, "conditions.all"},
		{"non-object element", `[1]`, credit(1), ``, "conditions[0]"},
		{"scalar conditions", `"yes"`, credit(1), ``, "conditions"},
		{"invalid json", `[{`, credit(1), ``, "conditions"},
		{"too deep", deep, credit(1), ``, "conditions.not.not.not.not.not"},
		{"too many conditions", "[" + strings.Join(many, ",") + "]", credit(1), ``, "conditions"},
		{"no actions", ``, ``, ``, "actions"},
		{"empty actions", ``, `[]`, ``, "actions"},
		{"actions object", ``, `{"type":"grant_xp","amount":1}`, ``, "actions"},
		{"unknown action key (B7: rule_id from JSON)", ``, `[{"type":"credit_points","amount":5,"rule_id":3}]`, ``, "actions[0].rule_id"},
		{"badge id not uuid", ``, `[{"type":"award_badge","badge_id":7}]`, ``, "actions[0].badge_id"},
		{"badge id missing", ``, `[{"type":"award_badge"}]`, ``, "actions[0].badge_id"},
		{"streak needs one key", ``, `[{"type":"record_streak"}]`, ``, "actions[0]"},
		{"streak both keys", ``, `[{"type":"record_streak","activity_key":"x","streak_id":"` + badge + `"}]`, ``, "actions[0]"},
		{"mission increment 0", ``, `[{"type":"progress_mission","mission_id":"` + badge + `","increment":0}]`, ``, "actions[0].increment"},
		{"reward missing", ``, `[{"type":"grant_reward"}]`, ``, "actions[0].reward_id"},
		{"xp amount huge", ``, `[{"type":"grant_xp","amount":1000000000000}]`, ``, "actions[0].amount"},
		{"description too long", ``, `[{"type":"grant_xp","amount":1,"description":"` + strings.Repeat("x", 300) + `"}]`, ``, "actions[0].description"},
		{"limits unknown", ``, credit(1), `{"max_per_month":1}`, "limits.max_per_month"},
		{"limits zero", ``, credit(1), `{"max_per_player":0}`, "limits.max_per_player"},
		{"limits not object", ``, credit(1), `[1]`, "limits"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CompileDefinition(json.RawMessage(tc.conditions), json.RawMessage(tc.actions), json.RawMessage(tc.limits), Options{})
			require.Error(t, err)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
			require.Equal(t, CodeInvalidDefinition, errs.CodeOf(err))
			require.Contains(t, errs.FieldsOf(err), tc.field, "fields: %v", errs.FieldsOf(err))
		})
	}
}

func TestCompileAcceptsFullGrammar(t *testing.T) {
	d, err := CompileDefinition(
		json.RawMessage(`{"all":[{"source":"activity","field":"properties.amount","op":"gte","value":100}]}`),
		json.RawMessage(`[
			{"type":"credit_points","amount":50,"description":"Purchase bonus"},
			{"type":"grant_xp","amount":100},
			{"type":"award_badge","badge_id":"`+badge+`"},
			{"type":"record_streak","streak_id":"`+badge+`"},
			{"type":"record_streak","activity_key":"daily_login"},
			{"type":"progress_mission","mission_id":"`+badge+`"},
			{"type":"grant_reward","reward_id":"`+badge+`"}
		]`),
		json.RawMessage(`{"max_per_player":1,"max_per_player_per_day":3,"max_per_player_per_week":5,"cooldown_seconds":3600}`),
		Options{})
	require.NoError(t, err)
	require.Len(t, d.Actions, 7)
	require.Equal(t, int64(1), d.Actions[5].Increment, "increment defaults to 1")
	require.Equal(t, Limits{MaxPerPlayer: 1, MaxPerPlayerPerDay: 3, MaxPerPlayerPerWeek: 5, CooldownSeconds: 3600}, d.Limits)
	require.Equal(t, map[string]any{"amount": int64(50), "description": "Purchase bonus"}, d.Actions[0].Params())
}

func TestCompileOptionsCapActions(t *testing.T) {
	_, err := CompileDefinition(nil, json.RawMessage(`[{"type":"grant_xp","amount":1},{"type":"grant_xp","amount":2}]`), nil, Options{MaxActions: 1})
	require.Error(t, err)
	require.Contains(t, errs.FieldsOf(err), "actions")
}

func TestProgramNeedsFlags(t *testing.T) {
	p := Compile([]RuleSource{
		{RuleID: ruleA, RuleVersionID: verA, Actions: json.RawMessage(credit(1)), Conditions: json.RawMessage(cond("player", "level", "gt", 1))},
		{RuleID: ruleB, RuleVersionID: verB, Actions: json.RawMessage(credit(1))},
	}, Options{})
	require.True(t, p.NeedsProgress())
	require.False(t, p.NeedsPoints())
	require.False(t, p.ProgramScoped())
	require.Equal(t, 2, p.Len())
}

func TestInvalidStoredRuleDoesNotBlockOthers(t *testing.T) {
	p := Compile([]RuleSource{
		{RuleID: ruleA, RuleVersionID: verA, Priority: 9, Actions: json.RawMessage(`[{"type":"nope"}]`)},
		{RuleID: ruleB, RuleVersionID: verB, Actions: json.RawMessage(credit(1))},
	}, Options{})
	res := Evaluate(p, Facts{})
	require.Equal(t, StatusInvalid, res.Rules[0].Status)
	require.NotEmpty(t, res.Rules[0].Error)
	require.Equal(t, StatusMatched, res.Rules[1].Status)
	require.Equal(t, 1, res.MatchedCount())
}

func TestProgramScoping(t *testing.T) {
	prog := "0198d000-0000-7000-8000-0000000000f1"
	p := Compile([]RuleSource{
		{RuleID: ruleA, RuleVersionID: verA, ProgramID: prog, Actions: json.RawMessage(credit(1))},
		{RuleID: ruleB, RuleVersionID: verB, Actions: json.RawMessage(credit(1))},
	}, Options{})
	require.True(t, p.ProgramScoped())

	// scoping disabled (no programs port): program rules apply tenant-wide
	res := Evaluate(p, Facts{})
	require.Equal(t, 2, res.MatchedCount())

	res = Evaluate(p, Facts{ProgramScoping: true})
	require.Equal(t, StatusOutOfScope, res.Rules[0].Status)
	require.Equal(t, StatusMatched, res.Rules[1].Status)

	res = Evaluate(p, Facts{ProgramScoping: true, EnrolledPrograms: []string{prog}})
	require.Equal(t, 2, res.MatchedCount())
}

func TestTraceExplainsDecision(t *testing.T) {
	p := Compile([]RuleSource{{RuleID: ruleA, RuleVersionID: verA, Actions: json.RawMessage(credit(1)),
		Conditions: json.RawMessage(cond("trigger", "amount", "gt", 200))}}, Options{})
	res := Evaluate(p, Facts{Activity: Activity{Properties: map[string]any{"amount": json.Number("100")}}})
	require.Equal(t, []CondTrace{{
		Path: "conditions[0]", Source: "trigger", Field: "amount", Operator: "gt",
		Expected: json.Number("200"), Actual: int64(100), Present: true, Result: false,
	}}, res.Rules[0].Trace)
}

// Determinism: same program + facts → identical result, regardless of the
// order sources were loaded in and of how many times it runs.
func TestEvaluateIsDeterministic(t *testing.T) {
	var srcs []RuleSource
	for i := range 40 {
		srcs = append(srcs, RuleSource{
			RuleID:        fmt.Sprintf("0198d000-0000-7000-8000-%012d", i),
			RuleVersionID: fmt.Sprintf("0198d000-0000-7000-9000-%012d", i),
			Priority:      i % 4,
			Conditions:    json.RawMessage(cond("trigger", "n", "gte", i)),
			Actions:       json.RawMessage(credit(i + 1)),
			Limits:        json.RawMessage(`{"max_per_player":2}`),
		})
	}
	f := Facts{Activity: Activity{Properties: map[string]any{"n": json.Number("20"), "x": []any{"a"}}},
		Player: &Player{ID: "p", Attributes: map[string]any{"k": "v"}}}
	want := Evaluate(Compile(srcs, Options{}), f)

	rng := rand.New(rand.NewPCG(1, 2))
	for range 50 {
		shuffled := append([]RuleSource(nil), srcs...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		require.Equal(t, want, Evaluate(Compile(shuffled, Options{}), f))
	}
	require.Equal(t, 21, want.MatchedCount())
}

func BenchmarkEvaluate200Rules20Conditions(b *testing.B) {
	var srcs []RuleSource
	for i := range 200 {
		conds := make([]string, 20)
		for j := range conds {
			conds[j] = fmt.Sprintf(`{"source":"trigger","field":"f%d","operator":"gte","value":%d}`, j, j)
		}
		srcs = append(srcs, RuleSource{
			RuleID: fmt.Sprintf("r%05d", i), RuleVersionID: fmt.Sprintf("v%05d", i),
			Conditions: json.RawMessage("[" + strings.Join(conds, ",") + "]"), Actions: json.RawMessage(credit(1)),
		})
	}
	p := Compile(srcs, Options{})
	propsMap := map[string]any{}
	for j := range 20 {
		propsMap[fmt.Sprintf("f%d", j)] = json.Number(fmt.Sprint(j + 1))
	}
	f := Facts{Activity: Activity{Properties: propsMap}}
	b.ReportAllocs()
	for b.Loop() {
		_ = Evaluate(p, f)
	}
}
