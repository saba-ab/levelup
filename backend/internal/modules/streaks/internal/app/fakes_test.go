package app

import (
	"context"
	"slices"
	"sync"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/streaks/internal/domain"
	"levelup/internal/modules/streaks/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// fakeRepo is an in-memory Repository. It mimics the unique keys the SQL
// schema enforces, which is what the service's idempotency relies on.
type fakeRepo struct {
	mu       sync.Mutex
	streaks  map[string]domain.Streak
	deleted  map[string]bool
	ps       map[string]domain.PlayerStreak // id → row
	periods  map[string]map[time.Time]bool  // ps id → starts
	awards   []domain.MilestoneAward
	requests map[string]domain.RecordRequest // tenant|key
	markers  map[string]time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		streaks:  map[string]domain.Streak{},
		deleted:  map[string]bool{},
		ps:       map[string]domain.PlayerStreak{},
		periods:  map[string]map[time.Time]bool{},
		requests: map[string]domain.RecordRequest{},
		markers:  map[string]time.Time{},
	}
}

var _ Repository = (*fakeRepo)(nil)

func (f *fakeRepo) CreateStreak(_ context.Context, _ *gorm.DB, s domain.Streak) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, x := range f.streaks {
		if f.deleted[id] || x.TenantID != s.TenantID {
			continue
		}
		if x.Slug == s.Slug {
			return domain.ErrSlugTaken
		}
		if x.ActivityKey == s.ActivityKey {
			return domain.ErrActivityKeyTaken
		}
	}
	f.streaks[s.ID] = s
	return nil
}

func (f *fakeRepo) SaveStreak(_ context.Context, _ *gorm.DB, s domain.Streak) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.streaks[s.ID]
	if !ok || f.deleted[s.ID] || cur.Version != s.Version {
		return domain.ErrVersionConflict
	}
	s.Version++
	f.streaks[s.ID] = s
	return nil
}

func (f *fakeRepo) SoftDeleteStreak(_ context.Context, _ *gorm.DB, tenantID, id string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.streaks[id]
	if !ok || f.deleted[id] || s.TenantID != tenantID {
		return domain.ErrStreakNotFound
	}
	f.deleted[id] = true
	return nil
}

func (f *fakeRepo) StreakByID(_ context.Context, tenantID, id string) (domain.Streak, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.streaks[id]
	if !ok || f.deleted[id] || s.TenantID != tenantID {
		return domain.Streak{}, domain.ErrStreakNotFound
	}
	return s, nil
}

func (f *fakeRepo) StreakByActivityKey(_ context.Context, tenantID, key string) (domain.Streak, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, s := range f.streaks {
		if !f.deleted[id] && s.TenantID == tenantID && s.ActivityKey == key {
			return s, nil
		}
	}
	return domain.Streak{}, domain.ErrStreakNotFound
}

func (f *fakeRepo) StreaksByIDs(_ context.Context, tenantID string, ids []string) (map[string]domain.Streak, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]domain.Streak{}
	for _, id := range ids {
		if s, ok := f.streaks[id]; ok && !f.deleted[id] && s.TenantID == tenantID {
			out[id] = s
		}
	}
	return out, nil
}

func (f *fakeRepo) ListStreaks(_ context.Context, tenantID string, flt ListFilter, p Page) ([]domain.Streak, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Streak
	for id, s := range f.streaks {
		if f.deleted[id] || s.TenantID != tenantID {
			continue
		}
		if flt.Active != nil && s.Active != *flt.Active {
			continue
		}
		if flt.Period != "" && string(s.Period) != flt.Period {
			continue
		}
		olderThanCursor := s.CreatedAt.Before(p.Before) || (s.CreatedAt.Equal(p.Before) && s.ID < p.BeforeID)
		if !p.Before.IsZero() && !olderThanCursor {
			continue
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b domain.Streak) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		if a.ID > b.ID {
			return -1
		}
		return 1
	})
	if len(out) > p.Limit {
		out = out[:p.Limit]
	}
	return out, nil
}

func (f *fakeRepo) EnsurePlayerStreak(_ context.Context, _ *gorm.DB, ps domain.PlayerStreak) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.ps {
		if x.PlayerID == ps.PlayerID && x.StreakID == ps.StreakID {
			return nil
		}
	}
	f.ps[ps.ID] = ps
	return nil
}

func (f *fakeRepo) PlayerStreakForUpdate(_ context.Context, _ *gorm.DB, tenantID, playerID, streakID string) (domain.PlayerStreak, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.ps {
		if x.TenantID == tenantID && x.PlayerID == playerID && x.StreakID == streakID {
			return x, nil
		}
	}
	return domain.PlayerStreak{}, domain.ErrPlayerStreakNotFound
}

func (f *fakeRepo) SavePlayerStreak(_ context.Context, _ *gorm.DB, ps domain.PlayerStreak) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.ps[ps.ID]
	if !ok || cur.Version != ps.Version {
		return domain.ErrVersionConflict
	}
	ps.Version++
	f.ps[ps.ID] = ps
	return nil
}

func (f *fakeRepo) PlayerStreaksByPlayers(_ context.Context, tenantID string, playerIDs []string) ([]domain.PlayerStreak, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.PlayerStreak
	for _, x := range f.ps {
		if x.TenantID == tenantID && slices.Contains(playerIDs, x.PlayerID) {
			out = append(out, x)
		}
	}
	return out, nil
}

