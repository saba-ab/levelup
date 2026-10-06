package eval

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

func compileSchedule1(t *testing.T, raw string) *Schedule {
	t.Helper()
	d, err := CompileSpec(Spec{Actions: json.RawMessage(credit(1)), Schedule: json.RawMessage(raw)}, Options{})
	require.NoError(t, err)
	return d.Schedule
}

func TestScheduleContains(t *testing.T) {
	mon := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) // Monday
	require.Nil(t, compileSchedule1(t, `null`))
	require.Nil(t, compileSchedule1(t, ``))
	var none *Schedule
	require.True(t, none.Contains(time.Time{}), "no schedule contains everything")

	cases := []struct {
		name     string
		schedule string
		at       time.Time
		want     bool
	}{
		{"empty object is always", `{}`, mon, true},
		{"zero time is outside a schedule", `{}`, time.Time{}, false},
		{"starts_at inclusive", `{"starts_at":"2026-10-05T12:00:00Z"}`, mon, true},
		{"before starts_at", `{"starts_at":"2026-10-05T12:00:01Z"}`, mon, false},
		{"ends_at exclusive", `{"ends_at":"2026-10-05T12:00:00Z"}`, mon, false},
		{"before ends_at", `{"ends_at":"2026-10-05T12:00:01Z"}`, mon, true},
		{"offset timestamps normalize", `{"starts_at":"2026-10-05T15:00:00+04:00"}`, mon, true},
		{"monday allowed", `{"days_of_week":[1]}`, mon, true},
		{"sunday only", `{"days_of_week":[0]}`, mon, false},
		{"weekday in timezone: Monday 23:30 UTC is Tuesday in Tbilisi", `{"days_of_week":[2],"timezone":"Asia/Tbilisi"}`,
			time.Date(2026, 10, 5, 23, 30, 0, 0, time.UTC), true},
		{"hours from inclusive", `{"hours":{"from":"12:00","to":"13:00"}}`, mon, true},
		{"hours to exclusive", `{"hours":{"from":"11:00","to":"12:00"}}`, mon, false},
		{"overnight window late", `{"hours":{"from":"22:00","to":"02:00"}}`, time.Date(2026, 10, 5, 23, 0, 0, 0, time.UTC), true},
		{"overnight window early", `{"hours":{"from":"22:00","to":"02:00"}}`, time.Date(2026, 10, 5, 1, 59, 0, 0, time.UTC), true},
		{"overnight window midday", `{"hours":{"from":"22:00","to":"02:00"}}`, mon, false},
		{"hours in timezone", `{"hours":{"from":"16:00","to":"17:00"},"timezone":"Asia/Tbilisi"}`, mon, true},
		{"nulls are ignored", `{"starts_at":null,"ends_at":null,"days_of_week":null,"hours":null,"timezone":null}`, mon, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, compileSchedule1(t, tc.schedule).Contains(tc.at))
		})
	}
}

