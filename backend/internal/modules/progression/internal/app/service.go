// Package app holds progression's use cases: transaction boundaries,
// authorization, and every outbox publish (ADR-0013: the tx handle is an
// explicit argument).
package app

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/modules/progression/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
)

// Repository is consumed by the service and implemented in internal/repo.
// Write methods take the transaction; read methods accept tx == nil to use
// the repository's own handle.
type Repository interface {
	// LockLadder serialises ladder edits of one tenant (advisory xact lock),
	// so two concurrent edits cannot jointly break the xp ordering.
	LockLadder(ctx context.Context, tx *gorm.DB, tenantID string) error
	// Ladder returns the tenant's non-deleted levels sorted by number;
	// includeInactive=false keeps only active ones.
	Ladder(ctx context.Context, tx *gorm.DB, tenantID string, includeInactive bool) (domain.Ladder, error)
	LevelByID(ctx context.Context, tenantID, levelID string) (domain.Level, error)
	CreateLevel(ctx context.Context, tx *gorm.DB, l domain.Level) error
	UpdateLevel(ctx context.Context, tx *gorm.DB, l domain.Level) error
	SoftDeleteLevel(ctx context.Context, tx *gorm.DB, tenantID, levelID string, at time.Time) error

	// InsertGrant is INSERT ... ON CONFLICT (tenant_id, idempotency_key) DO
	// NOTHING; false means the key was already applied.
	InsertGrant(ctx context.Context, tx *gorm.DB, g domain.XPGrant) (bool, error)
	GrantByKey(ctx context.Context, tx *gorm.DB, tenantID, key string) (domain.XPGrant, error)
	ListGrants(ctx context.Context, tenantID, playerID string, before time.Time, beforeID string, limit int) ([]domain.XPGrant, error)

	// EnsureProgress inserts the zero row unless one exists.
	EnsureProgress(ctx context.Context, tx *gorm.DB, p domain.Progress) error
	// ProgressForUpdate takes the row lock (SELECT ... FOR UPDATE).
	ProgressForUpdate(ctx context.Context, tx *gorm.DB, tenantID, playerID string) (domain.Progress, error)
	// SaveProgress writes back guarded by version; a stale write is
	// domain.ErrVersionConflict.
	SaveProgress(ctx context.Context, tx *gorm.DB, p domain.Progress) error
	ProgressByPlayers(ctx context.Context, tenantID string, playerIDs []string) (map[string]domain.Progress, error)

	// InsertLevelReward is ON CONFLICT (tenant_id, player_id, level_id) DO NOTHING.
	InsertLevelReward(ctx context.Context, tx *gorm.DB, r domain.LevelReward) (bool, error)
	// InsertRejection is ON CONFLICT (tenant_id, idempotency_key) DO NOTHING.
	InsertRejection(ctx context.Context, tx *gorm.DB, r domain.GrantRejection) (bool, error)

	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
}

// DriftRecorder counts reconcile findings (kind = "total_xp" | "level").
type DriftRecorder func(kind string, n int)

// Options are the service's tunables, from the module Config.
type Options struct {
	ReplaceLevels      bool
	ReconcileBatchSize int
	Drift              DriftRecorder
}

type Service struct {
	repo    Repository
	players ports.PlayerReader
	outbox  outbox.Store
	authz   authz.Enforcer
	db      *gorm.DB
	clock   clock.Clock
	log     *zap.Logger
	opts    Options

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, players ports.PlayerReader, ob outbox.Store, enf authz.Enforcer,
	db *gorm.DB, c clock.Clock, log *zap.Logger, opts Options) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	if opts.ReconcileBatchSize <= 0 {
		opts.ReconcileBatchSize = 500
	}
	if opts.Drift == nil {
		opts.Drift = func(string, int) {}
	}
	s := &Service{repo: repo, players: players, outbox: ob, authz: enf, db: db, clock: c, log: log, opts: opts}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}
