// Package apptest holds hand-written test doubles for player's service:
// an in-memory repository with the same tenant / soft-delete / live-unique
// semantics as the Postgres one, a recording outbox and an allow-list
// enforcer. Test-only by convention; nothing in production imports it.
package apptest

import (
	"context"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"

	"levelup/internal/modules/player/contracts"
	"levelup/internal/modules/player/internal/app"
	"levelup/internal/modules/player/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// TxState lets the fakes observe whether a call happens inside the
// service's transaction runner.
type TxState struct {
	mu   sync.Mutex
	open bool
}

func (t *TxState) Set(open bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.open = open
}

func (t *TxState) Open() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.open
}

// Eviction is one recorded Evict call.
type Eviction struct {
	TenantID string
	Refs     []app.Ref
	InTx     bool
}

type Repo struct {
	mu        sync.Mutex
	Rows      map[string]domain.Player
	Evictions []Eviction
	Tx        *TxState
}

func NewRepo(tx *TxState) *Repo {
	return &Repo{Rows: map[string]domain.Player{}, Tx: tx}
}

func clone(p domain.Player) domain.Player {
	p.Attributes = maps.Clone(p.Attributes)
	return p
}

func (r *Repo) Create(_ context.Context, _ *gorm.DB, p domain.Player) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.Rows {
		if x.TenantID == p.TenantID && x.ExternalID == p.ExternalID && x.DeletedAt == nil {
			return domain.ErrExternalIDTaken
		}
	}
	r.Rows[p.ID] = clone(p)
	return nil
}

func (r *Repo) CreateIfAbsent(_ context.Context, _ *gorm.DB, p domain.Player) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, taken := r.Rows[p.ID]; taken {
		return false, nil
	}
	for _, x := range r.Rows {
		if x.TenantID == p.TenantID && x.ExternalID == p.ExternalID && x.DeletedAt == nil {
			return false, nil
		}
	}
	r.Rows[p.ID] = clone(p)
	return true, nil
}

func (r *Repo) live(tenantID string, match func(domain.Player) bool) []domain.Player {
	var out []domain.Player
	for _, x := range r.Rows {
		if x.TenantID == tenantID && x.DeletedAt == nil && match(x) {
			out = append(out, clone(x))
		}
	}
	return out
}

func (r *Repo) one(tenantID string, match func(domain.Player) bool) (domain.Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	got := r.live(tenantID, match)
	if len(got) == 0 {
		return domain.Player{}, domain.ErrNotFound
	}
	return got[0], nil
}

func (r *Repo) ByID(_ context.Context, tenantID, id string) (domain.Player, error) {
	return r.one(tenantID, func(p domain.Player) bool { return p.ID == id })
}

func (r *Repo) ByExternalID(_ context.Context, tenantID, ext string) (domain.Player, error) {
	return r.one(tenantID, func(p domain.Player) bool { return p.ExternalID == ext })
}

func (r *Repo) ByIDForUpdate(_ context.Context, _ *gorm.DB, tenantID, id string) (domain.Player, error) {
	return r.one(tenantID, func(p domain.Player) bool { return p.ID == id })
}

func (r *Repo) Save(_ context.Context, _ *gorm.DB, p domain.Player) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.Rows[p.ID]
	if !ok || cur.TenantID != p.TenantID || cur.Version != p.Version {
		return domain.ErrVersionConflict
	}
	p = clone(p)
	p.Version++
	r.Rows[p.ID] = p
	return nil
}

func (r *Repo) List(_ context.Context, tenantID string, f app.ListFilter) ([]domain.Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	search := strings.ToLower(f.Search)
	rows := r.live(tenantID, func(p domain.Player) bool {
		if f.Active != nil && p.Active != *f.Active {
			return false
		}
		if search != "" &&
			!strings.HasPrefix(strings.ToLower(p.ExternalID), search) &&
			!strings.HasPrefix(strings.ToLower(p.DisplayName), search) &&
			!strings.HasPrefix(strings.ToLower(p.Email), search) {
			return false
		}
		if f.CreatedFrom != nil && p.CreatedAt.Before(*f.CreatedFrom) {
			return false
		}
		if f.CreatedTo != nil && !p.CreatedAt.Before(*f.CreatedTo) {
			return false
		}
		return !f.HasAfter || after(p, f)
	})
	sort.Slice(rows, func(i, j int) bool { return less(rows[i], rows[j], f.Sort) })
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
	}
	return rows, nil
}

// less is the fake's ordering for each contracts.Sort* value.
func less(a, b domain.Player, sortBy string) bool {
	switch sortBy {
	case contracts.SortCreatedAsc:
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID < b.ID
	case contracts.SortDisplayName:
		if a.SortName() != b.SortName() {
			return a.SortName() < b.SortName()
		}
		return a.ID < b.ID
	default:
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		return a.ID > b.ID
	}
}

// after reports whether p sorts strictly after the cursor.
func after(p domain.Player, f app.ListFilter) bool {
	cur := domain.Player{ID: f.AfterID, CreatedAt: f.After, ExternalID: f.AfterKey}
	return less(cur, p, f.Sort)
}

func (r *Repo) ByIDs(_ context.Context, tenantID string, ids []string) ([]domain.Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.live(tenantID, func(p domain.Player) bool { return slices.Contains(ids, p.ID) }), nil
}

func (r *Repo) ByExternalIDs(_ context.Context, tenantID string, exts []string) ([]domain.Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.live(tenantID, func(p domain.Player) bool { return slices.Contains(exts, p.ExternalID) }), nil
}

func (r *Repo) IDsAfter(_ context.Context, tenantID, afterID string, limit int) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, id := range slices.Sorted(maps.Keys(r.Rows)) {
		if len(out) == limit {
			break
		}
		if p := r.Rows[id]; p.TenantID == tenantID && !p.Deleted() && id > afterID {
			out = append(out, id)
		}
	}
	return out, nil
}

func (r *Repo) PurgeTenantBatch(_ context.Context, _ *gorm.DB, tenantID string, limit int) ([]app.Ref, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := slices.Sorted(maps.Keys(r.Rows))
	var out []app.Ref
	for _, id := range ids {
		if len(out) == limit {
			break
		}
		if p := r.Rows[id]; p.TenantID == tenantID {
			out = append(out, app.Ref{ID: p.ID, ExternalID: p.ExternalID})
			delete(r.Rows, id)
		}
	}
	return out, nil
}

func (r *Repo) Evict(_ context.Context, tenantID string, refs ...app.Ref) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inTx := r.Tx != nil && r.Tx.Open()
	r.Evictions = append(r.Evictions, Eviction{TenantID: tenantID, Refs: refs, InTx: inTx})
}

// Event is one recorded outbox publish.
type Event struct {
	Topic   string
	Payload any
	InTx    bool
}

type Outbox struct {
	mu        sync.Mutex
	Published []Event
	Tx        *TxState
	Fail      error
}

func (o *Outbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Fail != nil {
		return o.Fail
	}
	o.Published = append(o.Published, Event{Topic: topic, Payload: payload, InTx: o.Tx != nil && o.Tx.Open()})
	return nil
}

func (o *Outbox) Topics() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]string, len(o.Published))
	for i, e := range o.Published {
		out[i] = e.Topic
	}
	return out
}

// AllowKeys enforces only the listed permission keys.
type AllowKeys map[string]bool

func (a AllowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}
