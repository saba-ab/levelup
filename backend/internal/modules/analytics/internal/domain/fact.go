// Package domain holds analytics' projection rules: facts, day bucketing
// (UTC), bounded query ranges, retention cohorts and funnel steps.
package domain

import "time"

// Counter is one increment of daily_counters.
type Counter struct {
	Metric    string
	Dimension string
	Value     int64
}

// Fact is one consumed event, reduced to what the projection stores. It is
// applied at most once per EventID.
type Fact struct {
	EventID  string
	TenantID string
	Day      time.Time // UTC midnight, see DayOf
	Counters []Counter
	// PlayerID and EventType are set for activities: the player is then
	// active on Day (player_days), has done EventType on Day
	// (player_event_days, used by funnels), and is first seen no later than
	// Day (player_first_seen).
	PlayerID  string
	EventType string
}

func (f Fact) Validate() error {
	switch {
	case f.EventID == "":
		return ErrFactNoEvent
	case f.TenantID == "":
		return ErrFactNoTenant
	case f.Day.IsZero():
		return ErrFactNoDay
	}
	return nil
}

// IsActivity reports whether the fact marks a player active.
func (f Fact) IsActivity() bool { return f.PlayerID != "" && f.EventType != "" }

// DayOf truncates t to its UTC calendar day.
func DayOf(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// FirstTime returns the first non-zero time, in UTC.
func FirstTime(ts ...time.Time) time.Time {
	for _, t := range ts {
		if !t.IsZero() {
			return t.UTC()
		}
	}
	return time.Time{}
}

// WeekStart is the Monday (UTC) of t's ISO week.
func WeekStart(t time.Time) time.Time {
	d := DayOf(t)
	offset := (int(d.Weekday()) + 6) % 7 // Monday → 0, Sunday → 6
	return d.AddDate(0, 0, -offset)
}
