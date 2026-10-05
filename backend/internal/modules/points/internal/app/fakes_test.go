package app

import (
	"context"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/modules/points/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

// fakeRepo is an in-memory Repository. snapshot/restore let the test tx
// runner roll back like a real transaction would.
type fakeRepo struct {
	mu         sync.Mutex
	wallets    map[string]domain.Wallet // key tenant|player
	entries    []domain.LedgerEntry
	rejections []domain.Rejection
}

func newFakeRepo() *fakeRepo { return &fakeRepo{wallets: map[string]domain.Wallet{}} }

type repoState struct {
	wallets    map[string]domain.Wallet
	entries    []domain.LedgerEntry
	rejections []domain.Rejection
}

func (f *fakeRepo) snapshot() repoState {
	f.mu.Lock()
	defer f.mu.Unlock()
	ws := make(map[string]domain.Wallet, len(f.wallets))
	for k, v := range f.wallets {
		ws[k] = v
	}
	return repoState{
		wallets:    ws,
		entries:    append([]domain.LedgerEntry(nil), f.entries...),
		rejections: append([]domain.Rejection(nil), f.rejections...),
	}
}

func (f *fakeRepo) restore(s repoState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.wallets, f.entries, f.rejections = s.wallets, s.entries, s.rejections
}

func wkey(tenantID, playerID string) string { return tenantID + "|" + playerID }

func (f *fakeRepo) EnsureWallet(_ context.Context, _ *gorm.DB, w domain.Wallet) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := wkey(w.TenantID, w.PlayerID)
	if _, ok := f.wallets[k]; ok {
		return false, nil
	}
	f.wallets[k] = w
	return true, nil
}

func (f *fakeRepo) WalletForUpdate(_ context.Context, _ *gorm.DB, tenantID, playerID string) (domain.Wallet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.wallets[wkey(tenantID, playerID)]
	if !ok {
		return domain.Wallet{}, domain.ErrWalletNotFound
	}
	return w, nil
}

func (f *fakeRepo) WalletsForUpdate(_ context.Context, _ *gorm.DB, tenantID string, playerIDs []string) ([]domain.Wallet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Wallet
	for _, p := range playerIDs {
		if w, ok := f.wallets[wkey(tenantID, p)]; ok {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeRepo) SaveWallet(_ context.Context, _ *gorm.DB, w domain.Wallet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := wkey(w.TenantID, w.PlayerID)
	cur, ok := f.wallets[k]
	if !ok || cur.Version != w.Version {
		return domain.ErrVersionConflict
	}
	w.Version++
	f.wallets[k] = w
	return nil
}

func (f *fakeRepo) InsertEntries(_ context.Context, _ *gorm.DB, entries ...domain.LedgerEntry) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range entries {
		for _, have := range f.entries {
			if have.TenantID == e.TenantID && have.IdempotencyKey == e.IdempotencyKey {
				return false, nil
			}
		}
	}
	f.entries = append(f.entries, entries...)
	return true, nil
}

func (f *fakeRepo) InsertRejection(_ context.Context, _ *gorm.DB, r domain.Rejection) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, have := range f.rejections {
		if have.TenantID == r.TenantID && have.IdempotencyKey == r.IdempotencyKey {
			return false, nil
		}
	}
	f.rejections = append(f.rejections, r)
	return true, nil
}

func (f *fakeRepo) EntryByKey(_ context.Context, _ *gorm.DB, tenantID, key string) (domain.LedgerEntry, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.entries {
		if e.TenantID == tenantID && e.IdempotencyKey == key {
			return e, true, nil
		}
	}
	return domain.LedgerEntry{}, false, nil
}

func (f *fakeRepo) RejectionByKey(_ context.Context, _ *gorm.DB, tenantID, key string) (domain.Rejection, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rejections {
		if r.TenantID == tenantID && r.IdempotencyKey == key {
			return r, true, nil
		}
	}
	return domain.Rejection{}, false, nil
}

