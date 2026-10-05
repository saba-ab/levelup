package app

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/modules/progression/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

var testNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// fakeRepo is an in-memory Repository + Reconciler. The pass-through tx
// runner gives it no rollback, which the tests do not rely on.
type fakeRepo struct {
	mu         sync.Mutex
	levels     map[string]domain.Level // id → level
	deleted    map[string]time.Time
	grants     []domain.XPGrant
	progress   map[string]domain.Progress // tenant|player
	rewards    map[string]domain.LevelReward
	rejections map[string]domain.GrantRejection
	lastRun    time.Time
	saves      int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		levels:     map[string]domain.Level{},
		deleted:    map[string]time.Time{},
		progress:   map[string]domain.Progress{},
		rewards:    map[string]domain.LevelReward{},
		rejections: map[string]domain.GrantRejection{},
	}
}

func pk(a, b string) string { return a + "|" + b }

func (f *fakeRepo) LockLadder(context.Context, *gorm.DB, string) error { return nil }

func (f *fakeRepo) Ladder(_ context.Context, _ *gorm.DB, tenantID string, includeInactive bool) (domain.Ladder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Level
	for id, l := range f.levels {
		if l.TenantID != tenantID {
			continue
		}
		if _, gone := f.deleted[id]; gone {
			continue
		}
		if !includeInactive && !l.Active {
			continue
		}
		out = append(out, l)
	}
	return domain.NewLadder(out), nil
}

func (f *fakeRepo) LevelByID(_ context.Context, tenantID, levelID string) (domain.Level, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.levels[levelID]
	if _, gone := f.deleted[levelID]; !ok || gone || l.TenantID != tenantID {
		return domain.Level{}, domain.ErrLevelNotFound
	}
	return l, nil
}

func (f *fakeRepo) CreateLevel(_ context.Context, _ *gorm.DB, l domain.Level) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.levels[l.ID] = l
	return nil
}

func (f *fakeRepo) UpdateLevel(_ context.Context, _ *gorm.DB, l domain.Level) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.levels[l.ID]; !ok {
		return domain.ErrLevelNotFound
	}
	f.levels[l.ID] = l
	return nil
}

func (f *fakeRepo) SoftDeleteLevel(_ context.Context, _ *gorm.DB, tenantID, levelID string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.levels[levelID]
	if !ok || l.TenantID != tenantID {
		return domain.ErrLevelNotFound
	}
	f.deleted[levelID] = at
	return nil
}

func (f *fakeRepo) InsertGrant(_ context.Context, _ *gorm.DB, g domain.XPGrant) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.grants {
		if x.TenantID == g.TenantID && x.IdempotencyKey == g.IdempotencyKey {
			return false, nil
		}
	}
	f.grants = append(f.grants, g)
	return true, nil
}

func (f *fakeRepo) GrantByKey(_ context.Context, _ *gorm.DB, tenantID, key string) (domain.XPGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.grants {
		if x.TenantID == tenantID && x.IdempotencyKey == key {
			return x, nil
		}
	}
	return domain.XPGrant{}, errs.New(errs.NotFound, "xp grant not found")
}

func (f *fakeRepo) ListGrants(_ context.Context, tenantID, playerID string, before time.Time, beforeID string, limit int) ([]domain.XPGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.XPGrant
	for _, g := range f.grants {
		if g.TenantID == tenantID && g.PlayerID == playerID {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if !before.IsZero() {
		var page []domain.XPGrant
		for _, g := range out {
			if g.CreatedAt.Before(before) || (g.CreatedAt.Equal(before) && g.ID < beforeID) {
				page = append(page, g)
			}
		}
		out = page
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) EnsureProgress(_ context.Context, _ *gorm.DB, p domain.Progress) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.progress[pk(p.TenantID, p.PlayerID)]; !ok {
		f.progress[pk(p.TenantID, p.PlayerID)] = p
	}
	return nil
}

func (f *fakeRepo) ProgressForUpdate(_ context.Context, _ *gorm.DB, tenantID, playerID string) (domain.Progress, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.progress[pk(tenantID, playerID)]
	if !ok {
		return domain.Progress{}, errs.New(errs.NotFound, "player progress not found")
	}
	return p, nil
}

func (f *fakeRepo) SaveProgress(_ context.Context, _ *gorm.DB, p domain.Progress) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.progress[pk(p.TenantID, p.PlayerID)]
	if !ok || cur.Version != p.Version {
		return domain.ErrVersionConflict
	}
	p.Version++
	f.progress[pk(p.TenantID, p.PlayerID)] = p
	f.saves++
	return nil
}

func (f *fakeRepo) ProgressByPlayers(_ context.Context, tenantID string, ids []string) (map[string]domain.Progress, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]domain.Progress{}
	for _, id := range ids {
		if p, ok := f.progress[pk(tenantID, id)]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func (f *fakeRepo) InsertLevelReward(_ context.Context, _ *gorm.DB, r domain.LevelReward) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := r.TenantID + "|" + r.PlayerID + "|" + r.LevelID
	if _, ok := f.rewards[key]; ok {
		return false, nil
	}
	f.rewards[key] = r
	return true, nil
}

func (f *fakeRepo) InsertRejection(_ context.Context, _ *gorm.DB, r domain.GrantRejection) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := pk(r.TenantID, r.IdempotencyKey)
	if _, ok := f.rejections[key]; ok {
		return false, nil
	}
	f.rejections[key] = r
	return true, nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, l := range f.levels {
		if l.TenantID == tenantID {
			delete(f.levels, id)
		}
	}
	var keep []domain.XPGrant
	for _, g := range f.grants {
		if g.TenantID != tenantID {
			keep = append(keep, g)
		}
	}
	f.grants = keep
	for k, p := range f.progress {
		if p.TenantID == tenantID {
			delete(f.progress, k)
		}
	}
	for k, r := range f.rewards {
		if r.TenantID == tenantID {
			delete(f.rewards, k)
		}
	}
	for k, r := range f.rejections {
		if r.TenantID == tenantID {
			delete(f.rejections, k)
		}
	}
	return nil
}

