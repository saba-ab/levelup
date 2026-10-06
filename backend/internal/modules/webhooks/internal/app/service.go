// Package app holds webhooks' use cases: transaction boundaries,
// authorization, every outbox publish, the fan-out subscription, the
// delivery job and the retry sweep.
package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/webhooks/contracts"
	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/pagination"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
	sweepBatchSize  = 200
)

// Page is a keyset page position: rows strictly older than (Before, BeforeID).
type Page struct {
	Before   time.Time
	BeforeID string
	Limit    int
}

// DeliveryFilter narrows a delivery listing; empty fields do not filter.
type DeliveryFilter struct {
	EndpointID string
	Status     string
	Event      string
}

// Repository is implemented by internal/repo. Write methods take the tx.
type Repository interface {
	CreateEndpoint(ctx context.Context, tx *gorm.DB, e domain.Endpoint) error
	// SaveEndpoint writes the admin-owned fields under the version guard;
	// resetFailures also zeroes consecutive_failures (re-activation).
	SaveEndpoint(ctx context.Context, tx *gorm.DB, e domain.Endpoint, resetFailures bool) error
	SoftDeleteEndpoint(ctx context.Context, tx *gorm.DB, tenantID, id string, at time.Time) error
	EndpointByID(ctx context.Context, tenantID, id string) (domain.Endpoint, error)
	CountEndpoints(ctx context.Context, tenantID string) (int64, error)
	ListEndpoints(ctx context.Context, tenantID string, p Page) ([]domain.Endpoint, error)
	// ActiveEndpointsFor returns the tenant's live, active endpoints
	// subscribed to event (directly or through "*").
	ActiveEndpointsFor(ctx context.Context, tenantID, event string) ([]domain.Endpoint, error)
	// IncrementFailures adds one to the failure streak and returns it.
	IncrementFailures(ctx context.Context, tx *gorm.DB, tenantID, id string) (int, error)
	ResetFailures(ctx context.Context, tx *gorm.DB, tenantID, id string) error
	// DisableEndpoint switches an active endpoint off; false when it was
	// already inactive (or gone), so the caller publishes at most once.
	DisableEndpoint(ctx context.Context, tx *gorm.DB, tenantID, id, reason string, at time.Time) (bool, error)
	ActiveEndpointsOverThreshold(ctx context.Context, threshold, limit int) ([]domain.Endpoint, error)

	// InsertDelivery is ON CONFLICT (endpoint_id, event_id) DO NOTHING;
	// false means the delivery already existed.
	InsertDelivery(ctx context.Context, tx *gorm.DB, d domain.Delivery) (bool, error)
	SaveDelivery(ctx context.Context, tx *gorm.DB, d domain.Delivery) error
	DeliveryByID(ctx context.Context, tenantID, id string) (domain.Delivery, error)
	DeliveryForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Delivery, error)
	ListDeliveries(ctx context.Context, tenantID string, f DeliveryFilter, p Page) ([]domain.Delivery, error)
	// StalePending returns pending deliveries with no live lease whose last
	// enqueue and last attempt are both before `before`.
	StalePending(ctx context.Context, before, now time.Time, limit int) ([]domain.Delivery, error)

	LastRun(ctx context.Context, job string) (time.Time, error)
	MarkRun(ctx context.Context, job string, at time.Time) error

	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
}

// Request is one outbound POST.
type Request struct {
	URL        string
	Event      string
	DeliveryID string
	Secret     string
	Body       []byte
	At         time.Time // signing timestamp
}

// Sender performs a delivery POST. It never returns an error: every outcome,
// including transport failures, is an AttemptResult.
type Sender interface {
	Send(ctx context.Context, req Request) domain.AttemptResult
}

// Options are the service's tunables (from the module Config).
type Options struct {
	Policy               domain.URLPolicy
	Retry                domain.RetryPolicy
	StaleAfter           time.Duration
	DisableAfterFailures int
	// Lease bounds how long one attempt holds a delivery (the HTTP timeout
	// plus slack).
	Lease time.Duration
}

