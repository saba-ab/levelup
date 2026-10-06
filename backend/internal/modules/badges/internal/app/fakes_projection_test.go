package app

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/badges/internal/domain"
)

type fakeStats struct {
	domain.PlayerStats
	lifetimeAt time.Time
}

func (f *fakeRepo) MarkEventApplied(_ context.Context, _ *gorm.DB, tenantID, key string, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := ak(tenantID, key)
	if _, ok := f.applied[k]; ok {
		return false, nil
	}
	f.applied[k] = at
	return true, nil
}

func (f *fakeRepo) ApplyPlayerStats(_ context.Context, _ *gorm.DB, tenantID, playerID string, u StatsUpdate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := ak(tenantID, playerID)
	st, ok := f.stats[k]
	if !ok {
		st = &fakeStats{PlayerStats: domain.PlayerStats{TenantID: tenantID, PlayerID: playerID, ActivityCounts: map[string]int64{}}}
		f.stats[k] = st
	}
	if u.LifetimePoints != nil {
		newer := st.lifetimeAt.IsZero() || u.LifetimeAt.After(st.lifetimeAt)
		tie := u.LifetimeAt.Equal(st.lifetimeAt) && *u.LifetimePoints > st.LifetimePoints
		if newer || tie {
			st.LifetimePoints = *u.LifetimePoints
			st.lifetimeAt = u.LifetimeAt
		}
	}
	st.MissionsCompleted += u.MissionsCompleted
	st.BadgesEarned += u.BadgesEarned
	st.MaxStreak = max(st.MaxStreak, u.MaxStreak)
	st.Level = max(st.Level, u.Level)
	if u.ActivityType != "" {
		st.ActivityCounts[u.ActivityType]++
	}
	return nil
}

func (f *fakeRepo) PlayerStats(_ context.Context, tenantID, playerID string) (domain.PlayerStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := domain.PlayerStats{TenantID: tenantID, PlayerID: playerID, ActivityCounts: map[string]int64{}}
	if st, ok := f.stats[ak(tenantID, playerID)]; ok {
		out = st.PlayerStats
		out.ActivityCounts = map[string]int64{}
		for k, v := range st.ActivityCounts {
			out.ActivityCounts[k] = v
		}
	}
	return out, nil
}

func (f *fakeRepo) AutoAwardBadges(_ context.Context, tenantID string) ([]domain.Badge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Badge
	for _, b := range f.badges {
		if b.TenantID == tenantID && b.Requirements != nil && b.Active && !b.Deleted() {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeRepo) PruneAppliedEvents(_ context.Context, before time.Time, limit int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for k, at := range f.applied {
		if n >= limit {
			break
		}
		if at.Before(before) {
			delete(f.applied, k)
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) AwardStats(_ context.Context, _ string, since time.Time) (AwardStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statsSince = since
	return f.awardStats, nil
}

func (f *fakeRepo) playerStats(tenantID, playerID string) domain.PlayerStats {
	st, _ := f.PlayerStats(context.Background(), tenantID, playerID)
	return st
}
