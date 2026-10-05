// Package app holds points' use cases: transaction boundaries, wallet row
// locks, idempotency, authorization and every outbox publish.
package app

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/modules/points/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
)

// Repository is implemented by internal/repo. Write-path methods take the
// caller's tx; methods documented "tx may be nil" fall back to the repo's
// own handle.
type Repository interface {
	// EnsureWallet inserts w unless (tenant, player) already has a wallet;
	// created reports whether this call inserted it.
	EnsureWallet(ctx context.Context, tx *gorm.DB, w domain.Wallet) (created bool, err error)
	// WalletForUpdate takes the row lock (SELECT ... FOR UPDATE).
	WalletForUpdate(ctx context.Context, tx *gorm.DB, tenantID, playerID string) (domain.Wallet, error)
	// WalletsForUpdate locks several wallets in ascending id order, so two
	// opposite transfers can never deadlock (B9).
	WalletsForUpdate(ctx context.Context, tx *gorm.DB, tenantID string, playerIDs []string) ([]domain.Wallet, error)
	// SaveWallet writes counters guarded by the version that was read and
	// bumps it; a moved version is ErrVersionConflict.
	SaveWallet(ctx context.Context, tx *gorm.DB, w domain.Wallet) error
	// InsertEntries appends entries with ON CONFLICT (tenant_id,
	// idempotency_key) DO NOTHING; inserted is false when any key existed.
	InsertEntries(ctx context.Context, tx *gorm.DB, entries ...domain.LedgerEntry) (inserted bool, err error)
	InsertRejection(ctx context.Context, tx *gorm.DB, r domain.Rejection) (inserted bool, err error)
	// EntryByKey / RejectionByKey / RefundOf: tx may be nil.
	EntryByKey(ctx context.Context, tx *gorm.DB, tenantID, key string) (domain.LedgerEntry, bool, error)
	RejectionByKey(ctx context.Context, tx *gorm.DB, tenantID, key string) (domain.Rejection, bool, error)
	RefundOf(ctx context.Context, tx *gorm.DB, tenantID, debitEntryID string) (domain.LedgerEntry, bool, error)
	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error

	WalletByPlayer(ctx context.Context, tenantID, playerID string) (domain.Wallet, bool, error)
	WalletsByPlayers(ctx context.Context, tenantID string, playerIDs []string) ([]domain.Wallet, error)
	ListEntries(ctx context.Context, tenantID, playerID string, f LedgerFilter) ([]domain.LedgerEntry, error)
}

// LedgerFilter pages a player's ledger newest first by (created_at, id).
type LedgerFilter struct {
	Kind      string
	Direction domain.Direction // 0 = both
	BeforeAt  time.Time        // zero = first page
	BeforeID  string
	Limit     int
}

// Reconciler is the persistence the reconcile job sweeps with.
type Reconciler interface {
	LastRun(ctx context.Context) (time.Time, error)
	MarkRun(ctx context.Context, at time.Time) error
	FindDrift(ctx context.Context, since time.Time) ([]domain.Drift, error)
}

type Service struct {
	repo    Repository
	players ports.PlayerReader
	outbox  outbox.Store
	authz   authz.Enforcer
	db      *gorm.DB
	clock   clock.Clock
	log     *zap.Logger
	drift   prometheus.Counter

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

// NewService wires the service. log and drift may be nil.
func NewService(repo Repository, players ports.PlayerReader, ob outbox.Store, enf authz.Enforcer,
	db *gorm.DB, c clock.Clock, log *zap.Logger, drift prometheus.Counter) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{repo: repo, players: players, outbox: ob, authz: enf, db: db, clock: c, log: log, drift: drift}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

// player resolves one player in the tenant; unknown or foreign → NotFound.
func (s *Service) player(ctx context.Context, tenantID, playerID string) (ports.PlayerSnapshot, error) {
	got, err := s.players.PlayersByIDs(ctx, tenantID, []string{playerID})
	if err != nil {
		return ports.PlayerSnapshot{}, err
	}
	p, ok := got[playerID]
	if !ok || p.TenantID != tenantID {
		return ports.PlayerSnapshot{}, domain.ErrPlayerNotFound
	}
	return p, nil
}
