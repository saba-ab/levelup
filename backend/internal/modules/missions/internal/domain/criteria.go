package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"levelup/internal/shared/errs"
)

// Criteria grammar (mission.criteria JSON):
//
//	{
//	  "event_type": "purchase_completed",              // required for automatic progress
//	  "where": [                                       // optional, all must hold
//	    {"field": "cart.total", "operator": "gte", "value": 100}
//	  ],
//	  "increment": {"by": "count"}                     // default
//	             | {"by": "property", "field": "quantity"}
//	}
//
// field is a dot path into the activity's properties. Operators: eq, neq,
// gt, gte, lt, lte, in, contains, exists. A missing field fails every
// condition except exists:false. An empty criteria object ({}) keeps the
// mission manual / rule-driven.
const (
	OpEq       = "eq"
	OpNeq      = "neq"
	OpGt       = "gt"
	OpGte      = "gte"
	OpLt       = "lt"
	OpLte      = "lte"
	OpIn       = "in"
	OpContains = "contains"
	OpExists   = "exists"

	IncrementByCount    = "count"
	IncrementByProperty = "property"

	maxConditions   = 20
	maxInValues     = 100
	maxFieldLength  = 200
	maxEventTypeLen = 100
	// MaxPropertyIncrement caps one property-driven increment so progress
	// arithmetic can never overflow.
	MaxPropertyIncrement int64 = 1_000_000_000_000
)

// CodeInvalidCriteria is the problem code of a criteria validation failure.
const CodeInvalidCriteria = "invalid_mission_criteria"

