// Package repo implements player's persistence. The GORM model stays
// unexported here and maps to domain.Player (PRD §7.1 Mechanism 2).
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

	"levelup/internal/modules/player/internal/app"
	"levelup/internal/modules/player/internal/domain"
	"levelup/internal/shared/errs"
)

// player derives table name "players"; the module session prefixes it to
// player_svc.players. NO TableName() override. deleted_at is a plain
// pointer, not gorm.DeletedAt: every query filters it explicitly.
type player struct {
	ID          string         `gorm:"primaryKey;type:uuid"`
	TenantID    string         `gorm:"type:uuid;not null"`
	ExternalID  string         `gorm:"not null"`
	DisplayName *string        `gorm:"column:display_name"`
	Email       *string        `gorm:"column:email"`
	Attributes  map[string]any `gorm:"type:jsonb;serializer:json;not null"`
	IsActive    bool           `gorm:"not null"`
	CreatedBy   *string        `gorm:"type:uuid"`
	Version     int            `gorm:"not null"`
	CreatedAt   time.Time      `gorm:"not null"`
	UpdatedAt   time.Time      `gorm:"not null"`
	DeletedAt   *time.Time
}

func (m player) toDomain() domain.Player {
	attrs := m.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	return domain.Player{
		ID:          m.ID,
		TenantID:    m.TenantID,
		ExternalID:  m.ExternalID,
		DisplayName: deref(m.DisplayName),
		Email:       deref(m.Email),
		Attributes:  attrs,
		Active:      m.IsActive,
		CreatedBy:   deref(m.CreatedBy),
		Version:     m.Version,
		CreatedAt:   m.CreatedAt.UTC(),
		UpdatedAt:   m.UpdatedAt.UTC(),
		DeletedAt:   utcPtr(m.DeletedAt),
	}
}

func fromDomain(p domain.Player) player {
	attrs := p.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	return player{
		ID:          p.ID,
		TenantID:    p.TenantID,
		ExternalID:  p.ExternalID,
		DisplayName: nullable(p.DisplayName),
		Email:       nullable(p.Email),
		Attributes:  attrs,
		IsActive:    p.Active,
		CreatedBy:   nullable(p.CreatedBy),
		Version:     p.Version,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
		DeletedAt:   p.DeletedAt,
	}
}

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

// liveUnique is the partial unique index's predicate: ON CONFLICT must name
// it to infer ux_players_tenant_external.
var liveUnique = clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}}

// Create inserts unless a LIVE player already holds (tenant, external_id).
// Soft-deleted rows do not count, so re-creating a deleted external id works
// (the Laravel 500, doc 03 §10.6).
func (r *Postgres) Create(ctx context.Context, tx *gorm.DB, p domain.Player) error {
	m := fromDomain(p)
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "tenant_id"}, {Name: "external_id"}},
			TargetWhere: liveUnique,
			DoNothing:   true,
		}).
		Create(&m)
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return domain.ErrExternalIDTaken
		}
		return errs.Wrap(errs.Internal, "insert player", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrExternalIDTaken
	}
	return nil
}

func (r *Postgres) live(ctx context.Context, db *gorm.DB, tenantID string) *gorm.DB {
	return db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
}

func (r *Postgres) ByID(ctx context.Context, tenantID, id string) (domain.Player, error) {
	return r.first(r.live(ctx, r.db, tenantID).Where("id = ?", id), "load player")
}

func (r *Postgres) ByExternalID(ctx context.Context, tenantID, externalID string) (domain.Player, error) {
	return r.first(r.live(ctx, r.db, tenantID).Where("external_id = ?", externalID), "load player by external id")
}

// ByIDForUpdate takes the row lock; concurrent writes to one player
// serialize here.
func (r *Postgres) ByIDForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Player, error) {
	q := r.live(ctx, tx, tenantID).Where("id = ?", id).Clauses(clause.Locking{Strength: "UPDATE"})
	return r.first(q, "lock player")
}

