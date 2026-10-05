package eval

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// kind is the type of a resolved value.
type kind uint8

const (
	kMissing kind = iota // path absent
	kNull                // JSON null
	kBool
	kNumber
	kString
	kArray
	kObject
)

// num keeps integers exact: a JSON integer stays an int64, everything else
// is a float64.
type num struct {
	i     int64
	f     float64
	isInt bool
}

func (n num) float() float64 {
	if n.isInt {
		return float64(n.i)
	}
	return n.f
}

func cmpNum(a, b num) int {
	if a.isInt && b.isInt {
		switch {
		case a.i < b.i:
			return -1
		case a.i > b.i:
			return 1
		}
		return 0
	}
	fa, fb := a.float(), b.float()
	switch {
	case fa < fb:
		return -1
	case fa > fb:
		return 1
	}
	return 0
}

// value is a resolved fact or a compiled constant.
type value struct {
	k   kind
	b   bool
	n   num
	s   string
	arr []any // raw elements, converted lazily
}

func (v value) present() bool { return v.k != kMissing && v.k != kNull }

// parseNumber parses a JSON-style number string. Integers that fit int64
// stay exact.
func parseNumber(s string) (num, bool) {
	if s == "" {
		return num{}, false
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return num{i: i, isInt: true}, true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return num{}, false
	}
	return fromFloat(f), true
}

// fromFloat normalises an integral float into an exact int so 100.0 and 100
// are the same number regardless of how the JSON was decoded.
func fromFloat(f float64) num {
	if f == math.Trunc(f) && f >= -9.007199254740992e15 && f <= 9.007199254740992e15 {
		return num{i: int64(f), isInt: true}
	}
	return num{f: f}
}

// toValue converts a decoded JSON value (or a Go value from a port snapshot)
// into a value. Unknown Go types resolve to an object so they never compare
// equal to anything.
func toValue(x any) value {
	switch t := x.(type) {
	case nil:
		return value{k: kNull}
	case bool:
		return value{k: kBool, b: t}
	case string:
		return value{k: kString, s: t}
	case json.Number:
		if n, ok := parseNumber(string(t)); ok {
			return value{k: kNumber, n: n}
		}
		return value{k: kString, s: string(t)}
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return value{k: kObject}
		}
		return value{k: kNumber, n: fromFloat(t)}
	case float32:
		return toValue(float64(t))
	case int:
		return value{k: kNumber, n: num{i: int64(t), isInt: true}}
	case int8:
		return value{k: kNumber, n: num{i: int64(t), isInt: true}}
	case int16:
		return value{k: kNumber, n: num{i: int64(t), isInt: true}}
	case int32:
		return value{k: kNumber, n: num{i: int64(t), isInt: true}}
	case int64:
		return value{k: kNumber, n: num{i: t, isInt: true}}
	case uint:
		return uintValue(uint64(t))
	case uint8:
		return uintValue(uint64(t))
	case uint16:
		return uintValue(uint64(t))
	case uint32:
		return uintValue(uint64(t))
	case uint64:
		return uintValue(t)
	case []any:
		return value{k: kArray, arr: t}
	case []string:
		arr := make([]any, len(t))
		for i, s := range t {
			arr[i] = s
		}
		return value{k: kArray, arr: arr}
	default:
		return value{k: kObject}
	}
}

func uintValue(u uint64) value {
	if u > math.MaxInt64 {
		return value{k: kNumber, n: num{f: float64(u)}}
	}
	return value{k: kNumber, n: num{i: int64(u), isInt: true}}
}

// asNumber returns v as a number, coercing a numeric string.
func asNumber(v value) (num, bool) {
	switch v.k {
	case kNumber:
		return v.n, true
	case kString:
		return parseNumber(v.s)
	default:
		return num{}, false
	}
}

// equalValues is typed equality with the one documented coercion: a numeric
// string equals the number it spells.
func equalValues(a, e value) bool {
	switch {
	case a.k == kNumber && e.k == kNumber:
		return cmpNum(a.n, e.n) == 0
	case a.k == kNumber && e.k == kString, a.k == kString && e.k == kNumber:
		an, ok1 := asNumber(a)
		en, ok2 := asNumber(e)
		return ok1 && ok2 && cmpNum(an, en) == 0
	case a.k == kString && e.k == kString:
		return a.s == e.s
	case a.k == kBool && e.k == kBool:
		return a.b == e.b
	default:
		return false
	}
}

// order compares a to e for gt/gte/lt/lte. ok is false when the two are not
// comparable (a type mismatch, a missing fact): the operator is then false.
func order(a, e value) (int, bool) {
	if a.k == kNumber || e.k == kNumber {
		an, ok1 := asNumber(a)
		en, ok2 := asNumber(e)
		if !ok1 || !ok2 {
			return 0, false
		}
		return cmpNum(an, en), true
	}
	if a.k == kString && e.k == kString {
		an, ok1 := parseNumber(a.s)
		en, ok2 := parseNumber(e.s)
		if ok1 && ok2 {
			return cmpNum(an, en), true
		}
		return strings.Compare(a.s, e.s), true
	}
	return 0, false
}

// compare applies a (compiled, well-formed) operator.
func compare(op string, a value, e value, list []value) bool {
	switch op {
	case OpExists:
		return a.present()
	case OpNotExists:
		return !a.present()
	}
	if !a.present() {
		// Missing or null: only the negative operators hold (G10).
		return op == OpNeq || op == OpNotIn
	}
	switch op {
	case OpEq:
		return equalValues(a, e)
	case OpNeq:
		return !equalValues(a, e)
	case OpGt, OpGte, OpLt, OpLte:
		c, ok := order(a, e)
		if !ok {
			return false
		}
		switch op {
		case OpGt:
			return c > 0
		case OpGte:
			return c >= 0
		case OpLt:
			return c < 0
		default:
			return c <= 0
		}
	case OpIn:
		return inList(a, list)
	case OpNotIn:
		return !inList(a, list)
	case OpContains:
		switch a.k {
		case kString:
			return e.k == kString && strings.Contains(a.s, e.s)
		case kArray:
			for _, el := range a.arr {
				if equalValues(toValue(el), e) {
					return true
				}
			}
		}
		return false
	}
	return false
}

func inList(a value, list []value) bool {
	for _, el := range list {
		if equalValues(a, el) {
			return true
		}
	}
	return false
}

// lookup walks a dot path through decoded JSON maps and arrays.
func lookup(root any, segs []string) value {
	cur := root
	for _, s := range segs {
		switch t := cur.(type) {
		case map[string]any:
			next, ok := t[s]
			if !ok {
				return value{k: kMissing}
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(s)
			if err != nil || i < 0 || i >= len(t) {
				return value{k: kMissing}
			}
			cur = t[i]
		default:
			return value{k: kMissing}
		}
	}
	return toValue(cur)
}

// exported converts a value back into a JSON-friendly Go value for traces.
func (v value) exported() any {
	switch v.k {
	case kNull:
		return nil
	case kBool:
		return v.b
	case kNumber:
		if v.n.isInt {
			return v.n.i
		}
		return v.n.f
	case kString:
		return v.s
	case kArray:
		return v.arr
	case kObject:
		return "<object>"
	default:
		return nil
	}
}
