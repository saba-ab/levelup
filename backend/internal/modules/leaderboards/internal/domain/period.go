package domain

import (
	"time"

	"levelup/internal/modules/leaderboards/contracts"
)

// AllTimeStart is the period start of reset_frequency=never boards: there is
// exactly one period and it starts at the Unix epoch.
var AllTimeStart = time.Unix(0, 0).UTC()

// PeriodStart buckets t into the period it belongs to, in UTC. Weeks are ISO
// weeks (Monday 00:00 UTC); months are calendar months.
func PeriodStart(freq string, t time.Time) time.Time {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	switch freq {
	case contracts.ResetDaily:
		return day
	case contracts.ResetWeekly:
		offset := (int(day.Weekday()) + 6) % 7 // Monday = 0
		return day.AddDate(0, 0, -offset)
	case contracts.ResetMonthly:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	default:
		return AllTimeStart
	}
}

// PeriodEnd is the exclusive end of the period starting at start. ok is false
// for never-resetting boards, whose single period never closes.
func PeriodEnd(freq string, start time.Time) (end time.Time, ok bool) {
	start = start.UTC()
	switch freq {
	case contracts.ResetDaily:
		return start.AddDate(0, 0, 1), true
	case contracts.ResetWeekly:
		return start.AddDate(0, 0, 7), true
	case contracts.ResetMonthly:
		return start.AddDate(0, 1, 0), true
	default:
		return time.Time{}, false
	}
}

// Period is one bucket of one leaderboard. It is also the identity of the
// redis read model for that bucket.
type Period struct {
	LeaderboardID string
	TenantID      string
	Start         time.Time
	End           time.Time // zero for never-resetting boards
}

// HasEnd reports whether the period closes at all.
func (p Period) HasEnd() bool { return !p.End.IsZero() }

// PeriodOf returns the period of board b that contains t.
func PeriodOf(b Leaderboard, t time.Time) Period {
	start := PeriodStart(b.ResetFrequency, t)
	end, _ := PeriodEnd(b.ResetFrequency, start)
	return Period{LeaderboardID: b.ID, TenantID: b.TenantID, Start: start, End: end}
}

// ReadModelExpiry is when the redis copy of period p may disappear: a fixed
// time after a periodic board closes, or a rolling window for all-time boards
// (the daily rebuild refreshes it). redis-core is noeviction, so every key
// carries an explicit TTL.
func ReadModelExpiry(p Period, now time.Time, closedRetention, allTimeTTL time.Duration) time.Time {
	if p.HasEnd() {
		return p.End.Add(closedRetention)
	}
	return now.Add(allTimeTTL)
}
