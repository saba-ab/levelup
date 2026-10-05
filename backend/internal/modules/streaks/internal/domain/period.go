package domain

import "time"

// Period is the bucket size of a streak. Buckets are CALENDAR periods in the
// tenant's timezone (fixes S2: Laravel used a rolling now-minus-N-days window
// in the app timezone, so a daily streak broke at 24h+1s).
type Period string

const (
	Daily   Period = "daily"
	Weekly  Period = "weekly"  // ISO week, Monday start
	Monthly Period = "monthly" // calendar month (Laravel: rolling 30 days)
)

func ParsePeriod(s string) (Period, error) {
	switch p := Period(s); p {
	case Daily, Weekly, Monthly:
		return p, nil
	}
	return "", ErrBadPeriod
}

// Start returns the bucket containing instant t as seen in loc. The bucket is
// identified by its CIVIL start date, encoded as midnight UTC of that date:
// "2026-10-05 in Asia/Tbilisi" is stored as 2026-10-05T00:00:00Z. Encoding
// the civil date (not the local instant) keeps buckets comparable even if a
// tenant later changes timezone.
func (p Period) Start(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := t.In(loc).Date()
	civil := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	switch p {
	case Weekly:
		offset := (int(civil.Weekday()) + 6) % 7 // Monday = 0
		return civil.AddDate(0, 0, -offset)
	case Monthly:
		return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	default:
		return civil
	}
}

// Index numbers buckets so consecutive buckets differ by exactly one. start
// must be a value returned by Start.
func (p Period) Index(start time.Time) int64 {
	switch p {
	case Monthly:
		return int64(start.Year())*12 + int64(start.Month()) - 1
	case Weekly:
		return floorDiv(daysSinceEpoch(start)+3, 7) // 1970-01-01 was a Thursday
	default:
		return daysSinceEpoch(start)
	}
}

// Shift moves a bucket start by n buckets (n may be negative).
func (p Period) Shift(start time.Time, n int) time.Time {
	switch p {
	case Monthly:
		return start.AddDate(0, n, 0)
	case Weekly:
		return start.AddDate(0, 0, 7*n)
	default:
		return start.AddDate(0, 0, n)
	}
}

func daysSinceEpoch(t time.Time) int64 {
	return floorDiv(t.Unix(), 86400)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// DateKey renders a bucket start as YYYY-MM-DD for derived keys.
func DateKey(start time.Time) string { return start.UTC().Format(time.DateOnly) }
