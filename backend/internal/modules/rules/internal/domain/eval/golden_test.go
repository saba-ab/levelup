package eval

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

// The golden corpus of doc 06 §9.2, evaluator half. Cases that exercise
// I/O (persistence, player resolution, redelivery, settlement) are proven
// in internal/app (TestGolden_G16..., TestGolden_G22..., ...) and named there
// after the same ids. G21 (streak same-day idempotency) and G27 (level-up
// across two levels) are owned by the streaks and progression modules; the
// rules side of them is "one command with the right parameters", asserted
// here.

const (
	ruleA = "0198d000-0000-7000-8000-00000000000a"
	ruleB = "0198d000-0000-7000-8000-00000000000b"
	ruleC = "0198d000-0000-7000-8000-00000000000c"
	verA  = "0198d000-0000-7000-8000-0000000000a1"
	verB  = "0198d000-0000-7000-8000-0000000000b1"
	verC  = "0198d000-0000-7000-8000-0000000000c1"
	badge = "0198d000-0000-7000-8000-0000000000ee"
)

// props decodes JSON properties the way the activity subscriber does
// (UseNumber, so 100.00 and 100 stay distinct literals but equal numbers).
func props(t testing.TB, s string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	dec.UseNumber()
	var m map[string]any
	require.NoError(t, dec.Decode(&m))
	return m
}

type goldenRule struct {
	id, ver    string
	priority   int
	conditions string
	actions    string
	limits     string
	schedule   string
	stop       bool
}

func (g goldenRule) source() RuleSource {
	return RuleSource{
		RuleID: g.id, RuleVersionID: g.ver, Name: "r" + g.id[len(g.id)-1:], Priority: g.priority,
		Conditions: json.RawMessage(g.conditions), Actions: json.RawMessage(g.actions), Limits: json.RawMessage(g.limits),
		Schedule: json.RawMessage(g.schedule), StopProcessing: g.stop,
	}
}

// goldenNow is every golden activity's occurred_at: Monday 2026-10-05 12:00 UTC.
var goldenNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

type wantEffect struct {
	rule   string
	typ    string
	amount int64
}

