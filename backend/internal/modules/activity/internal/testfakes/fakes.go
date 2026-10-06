// Package testfakes holds hand-written fakes shared by activity's service
// and transport tests. Only test files import it.
package testfakes

import (
	"context"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/activity/internal/app"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/modules/activity/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// Repo is an in-memory app.Repository with the same conflict and
// conditional-update semantics as the SQL.
type Repo struct {
	mu        sync.Mutex
	Rows      map[string]domain.Activity // by id
	LastRepub map[string]time.Time
	Markers   map[string]time.Time
}

func NewRepo() *Repo {
	return &Repo{Rows: map[string]domain.Activity{}, LastRepub: map[string]time.Time{}, Markers: map[string]time.Time{}}
}

var _ app.Repository = (*Repo)(nil)

func (r *Repo) Insert(_ context.Context, _ *gorm.DB, a domain.Activity) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.Rows {
		if row.TenantID == a.TenantID && row.EventID == a.EventID {
			return false, nil
		}
	}
	r.Rows[a.ID] = a
	return true, nil
}

func (r *Repo) ByEventIDs(_ context.Context, _ *gorm.DB, tenantID string, eventIDs []string) (map[string]domain.Activity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := map[string]bool{}
	for _, e := range eventIDs {
		want[e] = true
	}
	out := map[string]domain.Activity{}
	for _, row := range r.Rows {
		if row.TenantID == tenantID && want[row.EventID] {
			out[row.EventID] = row
		}
	}
	return out, nil
}

func (r *Repo) ByID(_ context.Context, tenantID, id string) (domain.Activity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.Rows[id]
	if !ok || row.TenantID != tenantID {
		return domain.Activity{}, domain.ErrNotFound
	}
	return row, nil
}

func (r *Repo) List(_ context.Context, tenantID string, f app.ListFilter, before time.Time, beforeID string, limit int) ([]domain.Activity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Activity
	for _, row := range r.Rows {
		if row.TenantID != tenantID ||
			(f.EventType != "" && row.EventType != f.EventType) ||
			(f.PlayerExternalID != "" && row.PlayerExternalID != f.PlayerExternalID) ||
			(f.Status != "" && row.Status != f.Status) {
			continue
		}
		if !before.IsZero() && !less(row, before, beforeID) {
			continue
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[j], out[i].CreatedAt, out[i].ID) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// less reports (a.CreatedAt, a.ID) < (ts, id).
func less(a domain.Activity, ts time.Time, id string) bool {
	if a.CreatedAt.Equal(ts) {
		return a.ID < id
	}
	return a.CreatedAt.Before(ts)
}

func (r *Repo) ApplyDecision(_ context.Context, _ *gorm.DB, a domain.Activity) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.Rows[a.ID]
	if !ok || row.TenantID != a.TenantID || row.Status != domain.StatusPending {
		return false, nil
	}
	row.Status, row.DecisionID, row.Outcome, row.Reason = a.Status, a.DecisionID, a.Outcome, a.Reason
	row.DecidedAt, row.UpdatedAt = a.DecidedAt, a.UpdatedAt
	if row.PlayerID == "" {
		row.PlayerID = a.PlayerID
	}
	r.Rows[a.ID] = row
	return true, nil
}

func (r *Repo) LockStuck(_ context.Context, _ *gorm.DB, q app.StuckQuery) ([]domain.Activity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Activity
	for _, row := range r.Rows {
		last, republished := r.LastRepub[row.ID]
		if row.Status == domain.StatusPending && row.ReceivedAt.Before(q.ReceivedBefore) &&
			row.RepublishCount < q.MaxRepublishes && (!republished || last.Before(q.ReceivedBefore)) {
			out = append(out, row)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReceivedAt.Before(out[j].ReceivedAt) })
	if len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

func (r *Repo) MarkRepublished(_ context.Context, _ *gorm.DB, ids []string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		row := r.Rows[id]
		row.RepublishCount++
		r.Rows[id] = row
		r.LastRepub[id] = at
	}
	return nil
}

func (r *Repo) CountExhausted(_ context.Context, receivedBefore time.Time, maxRepublishes int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, row := range r.Rows {
		if row.Status == domain.StatusPending && row.ReceivedAt.Before(receivedBefore) && row.RepublishCount >= maxRepublishes {
			n++
		}
	}
	return n, nil
}

func (r *Repo) DeleteTenant(_ context.Context, _ *gorm.DB, tenantID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for id, row := range r.Rows {
		if row.TenantID == tenantID {
			delete(r.Rows, id)
			n++
		}
	}
	return n, nil
}

func (r *Repo) LastSeen(_ context.Context, tenantID string, playerIDs []string) ([]domain.LastSeen, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := map[string]bool{}
	for _, p := range playerIDs {
		want[p] = true
	}
	best := map[string]domain.Activity{}
	for _, row := range r.Rows {
		if row.TenantID != tenantID || row.PlayerID == "" || !want[row.PlayerID] {
			continue
		}
		cur, ok := best[row.PlayerID]
		if !ok || row.OccurredAt.After(cur.OccurredAt) || (row.OccurredAt.Equal(cur.OccurredAt) && row.ID > cur.ID) {
			best[row.PlayerID] = row
		}
	}
	out := make([]domain.LastSeen, 0, len(best))
	for pid, a := range best {
		out = append(out, domain.LastSeen{PlayerID: pid, At: a.OccurredAt, EventType: a.EventType})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PlayerID < out[j].PlayerID })
	return out, nil
}