func (f *fakeRepo) InsertPeriod(_ context.Context, _ *gorm.DB, b domain.PeriodBucket) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.periods[b.PlayerStreakID] == nil {
		f.periods[b.PlayerStreakID] = map[time.Time]bool{}
	}
	if f.periods[b.PlayerStreakID][b.PeriodStart] {
		return false, nil
	}
	f.periods[b.PlayerStreakID][b.PeriodStart] = true
	return true, nil
}

func (f *fakeRepo) PeriodStarts(_ context.Context, _ *gorm.DB, psID string) ([]time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []time.Time
	for t := range f.periods[psID] {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b time.Time) int { return a.Compare(b) })
	return out, nil
}

func (f *fakeRepo) AwardedMilestonesBetween(_ context.Context, _ *gorm.DB, psID string, from, to time.Time) (map[int]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[int]bool{}
	for _, a := range f.awards {
		if a.PlayerStreakID == psID && !a.RunStartedAt.Before(from) && !a.RunStartedAt.After(to) {
			out[a.Milestone] = true
		}
	}
	return out, nil
}

func (f *fakeRepo) InsertMilestoneAward(_ context.Context, _ *gorm.DB, a domain.MilestoneAward) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.awards {
		if x.PlayerStreakID == a.PlayerStreakID && x.Milestone == a.Milestone && x.RunStartedAt.Equal(a.RunStartedAt) {
			return false, nil
		}
	}
	f.awards = append(f.awards, a)
	return true, nil
}

func (f *fakeRepo) RequestExists(_ context.Context, tenantID, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.requests[tenantID+"|"+key]
	return ok, nil
}

func (f *fakeRepo) InsertRequest(_ context.Context, _ *gorm.DB, r domain.RecordRequest) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := r.TenantID + "|" + r.IdempotencyKey
	if _, ok := f.requests[k]; ok {
		return false, nil
	}
	f.requests[k] = r
	return true, nil
}

func (f *fakeRepo) PruneRequests(_ context.Context, before time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for k, r := range f.requests {
		if r.CreatedAt.Before(before) {
			delete(f.requests, k)
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) LiveStreakRefs(context.Context) ([]StreakRef, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := map[StreakRef]bool{}
	var out []StreakRef
	for _, x := range f.ps {
		ref := StreakRef{TenantID: x.TenantID, StreakID: x.StreakID}
		if x.CurrentCount > 0 && !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	return out, nil
}

func (f *fakeRepo) LapsedForUpdate(_ context.Context, _ *gorm.DB, tenantID, streakID string, before time.Time, limit int) ([]domain.PlayerStreak, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.PlayerStreak
	for _, x := range f.ps {
		if x.TenantID == tenantID && x.StreakID == streakID && x.CurrentCount > 0 &&
			x.LastPeriodStart != nil && x.LastPeriodStart.Before(before) && len(out) < limit {
			out = append(out, x)
		}
	}
	return out, nil
}

func (f *fakeRepo) LastRun(_ context.Context, job string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.markers[job], nil
}

func (f *fakeRepo) MarkRun(_ context.Context, job string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markers[job] = at
	return nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, s := range f.streaks {
		if s.TenantID == tenantID {
			delete(f.streaks, id)
		}
	}
	for id, x := range f.ps {
		if x.TenantID == tenantID {
			delete(f.ps, id)
			delete(f.periods, id)
		}
	}
	for k, r := range f.requests {
		if r.TenantID == tenantID {
			delete(f.requests, k)
		}
	}
	f.awards = slices.DeleteFunc(f.awards, func(a domain.MilestoneAward) bool { return a.TenantID == tenantID })
	return nil
}

func (f *fakeRepo) DeletePlayer(_ context.Context, _ *gorm.DB, tenantID, playerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, x := range f.ps {
		if x.TenantID == tenantID && x.PlayerID == playerID {
			delete(f.ps, id)
			delete(f.periods, id)
			f.awards = slices.DeleteFunc(f.awards, func(a domain.MilestoneAward) bool { return a.PlayerStreakID == id })
		}
	}
	return nil
}

// ---- other fakes ----

type recorded struct {
	topic   string
	payload any
}

type fakeOutbox struct{ published []recorded }

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.published = append(f.published, recorded{topic, payload})
	return nil
}

func (f *fakeOutbox) topics() []string {
	out := make([]string, len(f.published))
	for i, r := range f.published {
		out[i] = r.topic
	}
	return out
}

func (f *fakeOutbox) count(topic string) int {
	n := 0
	for _, r := range f.published {
		if r.topic == topic {
			n++
		}
	}
	return n
}

func (f *fakeOutbox) reset() { f.published = nil }

type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

type fakePlayers struct {
	players map[string]ports.PlayerSnapshot
}

func (f *fakePlayers) PlayersByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, id := range ids {
		if p, ok := f.players[id]; ok && p.TenantID == tenantID {
			out[id] = p
		}
	}
	return out, nil
}

func (f *fakePlayers) PlayersByExternalIDs(_ context.Context, tenantID string, ext []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, p := range f.players {
		if p.TenantID == tenantID && p.ExternalID != "" && slices.Contains(ext, p.ExternalID) {
			out[p.ExternalID] = p
		}
	}
	return out, nil
}

type fakeTenants struct{ tz map[string]string }

func (f *fakeTenants) TenantsByIDs(_ context.Context, ids []string) (map[string]ports.TenantSnapshot, error) {
	out := map[string]ports.TenantSnapshot{}
	for _, id := range ids {
		if tz, ok := f.tz[id]; ok {
			out[id] = ports.TenantSnapshot{ID: id, Timezone: tz}
		}
	}
	return out, nil
}
