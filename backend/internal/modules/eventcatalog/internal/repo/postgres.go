// Package repo implements eventcatalog's persistence. Models stay
// unexported and map to domain types (PRD §7.1).
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/eventcatalog/internal/app"
	"levelup/internal/modules/eventcatalog/internal/domain"
	"levelup/internal/shared/errs"
)

// eventType derives table event_types → eventcatalog_svc.event_types. NO
// TableName() override (the module TablePrefix ignores those).
type eventType struct {
	ID          string  `gorm:"primaryKey;type:uuid"`
	TenantID    *string `gorm:"type:uuid"` // NULL = platform-global
	CategoryID  *string `gorm:"type:uuid"`
	Slug        string
	Name        string
	Description *string
	// PropertySchema is JSONB carried as its text form: a string parameter
	// is valid for jsonb under every pgx exec mode, a []byte is not.
	PropertySchema *string `gorm:"type:jsonb"`
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

// eventCategory derives table event_categories.
type eventCategory struct {
	ID          string  `gorm:"primaryKey;type:uuid"`
	TenantID    *string `gorm:"type:uuid"`
	Slug        string
	Name        string
	Description *string
	SortOrder   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

// --- mapping ---------------------------------------------------------------

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func encodeSchema(schema map[string]any) (*string, error) {
	if schema == nil {
		return nil, nil
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return nil, errs.Wrap(errs.Invalid, "property_schema is not serialisable", err)
	}
	s := string(b)
	return &s, nil
}

func (m eventType) toDomain() (domain.EventType, error) {
	var schema map[string]any
	if m.PropertySchema != nil {
		if err := json.Unmarshal([]byte(*m.PropertySchema), &schema); err != nil {
			return domain.EventType{}, errs.Wrap(errs.Internal, "decode property_schema", err)
		}
	}
	return domain.EventType{
		ID:             m.ID,
		TenantID:       deref(m.TenantID),
		CategoryID:     deref(m.CategoryID),
		Slug:           m.Slug,
		Name:           m.Name,
		Description:    deref(m.Description),
		PropertySchema: schema,
		Active:         m.IsActive,
		CreatedAt:      m.CreatedAt.UTC(),
		UpdatedAt:      m.UpdatedAt.UTC(),
	}, nil
}

func typeFromDomain(et domain.EventType) (eventType, error) {
	schema, err := encodeSchema(et.PropertySchema)
	if err != nil {
		return eventType{}, err
	}
	return eventType{
		ID:             et.ID,
		TenantID:       nullable(et.TenantID),
		CategoryID:     nullable(et.CategoryID),
		Slug:           et.Slug,
		Name:           et.Name,
		Description:    nullable(et.Description),
		PropertySchema: schema,
		IsActive:       et.Active,
		CreatedAt:      et.CreatedAt,
		UpdatedAt:      et.UpdatedAt,
	}, nil
}

func (m eventCategory) toDomain() domain.Category {
	return domain.Category{
		ID:          m.ID,
		TenantID:    deref(m.TenantID),
		Slug:        m.Slug,
		Name:        m.Name,
		Description: deref(m.Description),
		SortOrder:   m.SortOrder,
		CreatedAt:   m.CreatedAt.UTC(),
		UpdatedAt:   m.UpdatedAt.UTC(),
	}
}

func categoryFromDomain(c domain.Category) eventCategory {
	return eventCategory{
		ID:          c.ID,
		TenantID:    nullable(c.TenantID),
		Slug:        c.Slug,
		Name:        c.Name,
		Description: nullable(c.Description),
		SortOrder:   c.SortOrder,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

func typesToDomain(ms []eventType) ([]domain.EventType, error) {
	out := make([]domain.EventType, 0, len(ms))
	for _, m := range ms {
		et, err := m.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, et)
	}
	return out, nil
}

// --- scoping -----------------------------------------------------------------

// scoped restricts q to the rows scope may see: globals only for the
// platform scope, own + global for a tenant. Never unscoped (ADR-0015).
func scoped(q *gorm.DB, s app.Scope) *gorm.DB {
	if s.TenantID == "" {
		return q.Where("tenant_id IS NULL")
	}
	return q.Where("(tenant_id = ? OR tenant_id IS NULL)", s.TenantID)
}

// table returns the schema-qualified table of a model, for the correlated
// shadowing subqueries GORM cannot express.
func (r *Postgres) table(model string) string {
	return r.db.NamingStrategy.TableName(model)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// likePattern escapes LIKE metacharacters so a search is a substring match.
func likePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

// --- event types ---------------------------------------------------------------

func (r *Postgres) CreateType(ctx context.Context, tx *gorm.DB, et domain.EventType) error {
	m, err := typeFromDomain(et)
	if err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		return mapTypeWriteErr("insert event type", err)
	}
	return nil
}

func (r *Postgres) SaveType(ctx context.Context, tx *gorm.DB, et domain.EventType) error {
	m, err := typeFromDomain(et)
	if err != nil {
		return err
	}
	res := tx.WithContext(ctx).Model(&eventType{}).
		Where("id = ? AND deleted_at IS NULL", m.ID).
		Updates(map[string]any{
			"name":            m.Name,
			"description":     m.Description,
			"category_id":     m.CategoryID,
			"property_schema": m.PropertySchema,
			"is_active":       m.IsActive,
			"updated_at":      m.UpdatedAt,
		})
	if res.Error != nil {
		return mapTypeWriteErr("save event type", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func mapTypeWriteErr(op string, err error) error {
	switch {
	case isUniqueViolation(err):
		return domain.ErrSlugTaken
	case isForeignKeyViolation(err):
		// The category vanished between the service's check and the write.
		return domain.ErrUnknownCategory
	}
	return errs.Wrap(errs.Internal, op, err)
}

func (r *Postgres) SoftDeleteType(ctx context.Context, tx *gorm.DB, id string, at time.Time) error {
	res := tx.WithContext(ctx).Model(&eventType{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]any{"deleted_at": at, "updated_at": at})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "delete event type", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Postgres) TypeByID(ctx context.Context, scope app.Scope, id string) (domain.EventType, error) {
	return r.typeByID(scoped(r.db.WithContext(ctx), scope), id)
}

// TypeByIDForUpdate locks the row: concurrent updates of one event type
// serialize instead of losing writes.
func (r *Postgres) TypeByIDForUpdate(ctx context.Context, tx *gorm.DB, scope app.Scope, id string) (domain.EventType, error) {
	q := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"})
	return r.typeByID(scoped(q, scope), id)
}

func (r *Postgres) typeByID(q *gorm.DB, id string) (domain.EventType, error) {
	var ms []eventType
	err := q.Where("id = ? AND deleted_at IS NULL", id).Limit(1).Find(&ms).Error
	if err != nil {
		return domain.EventType{}, errs.Wrap(errs.Internal, "load event type", err)
	}
	if len(ms) == 0 {
		return domain.EventType{}, domain.ErrNotFound
	}
	return ms[0].toDomain()
}

func (r *Postgres) ListTypes(ctx context.Context, f app.TypeFilter) ([]domain.EventType, error) {
	q := r.db.WithContext(ctx).Model(&eventType{}).Where("deleted_at IS NULL")
	switch {
	case f.Scope.TenantID == "":
		q = q.Where("tenant_id IS NULL")
	case f.IncludeGlobal:
		// A tenant row shadows the global row with its slug: show one.
		q = q.Where(`(tenant_id = ? OR (tenant_id IS NULL AND NOT EXISTS (
			SELECT 1 FROM `+r.table("eventType")+` s
			WHERE s.tenant_id = ? AND s.slug = event_types.slug AND s.deleted_at IS NULL)))`,
			f.Scope.TenantID, f.Scope.TenantID)
	default:
		q = q.Where("tenant_id = ?", f.Scope.TenantID)
	}
	if f.CategoryID != "" {
		q = q.Where("category_id = ?", f.CategoryID)
	}
	if f.Active != nil {
		q = q.Where("is_active = ?", *f.Active)
	}
	if f.Search != "" {
		p := likePattern(f.Search)
		q = q.Where("(name ILIKE ? OR slug ILIKE ? OR description ILIKE ?)", p, p, p)
	}
	if !f.AfterTime.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", f.AfterTime, f.AfterID)
	}
	var ms []eventType
	if err := q.Order("created_at DESC, id DESC").Limit(f.Limit).Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list event types", err)
	}
	return typesToDomain(ms)
}

func (r *Postgres) TypesBySlugs(ctx context.Context, scope app.Scope, slugs []string) ([]domain.EventType, error) {
	if len(slugs) == 0 {
		return []domain.EventType{}, nil
	}
	var ms []eventType
	err := scoped(r.db.WithContext(ctx), scope).
		Where("slug IN ? AND deleted_at IS NULL", slugs).
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load event types by slug", err)
	}
	return typesToDomain(ms)
}

// --- categories ------------------------------------------------------------------

func (r *Postgres) CreateCategory(ctx context.Context, tx *gorm.DB, c domain.Category) error {
	m := categoryFromDomain(c)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrCategorySlugTaken
		}
		return errs.Wrap(errs.Internal, "insert event category", err)
	}
	return nil
}

func (r *Postgres) SaveCategory(ctx context.Context, tx *gorm.DB, c domain.Category) error {
	m := categoryFromDomain(c)
	res := tx.WithContext(ctx).Model(&eventCategory{}).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"slug":        m.Slug,
			"name":        m.Name,
			"description": m.Description,
			"sort_order":  m.SortOrder,
			"updated_at":  m.UpdatedAt,
		})
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return domain.ErrCategorySlugTaken
		}
		return errs.Wrap(errs.Internal, "save event category", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrCategoryNotFound
	}
	return nil
}

