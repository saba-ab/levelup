package domain

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

// Match modes of a group.
const (
	MatchAll = "all"
	MatchAny = "any"
)

// Fields a condition may test. Attribute conditions use the prefix
// "attributes." followed by a dot-separated path into the player's
// attributes.
const (
	FieldAttributesPrefix = "attributes."
	FieldIsActive         = "is_active"
	FieldCreatedAt        = "created_at"
	FieldLevel            = "level"
	FieldBalance          = "balance"
	FieldLifetimeEarned   = "lifetime_earned"
	FieldBadgesEarned     = "badges_earned"
	FieldLastSeenDays     = "last_seen_days"
)

// Operators.
const (
	OpEq        = "eq"
	OpNeq       = "neq"
	OpGt        = "gt"
	OpGte       = "gte"
	OpLt        = "lt"
	OpLte       = "lte"
	OpIn        = "in"
	OpNotIn     = "not_in"
	OpContains  = "contains"
	OpExists    = "exists"
	OpNotExists = "not_exists"
	OpBefore    = "before"
	OpAfter     = "after"
	OpHas       = "has"
	OpNotHas    = "not_has"
)

const (
	MaxConditions = 50
	MaxDepth      = 3
)

var (
	numericOps   = set(OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte)
	attributeOps = set(OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpIn, OpNotIn, OpContains, OpExists, OpNotExists)
	attrPathRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}(\.[A-Za-z0-9_-]{1,64}){0,4}$`)
)

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// Group combines nodes with all (AND) or any (OR).
type Group struct {
	Match string
	Nodes []Node
}

// Node is exactly one of a condition or a nested group.
type Node struct {
	Cond  *Condition
	Group *Group
}

// Condition tests one field. Value keeps the JSON-decoded value as given;
// num, at and path are its parsed forms.
type Condition struct {
	Field string
	Op    string
	Value any

	num  float64
	at   time.Time
	path []string
}

// Needs names the data sources a set of conditions reads beyond the player
// itself, so a refresh only calls the readers it needs.
type Needs struct {
	Level    bool
	Wallet   bool
	Badges   bool
	LastSeen bool
}

// ParseConditions validates the JSON-decoded {"all"|"any": [...]} document.
func ParseConditions(raw map[string]any) (Group, error) {
	count := 0
	return parseGroup(raw, "", 1, &count)
}

func parseGroup(raw map[string]any, path string, depth int, count *int) (Group, error) {
	if depth > MaxDepth {
		return Group{}, conditionsError(path, fmt.Sprintf("groups may nest at most %d deep", MaxDepth))
	}
	if len(raw) != 1 {
		return Group{}, conditionsError(path, `a group must have exactly one key, "all" or "any"`)
	}
	var (
		match string
		items any
	)
	for k, v := range raw {
		match, items = k, v
	}
	if match != MatchAll && match != MatchAny {
		return Group{}, conditionsError(path, `a group must have exactly one key, "all" or "any"`)
	}
	list, ok := items.([]any)
	if !ok || len(list) == 0 {
		return Group{}, conditionsError(path, match+" must be a non-empty array")
	}
	g := Group{Match: match, Nodes: make([]Node, 0, len(list))}
	for i, item := range list {
		p := fmt.Sprintf("%s[%d]", match, i)
		if path != "" {
			p = path + "." + p
		}
		obj, ok := item.(map[string]any)
		if !ok {
			return Group{}, conditionsError(p, "must be an object")
		}
		_, hasAll := obj[MatchAll]
		_, hasAny := obj[MatchAny]
		if hasAll || hasAny {
			sub, err := parseGroup(obj, p, depth+1, count)
			if err != nil {
				return Group{}, err
			}
			g.Nodes = append(g.Nodes, Node{Group: &sub})
			continue
		}
		*count++
		if *count > MaxConditions {
			return Group{}, conditionsError(p, fmt.Sprintf("at most %d conditions", MaxConditions))
		}
		c, err := parseCondition(obj, p)
		if err != nil {
			return Group{}, err
		}
		g.Nodes = append(g.Nodes, Node{Cond: &c})
	}
	return g, nil
}

func parseCondition(obj map[string]any, path string) (Condition, error) {
	for k := range obj {
		if k != "field" && k != "op" && k != "value" {
			return Condition{}, conditionsError(path, "unknown key "+k)
		}
	}
	field, _ := obj["field"].(string)
	op, _ := obj["op"].(string)
	value, hasValue := obj["value"]
	c := Condition{Field: field, Op: op, Value: value}
	bad := func(msg string) (Condition, error) { return Condition{}, conditionsError(path, msg) }

	switch {
	case strings.HasPrefix(field, FieldAttributesPrefix):
		p := strings.TrimPrefix(field, FieldAttributesPrefix)
		if !attrPathRe.MatchString(p) {
			return bad("attribute path must be 1-5 dot-separated segments of A-Z, a-z, 0-9, '_' or '-'")
		}
		c.path = strings.Split(p, ".")
		if !attributeOps[op] {
			return bad("unsupported operator " + op + " for attributes")
		}
		switch op {
		case OpExists, OpNotExists:
			if hasValue {
				return bad(op + " takes no value")
			}
		case OpIn, OpNotIn:
			list, ok := value.([]any)
			if !ok || len(list) == 0 || len(list) > 100 {
				return bad(op + " needs an array of 1-100 values")
			}
		case OpGt, OpGte, OpLt, OpLte:
			if _, ok := toNumber(value); !ok {
				if _, ok := value.(string); !ok {
					return bad(op + " needs a number or a string")
				}
			}
		default:
			if !hasValue || !scalar(value) {
				return bad(op + " needs a string, number or boolean value")
			}
		}
	case field == FieldIsActive:
		if op != OpEq && op != OpNeq {
			return bad("is_active supports eq and neq")
		}
		if _, ok := value.(bool); !ok {
			return bad("is_active needs a boolean value")
		}
	case field == FieldCreatedAt:
		if op != OpBefore && op != OpAfter {
			return bad("created_at supports before and after")
		}
		s, _ := value.(string)
		at, ok := parseTime(s)
		if !ok {
			return bad("created_at needs an RFC 3339 time or a YYYY-MM-DD date")
		}
		c.at = at
	case field == FieldLevel, field == FieldBalance, field == FieldLifetimeEarned, field == FieldLastSeenDays:
		if !numericOps[op] {
			return bad("unsupported operator " + op + " for " + field)
		}
		n, ok := toNumber(value)
		if !ok {
			return bad(field + " needs a numeric value")
		}
		c.num = n
	case field == FieldBadgesEarned:
		switch {
		case op == OpHas || op == OpNotHas:
			if s, _ := value.(string); s == "" {
				return bad(op + " needs a badge id")
			}
		case numericOps[op]:
			n, ok := toNumber(value)
			if !ok {
				return bad("badges_earned needs a numeric value")
			}
			c.num = n
		default:
			return bad("unsupported operator " + op + " for badges_earned")
		}
	default:
		return bad("unknown field " + field)
	}
	return c, nil
}

func parseTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func scalar(v any) bool {
	switch v.(type) {
	case string, bool:
		return true
	}
	_, ok := toNumber(v)
	return ok
}

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
	case interface{ Float64() (float64, error) }: // json.Number
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// Needs reports which readers evaluating g requires.
func (g Group) Needs() Needs {
	var n Needs
	g.walk(func(c *Condition) {
		switch c.Field {
		case FieldLevel:
			n.Level = true
		case FieldBalance, FieldLifetimeEarned:
			n.Wallet = true
		case FieldBadgesEarned:
			n.Badges = true
		case FieldLastSeenDays:
			n.LastSeen = true
		}
	})
	return n
}

func (g Group) walk(fn func(*Condition)) {
	for _, n := range g.Nodes {
		if n.Cond != nil {
			fn(n.Cond)
		}
		if n.Group != nil {
			n.Group.walk(fn)
		}
	}
}

// ToMap renders g back to its JSON document shape.
func (g Group) ToMap() map[string]any {
	items := make([]any, len(g.Nodes))
	for i, n := range g.Nodes {
		if n.Group != nil {
			items[i] = n.Group.ToMap()
			continue
		}
		m := map[string]any{"field": n.Cond.Field, "op": n.Cond.Op}
		if n.Cond.Op != OpExists && n.Cond.Op != OpNotExists {
			m["value"] = n.Cond.Value
		}
		items[i] = m
	}
	return map[string]any{g.Match: items}
}

// PlayerFacts is everything a condition may read about one player.
type PlayerFacts struct {
	Active         bool
	CreatedAt      time.Time
	Attributes     map[string]any
	Level          int
	Balance        int64
	LifetimeEarned int64
	Badges         map[string]bool // badge ids earned at least once
	LastSeen       *time.Time      // nil: never seen
}

// Matches evaluates g against one player at now.
func (g Group) Matches(f PlayerFacts, now time.Time) bool {
	if g.Match == MatchAny {
		for _, n := range g.Nodes {
			if n.matches(f, now) {
				return true
			}
		}
		return false
	}
	for _, n := range g.Nodes {
		if !n.matches(f, now) {
			return false
		}
	}
	return len(g.Nodes) > 0
}

func (n Node) matches(f PlayerFacts, now time.Time) bool {
	if n.Group != nil {
		return n.Group.Matches(f, now)
	}
	return n.Cond != nil && n.Cond.matches(f, now)
}

func (c Condition) matches(f PlayerFacts, now time.Time) bool {
	switch c.Field {
	case FieldIsActive:
		want, _ := c.Value.(bool)
		return (f.Active == want) == (c.Op == OpEq)
	case FieldCreatedAt:
		if c.Op == OpBefore {
			return f.CreatedAt.Before(c.at)
		}
		return f.CreatedAt.After(c.at)
	case FieldLevel:
		return compareNum(float64(f.Level), c.Op, c.num)
	case FieldBalance:
		return compareNum(float64(f.Balance), c.Op, c.num)
	case FieldLifetimeEarned:
		return compareNum(float64(f.LifetimeEarned), c.Op, c.num)
	case FieldBadgesEarned:
		switch c.Op {
		case OpHas:
			return f.Badges[c.Value.(string)]
		case OpNotHas:
			return !f.Badges[c.Value.(string)]
		}
		return compareNum(float64(len(f.Badges)), c.Op, c.num)
	case FieldLastSeenDays:
		days := math.Inf(1) // never seen is infinitely long ago
		if f.LastSeen != nil {
			days = math.Floor(now.Sub(*f.LastSeen).Hours() / 24)
		}
		return compareNum(days, c.Op, c.num)
	}
	return c.matchesAttribute(f.Attributes)
}

func (c Condition) matchesAttribute(attrs map[string]any) bool {
	v, found := lookup(attrs, c.path)
	switch c.Op {
	case OpExists:
		return found
	case OpNotExists:
		return !found
	case OpNeq:
		return !found || !equal(v, c.Value)
	case OpNotIn:
		return !found || !inList(v, c.Value)
	}
	if !found {
		return false
	}
	switch c.Op {
	case OpEq:
		return equal(v, c.Value)
	case OpIn:
		return inList(v, c.Value)
	case OpContains:
		switch hay := v.(type) {
		case string:
			needle, ok := c.Value.(string)
			return ok && strings.Contains(hay, needle)
		case []any:
			for _, x := range hay {
				if equal(x, c.Value) {
					return true
				}
			}
		}
		return false
	}
	// Ordering: numeric when both are numbers, lexical when both are strings
	// (ISO dates compare correctly).
	if a, ok := toNumber(v); ok {
		if b, ok := toNumber(c.Value); ok {
			return compareNum(a, c.Op, b)
		}
		return false
	}
	as, ok1 := v.(string)
	bs, ok2 := c.Value.(string)
	if !ok1 || !ok2 {
		return false
	}
	return compareNum(float64(strings.Compare(as, bs)), c.Op, 0)
}

func lookup(attrs map[string]any, path []string) (any, bool) {
	var cur any = attrs
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[p]; !ok {
			return nil, false
		}
	}
	return cur, cur != nil
}

func equal(a, b any) bool {
	if x, ok := toNumber(a); ok {
		y, ok := toNumber(b)
		return ok && x == y
	}
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	}
	return false
}

func inList(v, list any) bool {
	items, _ := list.([]any)
	for _, x := range items {
		if equal(v, x) {
			return true
		}
	}
	return false
}

func compareNum(a float64, op string, b float64) bool {
	switch op {
	case OpEq:
		return a == b
	case OpNeq:
		return a != b
	case OpGt:
		return a > b
	case OpGte:
		return a >= b
	case OpLt:
		return a < b
	case OpLte:
		return a <= b
	}
	return false
}