func (r *Repo) LastRun(_ context.Context, name string) (time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Markers[name], nil
}

func (r *Repo) MarkRun(_ context.Context, name string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Markers[name] = at
	return nil
}

// Get returns a row by id without tenant scoping (assertions only).
func (r *Repo) Get(id string) (domain.Activity, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.Rows[id]
	return a, ok
}

// Count is the number of stored rows.
func (r *Repo) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Rows)
}

// Published is one recorded outbox write.
type Published struct {
	Topic   string
	Payload any
}

// Outbox records publishes.
type Outbox struct {
	mu        sync.Mutex
	Published []Published
}

func (o *Outbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Published = append(o.Published, Published{Topic: topic, Payload: payload})
	return nil
}

// Players serves fixed snapshots; Err fails every call.
type Players struct {
	ByExt map[string]ports.PlayerSnapshot
	Err   error
	Calls int
}

func (p *Players) ByExternalIDs(_ context.Context, _ string, externalIDs []string) (map[string]ports.PlayerSnapshot, error) {
	p.Calls++
	if p.Err != nil {
		return nil, p.Err
	}
	out := map[string]ports.PlayerSnapshot{}
	for _, e := range externalIDs {
		if s, ok := p.ByExt[e]; ok {
			out[e] = s
		}
	}
	return out, nil
}

func (p *Players) ByIDs(_ context.Context, _ string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	p.Calls++
	if p.Err != nil {
		return nil, p.Err
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := map[string]ports.PlayerSnapshot{}
	for _, s := range p.ByExt {
		if want[s.ID] {
			out[s.ID] = s
		}
	}
	return out, nil
}

// EventTypes serves fixed snapshots; Err fails every call.
type EventTypes struct {
	Types map[string]ports.EventTypeSnapshot
	Err   error
	Calls int
}

func (e *EventTypes) BySlugs(_ context.Context, _ string, slugs []string) (map[string]ports.EventTypeSnapshot, error) {
	e.Calls++
	if e.Err != nil {
		return nil, e.Err
	}
	out := map[string]ports.EventTypeSnapshot{}
	for _, s := range slugs {
		if t, ok := e.Types[s]; ok {
			out[s] = t
		}
	}
	return out, nil
}

// AllowKeys grants only the listed permission keys.
type AllowKeys map[string]bool

func (a AllowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

// PassThroughTx runs fn without a database.
func PassThroughTx(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
