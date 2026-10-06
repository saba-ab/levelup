package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	eventSlugPattern = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*$`)
	pathSegment      = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

const (
	maxEventSlugLen = 100
	maxPathSegments = 10
	maxFieldLen     = 200
)

// IsEventSlug reports a plausible event type slug.
func IsEventSlug(s string) bool {
	return s != "" && len(s) <= maxEventSlugLen && eventSlugPattern.MatchString(s)
}

// isPath reports a dot path of 1..maxPathSegments plain segments.
func isPath(s string) bool {
	if s == "" || len(s) > maxFieldLen {
		return false
	}
	parts := strings.Split(s, ".")
	if len(parts) > maxPathSegments {
		return false
	}
	for _, p := range parts {
		if !pathSegment.MatchString(p) {
			return false
		}
	}
	return true
}

// fields reads one model-produced JSON object (decoded with UseNumber) into a
// clean create-request body. Absent and null are the same thing; anything
// else of the wrong type or out of range is a reason, never coerced.
type fields struct {
	in      map[string]any
	out     map[string]any
	prefix  string
	reasons *[]string
}

func newFields(in map[string]any, prefix string, reasons *[]string) *fields {
	return &fields{in: in, out: map[string]any{}, prefix: prefix, reasons: reasons}
}

func (f *fields) fail(key, msg string) {
	*f.reasons = append(*f.reasons, f.prefix+key+": "+msg)
}

func (f *fields) present(key string) (any, bool) {
	v, ok := f.in[key]
	if !ok || v == nil {
		return nil, false
	}
	return v, true
}

// str reads a string. Empty strings count as absent.
func (f *fields) str(key string, required bool, maxLen int) (string, bool) {
	v, ok := f.present(key)
	if !ok {
		if required {
			f.fail(key, "is required")
		}
		return "", !required
	}
	s, isStr := v.(string)
	if !isStr {
		f.fail(key, "must be a string")
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		if required {
			f.fail(key, "is required")
		}
		return "", !required
	}
	if utf8.RuneCountInString(s) > maxLen {
		f.fail(key, fmt.Sprintf("must be at most %d characters", maxLen))
		return "", false
	}
	return s, true
}

// copyStr copies an optional or required string into the output.
func (f *fields) copyStr(key string, required bool, maxLen int) string {
	s, ok := f.str(key, required, maxLen)
	if ok && s != "" {
		f.out[key] = s
	}
	return s
}

func (f *fields) enum(key string, required bool, allowed ...string) string {
	s, ok := f.str(key, required, 100)
	if !ok || s == "" {
		return ""
	}
	if !slices.Contains(allowed, s) {
		f.fail(key, "must be one of "+strings.Join(allowed, ", "))
		return ""
	}
	f.out[key] = s
	return s
}

// integer reads a JSON integer in [minV, maxV]. Floats and numeric strings
// are rejected, matching the target modules' decoders.
func (f *fields) integer(key string, required bool, minV, maxV int64) (int64, bool) {
	v, ok := f.present(key)
	if !ok {
		if required {
			f.fail(key, "is required")
		}
		return 0, false
	}
	n, isNum := v.(json.Number)
	if !isNum {
		f.fail(key, "must be an integer")
		return 0, false
	}
	i, err := strconv.ParseInt(string(n), 10, 64)
	if err != nil {
		f.fail(key, "must be an integer")
		return 0, false
	}
	if i < minV || i > maxV {
		f.fail(key, fmt.Sprintf("must be between %d and %d", minV, maxV))
		return 0, false
	}
	return i, true
}

func (f *fields) copyInt(key string, required bool, minV, maxV int64) (int64, bool) {
	i, ok := f.integer(key, required, minV, maxV)
	if ok {
		f.out[key] = i
	}
	return i, ok
}

func (f *fields) boolean(key string) bool {
	v, ok := f.present(key)
	if !ok {
		return false
	}
	b, isBool := v.(bool)
	if !isBool {
		f.fail(key, "must be a boolean")
		return false
	}
	f.out[key] = b
	return b
}

func (f *fields) object(key string) (map[string]any, bool) {
	v, ok := f.present(key)
	if !ok {
		return nil, false
	}
	m, isObj := v.(map[string]any)
	if !isObj {
		f.fail(key, "must be an object")
		return nil, false
	}
	return m, true
}

func (f *fields) list(key string) ([]any, bool) {
	v, ok := f.present(key)
	if !ok {
		return nil, false
	}
	l, isList := v.([]any)
	if !isList {
		f.fail(key, "must be a list")
		return nil, false
	}
	return l, true
}

// ref copies an id that must name an entity listed in the context.
func (f *fields) ref(key string, list []EntityRef, what string) {
	s, ok := f.str(key, false, 36)
	if !ok || s == "" {
		return
	}
	if !isUUID(s) || !hasRef(list, s) {
		f.fail(key, "must be the id of a "+what+" listed in context")
		return
	}
	f.out[key] = strings.ToLower(s)
}

// eventType copies an event type slug; when the context lists event types
// it must be one of them.
func (f *fields) eventType(key string, required bool, c Context) string {
	s, ok := f.str(key, required, maxEventSlugLen)
	if !ok || s == "" {
		return ""
	}
	if !IsEventSlug(s) {
		f.fail(key, "must be an event type slug")
		return ""
	}
	if len(c.EventTypes) > 0 && !c.hasEventType(s) {
		f.fail(key, "must be one of the event types listed in context")
		return ""
	}
	f.out[key] = s
	return s
}

func (f *fields) sub(in map[string]any, prefix string) *fields {
	return newFields(in, f.prefix+prefix, f.reasons)
}
