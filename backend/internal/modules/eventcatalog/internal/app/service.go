// Package app holds eventcatalog's use cases: transaction boundaries,
// authorization, tenant visibility and every outbox publish (ADR-0013).
package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/eventcatalog/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
)

const (
	DefaultPageSize = 25
	MaxPageSize     = 100
	// MaxCategories bounds the (unpaginated, sort_order-ordered) category
	// listing: categories are a small curated catalogue.
	MaxCategories = 500
	// MaxReaderSlugs bounds one Reader batch.
	MaxReaderSlugs = 500
)

// Scope is the tenant a repository read is evaluated for. TenantID ""
// means the platform scope: global rows only. A tenant scope sees its own
// rows plus the global ones; nothing ever reads across tenants.
type Scope struct{ TenantID string }

// TypeFilter selects event types for a list page.
type TypeFilter struct {
	Scope Scope
	// IncludeGlobal adds global rows to a tenant scope, hiding globals that
	// one of the tenant's own rows shadows by slug. Ignored for the platform
	// scope, which only has globals.
	IncludeGlobal bool
	CategoryID    string
	Active        *bool
	Search        string
	// After is the keyset position (created_at, id) of the previous page's
	// last row; zero = first page.
	AfterTime time.Time
	AfterID   string
	Limit     int
}

// Repository is consumed by this service and implemented in internal/repo.
// Every read takes the scope explicitly (ADR-0015); a row outside the scope
// is NotFound.
type Repository interface {
	CreateType(ctx context.Context, tx *gorm.DB, et domain.EventType) error
	SaveType(ctx context.Context, tx *gorm.DB, et domain.EventType) error
	SoftDeleteType(ctx context.Context, tx *gorm.DB, id string, at time.Time) error
	TypeByID(ctx context.Context, scope Scope, id string) (domain.EventType, error)
	TypeByIDForUpdate(ctx context.Context, tx *gorm.DB, scope Scope, id string) (domain.EventType, error)
	ListTypes(ctx context.Context, f TypeFilter) ([]domain.EventType, error)
	// TypesBySlugs returns every live row visible to scope carrying one of
	// slugs, tenant and global alike; the caller resolves shadowing.
	TypesBySlugs(ctx context.Context, scope Scope, slugs []string) ([]domain.EventType, error)

	CreateCategory(ctx context.Context, tx *gorm.DB, c domain.Category) error
	SaveCategory(ctx context.Context, tx *gorm.DB, c domain.Category) error
	DeleteCategory(ctx context.Context, tx *gorm.DB, id string) error
	CategoryByID(ctx context.Context, scope Scope, id string) (domain.Category, error)
	// CategoryBySlug resolves a slug for scope, a tenant row winning over a
	// global one.
	CategoryBySlug(ctx context.Context, scope Scope, slug string) (domain.Category, error)
	// ListCategories returns scope's categories (globals shadowed by a
	// tenant slug hidden), ordered by sort_order, name.
	ListCategories(ctx context.Context, scope Scope, limit int) ([]domain.Category, error)

	// PurgeTenant hard-deletes every row the tenant owns. Idempotent.
	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
}

type Service struct {
	repo   Repository
	outbox outbox.Store
	authz  authz.Enforcer
	db     *gorm.DB
	clock  clock.Clock

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, ob outbox.Store, enf authz.Enforcer, db *gorm.DB, c clock.Clock) *Service {
	s := &Service{repo: repo, outbox: ob, authz: enf, db: db, clock: c}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

// Page is one keyset page. NextCursor is "" on the last page.
type Page[T any] struct {
	Items      []T
	NextCursor string
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultPageSize
	case limit > MaxPageSize:
		return MaxPageSize
	}
	return limit
}
