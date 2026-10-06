package domain

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

// decode mirrors how criteria arrive: from an HTTP body or a JSONB column.
func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

// props mirrors the activity subscriber: numbers stay json.Number.
func props(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&m))
	return m
}

func fieldKeys(err error) []string {
	f := errs.FieldsOf(err)
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestParseCriteriaValid(t *testing.T) {
	cases := map[string]struct {
		raw       string
		automatic bool
		by        string
	}{
		"empty is manual":    {`{}`, false, IncrementByCount},
		"event type only":    {`{"event_type": "purchase_completed"}`, true, IncrementByCount},
		"explicit count":     {`{"event_type": "login", "increment": {"by": "count"}}`, true, IncrementByCount},
		"property increment": {`{"event_type": "purchase", "increment": {"by": "property", "field": "amount"}}`, true, IncrementByProperty},
		"null where":         {`{"event_type": "login", "where": null}`, true, IncrementByCount},
		"every operator": {`{"event_type": "purchase", "where": [
			{"field": "a", "operator": "eq", "value": "x"},
			{"field": "b", "operator": "neq", "value": 1},
			{"field": "c", "operator": "gt", "value": 1},
			{"field": "c", "operator": "gte", "value": 1.5},
			{"field": "c", "operator": "lt", "value": 10},
			{"field": "c", "operator": "lte", "value": 10},
			{"field": "d", "operator": "in", "value": ["x", 2, true]},
			{"field": "e", "operator": "contains", "value": "sub"},
			{"field": "f.g", "operator": "exists"},
			{"field": "h", "operator": "exists", "value": false}
		]}`, true, IncrementByCount},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseCriteria(decode(t, c.raw))
			require.NoError(t, err)
			require.Equal(t, c.automatic, got.Automatic())
			require.Equal(t, c.by, got.Increment.By)
		})
	}
	got, err := ParseCriteria(nil)
	require.NoError(t, err)
	require.False(t, got.Automatic())
}

func TestParseCriteriaInvalid(t *testing.T) {
	cases := map[string]struct {
		raw  string
		keys []string
	}{
		"unknown key":                  {`{"event_type": "x", "min_amount": 3}`, []string{"criteria.min_amount"}},
		"event type not string":        {`{"event_type": 3}`, []string{"criteria.event_type"}},
		"event type blank":             {`{"event_type": ""}`, []string{"criteria.event_type"}},
		"event type uppercase":         {`{"event_type": "Purchase"}`, []string{"criteria.event_type"}},
		"event type padded":            {`{"event_type": " purchase "}`, []string{"criteria.event_type"}},
		"where without event type":     {`{"where": []}`, []string{"criteria.event_type"}},
		"increment without event type": {`{"increment": {"by": "count"}}`, []string{"criteria.event_type"}},
		"where not a list":             {`{"event_type": "x", "where": {}}`, []string{"criteria.where"}},
		"condition not object":         {`{"event_type": "x", "where": [1]}`, []string{"criteria.where[0]"}},
		"bad operator":                 {`{"event_type": "x", "where": [{"field": "a", "operator": "like", "value": 1}]}`, []string{"criteria.where[0].operator"}},
		"bad field":                    {`{"event_type": "x", "where": [{"field": "a..b", "operator": "eq", "value": 1}]}`, []string{"criteria.where[0].field"}},
		"missing field":                {`{"event_type": "x", "where": [{"operator": "eq", "value": 1}]}`, []string{"criteria.where[0].field"}},
		"gt needs number":              {`{"event_type": "x", "where": [{"field": "a", "operator": "gt", "value": "5"}]}`, []string{"criteria.where[0].value"}},
		"eq needs scalar":              {`{"event_type": "x", "where": [{"field": "a", "operator": "eq", "value": [1]}]}`, []string{"criteria.where[0].value"}},
		"eq needs a value":             {`{"event_type": "x", "where": [{"field": "a", "operator": "eq"}]}`, []string{"criteria.where[0].value"}},
		"in needs list":                {`{"event_type": "x", "where": [{"field": "a", "operator": "in", "value": "x"}]}`, []string{"criteria.where[0].value"}},
		"in empty":                     {`{"event_type": "x", "where": [{"field": "a", "operator": "in", "value": []}]}`, []string{"criteria.where[0].value"}},
		"in nested":                    {`{"event_type": "x", "where": [{"field": "a", "operator": "in", "value": [[1]]}]}`, []string{"criteria.where[0].value"}},
		"exists non bool":              {`{"event_type": "x", "where": [{"field": "a", "operator": "exists", "value": 1}]}`, []string{"criteria.where[0].value"}},
		"condition extra key":          {`{"event_type": "x", "where": [{"field": "a", "operator": "eq", "value": 1, "op": "x"}]}`, []string{"criteria.where[0].op"}},
		"increment not object":         {`{"event_type": "x", "increment": "count"}`, []string{"criteria.increment"}},
		"increment bad by":             {`{"event_type": "x", "increment": {"by": "sum"}}`, []string{"criteria.increment.by"}},
		"property needs field":         {`{"event_type": "x", "increment": {"by": "property"}}`, []string{"criteria.increment.field"}},
		"count takes no field":         {`{"event_type": "x", "increment": {"by": "count", "field": "a"}}`, []string{"criteria.increment.field"}},
		"several problems": {`{"event_type": "x", "where": [{"field": "", "operator": "nope"}], "increment": {"by": "x"}}`,
			[]string{"criteria.increment.by", "criteria.where[0].field", "criteria.where[0].operator"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseCriteria(decode(t, c.raw))
			require.Error(t, err)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
			require.Equal(t, CodeInvalidCriteria, errs.CodeOf(err))
			require.Equal(t, c.keys, fieldKeys(err))
		})
	}
}

