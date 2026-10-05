package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"levelup/internal/shared/errs"
)

// CodeInvalidDefinition is the problem code of every compile error.
const CodeInvalidDefinition = "invalid_rule_definition"

// Options bounds a compile. Zero values take the defaults.
type Options struct {
	MaxActions    int
	MaxConditions int
}

func (o Options) withDefaults() Options {
	if o.MaxActions <= 0 {
		o.MaxActions = DefaultMaxActions
	}
	if o.MaxConditions <= 0 {
		o.MaxConditions = DefaultMaxConditions
	}
	return o
}

// Action is one compiled, fully validated action.
type Action struct {
	Type        string
	Amount      int64  // credit_points, grant_xp
	Description string // credit_points, grant_xp
	BadgeID     string // award_badge
	StreakID    string // record_streak
	ActivityKey string // record_streak
	MissionID   string // progress_mission
	Increment   int64  // progress_mission
	RewardID    string // grant_reward
}

// Params is the action's canonical parameter map (stored on the effect row
// and published in rules.decision_made.v1).
func (a Action) Params() map[string]any {
	p := map[string]any{}
	switch a.Type {
	case ActionCreditPoints, ActionGrantXP:
		p["amount"] = a.Amount
		if a.Description != "" {
			p["description"] = a.Description
		}
	case ActionAwardBadge:
		p["badge_id"] = a.BadgeID
	case ActionRecordStreak:
		if a.StreakID != "" {
			p["streak_id"] = a.StreakID
		}
		if a.ActivityKey != "" {
			p["activity_key"] = a.ActivityKey
		}
	case ActionProgressMission:
		p["mission_id"] = a.MissionID
		p["increment"] = a.Increment
	case ActionGrantReward:
		p["reward_id"] = a.RewardID
	}
	return p
}

// Limits caps how often a rule fires for one player.
type Limits struct {
	MaxPerPlayer        int64
	MaxPerPlayerPerDay  int64
	MaxPerPlayerPerWeek int64
	CooldownSeconds     int64
}

// IsZero reports a rule without limits.
func (l Limits) IsZero() bool { return l == Limits{} }

// Definition is one compiled rule body.
type Definition struct {
	cond          *node
	Actions       []Action
	Limits        Limits
	needsProgress bool
	needsPoints   bool
	needsPlayer   bool
}

// NeedsPlayer reports whether any condition reads player.*.
func (d *Definition) NeedsPlayer() bool { return d.needsPlayer }

type nodeKind uint8

const (
	nAll nodeKind = iota
	nAny
	nNot
	nLeaf
)

type node struct {
	kind     nodeKind
	path     string
	children []*node
	leaf     *leaf
}

type leaf struct {
	source   string
	field    string
	segs     []string
	pf       playerField
	af       activityField
	op       string
	val      value
	list     []value
	rawValue any
}

// fieldErrors accumulates compile errors keyed by location.
type fieldErrors map[string]string

func (f fieldErrors) add(path, msg string) {
	if _, ok := f[path]; !ok {
		f[path] = msg
	}
}

func (f fieldErrors) err() error {
	if len(f) == 0 {
		return nil
	}
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	msg := "invalid rule definition: " + keys[0] + " " + f[keys[0]]
	return errs.WithCode(errs.WithFields(errs.New(errs.Invalid, msg), f), CodeInvalidDefinition)
}

// CompileDefinition validates and compiles one rule body. It is the write-time
// gate (create, PATCH of a draft, new version, publish).
func CompileDefinition(conditions, actions, limits json.RawMessage, opts Options) (*Definition, error) {
	opts = opts.withDefaults()
	fe := fieldErrors{}
	d := &Definition{}

	c := &condCompiler{fe: fe, max: opts.MaxConditions, def: d}
	d.cond = c.compileRoot(conditions)
	d.Actions = compileActions(actions, opts.MaxActions, fe)
	d.Limits = compileLimits(limits, fe)

	if err := fe.err(); err != nil {
		return nil, err
	}
	return d, nil
}

// decode parses raw JSON keeping numbers exact. Empty input decodes to nil.
func decode(raw json.RawMessage) (any, bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, false, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, true, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, true, fmt.Errorf("trailing data after JSON value")
	}
	return v, true, nil
}

type condCompiler struct {
	fe     fieldErrors
	max    int
	leaves int
	def    *Definition
}

func (c *condCompiler) compileRoot(raw json.RawMessage) *node {
	v, present, err := decode(raw)
	if err != nil {
		c.fe.add("conditions", "is not valid JSON")
		return nil
	}
	if !present || v == nil {
		return nil // absent or null: always true (Laravel parity)
	}
	switch t := v.(type) {
	case []any:
		if len(t) == 0 {
			return nil
		}
		return c.group(nAll, t, "conditions", 1)
	case map[string]any:
		return c.compileNode(t, "conditions", 1)
	default:
		c.fe.add("conditions", "must be a list or an object")
		return nil
	}
}

