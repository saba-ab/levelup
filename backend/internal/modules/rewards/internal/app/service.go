// Package app holds rewards' use cases: the catalogue, the claim saga
// (tx1 hold → job.points.debit → settle on points' outcome), grants,
// redeem/cancel, and the reconciling sweeps. Every publish happens here,
// inside the transaction that records the fact (ADR-0013).
package app

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/modules/rewards/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
	codeRetries     = 3
)

// RewardFilter narrows the catalogue listing.
type RewardFilter struct {
	Status   string
	Type     string
	IsActive *bool
}

// Page is a keyset page over (created_at, id) DESC. Zero AfterTime means
// the first page.
type Page struct {
	AfterTime time.Time
	AfterID   string
	Limit     int
}

// Repository is implemented by internal/repo. Write methods take the tx.
type Repository interface {
	CreateReward(ctx context.Context, tx *gorm.DB, r domain.Reward) error
	RewardByID(ctx context.Context, tenantID, id string, includeDeleted bool) (domain.Reward, error)
	// RewardForUpdate takes the reward's row lock: every stock movement
	// serializes on it.
	RewardForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string, includeDeleted bool) (domain.Reward, error)
	SaveReward(ctx context.Context, tx *gorm.DB, r domain.Reward) error
	SoftDeleteReward(ctx context.Context, tx *gorm.DB, tenantID, id string, at time.Time) error
	ListRewards(ctx context.Context, tenantID string, f RewardFilter, p Page) ([]domain.Reward, error)

	// InsertClaim returns domain.ErrDuplicateRequest when the client
	// request id or grant key already has a claim, domain.ErrCodeCollision
	// when the voucher code is taken.
	InsertClaim(ctx context.Context, tx *gorm.DB, c domain.Claim) error
	ClaimByID(ctx context.Context, tenantID, id string) (domain.Claim, error)
	ClaimForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Claim, error)
	ClaimByClientRequest(ctx context.Context, tenantID, key string) (domain.Claim, error)
	ClaimByGrantKey(ctx context.Context, tenantID, key string) (domain.Claim, error)
	SaveClaim(ctx context.Context, tx *gorm.DB, c domain.Claim) error
	// CountHeldClaims counts the player's claims on the reward that hold a
	// unit: pending_payment, claimed, redeemed.
	CountHeldClaims(ctx context.Context, tx *gorm.DB, tenantID, rewardID, playerID string) (int, error)
	ListPlayerClaims(ctx context.Context, tenantID, playerID string, p Page) ([]domain.Claim, error)

	// Sweep reads span every tenant; each row carries its tenant id.
	DuePendingClaims(ctx context.Context, now time.Time, limit int) ([]domain.Claim, error)
	PaidCancelledSince(ctx context.Context, since time.Time, limit int) ([]domain.Claim, error)
	RefundPendingClaims(ctx context.Context, limit int) ([]domain.Claim, error)
	DueExpiringClaims(ctx context.Context, now time.Time, limit int) ([]domain.Claim, error)
	EndedRewards(ctx context.Context, now time.Time, limit int) ([]domain.Reward, error)
	LastRun(ctx context.Context, job string) (time.Time, error)
	MarkRun(ctx context.Context, job string, at time.Time) error

	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
}

// Settings are the service's tunables, taken from the module Config.
type Settings struct {
	HoldTTL           time.Duration
	SweepBatchSize    int
	LateDebitLookback time.Duration
}

