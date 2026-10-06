package eval

import (
	"fmt"
	"regexp"
	"sort"
)

// History is the player's prior activity, loaded by the service before
// evaluation (grammar v2). Counts maps event type → window → number of prior
// activities in that window, the current activity excluded. An absent event
// type or window counts zero.
type History struct {
	Counts map[string]map[string]int64
}

// Count returns the prior activities of eventType in window.
func (h *History) Count(eventType, window string) int64 {
	if h == nil {
		return 0
	}
	return h.Counts[eventType][window]
}

type historyField uint8

const (
	hfNone historyField = iota
	hfFirstTime
	hfCount
)

var eventTypeRe = regexp.MustCompile(`^[a-z0-9_.:-]+$`)

var historyOperators = map[string]bool{
	OpEq: true, OpNeq: true, OpGt: true, OpGte: true, OpLt: true, OpLte: true, OpIn: true, OpNotIn: true,
}

// compileHistoryLeaf compiles {"source":"history", ...}. op is already
// alias-resolved but may be empty (first_time defaults to eq true).
func (c *condCompiler) compileHistoryLeaf(obj map[string]any, path, field, op string) *node {
	c.def.needsHistory = true
	l := &leaf{source: SourceHistory, field: field}
	switch field {
	case HistoryFirstTime:
		l.hf = hfFirstTime
		for _, k := range []string{"event_type", "window"} {
			if _, ok := obj[k]; ok {
				c.fe.add(path+"."+k, "is not allowed for history.first_time")
				return nil
			}
		}
		if op == "" {
			op = OpEq
		}
		if op != OpEq && op != OpNeq {
			c.fe.add(path+".operator", "must be eq or neq for history.first_time")
			return nil
		}
		raw, hasValue := obj["value"]
		l.rawValue = raw
		if !hasValue || raw == nil {
			raw, l.rawValue = true, true
		}
		b, ok := raw.(bool)
		if !ok {
			c.fe.add(path+".value", "must be a boolean")
			return nil
		}
		l.op, l.val = op, value{k: kBool, b: b}
		c.def.historyTypes[""] = true
		return &node{kind: nLeaf, path: path, leaf: l}
	case HistoryCount:
		l.hf = hfCount
	case "":
		c.fe.add(path+".field", "is required (first_time or count)")
		return nil
	default:
		c.fe.add(path+".field", fmt.Sprintf("unknown history field %q (first_time or count)", field))
		return nil
	}

	if raw, ok := obj["event_type"]; ok && raw != nil {
		et, isStr := raw.(string)
		if !isStr || et == "" || len(et) > MaxEventType || !eventTypeRe.MatchString(et) {
			c.fe.add(path+".event_type", "must be an event type slug")
			return nil
		}
		l.eventType = et
	}
	w, _ := obj["window"].(string)
	if _, ok := WindowDays[w]; !ok {
		c.fe.add(path+".window", "must be one of 1d, 7d, 30d, 90d, all")
		return nil
	}
	l.window = w
	if op == "" {
		c.fe.add(path+".operator", "is required")
		return nil
	}
	if !historyOperators[op] {
		c.fe.add(path+".operator", fmt.Sprintf("operator %q is not allowed for history.count", op))
		return nil
	}
	l.op = op
	raw, hasValue := obj["value"]
	l.rawValue = raw
	vp := path + ".value"
	if !hasValue || raw == nil {
		c.fe.add(vp, "is required")
		return nil
	}
	if op == OpIn || op == OpNotIn {
		items, isList := raw.([]any)
		if !isList {
			items = []any{raw}
		}
		if len(items) == 0 || len(items) > MaxListValues {
			c.fe.add(vp, fmt.Sprintf("must have 1 to %d numbers", MaxListValues))
			return nil
		}
		l.list = make([]value, len(items))
		for i, it := range items {
			v := toValue(it)
			if v.k != kNumber {
				c.fe.add(fmt.Sprintf("%s[%d]", vp, i), "must be a number")
				return nil
			}
			l.list[i] = v
		}
	} else {
		v := toValue(raw)
		if v.k != kNumber {
			c.fe.add(vp, "must be a number")
			return nil
		}
		l.val = v
	}
	c.def.historyTypes[l.eventType] = true
	return &node{kind: nLeaf, path: path, leaf: l}
}

func resolveHistory(l *leaf, f *Facts) value {
	if f.History == nil {
		return value{k: kMissing}
	}
	switch l.hf {
	case hfFirstTime:
		return value{k: kBool, b: f.History.Count(f.Activity.EventType, WindowAll) == 0}
	case hfCount:
		et := l.eventType
		if et == "" {
			et = f.Activity.EventType
		}
		return value{k: kNumber, n: num{i: f.History.Count(et, l.window), isInt: true}}
	}
	return value{k: kMissing}
}

// HistoryEventTypes lists the event types whose history the program reads,
// with "" (the trigger) resolved to trigger. Empty when no rule reads
// history: the service then skips the history query.
func (p *Program) HistoryEventTypes(trigger string) []string {
	if len(p.historyTypes) == 0 {
		return nil
	}
	set := make(map[string]bool, len(p.historyTypes))
	for et := range p.historyTypes {
		if et == "" {
			et = trigger
		}
		set[et] = true
	}
	out := make([]string, 0, len(set))
	for et := range set {
		out = append(out, et)
	}
	sort.Strings(out)
	return out
}

// NeedsHistory reports whether any rule reads history.*.
func (p *Program) NeedsHistory() bool { return len(p.historyTypes) > 0 }