func (c *condCompiler) group(k nodeKind, items []any, path string, depth int) *node {
	if depth > MaxNestingDepth {
		c.fe.add(path, fmt.Sprintf("nesting deeper than %d levels", MaxNestingDepth))
		return nil
	}
	n := &node{kind: k, path: path, children: make([]*node, 0, len(items))}
	for i, it := range items {
		p := fmt.Sprintf("%s[%d]", path, i)
		obj, ok := it.(map[string]any)
		if !ok {
			c.fe.add(p, "must be an object")
			continue
		}
		if ch := c.compileNode(obj, p, depth); ch != nil {
			n.children = append(n.children, ch)
		}
	}
	return n
}

func (c *condCompiler) compileNode(obj map[string]any, path string, depth int) *node {
	for _, comb := range []string{"all", "any", "not"} {
		inner, ok := obj[comb]
		if !ok {
			continue
		}
		if len(obj) != 1 {
			c.fe.add(path, fmt.Sprintf("a %q group must not have sibling keys", comb))
			return nil
		}
		p := path + "." + comb
		switch comb {
		case "all", "any":
			items, ok := inner.([]any)
			if !ok || len(items) == 0 {
				c.fe.add(p, "must be a non-empty list of conditions")
				return nil
			}
			k := nAll
			if comb == "any" {
				k = nAny
			}
			return c.group(k, items, p, depth+1)
		default:
			if depth+1 > MaxNestingDepth {
				c.fe.add(p, fmt.Sprintf("nesting deeper than %d levels", MaxNestingDepth))
				return nil
			}
			child, ok := inner.(map[string]any)
			if !ok {
				c.fe.add(p, "must be a condition object")
				return nil
			}
			ch := c.compileNode(child, p, depth+1)
			if ch == nil {
				return nil
			}
			return &node{kind: nNot, path: p, children: []*node{ch}}
		}
	}
	return c.compileLeaf(obj, path)
}

var segmentRe = regexp.MustCompile(`^[A-Za-z0-9_\-]+$`)

func splitPath(field string) ([]string, bool) {
	if field == "" {
		return nil, false
	}
	segs := strings.Split(field, ".")
	if len(segs) > MaxPathSegments {
		return nil, false
	}
	for _, s := range segs {
		if !segmentRe.MatchString(s) {
			return nil, false
		}
	}
	return segs, true
}