func TestParseCriteriaLimits(t *testing.T) {
	where := make([]any, maxConditions+1)
	for i := range where {
		where[i] = map[string]any{"field": "a", "operator": "exists"}
	}
	_, err := ParseCriteria(map[string]any{"event_type": "x", "where": where})
	require.Equal(t, []string{"criteria.where"}, fieldKeys(err))
}

func TestCriteriaMatches(t *testing.T) {
	activity := `{"amount": 150, "currency": "USD", "tags": ["vip", "new"], "cart": {"total": "99.5", "items": 3},
		"note": "first purchase", "gift": false, "nothing": null}`
	cases := []struct {
		name  string
		where string
		want  bool
	}{
		{"no conditions", `[]`, true},
		{"eq string", `[{"field": "currency", "operator": "eq", "value": "USD"}]`, true},
		{"eq string mismatch", `[{"field": "currency", "operator": "eq", "value": "EUR"}]`, false},
		{"eq number", `[{"field": "amount", "operator": "eq", "value": 150}]`, true},
		{"eq number vs numeric string", `[{"field": "cart.total", "operator": "eq", "value": 99.5}]`, true},
		{"eq string never matches number", `[{"field": "amount", "operator": "eq", "value": "150"}]`, false},
		{"eq bool", `[{"field": "gift", "operator": "eq", "value": false}]`, true},
		{"neq", `[{"field": "currency", "operator": "neq", "value": "EUR"}]`, true},
		{"neq on missing field fails", `[{"field": "missing", "operator": "neq", "value": "EUR"}]`, false},
		{"gt", `[{"field": "amount", "operator": "gt", "value": 100}]`, true},
		{"gt boundary", `[{"field": "amount", "operator": "gt", "value": 150}]`, false},
		{"gte boundary", `[{"field": "amount", "operator": "gte", "value": 150}]`, true},
		{"lt", `[{"field": "cart.items", "operator": "lt", "value": 4}]`, true},
		{"lte nested numeric string", `[{"field": "cart.total", "operator": "lte", "value": 99.5}]`, true},
		{"gt on string fails", `[{"field": "currency", "operator": "gt", "value": 1}]`, false},
		{"in", `[{"field": "currency", "operator": "in", "value": ["EUR", "USD"]}]`, true},
		{"in numbers", `[{"field": "cart.items", "operator": "in", "value": [1, 3]}]`, true},
		{"in miss", `[{"field": "currency", "operator": "in", "value": ["EUR"]}]`, false},
		{"contains substring", `[{"field": "note", "operator": "contains", "value": "purchase"}]`, true},
		{"contains array element", `[{"field": "tags", "operator": "contains", "value": "vip"}]`, true},
		{"contains array miss", `[{"field": "tags", "operator": "contains", "value": "old"}]`, false},
		{"contains on number fails", `[{"field": "amount", "operator": "contains", "value": "1"}]`, false},
		{"exists", `[{"field": "cart.items", "operator": "exists"}]`, true},
		{"exists null is absent", `[{"field": "nothing", "operator": "exists"}]`, false},
		{"exists false on missing", `[{"field": "cart.coupon", "operator": "exists", "value": false}]`, true},
		{"path through scalar", `[{"field": "amount.x", "operator": "exists"}]`, false},
		{"all must hold", `[{"field": "currency", "operator": "eq", "value": "USD"}, {"field": "amount", "operator": "lt", "value": 100}]`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			crit, err := ParseCriteria(decode(t, `{"event_type": "purchase", "where": `+c.where+`}`))
			require.NoError(t, err)
			require.Equal(t, c.want, crit.Matches("purchase", props(t, activity)))
		})
	}

	crit, err := ParseCriteria(decode(t, `{"event_type": "purchase"}`))
	require.NoError(t, err)
	require.False(t, crit.Matches("refund", nil), "another event type never matches")
	require.True(t, crit.Matches("purchase", nil), "nil properties match a condition-free criteria")
	manual, err := ParseCriteria(map[string]any{})
	require.NoError(t, err)
	require.False(t, manual.Matches("purchase", nil), "manual criteria never match")
}

