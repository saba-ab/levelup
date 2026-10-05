package eval

import (
	"encoding/json"
	"sort"
)

// RuleSource is one live rule version as stored: the input of Compile.
type RuleSource struct {
	RuleID        string
	RuleVersionID string
	Name          string
	ProgramID     string
	Priority      int
	Conditions    json.RawMessage
	Actions       json.RawMessage
	Limits        json.RawMessage
}

type compiledRule struct {
	src RuleSource
	def *Definition
	err error
}

// Program is an immutable, ordered, compiled ruleset for one trigger. It is
// safe for concurrent use.
type Program struct {
	rules         []compiledRule
	needsProgress bool
	needsPoints   bool
	programScoped bool
}

// Compile builds a Program from stored versions. Rules are ordered priority
// DESC, rule id ASC (uuidv7 ids = creation order, golden G05/G06). A stored
// version that no longer compiles is kept as an invalid entry instead of
// failing the whole trigger: one bad rule must not block the others (B2).
func Compile(sources []RuleSource, opts Options) *Program {
	sorted := make([]RuleSource, len(sources))
	copy(sorted, sources)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority > sorted[j].Priority
		}
		if sorted[i].RuleID != sorted[j].RuleID {
			return sorted[i].RuleID < sorted[j].RuleID
		}
		return sorted[i].RuleVersionID < sorted[j].RuleVersionID
	})
	p := &Program{rules: make([]compiledRule, len(sorted))}
	for i, s := range sorted {
		def, err := CompileDefinition(s.Conditions, s.Actions, s.Limits, opts)
		p.rules[i] = compiledRule{src: s, def: def, err: err}
		if err != nil {
			continue
		}
		p.needsProgress = p.needsProgress || def.needsProgress
		p.needsPoints = p.needsPoints || def.needsPoints
		p.programScoped = p.programScoped || s.ProgramID != ""
	}
	return p
}

// Len is the number of rules in the program.
func (p *Program) Len() int { return len(p.rules) }

// NeedsProgress reports whether any rule reads player.level or player.xp.
func (p *Program) NeedsProgress() bool { return p.needsProgress }

// NeedsPoints reports whether any rule reads player.points.
func (p *Program) NeedsPoints() bool { return p.needsPoints }

// ProgramScoped reports whether any rule is scoped to a program.
func (p *Program) ProgramScoped() bool { return p.programScoped }

// Activity is the activity part of Facts.
type Activity struct {
	EventID        string
	EventType      string
	Properties     map[string]any
	Context        map[string]any
	CausationDepth int
}

// Player is the player part of Facts: one snapshot taken before evaluation,
// so every rule sees the same state (golden G14, fixes B17).
type Player struct {
	ID          string
	ExternalID  string
	DisplayName string
	Email       string
	Active      bool
	Attributes  map[string]any
	Level       int
	XP          int64
	Points      int64
	HasProgress bool // Level/XP were loaded
	HasPoints   bool // Points was loaded
}

// Facts is everything a condition may read.
type Facts struct {
	Activity Activity
	Player   *Player
	// ProgramScoping enables program_id filtering; EnrolledPrograms is then
	// the set of programs the player is enrolled in.
	ProgramScoping   bool
	EnrolledPrograms []string
}

// CondTrace records one evaluated leaf ("why did the player get X").
type CondTrace struct {
	Path     string
	Source   string
	Field    string
	Operator string
	Expected any
	Actual   any
	Present  bool
	Result   bool
}

// RuleResult is the outcome of one rule.
type RuleResult struct {
	RuleID        string
	RuleVersionID string
	Name          string
	ProgramID     string
	Priority      int
	Status        string
	Matched       bool
	Trace         []CondTrace
	Actions       []Action
	Limits        Limits
	Error         string
}

// Result lists every rule of the program in evaluation order.
type Result struct {
	Rules []RuleResult
}

// MatchedCount is the number of matched rules.
func (r Result) MatchedCount() int {
	n := 0
	for _, rr := range r.Rules {
		if rr.Matched {
			n++
		}
	}
	return n
}

