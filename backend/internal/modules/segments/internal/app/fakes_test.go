package app

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/segments/internal/domain"
	"levelup/internal/modules/segments/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

const (
	tenantA = "0198d000-0000-7000-8000-00000000000a"
	tenantB = "0198d000-0000-7000-8000-00000000000b"
)

var t0 = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

// ---- repository ----

type memberRow struct {
	tenant  string
	addedAt time.Time
	runID   string
}

type fakeRepo struct {
	segs    map[string]domain.Segment
	deleted map[string]bool
	members map[string]map[string]memberRow // segment → player → row
	marked  []string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{segs: map[string]domain.Segment{}, deleted: map[string]bool{}, members: map[string]map[string]memberRow{}}
}

func (f *fakeRepo) live(tenantID, id string) (domain.Segment, bool) {
	s, ok := f.segs[id]
	return s, ok && s.TenantID == tenantID && !f.deleted[id]
}

func (f *fakeRepo) CreateSegment(_ context.Context, _ *gorm.DB, s domain.Segment) error {
	for id, x := range f.segs {
		if !f.deleted[id] && x.TenantID == s.TenantID && x.Name == s.Name {
			return domain.ErrNameTaken
		}
	}
	f.segs[s.ID] = s
	return nil
}

func (f *fakeRepo) SaveSegment(_ context.Context, _ *gorm.DB, s domain.Segment) error {
	cur, ok := f.live(s.TenantID, s.ID)
	if !ok || cur.Version != s.Version {
		return domain.ErrVersionConflict
	}
	s.Version++
	s.RefreshLeaseUntil, s.RefreshRunID = cur.RefreshLeaseUntil, cur.RefreshRunID
	f.segs[s.ID] = s
	return nil
}

func (f *fakeRepo) SoftDeleteSegment(_ context.Context, _ *gorm.DB, tenantID, id string, _ time.Time) error {
	if _, ok := f.live(tenantID, id); !ok {
		return domain.ErrSegmentNotFound
	}
	f.deleted[id] = true
	delete(f.members, id)
	return nil
}

func (f *fakeRepo) SegmentByID(_ context.Context, tenantID, id string) (domain.Segment, error) {
	s, ok := f.live(tenantID, id)
	if !ok {
		return domain.Segment{}, domain.ErrSegmentNotFound
	}
	return s, nil
}