func TestCriteriaIncrementFor(t *testing.T) {
	count, err := ParseCriteria(decode(t, `{"event_type": "x"}`))
	require.NoError(t, err)
	n, ok := count.IncrementFor(nil)
	require.True(t, ok)
	require.EqualValues(t, 1, n)

	byAmount, err := ParseCriteria(decode(t, `{"event_type": "x", "increment": {"by": "property", "field": "order.qty"}}`))
	require.NoError(t, err)
	cases := []struct {
		props string
		want  int64
		ok    bool
	}{
		{`{"order": {"qty": 3}}`, 3, true},
		{`{"order": {"qty": 2.9}}`, 2, true},
		{`{"order": {"qty": "4"}}`, 4, true},
		{`{"order": {"qty": 0.5}}`, 0, false},
		{`{"order": {"qty": 0}}`, 0, false},
		{`{"order": {"qty": -2}}`, 0, false},
		{`{"order": {"qty": "many"}}`, 0, false},
		{`{"order": {"qty": true}}`, 0, false},
		{`{"order": {}}`, 0, false},
		{`{"order": {"qty": 1e30}}`, MaxPropertyIncrement, true},
	}
	for _, c := range cases {
		t.Run(c.props, func(t *testing.T) {
			n, ok := byAmount.IncrementFor(props(t, c.props))
			require.Equal(t, c.ok, ok)
			require.Equal(t, c.want, n)
		})
	}
}

func TestNewMissionRejectsInvalidCriteria(t *testing.T) {
	_, err := NewMission(NewMissionParams{
		TenantID: "t", Name: "M", Type: "repeating", Target: 1,
		Criteria: map[string]any{"event_type": "x", "where": "nope"},
	}, t0)
	require.Equal(t, CodeInvalidCriteria, errs.CodeOf(err))
	require.Equal(t, []string{"criteria.where"}, fieldKeys(err))
}
