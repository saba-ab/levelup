// Package app holds identity's use cases: registration and sessions, user
// administration inside a tenant, tenant settings, and the platform tenant
// surface. Every state change and its outbox event share one transaction
// (ADR-0013); refresh-token revocation happens after commit.
package app

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
)

// Page is a keyset position over (created_at, id) DESC. Zero After* means
// "from the newest". Limit is already clamped; repositories return at most
// Limit+1 rows so the service can tell whether a next page exists.
type Page struct {
	Limit        int
	AfterCreated time.Time
	AfterID      string
}

// Repository is consumed here and implemented in internal/repo. Reads use
// the repo's own handle; writes take the transaction explicitly.
type Repository interface {
	CreateTenant(ctx context.Context, tx *gorm.DB, t domain.Tenant) error
	// TenantByID and TenantByIDForUpdate never return soft-deleted tenants.
	TenantByID(ctx context.Context, id string) (domain.Tenant, error)
	TenantByIDForUpdate(ctx context.Context, tx *gorm.DB, id string) (domain.Tenant, error)
	TenantsByIDs(ctx context.Context, ids []string) ([]domain.Tenant, error)
	ListTenants(ctx context.Context, page Page) ([]domain.Tenant, error)
	SaveTenant(ctx context.Context, tx *gorm.DB, t domain.Tenant) error

	// CreateUser inserts the user and its role rows.
	CreateUser(ctx context.Context, tx *gorm.DB, u domain.User) error
	// UserInTenant is the tenant-scoped lookup: another tenant's user is
	// domain.ErrUserNotFound.
	UserInTenant(ctx context.Context, tenantID, id string) (domain.User, error)
	UserInTenantForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.User, error)
	// UserByID ignores tenancy: only session flows (refresh, me) use it.
	UserByID(ctx context.Context, id string) (domain.User, error)
	UserByEmail(ctx context.Context, email string) (domain.User, error)
	ListUsers(ctx context.Context, tenantID string, page Page) ([]domain.User, error)
	UserIDsInTenant(ctx context.Context, tenantID string) ([]string, error)
	// SaveUser writes profile fields guarded by version; roles are separate.
	SaveUser(ctx context.Context, tx *gorm.DB, u domain.User) error
	SetUserRoles(ctx context.Context, tx *gorm.DB, userID string, roleIDs []int64, at time.Time) error
	UpdatePasswordHash(ctx context.Context, tx *gorm.DB, userID, hash string) error
	DeleteUser(ctx context.Context, tx *gorm.DB, tenantID, id string) error
}

// AccessIssuer is the slice of *authn.Issuer this module uses.
type AccessIssuer interface {
	IssueAccess(userID, tenantID string, roleIDs []int64) (token, jti string, err error)
	AccessTTL() time.Duration
}

// RefreshTokens is the slice of *authn.RefreshStore this module uses.
type RefreshTokens interface {
	Issue(ctx context.Context, userID string) (string, error)
	Rotate(ctx context.Context, raw string) (string, error)
	RevokeAll(ctx context.Context, userID string) error
	Deny(ctx context.Context, jti string, ttl time.Duration) error
}

// Settings are the module config values the service needs.
type Settings struct {
	BcryptCost      int
	AllowSelfSignup bool
}

type Service struct {
	repo     Repository
	outbox   outbox.Store
	authz    authz.Enforcer
	issuer   AccessIssuer
	refresh  RefreshTokens
	db       *gorm.DB
	clock    clock.Clock
	log      *zap.Logger
	settings Settings
	pw       *passwords
	keys     APIKeyRepository
	keyCache KeyCache

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, ob outbox.Store, enf authz.Enforcer, issuer AccessIssuer,
	refresh RefreshTokens, db *gorm.DB, c clock.Clock, log *zap.Logger, st Settings) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{
		repo: repo, outbox: ob, authz: enf, issuer: issuer, refresh: refresh,
		db: db, clock: c, log: log, settings: st, pw: newPasswords(st.BcryptCost),
	}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

func pageFrom(limit int, cursor string) (Page, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	p := Page{Limit: limit}
	if cursor != "" {
		ts, id, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
		p.AfterCreated, p.AfterID = ts, id
	}
	return p, nil
}

// trimPage cuts the extra probe row and returns the next cursor.
func trimPage[T any](rows []T, limit int, key func(T) (time.Time, string)) ([]T, string) {
	if len(rows) <= limit {
		return rows, ""
	}
	rows = rows[:limit]
	ts, id := key(rows[len(rows)-1])
	return rows, pagination.EncodeCursor(ts, id)
}

func (s *Service) tokensWired() error {
	if s.issuer == nil || s.refresh == nil {
		return errs.New(errs.Internal, "identity: auth not wired")
	}
	return nil
}

// revokeSessions runs AFTER commit: a failure is logged, never undoes the
// committed change (the access TTL bounds the damage).
func (s *Service) revokeSessions(ctx context.Context, userIDs ...string) {
	if s.refresh == nil {
		return
	}
	for _, id := range userIDs {
		if err := s.refresh.RevokeAll(ctx, id); err != nil {
			s.log.Warn("revoke refresh tokens failed", zap.String("user_id", id), zap.Error(err))
		}
	}
}
