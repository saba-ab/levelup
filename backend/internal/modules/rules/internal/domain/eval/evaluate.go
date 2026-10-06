package eval

import (
	"encoding/json"
	"sort"
	"time"
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
	// Schedule and StopProcessing are part of the version (grammar v1).
	Schedule       json.RawMessage
	StopProcessing bool
}

// Spec is the source's rule body.
func (s RuleSource) Spec() Spec {
	return Spec{Conditions: s.Conditions, Actions: s.Actions, Limits: s.Limits,
		Schedule: s.Schedule, StopProcessing: s.StopProcessing}
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
	historyTypes  map[string]bool
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
	p := &Program{rules: make([]compiledRule, len(sorted)), historyTypes: map[string]bool{}}
	for i, s := range sorted {
		def, err := CompileSpec(s.Spec(), opts)
		p.rules[i] = compiledRule{src: s, def: def, err: err}
		if err != nil {
			continue
		}
		p.needsProgress = p.needsProgress || def.needsProgress
		p.needsPoints = p.needsPoints || def.needsPoints
		p.programScoped = p.programScoped || s.ProgramID != ""
		for et := range def.historyTypes {
			p.historyTypes[et] = true
		}
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
	// OccurredAt is when the activity happened; schedules are judged on it.
	OccurredAt time.Time
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
	// History is the player's prior activity (grammar v2); nil = not
	// loaded, and every history fact is then missing.
	History *History
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
	// StopProcessing is the version flag; StoppedBy names the rule whose
	// firing skipped this one (status skipped_by_stop).
	StopProcessing bool
	StoppedBy      string
	Error          string
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
		if r.def != nil {
			rr.StopProcessing = r.def.StopProcessing
		}
		switch {
		case r.err != nil:
			rr.Status = StatusInvalid
			rr.Error = r.err.Error()
		case f.ProgramScoping && r.src.ProgramID != "" && !enrolled[r.src.ProgramID]:
			rr.Status = StatusOutOfScope
		case !r.def.Schedule.Contains(f.Activity.OccurredAt):
			rr.Status = StatusOutOfSchedule
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
	case SourceHistory:
		return resolveHistory(l, f)
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

// StopGate applies stop_processing in evaluation order. Evaluate never cuts
// the list itself because only the caller knows whether a matched rule
// actually fired (limits are enforced outside the evaluator):
//
//	var g StopGate
//	for i := range res.Rules {
//		if !g.Admit(&res.Rules[i]) { continue }   // rewritten to skipped_by_stop
//		if fired(res.Rules[i]) { g.Fired(res.Rules[i]) }
//	}
type StopGate struct{ stoppedBy string }

// Stopped reports whether a stop_processing rule has fired.
func (g *StopGate) Stopped() bool { return g.stoppedBy != "" }

// Admit returns true when rr may proceed. Once the gate is closed it
// rewrites rr to skipped_by_stop (no trace, no actions, no limits) and
// returns false.
func (g *StopGate) Admit(rr *RuleResult) bool {
	if g.stoppedBy == "" {
		return true
	}
	*rr = RuleResult{
		RuleID: rr.RuleID, RuleVersionID: rr.RuleVersionID, Name: rr.Name, ProgramID: rr.ProgramID,
		Priority: rr.Priority, StopProcessing: rr.StopProcessing,
		Status: StatusSkippedByStop, StoppedBy: g.stoppedBy,
	}
	return false
}

// Fired records that rr fired; a stop_processing rule closes the gate.
func (g *StopGate) Fired(rr RuleResult) {
	if rr.StopProcessing && g.stoppedBy == "" {
		g.stoppedBy = rr.RuleID
	}
}

// ApplyStop applies stop_processing treating every matched rule as fired
// (simulation: limits are reported, not enforced).
func (r *Result) ApplyStop() {
	var g StopGate
	for i := range r.Rules {
		if g.Admit(&r.Rules[i]) && r.Rules[i].Matched {
			g.Fired(r.Rules[i])
		}
	}
}