func (f *fakeRepo) RefundOf(_ context.Context, _ *gorm.DB, tenantID, debitEntryID string) (domain.LedgerEntry, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.entries {
		if e.TenantID == tenantID && e.ReversalOf == debitEntryID {
			return e, true, nil
		}
	}
	return domain.LedgerEntry{}, false, nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k, w := range f.wallets {
		if w.TenantID == tenantID {
			delete(f.wallets, k)
		}
	}
	keepE := f.entries[:0:0]
	for _, e := range f.entries {
		if e.TenantID != tenantID {
			keepE = append(keepE, e)
		}
	}
	keepR := f.rejections[:0:0]
	for _, r := range f.rejections {
		if r.TenantID != tenantID {
			keepR = append(keepR, r)
		}
	}
	f.entries, f.rejections = keepE, keepR
	return nil
}

func (f *fakeRepo) WalletByPlayer(_ context.Context, tenantID, playerID string) (domain.Wallet, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.wallets[wkey(tenantID, playerID)]
	return w, ok, nil
}

func (f *fakeRepo) WalletsByPlayers(_ context.Context, tenantID string, playerIDs []string) ([]domain.Wallet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Wallet
	for _, p := range playerIDs {
		if w, ok := f.wallets[wkey(tenantID, p)]; ok {
			out = append(out, w)
		}
	}
	return out, nil
}

func (f *fakeRepo) ListEntries(_ context.Context, tenantID, playerID string, flt LedgerFilter) ([]domain.LedgerEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.LedgerEntry
	for _, e := range f.entries {
		if e.TenantID != tenantID || e.PlayerID != playerID {
			continue
		}
		if flt.Kind != "" && e.Kind != flt.Kind {
			continue
		}
		if flt.Direction != 0 && e.Direction != flt.Direction {
			continue
		}
		if !flt.BeforeAt.IsZero() {
			older := e.CreatedAt.Before(flt.BeforeAt) || (e.CreatedAt.Equal(flt.BeforeAt) && e.ID < flt.BeforeID)
			if !older {
				continue
			}
		}
		out = append(out, e)
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

func (f *fakeRepo) wallet(tenantID, playerID string) (domain.Wallet, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.wallets[wkey(tenantID, playerID)]
	return w, ok
}

type recordedEvent struct {
	topic   string
	payload any
}

type fakeOutbox struct {
	mu        sync.Mutex
	published []recordedEvent
}

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, recordedEvent{topic, payload})
	return nil
}

func (f *fakeOutbox) topics() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.published))
	for i, e := range f.published {
		out[i] = e.topic
	}
	return out
}

func (f *fakeOutbox) count(topic string) int {
	n := 0
	for _, t := range f.topics() {
		if t == topic {
			n++
		}
	}
	return n
}

func (f *fakeOutbox) last(topic string) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.published) - 1; i >= 0; i-- {
		if f.published[i].topic == topic {
			return f.published[i].payload
		}
	}
	return nil
}

// fakePlayers is a tenant-scoped player directory.
type fakePlayers struct {
	players map[string]ports.PlayerSnapshot
	err     error
	calls   int
}

func (f *fakePlayers) PlayersByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
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

// allowKeys grants only the listed permission keys.
type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

type fakeReconciler struct {
	last   time.Time
	marked time.Time
	since  time.Time
	drift  []domain.Drift
}

func (f *fakeReconciler) LastRun(context.Context) (time.Time, error) { return f.last, nil }
func (f *fakeReconciler) MarkRun(_ context.Context, at time.Time) error {
	f.marked = at
	return nil
}
func (f *fakeReconciler) FindDrift(_ context.Context, since time.Time) ([]domain.Drift, error) {
	f.since = since
	return f.drift, nil
}

var testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// newTestService wires fakes with a rollback-capable tx runner.
func newTestService(repo *fakeRepo, players ports.PlayerReader, ob *fakeOutbox, enf authz.Enforcer) (*Service, *clock.Fake) {
	clk := clock.NewFake(testNow)
	svc := NewService(repo, players, ob, enf, nil, clk, nil, nil)
	svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error {
		state := repo.snapshot()
		ob.mu.Lock()
		n := len(ob.published)
		ob.mu.Unlock()
		if err := fn(nil); err != nil {
			repo.restore(state)
			ob.mu.Lock()
			ob.published = ob.published[:n]
			ob.mu.Unlock()
			return err
		}
		return nil
	}
	return svc, clk
}

func newClock() *clock.Fake { return clock.NewFake(testNow) }