type Service struct {
	repo     Repository
	players  ports.PlayerReader
	progress ports.ProgressReader
	points   ports.PointsReader
	outbox   outbox.Store
	authz    authz.Enforcer
	db       *gorm.DB
	clock    clock.Clock
	cfg      Settings

	// code mints voucher codes; injectable so tests are deterministic.
	code func() string
	tx   func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, players ports.PlayerReader, progress ports.ProgressReader, points ports.PointsReader,
	ob outbox.Store, enf authz.Enforcer, db *gorm.DB, c clock.Clock, cfg Settings) *Service {
	if cfg.HoldTTL <= 0 {
		cfg.HoldTTL = 10 * time.Minute
	}
	if cfg.SweepBatchSize <= 0 {
		cfg.SweepBatchSize = 500
	}
	if cfg.LateDebitLookback <= 0 {
		cfg.LateDebitLookback = 24 * time.Hour
	}
	s := &Service{
		repo: repo, players: players, progress: progress, points: points,
		outbox: ob, authz: enf, db: db, clock: c, cfg: cfg,
		code: domain.NewVoucherCode,
	}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

// ---- catalogue ----

func (s *Service) CreateReward(ctx context.Context, patch domain.RewardPatch) (domain.Reward, error) {
	p, err := s.authorize(ctx, contracts.PermCreate)
	if err != nil {
		return domain.Reward{}, err
	}
	r, err := domain.NewReward(p.TenantID, patch, s.clock.Now())
	if err != nil {
		return domain.Reward{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error { return s.repo.CreateReward(ctx, tx, r) }); err != nil {
		return domain.Reward{}, err
	}
	return r, nil
}

func (s *Service) GetReward(ctx context.Context, id string) (domain.Reward, error) {
	p, err := s.authorize(ctx, contracts.PermView)
	if err != nil {
		return domain.Reward{}, err
	}
	return s.repo.RewardByID(ctx, p.TenantID, id, false)
}

func (s *Service) ListRewards(ctx context.Context, f RewardFilter, cursor string, limit int) ([]domain.Reward, string, error) {
	p, err := s.authorize(ctx, contracts.PermViewAny)
	if err != nil {
		return nil, "", err
	}
	page, err := pageOf(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	want := page.Limit
	page.Limit++
	rows, err := s.repo.ListRewards(ctx, p.TenantID, f, page)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > want {
		rows = rows[:want]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

func (s *Service) UpdateReward(ctx context.Context, id string, patch domain.RewardPatch) (domain.Reward, error) {
	p, err := s.authorize(ctx, contracts.PermUpdate)
	if err != nil {
		return domain.Reward{}, err
	}
	var out domain.Reward
	err = s.tx(ctx, func(tx *gorm.DB) error {
		r, err := s.repo.RewardForUpdate(ctx, tx, p.TenantID, id, false)
		if err != nil {
			return err
		}
		if err := r.Apply(patch, s.clock.Now()); err != nil {
			return err
		}
		if err := s.repo.SaveReward(ctx, tx, r); err != nil {
			return err
		}
		r.Version++
		out = r
		return nil
	})
	return out, err
}

func (s *Service) DeleteReward(ctx context.Context, id string) error {
	p, err := s.authorize(ctx, contracts.PermDelete)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.SoftDeleteReward(ctx, tx, p.TenantID, id, s.clock.Now())
	})
}

// PurgeTenant drops every row of a deleted tenant (tenant.deleted.v1).
// Idempotent: a second delivery deletes nothing.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return errs.New(errs.Invalid, "tenant.deleted.v1 without tenant_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error { return s.repo.PurgeTenant(ctx, tx, tenantID) })
}

// ---- helpers ----

func (s *Service) authorize(ctx context.Context, perm authz.Permission) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, err
	}
	if err := s.authz.Authorize(ctx, p, perm, nil); err != nil {
		return authz.Principal{}, err
	}
	return p, nil
}

// withCodeRetry reruns a whole transaction when a freshly minted voucher
// code collided with an existing one (the failed tx rolled back).
func (s *Service) withCodeRetry(fn func() error) error {
	var err error
	for range codeRetries {
		err = fn()
		if !errors.Is(err, domain.ErrCodeCollision) {
			return err
		}
	}
	return err
}

func pageOf(cursor string, limit int) (Page, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	p := Page{Limit: limit}
	if cursor != "" {
		t, id, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
		p.AfterTime, p.AfterID = t, id
	}
	return p, nil
}

func isNotFound(err error) bool { return errs.KindOf(err) == errs.NotFound }