type Service struct {
	repo   Repository
	sender Sender
	outbox outbox.Store
	authz  authz.Enforcer
	db     *gorm.DB
	clock  clock.Clock
	opts   Options

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, sender Sender, ob outbox.Store, enf authz.Enforcer, db *gorm.DB, c clock.Clock, opts Options) *Service {
	s := &Service{repo: repo, sender: sender, outbox: ob, authz: enf, db: db, clock: c, opts: opts}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

// guard resolves the tenant principal and checks a role-level permission.
func (s *Service) guard(ctx context.Context, perm authz.Permission, resource any) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, err
	}
	if err := s.authz.Authorize(ctx, p, perm, resource); err != nil {
		return authz.Principal{}, err
	}
	return p, nil
}

// ---- endpoints ----

func (s *Service) CreateEndpoint(ctx context.Context, in domain.NewEndpointInput) (domain.Endpoint, error) {
	p, err := s.guard(ctx, contracts.PermManage, nil)
	if err != nil {
		return domain.Endpoint{}, err
	}
	e, err := domain.NewEndpoint(p.TenantID, in, s.opts.Policy, s.clock.Now())
	if err != nil {
		return domain.Endpoint{}, err
	}
	n, err := s.repo.CountEndpoints(ctx, p.TenantID)
	if err != nil {
		return domain.Endpoint{}, err
	}
	if n >= domain.MaxEndpointsPerTenant {
		return domain.Endpoint{}, domain.ErrEndpointLimit
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.CreateEndpoint(ctx, tx, e)
	}); err != nil {
		return domain.Endpoint{}, err
	}
	return e, nil
}

func (s *Service) GetEndpoint(ctx context.Context, id string) (domain.Endpoint, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Endpoint{}, err
	}
	e, err := s.repo.EndpointByID(ctx, p.TenantID, id)
	if err != nil {
		return domain.Endpoint{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, e); err != nil {
		return domain.Endpoint{}, err
	}
	return e, nil
}

// ListEndpoints pages newest-first by (created_at, id).
func (s *Service) ListEndpoints(ctx context.Context, cursor string, limit int) ([]domain.Endpoint, string, error) {
	p, err := s.guard(ctx, contracts.PermView, nil)
	if err != nil {
		return nil, "", err
	}
	page, err := pageOf(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	page.Limit++
	rows, err := s.repo.ListEndpoints(ctx, p.TenantID, page)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) == page.Limit {
		rows = rows[:page.Limit-1]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

func (s *Service) UpdateEndpoint(ctx context.Context, id string, patch domain.EndpointPatch) (domain.Endpoint, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Endpoint{}, err
	}
	e, err := s.repo.EndpointByID(ctx, p.TenantID, id)
	if err != nil {
		return domain.Endpoint{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermManage, e); err != nil {
		return domain.Endpoint{}, err
	}
	wasActive := e.Active
	if err := e.Apply(patch, s.opts.Policy, s.clock.Now()); err != nil {
		return domain.Endpoint{}, err
	}
	reactivated := e.Active && !wasActive
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.SaveEndpoint(ctx, tx, e, reactivated)
	}); err != nil {
		return domain.Endpoint{}, err
	}
	e.Version++
	return e, nil
}

func (s *Service) DeleteEndpoint(ctx context.Context, id string) error {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return err
	}
	e, err := s.repo.EndpointByID(ctx, p.TenantID, id)
	if err != nil {
		return err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermManage, e); err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.SoftDeleteEndpoint(ctx, tx, p.TenantID, e.ID, s.clock.Now())
	})
}

// RotateSecret issues a new signing secret; the returned endpoint carries it
// (the only time besides creation that a client sees it).
func (s *Service) RotateSecret(ctx context.Context, id string) (domain.Endpoint, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Endpoint{}, err
	}
	e, err := s.repo.EndpointByID(ctx, p.TenantID, id)
	if err != nil {
		return domain.Endpoint{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermManage, e); err != nil {
		return domain.Endpoint{}, err
	}
	if err := e.RotateSecret(s.clock.Now()); err != nil {
		return domain.Endpoint{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.SaveEndpoint(ctx, tx, e, false)
	}); err != nil {
		return domain.Endpoint{}, err
	}
	e.Version++
	return e, nil
}

// EventTypes returns the subscribable catalogue.
func (s *Service) EventTypes(ctx context.Context) ([]domain.EventType, error) {
	if _, err := s.guard(ctx, contracts.PermView, nil); err != nil {
		return nil, err
	}
	return domain.Catalogue, nil
}

// ---- deliveries ----