func (r *Postgres) first(q *gorm.DB, what string) (domain.Player, error) {
	var m player
	err := q.Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Player{}, domain.ErrNotFound
	case err != nil:
		return domain.Player{}, errs.Wrap(errs.Internal, what, err)
	}
	return m.toDomain(), nil
}

// Save writes the mutable fields guarded by the optimistic version.
func (r *Postgres) Save(ctx context.Context, tx *gorm.DB, p domain.Player) error {
	m := fromDomain(p)
	attrs, err := encodeAttributes(m.Attributes)
	if err != nil {
		return err
	}
	res := tx.WithContext(ctx).Model(&player{}).
		Where("id = ? AND tenant_id = ? AND version = ?", m.ID, m.TenantID, m.Version).
		Updates(map[string]any{
			"display_name": m.DisplayName,
			"email":        m.Email,
			"attributes":   attrs,
			"is_active":    m.IsActive,
			"deleted_at":   m.DeletedAt,
			"updated_at":   m.UpdatedAt,
			"version":      gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "save player", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) List(ctx context.Context, tenantID string, f app.ListFilter) ([]domain.Player, error) {
	q := r.live(ctx, r.db, tenantID).Order("created_at DESC, id DESC").Limit(f.Limit)
	if f.Active != nil {
		q = q.Where("is_active = ?", *f.Active)
	}
	if f.Search != "" {
		pattern := escapeLike(strings.ToLower(f.Search)) + "%"
		q = q.Where(`(lower(external_id) LIKE ? ESCAPE '\' OR lower(display_name) LIKE ? ESCAPE '\' OR lower(email) LIKE ? ESCAPE '\')`,
			pattern, pattern, pattern)
	}
	if f.HasAfter {
		q = q.Where("(created_at, id) < (?, ?)", f.After, f.AfterID)
	}
	var ms []player
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list players", err)
	}
	return toDomainAll(ms), nil
}

func (r *Postgres) ByIDs(ctx context.Context, tenantID string, ids []string) ([]domain.Player, error) {
	if len(ids) == 0 {
		return []domain.Player{}, nil
	}
	var ms []player
	if err := r.live(ctx, r.db, tenantID).Where("id IN ?", ids).Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load players by ids", err)
	}
	return toDomainAll(ms), nil
}

func (r *Postgres) ByExternalIDs(ctx context.Context, tenantID string, externalIDs []string) ([]domain.Player, error) {
	if len(externalIDs) == 0 {
		return []domain.Player{}, nil
	}
	var ms []player
	if err := r.live(ctx, r.db, tenantID).Where("external_id IN ?", externalIDs).Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load players by external ids", err)
	}
	return toDomainAll(ms), nil
}

// PurgeTenantBatch hard-deletes up to limit rows of the tenant, live or
// soft-deleted, returning the removed keys for cache eviction.
func (r *Postgres) PurgeTenantBatch(ctx context.Context, tx *gorm.DB, tenantID string, limit int) ([]app.Ref, error) {
	db := tx.WithContext(ctx)
	sub := db.Session(&gorm.Session{NewDB: true}).Model(&player{}).
		Select("id").Where("tenant_id = ?", tenantID).Limit(limit)
	var gone []player
	err := db.Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}, {Name: "external_id"}}}).
		Where("id IN (?)", sub).
		Delete(&gone).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "purge tenant players", err)
	}
	out := make([]app.Ref, len(gone))
	for i, m := range gone {
		out[i] = app.Ref{ID: m.ID, ExternalID: m.ExternalID}
	}
	return out, nil
}

// Evict is a no-op on the uncached repository.
func (r *Postgres) Evict(context.Context, string, ...app.Ref) {}

func toDomainAll(ms []player) []domain.Player {
	out := make([]domain.Player, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

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

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func encodeAttributes(a map[string]any) (string, error) {
	if a == nil {
		return "{}", nil
	}
	b, err := json.Marshal(a)
	if err != nil {
		return "", errs.Wrap(errs.Invalid, "attributes are not JSON-encodable", err)
	}
	return string(b), nil
}
