package app

import (
	"context"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/modules/badges/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// fakeRepo is an in-memory Repository + Reconciler. It is not transactional;
// the service tests use a pass-through tx runner.
type fakeRepo struct {
	mu          sync.Mutex
	badges      map[string]domain.Badge
	holdings    map[string]domain.PlayerBadge // key player|badge
	awards      map[string]domain.Award       // key tenant|idempotency_key
	revocations []domain.Revocation
	lastRun     time.Time
	drift       []domain.Drift
	markedAt    time.Time
	sweptSince  time.Time

	// requirements projection
	stats      map[string]*fakeStats // key tenant|player
	applied    map[string]time.Time  // key tenant|event_key
	awardStats AwardStats
	statsSince time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		badges:   map[string]domain.Badge{},
		holdings: map[string]domain.PlayerBadge{},
		awards:   map[string]domain.Award{},
		stats:    map[string]*fakeStats{},
		applied:  map[string]time.Time{},
	}
}

func hk(player, badge string) string { return player + "|" + badge }
func ak(tenant, key string) string   { return tenant + "|" + key }

func (f *fakeRepo) CreateBadge(_ context.Context, _ *gorm.DB, b domain.Badge) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, other := range f.badges {
		if other.TenantID == b.TenantID && other.Slug == b.Slug && !other.Deleted() {
			return domain.ErrSlugTaken
		}
	}
	f.badges[b.ID] = b
	return nil
}

func (f *fakeRepo) live(tenantID, id string) (domain.Badge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.badges[id]
	if !ok || b.TenantID != tenantID || b.Deleted() {
		return domain.Badge{}, domain.ErrBadgeNotFound
	}
	return b, nil
}

func (f *fakeRepo) BadgeByID(_ context.Context, tenantID, id string) (domain.Badge, error) {
	return f.live(tenantID, id)
}

func (f *fakeRepo) BadgeByIDTx(_ context.Context, _ *gorm.DB, tenantID, id string) (domain.Badge, error) {
	return f.live(tenantID, id)
}

func (f *fakeRepo) BadgeByIDForUpdate(_ context.Context, _ *gorm.DB, tenantID, id string) (domain.Badge, error) {
	return f.live(tenantID, id)
}

func (f *fakeRepo) SaveBadge(_ context.Context, _ *gorm.DB, b domain.Badge) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.badges[b.ID]
	if !ok || cur.Version != b.Version {
		return domain.ErrVersionConflict
	}
	for _, other := range f.badges {
		if other.ID != b.ID && other.TenantID == b.TenantID && other.Slug == b.Slug && !other.Deleted() && !b.Deleted() {
			return domain.ErrSlugTaken
		}
	}
	b.Version++
	f.badges[b.ID] = b
	return nil
}

func (f *fakeRepo) ListBadges(_ context.Context, tenantID string, flt BadgeFilter) ([]domain.Badge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Badge
	for _, b := range f.badges {
		if b.TenantID != tenantID || b.Deleted() {
			continue
		}
		if flt.Tier != "" && string(b.Tier) != flt.Tier {
			continue
		}
		if flt.Category != "" && string(b.Category) != flt.Category {
			continue
		}
		if flt.Active != nil && b.Active != *flt.Active {
			continue
		}
		if !flt.Before.IsZero() && !beforeCursor(b, flt) {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > flt.Limit {
		out = out[:flt.Limit]
	}
	return out, nil
}

func (f *fakeRepo) BadgesByIDs(_ context.Context, tenantID string, ids []string, includeDeleted bool) ([]domain.Badge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Badge
	for _, id := range ids {
		if b, ok := f.badges[id]; ok && b.TenantID == tenantID && (includeDeleted || !b.Deleted()) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeRepo) LockOrCreatePlayerBadge(_ context.Context, _ *gorm.DB, placeholder domain.PlayerBadge) (domain.PlayerBadge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := hk(placeholder.PlayerID, placeholder.BadgeID)
	if pb, ok := f.holdings[k]; ok {
		return pb, nil
	}
	f.holdings[k] = placeholder
	return placeholder, nil
}

func (f *fakeRepo) LockPlayerBadge(ctx context.Context, _ *gorm.DB, tenantID, playerID, badgeID string) (domain.PlayerBadge, bool, error) {
	return f.PlayerBadge(ctx, tenantID, playerID, badgeID)
}

func (f *fakeRepo) PlayerBadge(_ context.Context, tenantID, playerID, badgeID string) (domain.PlayerBadge, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pb, ok := f.holdings[hk(playerID, badgeID)]
	if !ok || pb.TenantID != tenantID {
		return domain.PlayerBadge{}, false, nil
	}
	return pb, true, nil
}

func (f *fakeRepo) SavePlayerBadge(_ context.Context, _ *gorm.DB, pb domain.PlayerBadge) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := hk(pb.PlayerID, pb.BadgeID)
	cur, ok := f.holdings[k]
	if !ok || cur.Version != pb.Version {
		return domain.ErrVersionConflict
	}
	pb.Version++
	f.holdings[k] = pb
	return nil
}

func (f *fakeRepo) DeletePlayerBadge(_ context.Context, _ *gorm.DB, pb domain.PlayerBadge) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.holdings, hk(pb.PlayerID, pb.BadgeID))
	return nil
}

