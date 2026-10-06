package domain

import (
	"fmt"
	"math"
	"strings"
)

// Requirement metrics: the per-player facts badges projects from other
// modules' events (badge_player_stats).
const (
	MetricLifetimePoints    = "lifetime_points"    // points.credited.v1 lifetime_earned
	MetricMissionsCompleted = "missions_completed" // missions.completed.v1, one per attempt
	MetricStreakDays        = "streak_days"        // highest current_count of any streak
	MetricLevel             = "level"              // highest level reached
	MetricBadgesEarned      = "badges_earned"      // applied awards (stacks), any badge
	MetricActivityCount     = "activity_count"     // activity.received.v1, optionally per event_type
)

const (
	maxConditions   = 20
	maxEventTypeLen = 255
)

var metrics = map[string]bool{
	MetricLifetimePoints:    true,
	MetricMissionsCompleted: true,
	MetricStreakDays:        true,
	MetricLevel:             true,
	MetricBadgesEarned:      true,
	MetricActivityCount:     true,
}

// Condition is one threshold: the metric's value must be >= Gte.
// EventType narrows activity_count to one activity type; empty counts every
// type.
type Condition struct {
	Metric    string
	EventType string
	Gte       int64
}

// Requirements is the evaluated form of a badge's requirements JSON:
//
//	{"all": [cond, ...], "any": [cond, ...]}
//	cond = {"metric": "<metric>", "event_type": "<type>" (activity_count only), "gte": <int >= 1>}
//
// Every condition of All must hold, and at least one of Any when Any is
// present. At least one of the two lists is non-empty.
type Requirements struct {
	All []Condition
	Any []Condition
}

// PlayerStats is badges' projection of one player, the input of
// Requirements.Satisfied.
type PlayerStats struct {
	TenantID          string
	PlayerID          string
	LifetimePoints    int64
	MissionsCompleted int64
	MaxStreak         int64
	Level             int64
	BadgesEarned      int64
	// ActivityCounts is per event type.
	ActivityCounts map[string]int64
}

// Value returns the metric's current value for the condition.
func (s PlayerStats) Value(c Condition) int64 {
	switch c.Metric {
	case MetricLifetimePoints:
		return s.LifetimePoints
	case MetricMissionsCompleted:
		return s.MissionsCompleted
	case MetricStreakDays:
		return s.MaxStreak
	case MetricLevel:
		return s.Level
	case MetricBadgesEarned:
		return s.BadgesEarned
	case MetricActivityCount:
		if c.EventType != "" {
			return s.ActivityCounts[c.EventType]
		}
		var total int64
		for _, n := range s.ActivityCounts {
			total += n
		}
		return total
	}
	return 0
}

// Satisfied reports whether the player's stats meet the requirements.
func (r Requirements) Satisfied(s PlayerStats) bool {
	if len(r.All) == 0 && len(r.Any) == 0 {
		return false
	}
	for _, c := range r.All {
		if s.Value(c) < c.Gte {
			return false
		}
	}
	if len(r.Any) == 0 {
		return true
	}
	for _, c := range r.Any {
		if s.Value(c) >= c.Gte {
			return true
		}
	}
	return false
}

// Uses reports whether a change of metric (and, for activity_count, of
// eventType) can change the outcome of Satisfied.
func (r Requirements) Uses(metric, eventType string) bool {
	for _, list := range [][]Condition{r.All, r.Any} {
		for _, c := range list {
			if c.Metric != metric {
				continue
			}
			if metric != MetricActivityCount || c.EventType == "" || c.EventType == eventType {
				return true
			}
		}
	}
	return false
}

// ParseRequirements validates raw (the JSON-decoded requirements) against
// the grammar. A nil or empty map means "no requirements" (ok=false, no
// error): the badge is awarded only explicitly.
func ParseRequirements(raw map[string]any) (req Requirements, ok bool, err error) {
	if len(raw) == 0 {
		return Requirements{}, false, nil
	}
	for k := range raw {
		if k != "all" && k != "any" {
			return Requirements{}, false, requirementsError("unknown key %q: only all and any are allowed", k)
		}
	}
	if req.All, err = parseConditions(raw, "all"); err != nil {
		return Requirements{}, false, err
	}
	if req.Any, err = parseConditions(raw, "any"); err != nil {
		return Requirements{}, false, err
	}
	if len(req.All) == 0 && len(req.Any) == 0 {
		return Requirements{}, false, requirementsError("all or any must list at least one condition")
	}
	return req, true, nil
}

func parseConditions(raw map[string]any, key string) ([]Condition, error) {
	v, present := raw[key]
	if !present || v == nil {
		return nil, nil
	}
	list, isList := v.([]any)
	if !isList {
		return nil, requirementsError("%s must be an array of conditions", key)
	}
	if len(list) > maxConditions {
		return nil, requirementsError("%s holds at most %d conditions", key, maxConditions)
	}
	out := make([]Condition, 0, len(list))
	for i, item := range list {
		c, err := parseCondition(item)
		if err != nil {
			return nil, requirementsError("%s[%d]: %s", key, i, err.Error())
		}
		out = append(out, c)
	}
	return out, nil
}

func parseCondition(item any) (Condition, error) {
	m, isObj := item.(map[string]any)
	if !isObj {
		return Condition{}, fmt.Errorf("must be an object")
	}
	var c Condition
	for k, v := range m {
		switch k {
		case "metric":
			s, isStr := v.(string)
			if !isStr || !metrics[s] {
				return Condition{}, fmt.Errorf("metric must be one of lifetime_points, missions_completed, streak_days, level, badges_earned, activity_count")
			}
			c.Metric = s
		case "event_type":
			s, isStr := v.(string)
			if !isStr || strings.TrimSpace(s) == "" || len(s) > maxEventTypeLen {
				return Condition{}, fmt.Errorf("event_type must be a non-empty string of at most %d characters", maxEventTypeLen)
			}
			c.EventType = s
		case "gte":
			n, isInt := wholeNumber(v)
			if !isInt || n < 1 {
				return Condition{}, fmt.Errorf("gte must be an integer >= 1")
			}
			c.Gte = n
		default:
			return Condition{}, fmt.Errorf("unknown key %q", k)
		}
	}
	switch {
	case c.Metric == "":
		return Condition{}, fmt.Errorf("metric is required")
	case c.Gte == 0:
		return Condition{}, fmt.Errorf("gte is required")
	case c.EventType != "" && c.Metric != MetricActivityCount:
		return Condition{}, fmt.Errorf("event_type is allowed only with activity_count")
	}
	return c, nil
}

// wholeNumber accepts JSON numbers (float64 after decoding) and Go ints.
func wholeNumber(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if n != math.Trunc(n) || n > math.MaxInt64/2 || n < math.MinInt64/2 {
			return 0, false
		}
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}

func requirementsError(format string, args ...any) error {
	return withRequirementsDetail(fmt.Sprintf(format, args...))
}