// Reconciler.

func (f *fakeRepo) LastRun(context.Context) (time.Time, error) { return f.lastRun, nil }

func (f *fakeRepo) MarkRun(_ context.Context, at time.Time) error {
	f.lastRun = at
	return nil
}

func (f *fakeRepo) XPDrift(_ context.Context, since time.Time) ([]XPDrift, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []XPDrift
	for _, p := range f.progress {
		if p.UpdatedAt.Before(since) {
			continue
		}
		var sum int64
		for _, g := range f.grants {
			if g.TenantID == p.TenantID && g.PlayerID == p.PlayerID {
				sum += g.Amount
			}
		}
		if sum != p.TotalXP {
			out = append(out, XPDrift{TenantID: p.TenantID, PlayerID: p.PlayerID, TotalXP: p.TotalXP, LedgerXP: sum})
		}
	}
	return out, nil
}

func (f *fakeRepo) ProgressCandidates(_ context.Context, _ time.Time, afterTenant, afterPlayer string, limit int) ([]domain.Progress, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []domain.Progress
	for _, p := range f.progress {
		all = append(all, p)
	}
	sort.Slice(all, func(i, j int) bool {
		return pk(all[i].TenantID, all[i].PlayerID) < pk(all[j].TenantID, all[j].PlayerID)
	})
	var out []domain.Progress
	for _, p := range all {
		if afterTenant != "" && pk(p.TenantID, p.PlayerID) <= pk(afterTenant, afterPlayer) {
			continue
		}
		out = append(out, p)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

type recorded struct {
	topic   string
	payload any
}

type fakeOutbox struct {
	mu        sync.Mutex
	published []recorded
}

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, recorded{topic, payload})
	return nil
}

func (f *fakeOutbox) byTopic(topic string) []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []any
	for _, r := range f.published {
		if r.topic == topic {
			out = append(out, r.payload)
		}
	}
	return out
}

type fakePlayers struct {
	players map[string]ports.PlayerSnapshot // tenant|id
}

func newFakePlayers() *fakePlayers { return &fakePlayers{players: map[string]ports.PlayerSnapshot{}} }

func (f *fakePlayers) add(tenantID, id string, active bool) {
	f.players[pk(tenantID, id)] = ports.PlayerSnapshot{ID: id, TenantID: tenantID, Active: active}
}

func (f *fakePlayers) ByID(_ context.Context, tenantID, id string) (ports.PlayerSnapshot, error) {
	p, ok := f.players[pk(tenantID, id)]
	if !ok {
		return ports.PlayerSnapshot{}, errs.New(errs.NotFound, "player not found")
	}
	return p, nil
}

func (f *fakePlayers) ByIDs(ctx context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, id := range ids {
		if p, err := f.ByID(ctx, tenantID, id); err == nil {
			out[id] = p
		}
	}
	return out, nil
}

// allowKeys enforces only the listed permission keys.
type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

var (
	adminKeys = allowKeys{
		"progression:view_any": true, "progression:view": true, "progression:create": true,
		"progression:update": true, "progression:delete": true, "progression:grant_xp": true,
	}
	memberKeys = allowKeys{"progression:view_any": true, "progression:view": true, "progression:grant_xp": true}
)

type harness struct {
	svc     *Service
	repo    *fakeRepo
	outbox  *fakeOutbox
	players *fakePlayers
	clock   *clock.Fake
	drift   map[string]int
}

func newHarness(t *testing.T, enf authz.Enforcer, opts Options) *harness {
	t.Helper()
	h := &harness{
		repo:    newFakeRepo(),
		outbox:  &fakeOutbox{},
		players: newFakePlayers(),
		clock:   clock.NewFake(testNow),
		drift:   map[string]int{},
	}
	opts.Drift = func(kind string, n int) { h.drift[kind] += n }
	h.svc = NewService(h.repo, h.players, h.outbox, enf, nil, h.clock, nil, opts)
	h.svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
	return h
}

func asUser(tenantID string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "user-1", TenantID: tenantID, RoleIDs: []int64{2}})
}

// seedLevel stores a level directly, bypassing the service.
func (h *harness) seedLevel(tenantID, id string, number int, xp, points int64, badge string) domain.Level {
	l := domain.Level{
		ID: id, TenantID: tenantID, Number: number, Name: id, XPRequired: xp,
		PointsReward: points, BadgeRewardID: badge, Active: true, CreatedAt: testNow, UpdatedAt: testNow,
	}
	h.repo.levels[id] = l
	return l
}