// Evaluate runs the program against facts. Pure and deterministic: no I/O,
// no clock, no shared mutable state.
func Evaluate(p *Program, f Facts) Result {
	if p == nil {
		return Result{}
	}
	res := Result{Rules: make([]RuleResult, 0, len(p.rules))}
	var enrolled map[string]bool
	if f.ProgramScoping && p.programScoped {
		enrolled = make(map[string]bool, len(f.EnrolledPrograms))
		for _, id := range f.EnrolledPrograms {
			enrolled[id] = true
		}
	}
	for _, r := range p.rules {
		rr := RuleResult{
			RuleID:        r.src.RuleID,
			RuleVersionID: r.src.RuleVersionID,
			Name:          r.src.Name,
			ProgramID:     r.src.ProgramID,
			Priority:      r.src.Priority,
		}
		switch {
		case r.err != nil:
			rr.Status = StatusInvalid
			rr.Error = r.err.Error()
		case f.ProgramScoping && r.src.ProgramID != "" && !enrolled[r.src.ProgramID]:
			rr.Status = StatusOutOfScope
		default:
			rr.Matched = evalNode(r.def.cond, &f, &rr.Trace)
			rr.Status = StatusNotMatched
			if rr.Matched {
				rr.Status = StatusMatched
				rr.Actions = r.def.Actions
				rr.Limits = r.def.Limits
			}
		}
		res.Rules = append(res.Rules, rr)
	}
	return res
}

func evalNode(n *node, f *Facts, trace *[]CondTrace) bool {
	if n == nil {
		return true
	}
	switch n.kind {
	case nAll:
		for _, c := range n.children {
			if !evalNode(c, f, trace) {
				return false
			}
		}
		return true
	case nAny:
		for _, c := range n.children {
			if evalNode(c, f, trace) {
				return true
			}
		}
		return false
	case nNot:
		return !evalNode(n.children[0], f, trace)
	default:
		l := n.leaf
		actual := resolve(l, f)
		ok := compare(l.op, actual, l.val, l.list)
		*trace = append(*trace, CondTrace{
			Path:     n.path,
			Source:   l.source,
			Field:    l.field,
			Operator: l.op,
			Expected: l.rawValue,
			Actual:   actual.exported(),
			Present:  actual.present(),
			Result:   ok,
		})
		return ok
	}
}

func resolve(l *leaf, f *Facts) value {
	switch l.source {
	case SourceTrigger, SourceActivity:
		switch l.af {
		case afEventType:
			return value{k: kString, s: f.Activity.EventType}
		case afEventID:
			return value{k: kString, s: f.Activity.EventID}
		case afCausationDepth:
			return value{k: kNumber, n: num{i: int64(f.Activity.CausationDepth), isInt: true}}
		case afContext:
			return lookupMap(f.Activity.Context, l.segs)
		default:
			return lookupMap(f.Activity.Properties, l.segs)
		}
	case SourcePlayer:
		pl := f.Player
		if pl == nil {
			return value{k: kMissing}
		}
		switch l.pf {
		case pfID:
			return value{k: kString, s: pl.ID}
		case pfExternalID:
			return value{k: kString, s: pl.ExternalID}
		case pfDisplayName:
			return value{k: kString, s: pl.DisplayName}
		case pfEmail:
			if pl.Email == "" {
				return value{k: kMissing}
			}
			return value{k: kString, s: pl.Email}
		case pfActive:
			return value{k: kBool, b: pl.Active}
		case pfLevel:
			if !pl.HasProgress {
				return value{k: kMissing}
			}
			return value{k: kNumber, n: num{i: int64(pl.Level), isInt: true}}
		case pfXP:
			if !pl.HasProgress {
				return value{k: kMissing}
			}
			return value{k: kNumber, n: num{i: pl.XP, isInt: true}}
		case pfPoints:
			if !pl.HasPoints {
				return value{k: kMissing}
			}
			return value{k: kNumber, n: num{i: pl.Points, isInt: true}}
		case pfAttributes:
			return lookupMap(pl.Attributes, l.segs)
		}
	}
	return value{k: kMissing}
}

func lookupMap(m map[string]any, segs []string) value {
	if m == nil {
		return value{k: kMissing}
	}
	return lookup(m, segs)
}
