// Package app holds badges' use cases: transaction boundaries, tenant and
// permission checks, every outbox publish (ADR-0013, docs/examples.md §6).
package app

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/modules/badges/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
)

// BadgeFilter narrows a catalogue listing. Before/BeforeID is the decoded
// keyset cursor over (created_at, id) DESC.
type BadgeFilter struct {
	Tier     string
	Category string
	Active   *bool
	Before   time.Time
	BeforeID string
	Limit    int
}

// PageCursor is a decoded (created_at, id) keyset position.
type PageCursor struct {
	Before   time.Time
	BeforeID string
}

// Repository is implemented by internal/repo. Write methods take the tx
// right after ctx; reads use the repository's own handle unless named *Tx.
type Repository interface {
	CreateBadge(ctx context.Context, tx *gorm.DB, b domain.Badge) error
	// BadgeByID returns a live (not soft-deleted) badge of the tenant, else
	// domain.ErrBadgeNotFound.
	BadgeByID(ctx context.Context, tenantID, id string) (domain.Badge, error)
	BadgeByIDTx(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Badge, error)
	BadgeByIDForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Badge, error)
	// SaveBadge writes back guarded by version; ErrVersionConflict otherwise.
	SaveBadge(ctx context.Context, tx *gorm.DB, b domain.Badge) error
	ListBadges(ctx context.Context, tenantID string, f BadgeFilter) ([]domain.Badge, error)
	BadgesByIDs(ctx context.Context, tenantID string, ids []string, includeDeleted bool) ([]domain.Badge, error)

	// LockOrCreatePlayerBadge is insert-or-lock: INSERT the placeholder ON
	// CONFLICT DO NOTHING, then SELECT ... FOR UPDATE the (player, badge) row.
	LockOrCreatePlayerBadge(ctx context.Context, tx *gorm.DB, placeholder domain.PlayerBadge) (domain.PlayerBadge, error)
	// LockPlayerBadge returns found=false when the player holds no such badge.
	LockPlayerBadge(ctx context.Context, tx *gorm.DB, tenantID, playerID, badgeID string) (domain.PlayerBadge, bool, error)
	PlayerBadge(ctx context.Context, tenantID, playerID, badgeID string) (domain.PlayerBadge, bool, error)
	SavePlayerBadge(ctx context.Context, tx *gorm.DB, pb domain.PlayerBadge) error
	DeletePlayerBadge(ctx context.Context, tx *gorm.DB, pb domain.PlayerBadge) error
	ListPlayerBadges(ctx context.Context, tenantID, playerID string, cur PageCursor, limit int) ([]domain.PlayerBadge, error)
	PlayerBadgesByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) ([]domain.PlayerBadge, error)

	AwardByKey(ctx context.Context, tenantID, key string) (domain.Award, bool, error)
	AwardByKeyTx(ctx context.Context, tx *gorm.DB, tenantID, key string) (domain.Award, bool, error)
	// InsertAward is INSERT ... ON CONFLICT (tenant_id, idempotency_key) DO
	// NOTHING; inserted=false means another delivery already recorded it.
	InsertAward(ctx context.Context, tx *gorm.DB, a domain.Award) (bool, error)
	InsertRevocation(ctx context.Context, tx *gorm.DB, r domain.Revocation) error

	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
}

// Reconciler is the sweep side of the repository (R47).
type Reconciler interface {
	LastRun(ctx context.Context) (time.Time, error)
	MarkRun(ctx context.Context, at time.Time) error
	// DriftSince returns holdings touched at or after since that break
	// earned_count == applied awards or earned_count <= max_awards.
	DriftSince(ctx context.Context, since time.Time) ([]domain.Drift, error)
}

// Service implements every badges use case and contracts.Reader.
type Service struct {
	repo    Repository
	players ports.PlayerReader
	outbox  outbox.Store
	authz   authz.Enforcer
	db      *gorm.DB
	clock   clock.Clock
	log     *zap.Logger
	drift   *prometheus.CounterVec

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

// NewService wires the service. log and drift may be nil.
func NewService(repo Repository, players ports.PlayerReader, ob outbox.Store, enf authz.Enforcer,
	db *gorm.DB, c clock.Clock, log *zap.Logger, drift *prometheus.CounterVec) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{repo: repo, players: players, outbox: ob, authz: enf, db: db, clock: c, log: log, drift: drift}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

func (s *Service) now() time.Time { return s.clock.Now().UTC().Truncate(time.Microsecond) }

// requireTenantPerm is the opening of every HTTP-facing method (ADR-0015).
func (s *Service) requireTenantPerm(ctx context.Context, perm authz.Permission, resource any) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, err
	}
	if err := s.authz.Authorize(ctx, p, perm, resource); err != nil {
		return authz.Principal{}, err
	}
	return p, nil
}

// requirePlayer resolves a player of the caller's tenant for HTTP paths:
// unknown → 404 player_not_found.
func (s *Service) requirePlayer(ctx context.Context, tenantID, playerID string) (ports.PlayerSnapshot, error) {
	snap, ok, err := s.players.ByID(ctx, tenantID, playerID)
	if err != nil {
		return ports.PlayerSnapshot{}, err
	}
	if !ok || snap.TenantID != tenantID {
		return ports.PlayerSnapshot{}, domain.ErrPlayerNotFound
	}
	return snap, nil
}