func TestScheduleCompileErrors(t *testing.T) {
	cases := []struct{ schedule, field string }{
		{`[]`, "schedule"},
		{`{`, "schedule"},
		{`{"start":"x"}`, "schedule.start"},
		{`{"starts_at":"2026-10-05"}`, "schedule.starts_at"},
		{`{"ends_at":5}`, "schedule.ends_at"},
		{`{"starts_at":"2026-10-05T00:00:00Z","ends_at":"2026-10-05T00:00:00Z"}`, "schedule.ends_at"},
		{`{"days_of_week":[]}`, "schedule.days_of_week"},
		{`{"days_of_week":"mon"}`, "schedule.days_of_week"},
		{`{"days_of_week":[7]}`, "schedule.days_of_week[0]"},
		{`{"days_of_week":[1,-1]}`, "schedule.days_of_week[1]"},
		{`{"days_of_week":[1.5]}`, "schedule.days_of_week[0]"},
		{`{"hours":"9-17"}`, "schedule.hours"},
		{`{"hours":{"from":"9:00","to":"17:00"}}`, "schedule.hours.from"},
		{`{"hours":{"from":"09:00","to":"24:00"}}`, "schedule.hours.to"},
		{`{"hours":{"from":"09:60","to":"10:00"}}`, "schedule.hours.from"},
		{`{"hours":{"from":"+9:00","to":"10:00"}}`, "schedule.hours.from"},
		{`{"hours":{"from":"09:00"}}`, "schedule.hours.to"},
		{`{"hours":{"from":"09:00","to":"09:00"}}`, "schedule.hours.to"},
		{`{"hours":{"from":"09:00","to":"10:00","tz":"x"}}`, "schedule.hours.tz"},
		{`{"timezone":"Mars/Olympus"}`, "schedule.timezone"},
		{`{"timezone":"Local"}`, "schedule.timezone"},
		{`{"timezone":""}`, "schedule.timezone"},
		{`{"timezone":3}`, "schedule.timezone"},
	}
	for _, tc := range cases {
		t.Run(tc.schedule, func(t *testing.T) {
			_, err := CompileSpec(Spec{Actions: json.RawMessage(credit(1)), Schedule: json.RawMessage(tc.schedule)}, Options{})
			require.Error(t, err)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
			require.Equal(t, CodeInvalidDefinition, errs.CodeOf(err))
			require.Contains(t, errs.FieldsOf(err), tc.field)
		})
	}
}

// The service only knows after limits whether a matched rule fired: a
// limited stop rule must not stop anything, the next fired one does.
func TestStopGateFollowsFiringNotMatching(t *testing.T) {
	prog := Compile([]RuleSource{
		{RuleID: ruleA, RuleVersionID: verA, Priority: 3, Actions: json.RawMessage(credit(1)), StopProcessing: true},
		{RuleID: ruleB, RuleVersionID: verB, Priority: 2, Actions: json.RawMessage(credit(2)), StopProcessing: true},
		{RuleID: ruleC, RuleVersionID: verC, Priority: 1, Actions: json.RawMessage(credit(3)),
			Conditions: json.RawMessage(`[{"source":"trigger","field":"a","operator":"exists"}]`)},
	}, Options{})
	res := Evaluate(prog, Facts{Activity: Activity{EventType: "e", Properties: map[string]any{"a": true}}})
	for _, rr := range res.Rules {
		require.Equal(t, StatusMatched, rr.Status, "Evaluate never cuts the list")
	}

	limited := map[string]bool{ruleA: true}
	var g StopGate
	var statuses []string
	for _, rr := range res.Rules {
		if g.Admit(&rr) && !limited[rr.RuleID] {
			g.Fired(rr)
		}
		statuses = append(statuses, rr.Status)
		if rr.Status == StatusSkippedByStop {
			require.Equal(t, ruleB, rr.StoppedBy)
			require.Empty(t, rr.Trace)
			require.Empty(t, rr.Actions)
			require.False(t, rr.Matched)
		}
	}
	require.True(t, g.Stopped())
	require.Equal(t, []string{StatusMatched, StatusMatched, StatusSkippedByStop}, statuses)
	require.True(t, res.Rules[0].StopProcessing)
}