func (c *condCompiler) compileLeaf(obj map[string]any, path string) *node {
	c.leaves++
	if c.leaves == c.max+1 {
		c.fe.add("conditions", fmt.Sprintf("more than %d conditions", c.max))
	}
	for k := range obj {
		switch k {
		case "source", "type", "field", "operator", "op", "value":
		default:
			c.fe.add(path+"."+k, "unknown key")
		}
	}
	str := func(keys ...string) (string, string, bool) {
		var found, key string
		for _, k := range keys {
			raw, ok := obj[k]
			if !ok {
				continue
			}
			if found != "" || key != "" {
				c.fe.add(path+"."+k, "duplicates "+key)
				return "", k, false
			}
			s, ok := raw.(string)
			if !ok {
				c.fe.add(path+"."+k, "must be a string")
				return "", k, false
			}
			found, key = s, k
		}
		return found, key, true
	}

	l := &leaf{}
	src, _, ok1 := str("source", "type")
	field, _, ok2 := str("field")
	op, _, ok3 := str("operator", "op")
	if !ok1 || !ok2 || !ok3 {
		return nil
	}
	bad := false
	if src == "" {
		c.fe.add(path+".source", "is required (trigger, player or activity)")
		bad = true
	}
	if field == "" {
		c.fe.add(path+".field", "is required")
		bad = true
	}
	if op == "" {
		c.fe.add(path+".operator", "is required")
		bad = true
	}
	if bad {
		return nil
	}
	if alias, ok := operatorAliases[op]; ok {
		op = alias
	}
	if !knownOperators[op] {
		c.fe.add(path+".operator", fmt.Sprintf("unknown operator %q", op))
		return nil
	}
	l.op = op
	l.source = src
	l.field = field

	segs, okPath := splitPath(field)
	if !okPath {
		c.fe.add(path+".field", "must be a dot path of letters, digits, '_' or '-' (at most 10 segments)")
		return nil
	}
	switch src {
	case SourceTrigger:
		l.af = afProperties
		l.segs = segs
	case SourceActivity:
		if f, ok := activityFields[segs[0]]; ok && len(segs) == 1 {
			l.af = f
		} else if (segs[0] == "properties" || segs[0] == "context") && len(segs) > 1 {
			l.af = afProperties
			if segs[0] == "context" {
				l.af = afContext
			}
			l.segs = segs[1:]
		} else {
			c.fe.add(path+".field", fmt.Sprintf("unknown activity field %q", field))
			return nil
		}
	case SourcePlayer:
		c.def.needsPlayer = true
		if f, ok := playerFields[segs[0]]; ok && len(segs) == 1 {
			l.pf = f
			switch f {
			case pfLevel, pfXP:
				c.def.needsProgress = true
			case pfPoints:
				c.def.needsPoints = true
			}
		} else if (segs[0] == "attributes" || segs[0] == "metadata") && len(segs) > 1 {
			l.pf = pfAttributes
			l.segs = segs[1:]
		} else {
			c.fe.add(path+".field", fmt.Sprintf("unknown player field %q", field))
			return nil
		}
	default:
		c.fe.add(path+".source", fmt.Sprintf("unknown source %q", src))
		return nil
	}

	raw, hasValue := obj["value"]
	l.rawValue = raw
	vp := path + ".value"
	switch op {
	case OpExists, OpNotExists:
		if hasValue && raw != nil {
			c.fe.add(vp, "must be omitted for "+op)
			return nil
		}
	case OpIn, OpNotIn:
		items, isList := raw.([]any)
		if !hasValue || raw == nil {
			c.fe.add(vp, "is required")
			return nil
		}
		if !isList {
			items = []any{raw} // Laravel's (array) cast
		}
		if len(items) == 0 {
			c.fe.add(vp, "must not be an empty list")
			return nil
		}
		if len(items) > MaxListValues {
			c.fe.add(vp, fmt.Sprintf("must have at most %d values", MaxListValues))
			return nil
		}
		l.list = make([]value, len(items))
		for i, it := range items {
			v, ok := scalar(it)
			if !ok {
				c.fe.add(fmt.Sprintf("%s[%d]", vp, i), "must be a string, number or boolean")
				return nil
			}
			l.list[i] = v
		}
	default:
		if !hasValue || raw == nil {
			c.fe.add(vp, "is required (use exists / not_exists to test presence)")
			return nil
		}
		v, ok := scalar(raw)
		if !ok {
			c.fe.add(vp, "must be a string, number or boolean")
			return nil
		}
		switch op {
		case OpGt, OpGte, OpLt, OpLte:
			if v.k == kBool {
				c.fe.add(vp, "must be a number or a string for "+op)
				return nil
			}
		case OpContains:
			if v.k == kString && v.s == "" {
				c.fe.add(vp, "must not be empty") // G11
				return nil
			}
		}
		l.val = v
	}
	return &node{kind: nLeaf, path: path, leaf: l}
}

func scalar(x any) (value, bool) {
	v := toValue(x)
	switch v.k {
	case kBool, kNumber:
		return v, true
	case kString:
		if len(v.s) > MaxStringValue {
			return value{}, false
		}
		return v, true
	}
	return value{}, false
}

var actionKeys = map[string]map[string]bool{
	ActionCreditPoints:    {"amount": true, "description": true},
	ActionGrantXP:         {"amount": true, "description": true},
	ActionAwardBadge:      {"badge_id": true},
	ActionRecordStreak:    {"streak_id": true, "activity_key": true},
	ActionProgressMission: {"mission_id": true, "increment": true},
	ActionGrantReward:     {"reward_id": true},
}