func credit(n int) string { return `[{"type":"credit_points","amount":` + itoa(n) + `}]` }

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestGoldenCorpusEvaluator(t *testing.T) {
	cases := []struct {
		id         string
		rules      []goldenRule
		properties string
		player     *Player
		history    *History
		wantOrder  []string // rule ids in evaluation order (all rules)
		wantStatus []string // per rule, in order
		effects    []wantEffect
	}{
		{
			id: "G01 no rules → no executions", rules: nil, properties: `{}`,
		},
		{
			id:         "G02 credit 50",
			rules:      []goldenRule{{id: ruleA, ver: verA, actions: credit(50)}},
			properties: `{"order_id":"1"}`,
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusMatched},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 50}},
		},
		{
			id: "G03 amount > 200 with 100 → skipped",
			rules: []goldenRule{{id: ruleA, ver: verA, actions: credit(10),
				conditions: `[{"type":"trigger","field":"amount","operator":"greater_than","value":200}]`}},
			properties: `{"amount":100}`,
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusNotMatched},
		},
		{
			id: "G04 amount >= 100 with 100.00 → matched",
			rules: []goldenRule{{id: ruleA, ver: verA, actions: credit(10),
				conditions: `[{"type":"trigger","field":"amount","operator":"greater_than_or_equal","value":100}]`}},
			properties: `{"amount":100.00}`,
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusMatched},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 10}},
		},
		{
			id: "G05 priority 20 runs before priority 10; total 30",
			rules: []goldenRule{
				{id: ruleA, ver: verA, priority: 10, actions: credit(10)},
				{id: ruleB, ver: verB, priority: 20, actions: credit(20)},
			},
			properties: `{}`,
			wantOrder:  []string{ruleB, ruleA}, wantStatus: []string{StatusMatched, StatusMatched},
			effects: []wantEffect{{ruleB, ActionCreditPoints, 20}, {ruleA, ActionCreditPoints, 10}},
		},
		{
			id: "G06 equal priority → creation order (uuidv7 id ASC)",
			rules: []goldenRule{
				{id: ruleC, ver: verC, priority: 5, actions: credit(5)},
				{id: ruleA, ver: verA, priority: 5, actions: credit(3)},
			},
			properties: `{}`,
			wantOrder:  []string{ruleA, ruleC}, wantStatus: []string{StatusMatched, StatusMatched},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 3}, {ruleC, ActionCreditPoints, 5}},
		},
		{
			id: "G09 amount equals \"100\" with 100 → documented numeric coercion",
			rules: []goldenRule{{id: ruleA, ver: verA, actions: credit(1),
				conditions: `[{"type":"trigger","field":"amount","operator":"equals","value":"100"}]`}},
			properties: `{"amount":100}`,
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusMatched},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 1}},
		},
		{
			id: "G10 coupon < 5 with coupon absent → false (PHP: null < 5 is true)",
			rules: []goldenRule{{id: ruleA, ver: verA, actions: credit(1),
				conditions: `[{"type":"trigger","field":"coupon","operator":"less_than","value":5}]`}},
			properties: `{}`,
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusNotMatched},
		},
		{
			id: "G12 tag in [a,b] with b → matched",
			rules: []goldenRule{{id: ruleA, ver: verA, actions: credit(1),
				conditions: `[{"type":"trigger","field":"tag","operator":"in","value":["a","b"]}]`}},
			properties: `{"tag":"b"}`,
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusMatched},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 1}},
		},
		{
			id: "G13 player.points >= 100 with balance 150 → matched",
			rules: []goldenRule{{id: ruleA, ver: verA, actions: credit(1),
				conditions: `[{"type":"player","field":"points","operator":"greater_than_or_equal","value":100}]`}},
			properties: `{}`,
			player:     &Player{ID: "p", Active: true, Points: 150, HasPoints: true},
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusMatched},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 1}},
		},
		{
			id: "G14 R1 credits 100, R2 needs points >= 100, balance 0 → R2 skipped (one pre-activity snapshot)",
			rules: []goldenRule{
				{id: ruleA, ver: verA, priority: 10, actions: credit(100)},
				{id: ruleB, ver: verB, priority: 5, actions: credit(1),
					conditions: `[{"type":"player","field":"points","operator":"gte","value":100}]`},
			},
			properties: `{}`,
			player:     &Player{ID: "p", Active: true, Points: 0, HasPoints: true},
			wantOrder:  []string{ruleA, ruleB}, wantStatus: []string{StatusMatched, StatusNotMatched},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 100}},
		},
		{
			id: "G15 player.level >= 2 with level 3 → matched (Laravel: always 0)",
			rules: []goldenRule{{id: ruleA, ver: verA, actions: credit(1),
				conditions: `[{"type":"player","field":"level","operator":"gte","value":2}]`}},
			properties: `{}`,
			player:     &Player{ID: "p", Active: true, Level: 3, HasProgress: true},
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusMatched},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 1}},
		},
		{
			id: "G16 award_badge rule matches; the rejection is settled later, other rules unaffected",
			rules: []goldenRule{
				{id: ruleA, ver: verA, priority: 9, actions: `[{"type":"award_badge","badge_id":"` + badge + `"}]`},
				{id: ruleB, ver: verB, priority: 1, actions: credit(7)},
			},
			properties: `{}`,
			wantOrder:  []string{ruleA, ruleB}, wantStatus: []string{StatusMatched, StatusMatched},
			effects: []wantEffect{{ruleA, ActionAwardBadge, 0}, {ruleB, ActionCreditPoints, 7}},
		},
		{
			id:         "G21 record_streak → one command per activity (same-day dedupe is the streaks module's)",
			rules:      []goldenRule{{id: ruleA, ver: verA, actions: `[{"type":"record_streak","activity_key":"daily_login"}]`}},
			properties: `{}`,
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusMatched},
			effects: []wantEffect{{ruleA, ActionRecordStreak, 0}},
		},
		{
			id:         "G27 grant_xp 500 → one progression.grant_xp 500 (level math is progression's)",
			rules:      []goldenRule{{id: ruleA, ver: verA, actions: `[{"type":"grant_xp","amount":500}]`}},
			properties: `{}`,
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusMatched},
			effects: []wantEffect{{ruleA, ActionGrantXP, 500}},
		},
		// Grammar v1 completion and v2 (doc 06 §11.5): stop_processing,
		// schedule, history. Stop is applied with "matched = fired"
		// (ApplyStop); the limited-stopper case is proven in internal/app.
		{
			id: "V01 stop_processing rule fires → lower-priority rules skipped_by_stop",
			rules: []goldenRule{
				{id: ruleA, ver: verA, priority: 10, actions: credit(10), stop: true},
				{id: ruleB, ver: verB, priority: 5, actions: credit(5)},
				{id: ruleC, ver: verC, priority: 1, actions: credit(1)},
			},
			properties: `{}`,
			wantOrder:  []string{ruleA, ruleB, ruleC},
			wantStatus: []string{StatusMatched, StatusSkippedByStop, StatusSkippedByStop},
			effects:    []wantEffect{{ruleA, ActionCreditPoints, 10}},
		},
		{
			id: "V02 stop_processing rule not matched → nothing stops",
			rules: []goldenRule{
				{id: ruleA, ver: verA, priority: 10, actions: credit(10), stop: true,
					conditions: `[{"source":"trigger","field":"amount","operator":"gt","value":1000}]`},
				{id: ruleB, ver: verB, priority: 5, actions: credit(5)},
			},
			properties: `{"amount":10}`,
			wantOrder:  []string{ruleA, ruleB}, wantStatus: []string{StatusNotMatched, StatusMatched},
			effects: []wantEffect{{ruleB, ActionCreditPoints, 5}},
		},
		{
			id: "V03 equal priority: stop rule with the lower id stops the other (id ASC tiebreak)",
			rules: []goldenRule{
				{id: ruleB, ver: verB, priority: 5, actions: credit(2), stop: true},
				{id: ruleA, ver: verA, priority: 5, actions: credit(1), stop: true},
			},
			properties: `{}`,
			wantOrder:  []string{ruleA, ruleB}, wantStatus: []string{StatusMatched, StatusSkippedByStop},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 1}},
		},
		{
			id: "V04 schedule outside (weekend only, activity on Monday) → out_of_schedule, does not stop",
			rules: []goldenRule{
				{id: ruleA, ver: verA, priority: 10, actions: credit(10), stop: true, schedule: `{"days_of_week":[0,6]}`},
				{id: ruleB, ver: verB, priority: 5, actions: credit(5),
					schedule: `{"starts_at":"2026-10-01T00:00:00Z","ends_at":"2026-11-01T00:00:00Z","hours":{"from":"09:00","to":"17:00"}}`},
			},
			properties: `{}`,
			wantOrder:  []string{ruleA, ruleB}, wantStatus: []string{StatusOutOfSchedule, StatusMatched},
			effects: []wantEffect{{ruleB, ActionCreditPoints, 5}},
		},
		{
			id: "V05 schedule timezone: 12:00 UTC is 16:00 in Tbilisi, outside 09:00-15:00 local",
			rules: []goldenRule{{id: ruleA, ver: verA, actions: credit(1),
				schedule: `{"hours":{"from":"09:00","to":"15:00"},"timezone":"Asia/Tbilisi"}`}},
			properties: `{}`,
			wantOrder:  []string{ruleA}, wantStatus: []string{StatusOutOfSchedule},
		},
		{
			id: "V06 history.first_time with no prior activity → matched; count(all) gte 1 → not matched",
			rules: []goldenRule{
				{id: ruleA, ver: verA, priority: 2, actions: credit(100),
					conditions: `[{"source":"history","field":"first_time"}]`},
				{id: ruleB, ver: verB, priority: 1, actions: credit(1),
					conditions: `[{"source":"history","field":"count","window":"all","operator":"gte","value":1}]`},
			},
			properties: `{}`,
			history:    &History{},
			wantOrder:  []string{ruleA, ruleB}, wantStatus: []string{StatusMatched, StatusNotMatched},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 100}},
		},
		{
			id: "V07 history.count of another event type in 7d (3rd login this week) and first_time false",
			rules: []goldenRule{
				{id: ruleA, ver: verA, priority: 2, actions: credit(30),
					conditions: `{"all":[{"source":"history","field":"count","event_type":"login","window":"7d","operator":"eq","value":2},` +
						`{"source":"history","field":"first_time","operator":"eq","value":false}]}`},
				{id: ruleB, ver: verB, priority: 1, actions: credit(1),
					conditions: `[{"source":"history","field":"first_time"}]`},
			},
			properties: `{}`,
			history: &History{Counts: map[string]map[string]int64{
				"login":              {Window7d: 2, WindowAll: 9},
				"purchase_completed": {WindowAll: 4},
			}},
			wantOrder: []string{ruleA, ruleB}, wantStatus: []string{StatusMatched, StatusNotMatched},
			effects: []wantEffect{{ruleA, ActionCreditPoints, 30}},
		},
		{
			id: "V08 history not loaded → history facts missing (only neq-style operators hold)",
			rules: []goldenRule{
				{id: ruleA, ver: verA, priority: 2, actions: credit(1),
					conditions: `[{"source":"history","field":"first_time"}]`},
				{id: ruleB, ver: verB, priority: 1, actions: credit(2),
					conditions: `[{"source":"history","field":"count","window":"1d","operator":"neq","value":3}]`},
			},
			properties: `{}`,
			wantOrder:  []string{ruleA, ruleB}, wantStatus: []string{StatusNotMatched, StatusMatched},
			effects: []wantEffect{{ruleB, ActionCreditPoints, 2}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			srcs := make([]RuleSource, len(tc.rules))
			for i, r := range tc.rules {
				srcs[i] = r.source()
				// every golden rule is also valid at write time
				_, err := CompileSpec(srcs[i].Spec(), Options{})
				require.NoError(t, err)
			}
			prog := Compile(srcs, Options{})
			res := Evaluate(prog, Facts{
				Activity: Activity{EventType: "purchase_completed", Properties: props(t, tc.properties), OccurredAt: goldenNow},
				Player:   tc.player,
				History:  tc.history,
			})
			res.ApplyStop()

			var order, status []string
			var effects []wantEffect
			for _, rr := range res.Rules {
				order = append(order, rr.RuleID)
				status = append(status, rr.Status)
				for _, a := range rr.Actions {
					effects = append(effects, wantEffect{rr.RuleID, a.Type, a.Amount})
				}
			}
			require.Equal(t, tc.wantOrder, order)
			require.Equal(t, tc.wantStatus, status)
			require.Equal(t, tc.effects, effects)
		})
	}
}