func (s *Service) GetDelivery(ctx context.Context, id string) (domain.Delivery, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Delivery{}, err
	}
	d, err := s.repo.DeliveryByID(ctx, p.TenantID, id)
	if err != nil {
		return domain.Delivery{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, d); err != nil {
		return domain.Delivery{}, err
	}
	return d, nil
}

// ListDeliveries pages newest-first by (created_at, id).
func (s *Service) ListDeliveries(ctx context.Context, f DeliveryFilter, cursor string, limit int) ([]domain.Delivery, string, error) {
	p, err := s.guard(ctx, contracts.PermView, nil)
	if err != nil {
		return nil, "", err
	}
	page, err := pageOf(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	page.Limit++
	rows, err := s.repo.ListDeliveries(ctx, p.TenantID, f, page)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) == page.Limit {
		rows = rows[:page.Limit-1]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

// Redeliver re-arms a delivery for a fresh retry cycle and queues it. The
// same body is sent again, signed with the endpoint's current secret.
func (s *Service) Redeliver(ctx context.Context, id string) (domain.Delivery, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Delivery{}, err
	}
	d, err := s.repo.DeliveryByID(ctx, p.TenantID, id)
	if err != nil {
		return domain.Delivery{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermManage, d); err != nil {
		return domain.Delivery{}, err
	}
	e, err := s.repo.EndpointByID(ctx, p.TenantID, d.EndpointID)
	if err != nil {
		return domain.Delivery{}, err
	}
	if !e.Active {
		return domain.Delivery{}, domain.ErrEndpointInactive
	}
	now := s.clock.Now()
	err = s.tx(ctx, func(tx *gorm.DB) error {
		locked, err := s.repo.DeliveryForUpdate(ctx, tx, p.TenantID, id)
		if err != nil {
			return err
		}
		if err := locked.Requeue(now); err != nil {
			return err
		}
		if err := s.repo.SaveDelivery(ctx, tx, locked); err != nil {
			return err
		}
		d = locked
		return s.enqueue(ctx, tx, locked)
	})
	if err != nil {
		return domain.Delivery{}, err
	}
	return d, nil
}

// SendTest POSTs a synthetic "webhook.test" event to the endpoint right now
// and records the attempt as a delivery (no retries). Works on inactive
// endpoints too, so a receiver can be checked before re-enabling it.
func (s *Service) SendTest(ctx context.Context, endpointID string) (domain.Delivery, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Delivery{}, err
	}
	e, err := s.repo.EndpointByID(ctx, p.TenantID, endpointID)
	if err != nil {
		return domain.Delivery{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermManage, e); err != nil {
		return domain.Delivery{}, err
	}
	now := s.clock.Now()
	d := domain.NewDelivery(p.TenantID, e.ID, newEventID(), domain.TestEvent, nil, now)
	data, err := testData(e.ID)
	if err != nil {
		return domain.Delivery{}, err
	}
	body, err := domain.BuildPayload(domain.TestEvent, d.EventID, p.TenantID, now, data)
	if err != nil {
		return domain.Delivery{}, err
	}
	d.Payload = body
	if err := d.Begin(now, s.opts.Lease); err != nil {
		return domain.Delivery{}, err
	}
	res := s.sender.Send(ctx, Request{URL: e.URL, Event: d.Event, DeliveryID: d.ID, Secret: e.Secret, Body: body, At: now})
	d.Settle(res, true, s.clock.Now())
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		_, err := s.repo.InsertDelivery(ctx, tx, d)
		return err
	}); err != nil {
		return domain.Delivery{}, err
	}
	return d, nil
}

func (s *Service) enqueue(ctx context.Context, tx *gorm.DB, d domain.Delivery) error {
	return s.outbox.Publish(ctx, tx, contracts.Topic(contracts.JobDeliver), contracts.DeliverCmdV1{
		TenantID:    d.TenantID,
		DeliveryID:  d.ID,
		BaseAttempt: d.CycleAttempts,
	})
}

func pageOf(cursor string, limit int) (Page, error) {
	switch {
	case limit <= 0:
		limit = defaultPageSize
	case limit > maxPageSize:
		limit = maxPageSize
	}
	p := Page{Limit: limit}
	if cursor != "" {
		before, beforeID, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
		p.Before, p.BeforeID = before, beforeID
	}
	return p, nil
}