func compileActions(raw json.RawMessage, maxActions int, fe fieldErrors) []Action {
	v, present, err := decode(raw)
	if err != nil {
		fe.add("actions", "is not valid JSON")
		return nil
	}
	items, ok := v.([]any)
	if !present || !ok || len(items) == 0 {
		fe.add("actions", "must be a non-empty list")
		return nil
	}
	if len(items) > maxActions {
		fe.add("actions", fmt.Sprintf("must have at most %d actions", maxActions))
		return nil
	}
	out := make([]Action, 0, len(items))
	for i, it := range items {
		p := fmt.Sprintf("actions[%d]", i)
		obj, ok := it.(map[string]any)
		if !ok {
			fe.add(p, "must be an object")
			continue
		}
		typ, _ := obj["type"].(string)
		allowed, known := actionKeys[typ]
		if !known {
			fe.add(p+".type", fmt.Sprintf("unknown action type %q", typ)) // G20
			continue
		}
		for k := range obj {
			if k != "type" && !allowed[k] {
				fe.add(p+"."+k, "unknown key for "+typ)
			}
		}
		a := Action{Type: typ}
		switch typ {
		case ActionCreditPoints, ActionGrantXP:
			a.Amount, ok = positiveInt(obj, "amount", MaxAmount, p, true, fe)
			if !ok {
				continue
			}
			a.Description, ok = optString(obj, "description", MaxDescription, p, fe)
		case ActionAwardBadge:
			a.BadgeID, ok = uuidParam(obj, "badge_id", p, true, fe)
		case ActionRecordStreak:
			var ok1, ok2 bool
			a.StreakID, ok1 = uuidParam(obj, "streak_id", p, false, fe)
			a.ActivityKey, ok2 = optString(obj, "activity_key", MaxActivityKey, p, fe)
			ok = ok1 && ok2
			if ok && (a.StreakID == "") == (a.ActivityKey == "") {
				fe.add(p, "needs exactly one of streak_id or activity_key")
				ok = false
			}
		case ActionProgressMission:
			var ok1, ok2 bool
			a.MissionID, ok1 = uuidParam(obj, "mission_id", p, true, fe)
			a.Increment, ok2 = positiveInt(obj, "increment", MaxAmount, p, false, fe)
			if a.Increment == 0 {
				a.Increment = 1
			}
			ok = ok1 && ok2
		case ActionGrantReward:
			a.RewardID, ok = uuidParam(obj, "reward_id", p, true, fe)
		}
		if ok {
			out = append(out, a)
		}
	}
	return out
}

// positiveInt reads a strictly positive JSON integer: 0, negatives, floats
// (10.5) and numeric strings ("10") are all compile errors (G17-G19).
func positiveInt(obj map[string]any, key string, maxV int64, path string, required bool, fe fieldErrors) (int64, bool) {
	raw, ok := obj[key]
	if !ok {
		if required {
			fe.add(path+"."+key, "is required")
			return 0, false
		}
		return 0, true
	}
	n, isNum := raw.(json.Number)
	if !isNum {
		fe.add(path+"."+key, "must be an integer")
		return 0, false
	}
	i, err := strconv.ParseInt(string(n), 10, 64)
	if err != nil {
		fe.add(path+"."+key, "must be an integer")
		return 0, false
	}
	if i <= 0 {
		fe.add(path+"."+key, "must be greater than 0")
		return 0, false
	}
	if i > maxV {
		fe.add(path+"."+key, fmt.Sprintf("must be at most %d", maxV))
		return 0, false
	}
	return i, true
}

func optString(obj map[string]any, key string, maxLen int, path string, fe fieldErrors) (string, bool) {
	raw, ok := obj[key]
	if !ok || raw == nil {
		return "", true
	}
	s, isStr := raw.(string)
	if !isStr {
		fe.add(path+"."+key, "must be a string")
		return "", false
	}
	if len(s) > maxLen {
		fe.add(path+"."+key, fmt.Sprintf("must be at most %d characters", maxLen))
		return "", false
	}
	return s, true
}

func uuidParam(obj map[string]any, key, path string, required bool, fe fieldErrors) (string, bool) {
	raw, ok := obj[key]
	if !ok || raw == nil {
		if required {
			fe.add(path+"."+key, "is required")
			return "", false
		}
		return "", true
	}
	s, isStr := raw.(string)
	if !isStr || !IsUUID(s) {
		fe.add(path+"."+key, "must be a UUID")
		return "", false
	}
	return strings.ToLower(s), true
}

// IsUUID reports a canonical 36-character UUID.
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}

func compileLimits(raw json.RawMessage, fe fieldErrors) Limits {
	v, present, err := decode(raw)
	if err != nil {
		fe.add("limits", "is not valid JSON")
		return Limits{}
	}
	if !present || v == nil {
		return Limits{}
	}
	obj, ok := v.(map[string]any)
	if !ok {
		fe.add("limits", "must be an object")
		return Limits{}
	}
	var l Limits
	for k := range obj {
		switch k {
		case "max_per_player":
			l.MaxPerPlayer, _ = positiveInt(obj, k, MaxLimitCount, "limits", false, fe)
		case "max_per_player_per_day":
			l.MaxPerPlayerPerDay, _ = positiveInt(obj, k, MaxLimitCount, "limits", false, fe)
		case "max_per_player_per_week":
			l.MaxPerPlayerPerWeek, _ = positiveInt(obj, k, MaxLimitCount, "limits", false, fe)
		case "cooldown_seconds":
			l.CooldownSeconds, _ = positiveInt(obj, k, MaxCooldownSeconds, "limits", false, fe)
		default:
			fe.add("limits."+k, "unknown limit")
		}
	}
	return l
}
