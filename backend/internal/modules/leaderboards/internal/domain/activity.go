package domain

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/shared/errs"
)

// ActivityConfig scopes an activity board to one event type and says what
// a matching event is worth: 1 (value count) or the numeric value of one
// top-level property (value property, e.g. "amount").
type ActivityConfig struct {
	EventType string
	Value     string // contracts.ActivityValueCount | contracts.ActivityValueProperty
	Property  string // set only for value property
}

// maxActivityValue bounds a single property contribution so a malformed
// payload cannot overflow a score.
const maxActivityValue = 1e15

var (
	eventTypeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,99}$`)
	propertyRe  = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,99}$`)
)

func invalidConfig(field, msg string) error {
	return errs.WithFields(errs.WithCode(errs.New(errs.Invalid, "invalid activity config: "+msg), CodeInvalidConfig),
		map[string]string{"config." + field: msg})
}

// normaliseActivity validates the config of an activity board and returns
// it trimmed with the metric that goes with it. metric "" defaults from the
// value: count → count, property → earned.
func normaliseActivity(c *ActivityConfig, metric string) (ActivityConfig, string, error) {
	if c == nil {
		return ActivityConfig{}, "", invalidConfig("event_type", "config is required for activity leaderboards")
	}
	out := ActivityConfig{
		EventType: strings.TrimSpace(c.EventType),
		Value:     strings.TrimSpace(c.Value),
		Property:  strings.TrimSpace(c.Property),
	}
	if !eventTypeRe.MatchString(out.EventType) {
		return ActivityConfig{}, "", invalidConfig("event_type", "must be 1-100 characters: letters, digits, _ . : -")
	}
	switch out.Value {
	case contracts.ActivityValueCount:
		if out.Property != "" {
			return ActivityConfig{}, "", invalidConfig("property", "only allowed with value property")
		}
		if metric == "" {
			metric = contracts.MetricCount
		}
		if metric != contracts.MetricCount {
			return ActivityConfig{}, "", ErrInvalidMetric
		}
	case contracts.ActivityValueProperty:
		if !propertyRe.MatchString(out.Property) {
			return ActivityConfig{}, "", invalidConfig("property", "required with value property: 1-100 characters, letters, digits, _ . -")
		}
		if metric == "" {
			metric = contracts.MetricEarned
		}
		if metric != contracts.MetricEarned {
			return ActivityConfig{}, "", ErrInvalidMetric
		}
	default:
		return ActivityConfig{}, "", invalidConfig("value", "must be count or property")
	}
	return out, metric, nil
}

// activityContribution is what one activity fact adds to an activity board.
func activityContribution(b Leaderboard, f Fact) (Op, bool) {
	if b.Activity == nil || f.EventType != b.Activity.EventType {
		return Op{}, false
	}
	switch b.Metric {
	case contracts.MetricCount:
		return Op{Kind: OpIncrement, Value: 1}, true
	case contracts.MetricEarned:
		v, ok := NumericProperty(f.Properties[b.Activity.Property])
		if !ok || v <= 0 {
			return Op{}, false
		}
		return Op{Kind: OpIncrement, Value: v}, true
	}
	return Op{}, false
}

// NumericProperty reads a JSON property as a whole number: numbers and
// numeric strings, rounded half away from zero. Anything else (missing,
// bool, object, NaN, beyond ±1e15) is not a number.
func NumericProperty(v any) (int64, bool) {
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case float32:
		f = float64(x)
	case int:
		f = float64(x)
	case int64:
		f = float64(x)
	case int32:
		f = float64(x)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return 0, false
		}
		f = parsed
	default:
		if n, ok := v.(interface{ Float64() (float64, error) }); ok {
			parsed, err := n.Float64()
			if err != nil {
				return 0, false
			}
			f = parsed
			break
		}
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > maxActivityValue {
		return 0, false
	}
	return int64(math.Round(f)), true
}
