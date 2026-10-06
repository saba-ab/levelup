package app

import (
	"context"
	"time"

	"levelup/internal/modules/badges/contracts"
)

// StatsDays is the length of the awards-per-day series (today included).
const StatsDays = 30

// BadgeStat is one badge's award counters from the append-only ledger
// (applied awards; a later revoke does not take them back).
type BadgeStat struct {
	BadgeID       string
	Slug          string
	Name          string
	Tier          string
	Deleted       bool
	AwardedCount  int64
	UniquePlayers int64
	LastAwardedAt *time.Time
}

// DayCount is the number of applied awards on one UTC day (YYYY-MM-DD).
type DayCount struct {
	Day   string
	Count int64
}

// AwardStats is what the repository aggregates for a tenant.
type AwardStats struct {
	Badges        []BadgeStat
	PerDay        map[string]int64 // UTC day → applied awards, since the requested day
	TotalAwarded  int64
	UniquePlayers int64
}

// StatsReport is GET /badges/stats (the portal's Badge Unlock Velocity).
type StatsReport struct {
	Badges        []BadgeStat
	AwardsPerDay  []DayCount // StatsDays entries, oldest first, zero-filled
	TotalAwarded  int64
	UniquePlayers int64
}

// Stats reports per-badge award counters and the awards per day over the
// last StatsDays UTC days.
func (s *Service) Stats(ctx context.Context) (StatsReport, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermViewAny, nil)
	if err != nil {
		return StatsReport{}, err
	}
	today := s.now().UTC().Truncate(24 * time.Hour)
	first := today.AddDate(0, 0, -(StatsDays - 1))
	agg, err := s.repo.AwardStats(ctx, p.TenantID, first)
	if err != nil {
		return StatsReport{}, err
	}
	out := StatsReport{
		Badges:        agg.Badges,
		AwardsPerDay:  make([]DayCount, StatsDays),
		TotalAwarded:  agg.TotalAwarded,
		UniquePlayers: agg.UniquePlayers,
	}
	if out.Badges == nil {
		out.Badges = []BadgeStat{}
	}
	for i := range StatsDays {
		day := first.AddDate(0, 0, i).Format(time.DateOnly)
		out.AwardsPerDay[i] = DayCount{Day: day, Count: agg.PerDay[day]}
	}
	return out, nil
}
