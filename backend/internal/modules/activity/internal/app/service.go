// Package app holds activity's use cases: ingestion (the hot path), reads,
// the decision projection, internal triggers, the stuck sweep and the
// tenant purge. It owns every transaction boundary and every publish.
package app

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/modules/activity/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
)

// ListFilter narrows a tenant's activity list. Zero values do not filter.
type ListFilter struct {
	EventType        string
	PlayerExternalID string
	Status           string
}

// StuckQuery selects pending activities the sweep should re-publish.
type StuckQuery struct {
	ReceivedBefore time.Time // pending since before this instant
	MaxRepublishes int       // rows at the cap are left alone
	Limit          int
}

// Repository is consumed by the service and implemented in internal/repo.
// Write-path methods take the transaction explicitly (ADR-0013).
type Repository interface {
	// Insert is INSERT ... ON CONFLICT (tenant_id, event_id) DO NOTHING;
	// it reports whether a row was written.
	Insert(ctx context.Context, tx *gorm.DB, a domain.Activity) (bool, error)
	// ByEventIDs reads through tx so rows inserted earlier in the same
	// transaction are visible. Keyed by event id.
	ByEventIDs(ctx context.Context, tx *gorm.DB, tenantID string, eventIDs []string) (map[string]domain.Activity, error)
	ByID(ctx context.Context, tenantID, id string) (domain.Activity, error)
	// List pages newest-first by (created_at, id); before is exclusive.
	List(ctx context.Context, tenantID string, f ListFilter, before time.Time, beforeID string, limit int) ([]domain.Activity, error)
	// ApplyDecision settles a pending activity; false when it was not
	// pending (already decided, or gone).
	ApplyDecision(ctx context.Context, tx *gorm.DB, a domain.Activity) (bool, error)
	// LockStuck takes FOR UPDATE SKIP LOCKED on stuck pending rows.
	LockStuck(ctx context.Context, tx *gorm.DB, q StuckQuery) ([]domain.Activity, error)
	MarkRepublished(ctx context.Context, tx *gorm.DB, ids []string, at time.Time) error
	CountExhausted(ctx context.Context, receivedBefore time.Time, maxRepublishes int) (int64, error)
	DeleteTenant(ctx context.Context, tx *gorm.DB, tenantID string) (int64, error)
	LastRun(ctx context.Context, name string) (time.Time, error)
	MarkRun(ctx context.Context, name string, at time.Time) error
}

// Settings are the module config values the service needs.
type Settings struct {
	RequireKnownEventType bool
	AutoCreatePlayers     bool
	Limits                domain.Limits
	StuckAfter            time.Duration
	MaxRepublishes        int
	SweepBatch            int
}

type Service struct {
	repo       Repository
	players    ports.PlayerReader
	eventTypes ports.EventTypeReader
	outbox     outbox.Store
	authz      authz.Enforcer
	db         *gorm.DB
	clock      clock.Clock
	log        *zap.Logger
	cfg        Settings

	republished prometheus.Counter

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, players ports.PlayerReader, eventTypes ports.EventTypeReader,
	ob outbox.Store, enf authz.Enforcer, db *gorm.DB, c clock.Clock, log *zap.Logger, cfg Settings,
	republished prometheus.Counter, opts ...Option) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{
		repo: repo, players: players, eventTypes: eventTypes, outbox: ob, authz: enf,
		db: db, clock: c, log: log, cfg: cfg, republished: republished,
	}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// TxRunner runs fn in one transaction.
type TxRunner func(ctx context.Context, fn func(tx *gorm.DB) error) error

// Option customises a Service.
type Option func(*Service)

// WithTx replaces the transaction runner; tests pass a pass-through so
// they need no database.
func WithTx(run TxRunner) Option {
	return func(s *Service) { s.tx = run }
}

// toReceived builds the activity.received.v1 payload from a stored row.
func (s *Service) toReceived(a domain.Activity) contracts.ReceivedV1 {
	return contracts.ReceivedV1{
		ActivityID:       a.ID,
		TenantID:         a.TenantID,
		EventID:          a.EventID,
		EventType:        a.EventType,
		PlayerExternalID: a.PlayerExternalID,
		PlayerID:         a.PlayerID,
		Properties:       a.Properties,
		Context:          a.Context,
		OccurredAt:       a.OccurredAt,
		ReceivedAt:       a.ReceivedAt,
		CausationDepth:   a.CausationDepth,
		SourceEventID:    a.SourceEventID,
		AutoCreatePlayer: s.cfg.AutoCreatePlayers && a.PlayerID == "" && a.PlayerExternalID != "",
	}
}

// isUUID guards bare-uuid columns: a malformed id from a client is a 404,
// from a payload an errs.Invalid, never a database error.
func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