func (f *fakeRepo) ListPlayerBadges(_ context.Context, tenantID, playerID string, _ PageCursor, limit int) ([]domain.PlayerBadge, error) {
	rows, _ := f.PlayerBadgesByPlayerIDs(context.Background(), tenantID, []string{playerID})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func (f *fakeRepo) PlayerBadgesByPlayerIDs(_ context.Context, tenantID string, playerIDs []string) ([]domain.PlayerBadge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := map[string]bool{}
	for _, id := range playerIDs {
		want[id] = true
	}
	var out []domain.PlayerBadge
	for _, pb := range f.holdings {
		if pb.TenantID == tenantID && want[pb.PlayerID] && pb.EarnedCount > 0 {
			out = append(out, pb)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (f *fakeRepo) AwardByKey(_ context.Context, tenantID, key string) (domain.Award, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.awards[ak(tenantID, key)]
	return a, ok, nil
}

func (f *fakeRepo) AwardByKeyTx(ctx context.Context, _ *gorm.DB, tenantID, key string) (domain.Award, bool, error) {
	return f.AwardByKey(ctx, tenantID, key)
}

func (f *fakeRepo) InsertAward(_ context.Context, _ *gorm.DB, a domain.Award) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := ak(a.TenantID, a.IdempotencyKey)
	if _, ok := f.awards[k]; ok {
		return false, nil
	}
	f.awards[k] = a
	return true, nil
}

func (f *fakeRepo) InsertRevocation(_ context.Context, _ *gorm.DB, r domain.Revocation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revocations = append(f.revocations, r)
	return nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, v := range f.badges {
		if v.TenantID == tenantID {
			delete(f.badges, k)
		}
	}
	for k, v := range f.holdings {
		if v.TenantID == tenantID {
			delete(f.holdings, k)
		}
	}
	for k, v := range f.awards {
		if v.TenantID == tenantID {
			delete(f.awards, k)
		}
	}
	return nil
}

func (f *fakeRepo) LastRun(context.Context) (time.Time, error) { return f.lastRun, nil }

func (f *fakeRepo) MarkRun(_ context.Context, at time.Time) error {
	f.markedAt = at
	return nil
}

func (f *fakeRepo) DriftSince(_ context.Context, since time.Time) ([]domain.Drift, error) {
	f.sweptSince = since
	return f.drift, nil
}

func (f *fakeRepo) appliedAwards() []domain.Award {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Award
	for _, a := range f.awards {
		if a.Applied() {
			out = append(out, a)
		}
	}
	return out
}

// fakeOutbox records topics and payloads.
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

func (f *fakeOutbox) topics() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.published))
	for i, r := range f.published {
		out[i] = r.topic
	}
	return out
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

// fakePlayers is the PlayerReader port.
type fakePlayers struct {
	players map[string]ports.PlayerSnapshot
	// external maps external id → player id for ports.ExternalIDResolver.
	external map[string]string
	err      error
	calls    int
}

func (f *fakePlayers) IDByExternalID(_ context.Context, tenantID, externalID string) (string, bool, error) {
	pid, ok := f.external[externalID]
	if !ok || f.players[pid].TenantID != tenantID {
		return "", false, nil
	}
	return pid, true, nil
}

func (f *fakePlayers) ByID(ctx context.Context, tenantID, playerID string) (ports.PlayerSnapshot, bool, error) {
	got, err := f.ByIDs(ctx, tenantID, []string{playerID})
	if err != nil {
		return ports.PlayerSnapshot{}, false, err
	}
	snap, ok := got[playerID]
	return snap, ok, nil
}

func (f *fakePlayers) ByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]ports.PlayerSnapshot{}
	for _, id := range ids {
		if p, ok := f.players[id]; ok && p.TenantID == tenantID {
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

func beforeCursor(b domain.Badge, flt BadgeFilter) bool {
	if b.CreatedAt.Equal(flt.Before) {
		return b.ID < flt.BeforeID
	}
	return b.CreatedAt.Before(flt.Before)
}
