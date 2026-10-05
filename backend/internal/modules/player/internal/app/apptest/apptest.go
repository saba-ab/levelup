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
		if f.HasAfter {
			if p.CreatedAt.After(f.After) || (p.CreatedAt.Equal(f.After) && p.ID >= f.AfterID) {
				return false
			}
		}
		return true
	})
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].ID > rows[j].ID
	})
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
	}
	return rows, nil
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
