package app

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// fakeRepo is an in-memory Repository mimicking the SQL unique keys and
// guards the service relies on.
type fakeRepo struct {
	mu         sync.Mutex
	endpoints  map[string]domain.Endpoint
	deleted    map[string]bool
	deliveries map[string]domain.Delivery
	markers    map[string]time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		endpoints:  map[string]domain.Endpoint{},
		deleted:    map[string]bool{},
		deliveries: map[string]domain.Delivery{},
		markers:    map[string]time.Time{},
	}
}

var _ Repository = (*fakeRepo)(nil)

func (f *fakeRepo) CreateEndpoint(_ context.Context, _ *gorm.DB, e domain.Endpoint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.endpoints[e.ID] = e
	return nil
}

func (f *fakeRepo) SaveEndpoint(_ context.Context, _ *gorm.DB, e domain.Endpoint, resetFailures bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.endpoints[e.ID]
	if !ok || f.deleted[e.ID] || cur.Version != e.Version || cur.TenantID != e.TenantID {
		return domain.ErrVersionConflict
	}
	failures := cur.ConsecutiveFailures
	if resetFailures {
		failures = 0
	}
	e.ConsecutiveFailures = failures
	e.Version++
	f.endpoints[e.ID] = e
	return nil
}

func (f *fakeRepo) SoftDeleteEndpoint(_ context.Context, _ *gorm.DB, tenantID, id string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.endpoints[id]
	if !ok || f.deleted[id] || e.TenantID != tenantID {
		return domain.ErrEndpointNotFound
	}
	f.deleted[id] = true
	e.Active = false
	f.endpoints[id] = e
	return nil
}

func (f *fakeRepo) EndpointByID(_ context.Context, tenantID, id string) (domain.Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.endpoints[id]
	if !ok || f.deleted[id] || e.TenantID != tenantID {
		return domain.Endpoint{}, domain.ErrEndpointNotFound
	}
	return e, nil
}

