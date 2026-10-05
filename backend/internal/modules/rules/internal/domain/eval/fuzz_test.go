package eval

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

// FuzzCompileEvaluate: any input either compiles or returns an Invalid
// compile error (never a panic, never another kind), and a compiled rule
// evaluates deterministically against arbitrary facts.
func FuzzCompileEvaluate(f *testing.F) {
	f.Add(`[{"type":"trigger","field":"amount","operator":"greater_than","value":200}]`, credit(5), `{"max_per_player":1}`, `{"amount":250}`)
	f.Add(`{"all":[{"source":"player","field":"level","operator":"in","value":[3,4]},{"not":{"source":"trigger","field":"coupon","operator":"exists"}}]}`,
		`[{"type":"grant_xp","amount":10}]`, ``, `{"coupon":null}`)
	f.Add(`{"any":[{"source":"activity","field":"properties.tags","operator":"contains","value":"x"}]}`,
		`[{"type":"record_streak","activity_key":"login"}]`, `null`, `{"tags":["x",1,{"a":2}]}`)
	f.Add(`[{"field":"a","operator":"minimum","value":1}]`, `[{"type":"credit_points","amount":10.5}]`, `{"cooldown_seconds":-1}`, `[]`)
	f.Add(`[{"source":"trigger","field":"n.0.k","operator":"lte","value":"9e999"}]`, credit(1), `{}`, `{"n":[{"k":"1e308"}]}`)

	f.Fuzz(func(t *testing.T, conditions, actions, limits, properties string) {
		def, err := CompileDefinition(json.RawMessage(conditions), json.RawMessage(actions), json.RawMessage(limits), Options{})
		if err != nil {
			require.Equal(t, errs.Invalid, errs.KindOf(err))
			return
		}
		require.NotEmpty(t, def.Actions)

		var propsMap map[string]any
		dec := json.NewDecoder(bytes.NewReader([]byte(properties)))
		dec.UseNumber()
		_ = dec.Decode(&propsMap)

		prog := Compile([]RuleSource{{RuleID: "r", RuleVersionID: "v",
			Conditions: json.RawMessage(conditions), Actions: json.RawMessage(actions), Limits: json.RawMessage(limits)}}, Options{})
		facts := Facts{
			Activity: Activity{EventType: "e", Properties: propsMap, Context: propsMap},
			Player: &Player{ID: "p", Attributes: propsMap, Level: 3, XP: 10, Points: 5,
				HasProgress: true, HasPoints: true, Active: true},
		}
		first := Evaluate(prog, facts)
		require.Len(t, first.Rules, 1)
		require.NotEqual(t, StatusInvalid, first.Rules[0].Status)
		require.Equal(t, first, Evaluate(prog, facts))
	})
}