// G07, G08, G11, G17-G20: grammar Laravel silently accepted is a compile
// error at write time.
func TestGoldenCompileRejections(t *testing.T) {
	cases := []struct {
		id, conditions, actions, limits, field string
	}{
		{"G07 condition without type/source", `[{"field":"is_first_purchase","operator":"equals","value":true}]`, credit(1), ``, "conditions[0].source"},
		{"G08 operator minimum", `[{"type":"trigger","field":"amount","operator":"minimum","value":5}]`, credit(1), ``, "conditions[0].operator"},
		{"G11 contains empty needle", `[{"type":"trigger","field":"sku","operator":"contains","value":""}]`, credit(1), ``, "conditions[0].value"},
		{"G17 credit amount 0", ``, `[{"type":"credit_points","amount":0}]`, ``, "actions[0].amount"},
		{"G18 credit amount -5", ``, `[{"type":"credit_points","amount":-5}]`, ``, "actions[0].amount"},
		{"G19 credit amount 10.5", ``, `[{"type":"credit_points","amount":10.5}]`, ``, "actions[0].amount"},
		{"G19b credit amount string", ``, `[{"type":"credit_points","amount":"amount"}]`, ``, "actions[0].amount"},
		{"G20 unknown action type", ``, `[{"type":"emit_confetti"}]`, ``, "actions[0].type"},
		{"V20 history unknown field", `[{"source":"history","field":"sum"}]`, credit(1), ``, "conditions[0].field"},
		{"V21 history count without window", `[{"source":"history","field":"count","operator":"gt","value":1}]`, credit(1), ``, "conditions[0].window"},
		{"V22 history count bad window", `[{"source":"history","field":"count","window":"2w","operator":"gt","value":1}]`, credit(1), ``, "conditions[0].window"},
		{"V23 history count string value", `[{"source":"history","field":"count","window":"7d","operator":"gt","value":"1"}]`, credit(1), ``, "conditions[0].value"},
		{"V24 history count contains", `[{"source":"history","field":"count","window":"7d","operator":"contains","value":1}]`, credit(1), ``, "conditions[0].operator"},
		{"V25 history first_time gt", `[{"source":"history","field":"first_time","operator":"gt","value":true}]`, credit(1), ``, "conditions[0].operator"},
		{"V26 history first_time non-bool", `[{"source":"history","field":"first_time","value":1}]`, credit(1), ``, "conditions[0].value"},
		{"V27 history first_time with window", `[{"source":"history","field":"first_time","window":"7d"}]`, credit(1), ``, "conditions[0].window"},
		{"V28 window on a trigger leaf", `[{"source":"trigger","field":"a","operator":"eq","value":1,"window":"7d"}]`, credit(1), ``, "conditions[0].window"},
		{"V29 history bad event_type", `[{"source":"history","field":"count","event_type":"Bad Type","window":"7d","operator":"gt","value":1}]`, credit(1), ``, "conditions[0].event_type"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			_, err := CompileDefinition(json.RawMessage(tc.conditions), json.RawMessage(tc.actions), json.RawMessage(tc.limits), Options{})
			require.Error(t, err)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
			require.Equal(t, CodeInvalidDefinition, errs.CodeOf(err))
			require.Contains(t, errs.FieldsOf(err), tc.field)
		})
	}
}
