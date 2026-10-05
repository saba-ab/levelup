// Package repo implements program's persistence over schema program_svc.
// Models stay unexported; tables derive from struct names (no TableName()).
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/program/internal/app"
	"levelup/internal/modules/program/internal/domain"
	"levelup/internal/shared/errs"
)

// program → table program_svc.programs.
type program struct {
	ID          string `gorm:"primaryKey;type:uuid"`
	TenantID    string `gorm:"type:uuid;not null"`
	Name        string `gorm:"not null"`
	Slug        string `gorm:"not null"`
	Description *string
	Status      string `gorm:"not null"`
	StartsAt    *time.Time
	EndsAt      *time.Time
	// JSONB travels as string: under pgx's exec mode (PgBouncer) a []byte
	// argument is encoded as bytea and Postgres refuses it for jsonb.
	Settings  string `gorm:"type:jsonb;not null"`
	Mechanics string `gorm:"type:jsonb;not null"`
	Version   int    `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// enrollment → table program_svc.enrollments. PlayerID is a bare uuid.
type enrollment struct {
	ProgramID  string `gorm:"primaryKey;type:uuid"`
	PlayerID   string `gorm:"primaryKey;type:uuid"`
	TenantID   string `gorm:"type:uuid;not null"`
	EnrolledAt time.Time
}

func (m program) toDomain() (domain.Program, error) {
	settings, err := decodeObject(m.Settings)
	if err != nil {
		return domain.Program{}, err
	}
	mechanics, err := decodeObject(m.Mechanics)
	if err != nil {
		return domain.Program{}, err
	}
	return domain.Program{
		ID:          m.ID,
		TenantID:    m.TenantID,
		Name:        m.Name,
		Slug:        m.Slug,
		Description: m.Description,
		Status:      domain.Status(m.Status),
		StartsAt:    utc(m.StartsAt),
		EndsAt:      utc(m.EndsAt),
		Settings:    settings,
		Mechanics:   mechanics,
		Version:     m.Version,
		CreatedAt:   m.CreatedAt.UTC(),
		UpdatedAt:   m.UpdatedAt.UTC(),
		DeletedAt:   utc(m.DeletedAt),
	}, nil
}

func fromDomain(p domain.Program) (program, error) {
	settings, err := encodeObject(p.Settings)
	if err != nil {
		return program{}, err
	}
	mechanics, err := encodeObject(p.Mechanics)
	if err != nil {
		return program{}, err
	}
	return program{
		ID:          p.ID,
		TenantID:    p.TenantID,
		Name:        p.Name,
		Slug:        p.Slug,
		Description: p.Description,
		Status:      string(p.Status),
		StartsAt:    p.StartsAt,
		EndsAt:      p.EndsAt,
		Settings:    settings,
		Mechanics:   mechanics,
		Version:     p.Version,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
		DeletedAt:   p.DeletedAt,
	}, nil
}

func (m enrollment) toDomain() domain.Enrollment {
	return domain.Enrollment{
		ProgramID:  m.ProgramID,
		TenantID:   m.TenantID,
		PlayerID:   m.PlayerID,
		EnrolledAt: m.EnrolledAt.UTC(),
	}
}

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

func (r *Postgres) Create(ctx context.Context, tx *gorm.DB, p domain.Program) error {
	m, err := fromDomain(p)
	if err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "insert program", err)
	}
	return nil
}

func (r *Postgres) ByID(ctx context.Context, tenantID, id string) (domain.Program, error) {
	return r.first(r.db.WithContext(ctx), tenantID, id, "load program")
}

func (r *Postgres) ByIDForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Program, error) {
	return r.first(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}), tenantID, id, "lock program")
}

func (r *Postgres) ByIDForShare(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Program, error) {
	return r.first(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}), tenantID, id, "share-lock program")
}

func (r *Postgres) first(q *gorm.DB, tenantID, id, op string) (domain.Program, error) {
	var m program
	err := q.Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantID, id).First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Program{}, domain.ErrNotFound
	case err != nil:
		return domain.Program{}, errs.Wrap(errs.Internal, op, err)
	}
	return m.toDomain()
}

func (r *Postgres) ByIDs(ctx context.Context, tenantID string, ids []string) ([]domain.Program, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var ms []program
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id IN ? AND deleted_at IS NULL", tenantID, ids).
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load programs", err)
	}
	return toDomains(ms)
}

// Save writes every mutable column guarded by the optimistic version.
func (r *Postgres) Save(ctx context.Context, tx *gorm.DB, p domain.Program) error {
	m, err := fromDomain(p)
	if err != nil {
		return err
	}
	res := tx.WithContext(ctx).Model(&program{}).
		Where("tenant_id = ? AND id = ? AND version = ?", m.TenantID, m.ID, m.Version).
		Updates(map[string]any{
			"name":        m.Name,
			"slug":        m.Slug,
			"description": m.Description,
			"status":      m.Status,
			"starts_at":   m.StartsAt,
			"ends_at":     m.EndsAt,
			"settings":    m.Settings,
			"mechanics":   m.Mechanics,
			"updated_at":  m.UpdatedAt,
			"deleted_at":  m.DeletedAt,
			"version":     gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "save program", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) List(ctx context.Context, tenantID string, status domain.Status, page app.Page) ([]domain.Program, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("created_at DESC, id DESC").
		Limit(page.Limit)
	if status != "" {
		q = q.Where("status = ?", string(status))
	}
	if page.ID != "" {
		q = q.Where("(created_at, id) < (?, ?)", page.At, page.ID)
	}
	var ms []program
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list programs", err)
	}
	return toDomains(ms)
}

func (r *Postgres) DueForAutoEnd(ctx context.Context, now time.Time, limit int) ([]domain.Program, error) {
	var ms []program
	err := r.db.WithContext(ctx).
		Where("status IN ? AND ends_at IS NOT NULL AND ends_at <= ? AND deleted_at IS NULL",
			[]string{string(domain.StatusActive), string(domain.StatusPaused)}, now).
		Order("ends_at ASC, id ASC").
		Limit(limit).
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "list programs due for auto-end", err)
	}
	return toDomains(ms)
}

func (r *Postgres) Enroll(ctx context.Context, tx *gorm.DB, e domain.Enrollment) (bool, error) {
	m := enrollment{ProgramID: e.ProgramID, PlayerID: e.PlayerID, TenantID: e.TenantID, EnrolledAt: e.EnrolledAt}
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "program_id"}, {Name: "player_id"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert enrollment", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) Enrollment(ctx context.Context, tenantID, programID, playerID string) (domain.Enrollment, bool, error) {
	var ms []enrollment
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND program_id = ? AND player_id = ?", tenantID, programID, playerID).
		Limit(1).
		Find(&ms).Error
	if err != nil {
		return domain.Enrollment{}, false, errs.Wrap(errs.Internal, "load enrollment", err)
	}
	if len(ms) == 0 {
		return domain.Enrollment{}, false, nil
	}
	return ms[0].toDomain(), true, nil
}

func (r *Postgres) Unenroll(ctx context.Context, tx *gorm.DB, tenantID, programID, playerID string) (bool, error) {
	res := tx.WithContext(ctx).
		Where("tenant_id = ? AND program_id = ? AND player_id = ?", tenantID, programID, playerID).
		Delete(&enrollment{})
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "delete enrollment", res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (r *Postgres) ListEnrollments(ctx context.Context, tenantID, programID string, page app.Page) ([]domain.Enrollment, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND program_id = ?", tenantID, programID).
		Order("enrolled_at DESC, player_id DESC").
		Limit(page.Limit)
	if page.ID != "" {
		q = q.Where("(enrolled_at, player_id) < (?, ?)", page.At, page.ID)
	}
	var ms []enrollment
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list enrollments", err)
	}
	out := make([]domain.Enrollment, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// RemovePlayer is DELETE ... RETURNING: the caller publishes exactly one
// event per row this statement removed, so a redelivery publishes nothing.
func (r *Postgres) RemovePlayer(ctx context.Context, tx *gorm.DB, tenantID, playerID string) ([]domain.Enrollment, error) {
	var ms []enrollment
	err := tx.WithContext(ctx).
		Clauses(clause.Returning{}).
		Where("tenant_id = ? AND player_id = ?", tenantID, playerID).
		Delete(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "delete player enrollments", err)
	}
	out := make([]domain.Enrollment, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	db := tx.WithContext(ctx)
	if err := db.Where("tenant_id = ?", tenantID).Delete(&enrollment{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge tenant enrollments", err)
	}
	if err := db.Where("tenant_id = ?", tenantID).Delete(&program{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge tenant programs", err)
	}
	return nil
}

// ActiveProgramIDsForPlayer joins two tables of program's OWN schema.
func (r *Postgres) ActiveProgramIDsForPlayer(ctx context.Context, tenantID, playerID string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).
		Model(&enrollment{}).
		Joins("JOIN program_svc.programs p ON p.id = enrollments.program_id").
		Where("enrollments.tenant_id = ? AND enrollments.player_id = ? AND p.tenant_id = ? AND p.status = ? AND p.deleted_at IS NULL",
			tenantID, playerID, tenantID, string(domain.StatusActive)).
		Order("enrollments.enrolled_at ASC").
		Pluck("enrollments.program_id", &ids).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "list player programs", err)
	}
	return ids, nil
}

func toDomains(ms []program) ([]domain.Program, error) {
	out := make([]domain.Program, len(ms))
	for i, m := range ms {
		p, err := m.toDomain()
		if err != nil {
			return nil, err
		}
		out[i] = p
	}
	return out, nil
}

func decodeObject(raw string) (map[string]any, error) {
	out := map[string]any{}
	if raw == "" || raw == "null" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, errs.Wrap(errs.Internal, "decode program json column", err)
	}
	return out, nil
}

func encodeObject(m map[string]any) (string, error) {
	if m == nil {
		m = map[string]any{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", errs.Wrap(errs.Invalid, "program json column is not serialisable", err)
	}
	return string(b), nil
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