func (f *fakeRepo) ListSegments(_ context.Context, tenantID string, p Page) ([]domain.Segment, error) {
	var out []domain.Segment
	for id, s := range f.segs {
		if s.TenantID == tenantID && !f.deleted[id] {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > p.Limit {
		out = out[:p.Limit]
	}
	return out, nil
}

func (f *fakeRepo) LiveSegmentsAfter(_ context.Context, afterID string, limit int) ([]SegmentRef, error) {
	var out []SegmentRef
	for id, s := range f.segs {
		if !f.deleted[id] && id > afterID {
			out = append(out, SegmentRef{TenantID: s.TenantID, ID: id})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) AcquireRefresh(_ context.Context, tenantID, segmentID, runID string, now, until time.Time) (domain.Segment, bool, error) {
	s, ok := f.live(tenantID, segmentID)
	if !ok {
		return domain.Segment{}, false, domain.ErrSegmentNotFound
	}
	if s.RefreshLeaseUntil != nil && s.RefreshLeaseUntil.After(now) {
		return s, false, nil
	}
	s.RefreshRunID, s.RefreshLeaseUntil = runID, &until
	f.segs[segmentID] = s
	return s, true, nil
}

func (f *fakeRepo) ExtendLease(_ context.Context, _ *gorm.DB, segmentID, runID string, until time.Time) (bool, error) {
	s, ok := f.segs[segmentID]
	if !ok || f.deleted[segmentID] || s.RefreshRunID != runID {
		return false, nil
	}
	s.RefreshLeaseUntil = &until
	f.segs[segmentID] = s
	return true, nil
}

func (f *fakeRepo) ApplyPage(_ context.Context, _ *gorm.DB, tenantID, segmentID, runID string, pageIDs, matched []string, now time.Time) (PageResult, error) {
	res := PageResult{}
	if f.members[segmentID] == nil {
		f.members[segmentID] = map[string]memberRow{}
	}
	m := f.members[segmentID]
	isMatch := map[string]bool{}
	for _, p := range matched {
		isMatch[p] = true
		row, ok := m[p]
		if !ok {
			res.Added = append(res.Added, p)
			row = memberRow{tenant: tenantID, addedAt: now}
		}
		row.runID = runID
		m[p] = row
	}
	for _, p := range pageIDs {
		if _, ok := m[p]; ok && !isMatch[p] {
			delete(m, p)
			res.Removed = append(res.Removed, p)
		}
	}
	return res, nil
}

func (f *fakeRepo) SweepStale(_ context.Context, _ *gorm.DB, segmentID, runID string, limit int) ([]string, error) {
	var out []string
	for p, row := range f.members[segmentID] {
		if len(out) == limit {
			break
		}
		if row.runID != runID {
			delete(f.members[segmentID], p)
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeRepo) FinishRefresh(_ context.Context, segmentID, runID string, now time.Time) error {
	s := f.segs[segmentID]
	if s.RefreshRunID != runID {
		return nil
	}
	s.MemberCount = len(f.members[segmentID])
	s.LastRefreshedAt = &now
	s.RefreshLeaseUntil = nil
	f.segs[segmentID] = s
	return nil
}

func (f *fakeRepo) ListMembers(_ context.Context, tenantID, segmentID string, p Page) ([]Member, error) {
	var out []Member
	for pid, row := range f.members[segmentID] {
		if row.tenant == tenantID {
			out = append(out, Member{SegmentID: segmentID, PlayerID: pid, AddedAt: row.addedAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PlayerID > out[j].PlayerID })
	if p.BeforeID != "" {
		kept := out[:0]
		for _, m := range out {
			if m.PlayerID < p.BeforeID {
				kept = append(kept, m)
			}
		}
		out = kept
	}
	if len(out) > p.Limit {
		out = out[:p.Limit]
	}
	return out, nil
}

func (f *fakeRepo) RemovePlayer(_ context.Context, _ *gorm.DB, tenantID, playerID string) ([]string, error) {
	var segs []string
	for segID, m := range f.members {
		if row, ok := m[playerID]; ok && row.tenant == tenantID {
			delete(m, playerID)
			segs = append(segs, segID)
		}
	}
	return segs, nil
}

func (f *fakeRepo) SegmentsOfPlayers(_ context.Context, tenantID string, playerIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	for segID, m := range f.members {
		for _, p := range playerIDs {
			if row, ok := m[p]; ok && row.tenant == tenantID && !f.deleted[segID] {
				out[p] = append(out[p], segID)
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) MarkRun(_ context.Context, job string, _ time.Time) error {
	f.marked = append(f.marked, job)
	return nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	for id, s := range f.segs {
		if s.TenantID == tenantID {
			delete(f.segs, id)
			delete(f.members, id)
		}
	}
	return nil
}

// ---- outbox ----

type recorded struct {
	topic   string
	payload any
}

type fakeOutbox struct{ published []recorded }

func (o *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	o.published = append(o.published, recorded{topic, payload})
	return nil
}

func (o *fakeOutbox) topics() []string {
	out := make([]string, len(o.published))
	for i, p := range o.published {
		out[i] = p.topic
	}
	return out
}

func (o *fakeOutbox) reset() { o.published = nil }

// ---- ports ----

type fakePlayers struct {
	players map[string]ports.PlayerSnapshot // all tenants
	calls   int
	onList  func() // runs on every ListPlayerIDs call
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

func (f *fakePlayers) ListPlayerIDs(_ context.Context, tenantID, afterID string, limit int) ([]string, error) {
	f.calls++
	if f.onList != nil {
		f.onList()
	}
	var ids []string
	for id, p := range f.players {
		if p.TenantID == tenantID && id > afterID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func (f *fakePlayers) add(tenantID string, n int, attrs map[string]any) []string {
	ids := make([]string, n)
	for i := range n {
		id := fmt.Sprintf("%s-p%04d", tenantID[len(tenantID)-1:], len(f.players))
		f.players[id] = ports.PlayerSnapshot{ID: id, TenantID: tenantID, ExternalID: "ext-" + id, DisplayName: "P " + id,
			Active: true, Attributes: attrs, CreatedAt: t0.Add(-24 * time.Hour)}
		ids[i] = id
	}
	return ids
}

type fakeLevels struct {
	levels map[string]int
	calls  int
}

func (f *fakeLevels) LevelsByPlayerIDs(_ context.Context, _ string, ids []string) (map[string]int, error) {
	f.calls++
	out := map[string]int{}
	for _, id := range ids {
		if l, ok := f.levels[id]; ok {
			out[id] = l
		}
	}
	return out, nil
}

type fakeWallets struct {
	calls int
	err   error
}

func (f *fakeWallets) WalletsByPlayerIDs(context.Context, string, []string) (map[string]ports.WalletSnapshot, error) {
	f.calls++
	return map[string]ports.WalletSnapshot{}, f.err
}

type fakeBadges struct{ calls int }

func (f *fakeBadges) EarnedBadges(context.Context, string, []string) (map[string][]string, error) {
	f.calls++
	return map[string][]string{}, nil
}

type fakeActivity struct {
	seen  map[string]time.Time
	calls int
}

func (f *fakeActivity) LastSeen(_ context.Context, _ string, ids []string) (map[string]time.Time, error) {
	f.calls++
	out := map[string]time.Time{}
	for _, id := range ids {
		if t, ok := f.seen[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}

// ---- authz ----

type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

// ---- harness ----

type harness struct {
	svc      *Service
	repo     *fakeRepo
	ob       *fakeOutbox
	players  *fakePlayers
	levels   *fakeLevels
	wallets  *fakeWallets
	badges   *fakeBadges
	activity *fakeActivity
	clock    *clock.Fake
}

func newHarness(enf authz.Enforcer, pageSize int) *harness {
	h := &harness{
		repo: newFakeRepo(), ob: &fakeOutbox{},
		players: &fakePlayers{players: map[string]ports.PlayerSnapshot{}},
		levels:  &fakeLevels{levels: map[string]int{}}, wallets: &fakeWallets{}, badges: &fakeBadges{},
		activity: &fakeActivity{seen: map[string]time.Time{}}, clock: clock.NewFake(t0),
	}
	h.svc = NewService(h.repo, Readers{
		Players: h.players, Progress: h.levels, Wallets: h.wallets, Badges: h.badges, Activity: h.activity,
	}, h.ob, enf, nil, h.clock, Settings{PageSize: pageSize, Lease: 10 * time.Minute, PreviewLimit: 1000})
	h.svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
	return h
}

func asTenant(tenantID string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "u1", TenantID: tenantID, RoleIDs: []int64{2}})
}