var (
	eventTypeRe = regexp.MustCompile(`^[a-z0-9_.:-]+$`)
	fieldPathRe = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`)
)

// Criteria is the parsed, validated criteria of a mission.
type Criteria struct {
	EventType string
	Where     []Condition
	Increment Increment
}

// Condition is one where clause.
type Condition struct {
	Field    string
	Operator string
	Value    any
	path     []string
}

// Increment says how much one matching activity progresses the attempt.
type Increment struct {
	By    string // IncrementByCount | IncrementByProperty
	Field string // IncrementByProperty only
	path  []string
}

// Automatic reports whether the criteria drive progress from activities.
func (c Criteria) Automatic() bool { return c.EventType != "" }

// ParseCriteria validates raw criteria. An empty map parses to manual
// criteria. Every problem is reported under a "criteria.*" field key with
// code invalid_mission_criteria (422).
func ParseCriteria(raw map[string]any) (Criteria, error) {
	c := Criteria{Increment: Increment{By: IncrementByCount}}
	if len(raw) == 0 {
		return c, nil
	}
	fields := map[string]string{}
	for k := range raw {
		switch k {
		case "event_type", "where", "increment":
		default:
			fields["criteria."+k] = "unknown criteria key (allowed: event_type, where, increment)"
		}
	}

	if v, ok := raw["event_type"]; ok {
		s, isStr := v.(string)
		switch {
		case !isStr || s == "":
			fields["criteria.event_type"] = "must be a non-empty string"
		case len(s) > maxEventTypeLen || !eventTypeRe.MatchString(s):
			fields["criteria.event_type"] = "must be an event type slug (a-z, 0-9, _ . : -), at most 100 characters"
		default:
			c.EventType = s
		}
	} else if raw["where"] != nil || raw["increment"] != nil {
		fields["criteria.event_type"] = "is required when where or increment is set"
	}

	if v, ok := raw["where"]; ok && v != nil {
		list, isList := v.([]any)
		switch {
		case !isList:
			fields["criteria.where"] = "must be an array of conditions"
		case len(list) > maxConditions:
			fields["criteria.where"] = fmt.Sprintf("at most %d conditions", maxConditions)
		default:
			for i, item := range list {
				cond, ok := parseCondition(item, fmt.Sprintf("criteria.where[%d]", i), fields)
				if ok {
					c.Where = append(c.Where, cond)
				}
			}
		}
	}

	if v, ok := raw["increment"]; ok && v != nil {
		if inc, ok := parseIncrement(v, fields); ok {
			c.Increment = inc
		}
	}

	if len(fields) > 0 {
		return Criteria{}, errs.WithCode(errs.WithFields(errs.New(errs.Invalid, "invalid mission criteria"), fields), CodeInvalidCriteria)
	}
	return c, nil
}

func parseCondition(item any, at string, fields map[string]string) (Condition, bool) {
	obj, ok := item.(map[string]any)
	if !ok {
		fields[at] = "must be an object with field, operator and value"
		return Condition{}, false
	}
	valid := true
	for k := range obj {
		if k != "field" && k != "operator" && k != "value" {
			fields[at+"."+k] = "unknown condition key (allowed: field, operator, value)"
			valid = false
		}
	}
	field, _ := obj["field"].(string)
	if !validPath(field) {
		fields[at+".field"] = "must be a dot path into properties (letters, digits, _ and -), at most 200 characters"
		valid = false
	}
	op, _ := obj["operator"].(string)
	value, hasValue := obj["value"]
	switch op {
	case OpEq, OpNeq, OpContains:
		if !hasValue || !isScalar(value) {
			fields[at+".value"] = "must be a string, number or boolean"
			valid = false
		}
	case OpGt, OpGte, OpLt, OpLte:
		if !hasValue || !isNumber(value) {
			fields[at+".value"] = "must be a number"
			valid = false
		}
	case OpIn:
		list, isList := value.([]any)
		switch {
		case !isList || len(list) == 0:
			fields[at+".value"] = "must be a non-empty array"
			valid = false
		case len(list) > maxInValues:
			fields[at+".value"] = fmt.Sprintf("at most %d values", maxInValues)
			valid = false
		default:
			for _, x := range list {
				if !isScalar(x) {
					fields[at+".value"] = "every value must be a string, number or boolean"
					valid = false
					break
				}
			}
		}
	case OpExists:
		if hasValue && value != nil {
			if _, isBool := value.(bool); !isBool {
				fields[at+".value"] = "must be a boolean (default true)"
				valid = false
			}
		}
	default:
		fields[at+".operator"] = "must be one of eq, neq, gt, gte, lt, lte, in, contains, exists"
		valid = false
	}
	if !valid {
		return Condition{}, false
	}
	if op == OpExists && (value == nil || !hasValue) {
		value = true
	}
	return Condition{Field: field, Operator: op, Value: value, path: strings.Split(field, ".")}, true
}

func parseIncrement(v any, fields map[string]string) (Increment, bool) {
	obj, ok := v.(map[string]any)
	if !ok {
		fields["criteria.increment"] = `must be {"by": "count"} or {"by": "property", "field": "..."}`
		return Increment{}, false
	}
	valid := true
	for k := range obj {
		if k != "by" && k != "field" {
			fields["criteria.increment."+k] = "unknown increment key (allowed: by, field)"
			valid = false
		}
	}
	by, _ := obj["by"].(string)
	field, hasField := obj["field"]
	switch by {
	case IncrementByCount:
		if hasField {
			fields["criteria.increment.field"] = `is only allowed with "by": "property"`
			valid = false
		}
		if valid {
			return Increment{By: IncrementByCount}, true
		}
	case IncrementByProperty:
		f, _ := field.(string)
		if !validPath(f) {
			fields["criteria.increment.field"] = "must be a dot path into properties (letters, digits, _ and -), at most 200 characters"
			valid = false
		}
		if valid {
			return Increment{By: IncrementByProperty, Field: f, path: strings.Split(f, ".")}, true
		}
	default:
		fields["criteria.increment.by"] = "must be count or property"
	}
	return Increment{}, false
}

// Matches reports whether an activity of eventType with properties
// satisfies the criteria. Manual criteria never match.
func (c Criteria) Matches(eventType string, props map[string]any) bool {
	if !c.Automatic() || eventType != c.EventType {
		return false
	}
	for _, cond := range c.Where {
		if !cond.holds(props) {
			return false
		}
	}
	return true
}

// IncrementFor returns how far one matching activity progresses the
// attempt; ok=false when a property increment is missing, not a number, or
// below 1 after flooring (that activity does not progress the mission).
func (c Criteria) IncrementFor(props map[string]any) (int64, bool) {
	if c.Increment.By != IncrementByProperty {
		return 1, true
	}
	raw, found := lookup(props, c.Increment.path)
	if !found {
		return 0, false
	}
	f, ok := toNumber(raw)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	f = math.Floor(f)
	if f < 1 {
		return 0, false
	}
	if f >= float64(MaxPropertyIncrement) {
		return MaxPropertyIncrement, true
	}
	return int64(f), true
}

func (cond Condition) holds(props map[string]any) bool {
	got, found := lookup(props, cond.path)
	if found && got == nil {
		found = false
	}
	if cond.Operator == OpExists {
		want, _ := cond.Value.(bool)
		return found == want
	}
	if !found {
		return false
	}
	switch cond.Operator {
	case OpEq:
		return equal(got, cond.Value)
	case OpNeq:
		return !equal(got, cond.Value)
	case OpGt, OpGte, OpLt, OpLte:
		a, ok := toNumber(got)
		b, okB := toNumber(cond.Value)
		if !ok || !okB {
			return false
		}
		switch cond.Operator {
		case OpGt:
			return a > b
		case OpGte:
			return a >= b
		case OpLt:
			return a < b
		default:
			return a <= b
		}
	case OpIn:
		list, _ := cond.Value.([]any)
		for _, x := range list {
			if equal(got, x) {
				return true
			}
		}
		return false
	case OpContains:
		switch g := got.(type) {
		case string:
			want, ok := cond.Value.(string)
			return ok && strings.Contains(g, want)
		case []any:
			for _, x := range g {
				if equal(x, cond.Value) {
					return true
				}
			}
		}
		return false
	}
	return false
}

// lookup walks a dot path through nested objects.
func lookup(props map[string]any, path []string) (any, bool) {
	var cur any = props
	for _, seg := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// equal compares a property with a criteria value: numerically when the
// criteria value is a number (numeric strings in properties count),
// otherwise by type and value.
func equal(got, want any) bool {
	if isNumber(want) {
		a, ok := toNumber(got)
		b, _ := toNumber(want)
		return ok && a == b
	}
	switch w := want.(type) {
	case string:
		g, ok := got.(string)
		return ok && g == w
	case bool:
		g, ok := got.(bool)
		return ok && g == w
	}
	return false
}

func isScalar(v any) bool {
	switch v.(type) {
	case string, bool:
		return true
	}
	return isNumber(v)
}

func isNumber(v any) bool {
	switch v.(type) {
	case float64, float32, int, int32, int64, json.Number:
		return true
	}
	return false
}

// toNumber converts JSON numbers (float64 or json.Number), Go ints, and
// numeric strings.
func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	}
	return 0, false
}

func validPath(p string) bool {
	return p != "" && len(p) <= maxFieldLength && fieldPathRe.MatchString(p)
}