func TestHistoryEventTypesAndNeeds(t *testing.T) {
	plain := Compile([]RuleSource{{RuleID: ruleA, RuleVersionID: verA, Actions: json.RawMessage(credit(1))}}, Options{})
	require.False(t, plain.NeedsHistory())
	require.Nil(t, plain.HistoryEventTypes("purchase"))

	prog := Compile([]RuleSource{
		{RuleID: ruleA, RuleVersionID: verA, Actions: json.RawMessage(credit(1)),
			Conditions: json.RawMessage(`[{"source":"history","field":"first_time"}]`)},
		{RuleID: ruleB, RuleVersionID: verB, Actions: json.RawMessage(credit(1)),
			Conditions: json.RawMessage(`{"any":[{"source":"history","field":"count","event_type":"login","window":"30d","operator":"in","value":[1,2]},` +
				`{"source":"history","field":"count","event_type":"purchase","window":"all","operator":"lt","value":5}]}`)},
		{RuleID: ruleC, RuleVersionID: verC, Actions: json.RawMessage(credit(1)), // invalid: does not count
			Conditions: json.RawMessage(`[{"source":"history","field":"count","event_type":"zzz","window":"bad","operator":"gt","value":1}]`)},
	}, Options{})
	require.True(t, prog.NeedsHistory())
	require.Equal(t, []string{"login", "purchase"}, prog.HistoryEventTypes("purchase"))

	d, err := CompileSpec(Spec{Actions: json.RawMessage(credit(1)),
		Conditions: json.RawMessage(`[{"source":"history","field":"count","window":"1d","operator":"not_in","value":3}]`)}, Options{})
	require.NoError(t, err)
	require.True(t, d.NeedsHistory())

	f := Facts{Activity: Activity{EventType: "purchase"}, History: &History{Counts: map[string]map[string]int64{
		"purchase": {Window1d: 3},
	}}}
	require.False(t, matches(t, `[{"source":"history","field":"count","window":"1d","operator":"not_in","value":3}]`, f))
	require.True(t, matches(t, `[{"source":"history","field":"count","window":"7d","operator":"eq","value":0}]`, f),
		"an absent window counts zero")
	require.True(t, matches(t, `[{"source":"history","field":"first_time","operator":"neq","value":false}]`,
		Facts{Activity: Activity{EventType: "purchase"}, History: &History{}}))
	var nilHistory *History
	require.Zero(t, nilHistory.Count("x", WindowAll))
}

// FuzzCompileSpecV2: schedules and history leaves never panic, compile
// errors are Invalid, and evaluation is deterministic.
func FuzzCompileSpecV2(f *testing.F) {
	f.Add(`[{"source":"history","field":"count","window":"7d","operator":"gte","value":2}]`,
		`{"days_of_week":[1,2],"hours":{"from":"22:00","to":"02:00"},"timezone":"Asia/Tbilisi"}`, true, int64(1700000000), int64(3))
	f.Add(`[{"source":"history","field":"first_time"}]`, `{"starts_at":"2026-01-01T00:00:00Z"}`, false, int64(0), int64(0))
	f.Add(`[{"source":"history","field":"count","event_type":"x","window":"all","operator":"in","value":[1,"2"]}]`,
		`{"hours":{"from":"aa:bb"}}`, false, int64(-5), int64(-1))

	f.Fuzz(func(t *testing.T, conditions, schedule string, stop bool, unix, count int64) {
		spec := Spec{Conditions: json.RawMessage(conditions), Actions: json.RawMessage(credit(1)),
			Schedule: json.RawMessage(schedule), StopProcessing: stop}
		if _, err := CompileSpec(spec, Options{}); err != nil {
			require.Equal(t, errs.Invalid, errs.KindOf(err))
			return
		}
		prog := Compile([]RuleSource{{RuleID: "r", RuleVersionID: "v", Conditions: spec.Conditions,
			Actions: spec.Actions, Schedule: spec.Schedule, StopProcessing: stop}}, Options{})
		facts := Facts{
			Activity: Activity{EventType: "e", OccurredAt: time.Unix(unix, 0).UTC()},
			History: &History{Counts: map[string]map[string]int64{
				"e": {Window1d: count, Window7d: count, Window30d: count, Window90d: count, WindowAll: count},
				"x": {WindowAll: count},
			}},
		}
		first := Evaluate(prog, facts)
		require.Len(t, first.Rules, 1)
		require.NotEqual(t, StatusInvalid, first.Rules[0].Status)
		require.Equal(t, stop, first.Rules[0].StopProcessing)
		require.Equal(t, first, Evaluate(prog, facts))
	})
}