func (f *fakeRepo) CountEndpoints(_ context.Context, tenantID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for id, e := range f.endpoints {
		if !f.deleted[id] && e.TenantID == tenantID {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) ListEndpoints(_ context.Context, tenantID string, p Page) ([]domain.Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Endpoint
	for id, e := range f.endpoints {
		if f.deleted[id] || e.TenantID != tenantID {
			continue
		}
		if p.BeforeID != "" && !older(e.CreatedAt, e.ID, p.Before, p.BeforeID) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return older(out[j].CreatedAt, out[j].ID, out[i].CreatedAt, out[i].ID) })
	if len(out) > p.Limit {
		out = out[:p.Limit]
	}
	return out, nil
}

func older(t time.Time, id string, beforeT time.Time, beforeID string) bool {
	return t.Before(beforeT) || (t.Equal(beforeT) && id < beforeID)
}

func (f *fakeRepo) ActiveEndpointsFor(_ context.Context, tenantID, event string) ([]domain.Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Endpoint
	for id, e := range f.endpoints {
		if !f.deleted[id] && e.TenantID == tenantID && e.Active && e.Subscribes(event) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeRepo) IncrementFailures(_ context.Context, _ *gorm.DB, tenantID, id string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.endpoints[id]
	if !ok || e.TenantID != tenantID {
		return 0, nil
	}
	e.ConsecutiveFailures++
	f.endpoints[id] = e
	return e.ConsecutiveFailures, nil
}

func (f *fakeRepo) ResetFailures(_ context.Context, _ *gorm.DB, tenantID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.endpoints[id]; ok && e.TenantID == tenantID {
		e.ConsecutiveFailures = 0
		f.endpoints[id] = e
	}
	return nil
}

func (f *fakeRepo) DisableEndpoint(_ context.Context, _ *gorm.DB, tenantID, id, reason string, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.endpoints[id]
	if !ok || f.deleted[id] || e.TenantID != tenantID || !e.Active {
		return false, nil
	}
	e.Active = false
	e.DisabledReason = reason
	e.DisabledAt = &at
	e.Version++
	f.endpoints[id] = e
	return true, nil
}

func (f *fakeRepo) ActiveEndpointsOverThreshold(_ context.Context, threshold, limit int) ([]domain.Endpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Endpoint
	for id, e := range f.endpoints {
		if !f.deleted[id] && e.Active && e.ConsecutiveFailures >= threshold {
			out = append(out, e)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) InsertDelivery(_ context.Context, _ *gorm.DB, d domain.Delivery) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.deliveries {
		if x.EndpointID == d.EndpointID && x.EventID == d.EventID {
			return false, nil
		}
	}
	f.deliveries[d.ID] = d
	return true, nil
}

func (f *fakeRepo) SaveDelivery(_ context.Context, _ *gorm.DB, d domain.Delivery) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.deliveries[d.ID]
	if !ok || cur.TenantID != d.TenantID {
		return domain.ErrDeliveryNotFound
	}
	f.deliveries[d.ID] = d
	return nil
}

func (f *fakeRepo) DeliveryByID(_ context.Context, tenantID, id string) (domain.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.deliveries[id]
	if !ok || d.TenantID != tenantID {
		return domain.Delivery{}, domain.ErrDeliveryNotFound
	}
	return d, nil
}

func (f *fakeRepo) DeliveryForUpdate(ctx context.Context, _ *gorm.DB, tenantID, id string) (domain.Delivery, error) {
	return f.DeliveryByID(ctx, tenantID, id)
}

func (f *fakeRepo) ListDeliveries(_ context.Context, tenantID string, flt DeliveryFilter, p Page) ([]domain.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Delivery
	for _, d := range f.deliveries {
		if d.TenantID != tenantID ||
			(flt.EndpointID != "" && d.EndpointID != flt.EndpointID) ||
			(flt.Status != "" && d.Status != flt.Status) ||
			(flt.Event != "" && d.Event != flt.Event) {
			continue
		}
		if p.BeforeID != "" && !older(d.CreatedAt, d.ID, p.Before, p.BeforeID) {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return older(out[j].CreatedAt, out[j].ID, out[i].CreatedAt, out[i].ID) })
	if len(out) > p.Limit {
		out = out[:p.Limit]
	}
	return out, nil
}

func (f *fakeRepo) StalePending(_ context.Context, before, now time.Time, limit int) ([]domain.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Delivery
	for _, d := range f.deliveries {
		if d.Status != domain.StatusPending || !d.EnqueuedAt.Before(before) {
			continue
		}
		if d.LastAttemptAt != nil && !d.LastAttemptAt.Before(before) {
			continue
		}
		if d.LeaseUntil != nil && !d.LeaseUntil.Before(now) {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
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
	for id, e := range f.endpoints {
		if e.TenantID == tenantID {
			delete(f.endpoints, id)
			delete(f.deleted, id)
		}
	}
	for id, d := range f.deliveries {
		if d.TenantID == tenantID {
			delete(f.deliveries, id)
		}
	}
	return nil
}

func (f *fakeRepo) deliveriesFor(endpointID string) []domain.Delivery {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Delivery
	for _, d := range f.deliveries {
		if d.EndpointID == endpointID {
			out = append(out, d)
		}
	}
	return out
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

// fakeSender replays scripted results and records requests.
type fakeSender struct {
	mu       sync.Mutex
	results  []domain.AttemptResult // consumed in order; the last repeats
	requests []Request
}

func (f *fakeSender) Send(_ context.Context, r Request) domain.AttemptResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	if len(f.results) == 0 {
		return domain.AttemptResult{StatusCode: 200, LatencyMS: 3}
	}
	res := f.results[0]
	if len(f.results) > 1 {
		f.results = slices.Delete(f.results, 0, 1)
	}
	return res
}

func (f *fakeSender) sent() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}
