package domain

import (
	"math"
	"strings"
	"time"
)

const (
	// MaxRangeDays bounds every query (inclusive day count).
	MaxRangeDays = 366
	// DefaultRangeDays is used when from is omitted.
	DefaultRangeDays = 30
	MaxWeeks         = 52
	MinFunnelSteps   = 2
	MaxFunnelSteps   = 10
	maxStepLen       = 100
)

// Range is an inclusive span of UTC days.
type Range struct {
	From time.Time
	To   time.Time
}

// NewRange resolves optional bounds: to defaults to today, from to
// DefaultRangeDays days ending at to. Both are truncated to UTC days.
func NewRange(from, to *time.Time, today time.Time) (Range, error) {
	r := Range{To: DayOf(today)}
	if to != nil {
		r.To = DayOf(*to)
	}
	r.From = r.To.AddDate(0, 0, -(DefaultRangeDays - 1))
	if from != nil {
		r.From = DayOf(*from)
	}
	if r.From.After(r.To) {
		return Range{}, ErrRangeInverted
	}
	if r.Days() > MaxRangeDays {
		return Range{}, ErrRangeTooLarge
	}
	return r, nil
}

// Days is the inclusive number of days in the range.
func (r Range) Days() int { return int(r.To.Sub(r.From).Hours()/24) + 1 }

// EachDay lists every day of the range in order.
func (r Range) EachDay() []time.Time {
	out := make([]time.Time, 0, r.Days())
	for d := r.From; !d.After(r.To); d = d.AddDate(0, 0, 1) {
		out = append(out, d)
	}
	return out
}

// Trailing is the range of n days ending at day.
func Trailing(day time.Time, n int) Range {
	return Range{From: DayOf(day).AddDate(0, 0, -(n - 1)), To: DayOf(day)}
}

// ParseCohort validates the retention parameters.
func ParseCohort(cohort string, weeks int) error {
	if cohort != "" && cohort != "week" {
		return ErrBadCohort
	}
	if weeks < 1 || weeks > MaxWeeks {
		return ErrBadWeeks
	}
	return nil
}

// ParseSteps splits a comma-separated funnel definition.
func ParseSteps(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || len(p) > maxStepLen {
			return nil, ErrBadSteps
		}
		out = append(out, p)
	}
	if len(out) < MinFunnelSteps || len(out) > MaxFunnelSteps {
		return nil, ErrBadSteps
	}
	return out, nil
}

// Pct is part/whole as a percentage rounded to two decimals (0 when whole
// is 0).
func Pct(part, whole int64) float64 {
	if whole <= 0 {
		return 0
	}
	return math.Round(float64(part)*10000/float64(whole)) / 100
}
