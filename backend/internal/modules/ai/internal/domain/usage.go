package domain

import "time"

// UsageDay is one tenant's AI consumption on one UTC day.
type UsageDay struct {
	Day          time.Time // midnight UTC
	Requests     int64
	InputTokens  int64
	OutputTokens int64
}

// DayOf truncates t to its UTC day.
func DayOf(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Remaining is the day's headroom under limit; limit <= 0 means unlimited (-1).
func Remaining(limit int, used int64) int64 {
	if limit <= 0 {
		return -1
	}
	if r := int64(limit) - used; r > 0 {
		return r
	}
	return 0
}