func (r *Postgres) DeleteCategory(ctx context.Context, tx *gorm.DB, id string) error {
	res := tx.WithContext(ctx).Where("id = ?", id).Delete(&eventCategory{})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "delete event category", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrCategoryNotFound
	}
	return nil
}

func (r *Postgres) CategoryByID(ctx context.Context, scope app.Scope, id string) (domain.Category, error) {
	var ms []eventCategory
	err := scoped(r.db.WithContext(ctx), scope).Where("id = ?", id).Limit(1).Find(&ms).Error
	if err != nil {
		return domain.Category{}, errs.Wrap(errs.Internal, "load event category", err)
	}
	if len(ms) == 0 {
		return domain.Category{}, domain.ErrCategoryNotFound
	}
	return ms[0].toDomain(), nil
}

func (r *Postgres) CategoryBySlug(ctx context.Context, scope app.Scope, slug string) (domain.Category, error) {
	var ms []eventCategory
	err := scoped(r.db.WithContext(ctx), scope).
		Where("slug = ?", slug).
		Order("tenant_id NULLS LAST"). // the tenant's own row wins
		Limit(1).Find(&ms).Error
	if err != nil {
		return domain.Category{}, errs.Wrap(errs.Internal, "load event category by slug", err)
	}
	if len(ms) == 0 {
		return domain.Category{}, domain.ErrCategoryNotFound
	}
	return ms[0].toDomain(), nil
}

func (r *Postgres) ListCategories(ctx context.Context, scope app.Scope, limit int) ([]domain.Category, error) {
	q := r.db.WithContext(ctx).Model(&eventCategory{})
	if scope.TenantID == "" {
		q = q.Where("tenant_id IS NULL")
	} else {
		q = q.Where(`(tenant_id = ? OR (tenant_id IS NULL AND NOT EXISTS (
			SELECT 1 FROM `+r.table("eventCategory")+` s
			WHERE s.tenant_id = ? AND s.slug = event_categories.slug)))`,
			scope.TenantID, scope.TenantID)
	}
	var ms []eventCategory
	if err := q.Order("sort_order ASC, name ASC, id ASC").Limit(limit).Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list event categories", err)
	}
	out := make([]domain.Category, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// --- tenant purge ------------------------------------------------------------------

// PurgeTenant hard-deletes the tenant's rows, event types first. A second
// run deletes nothing, which is what makes the subscriber idempotent.
func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	if err := tx.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(&eventType{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge tenant event types", err)
	}
	if err := tx.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(&eventCategory{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge tenant event categories", err)
	}
	return nil
}
