// Package repo implements leaderboards' persistence: Postgres (the truth)
// and the redis-core sorted-set read model. Models stay unexported.
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/modules/leaderboards/internal/app"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/shared/errs"
)

// Raw SQL does not get GORM's TablePrefix, so it names the schema itself.
const (
	tBoards    = "leaderboards_svc.leaderboards"
	tPeriods   = "leaderboards_svc.leaderboard_periods"
	tScores    = "leaderboards_svc.leaderboard_scores"
	tSnapshots = "leaderboards_svc.leaderboard_snapshots"
	tApplied   = "leaderboards_svc.applied_events"
	tMembers   = "leaderboards_svc.program_members"
	tHidden    = "leaderboards_svc.hidden_players"
)

// leaderboard derives table "leaderboards" (prefixed leaderboards_svc.).
type leaderboard struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid;not null"`
	Slug           string `gorm:"not null"`
	Name           string `gorm:"not null"`
	Description    string `gorm:"not null"`
	Type           string `gorm:"not null"`
	Metric         string `gorm:"not null"`
	ResetFrequency string `gorm:"not null"`
	ProgramID      *string
	// JSONB travels as string: under pgx's exec mode (PgBouncer) a []byte
	// argument is encoded as bytea and Postgres refuses it for jsonb.
	Config     string `gorm:"type:jsonb;not null"`
	MaxEntries int    `gorm:"not null"`
	IsActive   bool   `gorm:"not null"`
	Version    int    `gorm:"not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  *time.Time
}

// boardConfig is the JSON shape of leaderboards.config.
type boardConfig struct {
	EventType string `json:"event_type,omitempty"`
	Value     string `json:"value,omitempty"`
	Property  string `json:"property,omitempty"`
}

func encodeConfig(a *domain.ActivityConfig) string {
	if a == nil {
		return "{}"
	}
	b, _ := json.Marshal(boardConfig{EventType: a.EventType, Value: a.Value, Property: a.Property}) // plain strings: cannot fail
	return string(b)
}

func decodeConfig(typ, raw string) *domain.ActivityConfig {
	if typ != contracts.TypeActivity {
		return nil
	}
	var c boardConfig
	_ = json.Unmarshal([]byte(raw), &c) // the CHECK constraint guarantees the keys
	return &domain.ActivityConfig{EventType: c.EventType, Value: c.Value, Property: c.Property}
}

func (m leaderboard) toDomain() domain.Leaderboard {
	lb := domain.Leaderboard{
		ID:             m.ID,
		TenantID:       m.TenantID,
		Slug:           m.Slug,
		Name:           m.Name,
		Description:    m.Description,
		Type:           m.Type,
		Metric:         m.Metric,
		ResetFrequency: m.ResetFrequency,
		Activity:       decodeConfig(m.Type, m.Config),
		MaxEntries:     m.MaxEntries,
		Active:         m.IsActive,
		Version:        m.Version,
		CreatedAt:      m.CreatedAt.UTC(),
		UpdatedAt:      m.UpdatedAt.UTC(),
	}
	if m.ProgramID != nil {
		lb.ProgramID = *m.ProgramID
	}
	if m.DeletedAt != nil {
		t := m.DeletedAt.UTC()
		lb.DeletedAt = &t
	}
	return lb
}

func fromDomain(lb domain.Leaderboard) leaderboard {
	m := leaderboard{
		ID:             lb.ID,
		TenantID:       lb.TenantID,
		Slug:           lb.Slug,
		Name:           lb.Name,
		Description:    lb.Description,
		Type:           lb.Type,
		Metric:         lb.Metric,
		ResetFrequency: lb.ResetFrequency,
		Config:         encodeConfig(lb.Activity),
		MaxEntries:     lb.MaxEntries,
		IsActive:       lb.Active,
		Version:        lb.Version,
		CreatedAt:      lb.CreatedAt,
		UpdatedAt:      lb.UpdatedAt,
		DeletedAt:      lb.DeletedAt,
	}
	if lb.ProgramID != "" {
		p := lb.ProgramID
		m.ProgramID = &p
	}
	return m
}

// reconcileMarker derives table "reconcile_markers".
type reconcileMarker struct {
	Job     string `gorm:"primaryKey"`
	LastRun time.Time
}

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func validUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func (r *Postgres) Create(ctx context.Context, tx *gorm.DB, lb domain.Leaderboard) error {
	m := fromDomain(lb)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "insert leaderboard", err)
	}
	return nil
}

func (r *Postgres) ByID(ctx context.Context, tenantID, id string) (domain.Leaderboard, error) {
	if !validUUID(id) || !validUUID(tenantID) {
		return domain.Leaderboard{}, domain.ErrNotFound
	}
	var m leaderboard
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Leaderboard{}, domain.ErrNotFound
	case err != nil:
		return domain.Leaderboard{}, errs.Wrap(errs.Internal, "load leaderboard", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) List(ctx context.Context, tenantID string, f app.ListFilter, beforeAt time.Time, beforeID string, limit int) ([]domain.Leaderboard, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("created_at DESC, id DESC").
		Limit(limit)
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.Active != nil {
		q = q.Where("is_active = ?", *f.Active)
	}
	if !beforeAt.IsZero() {
		if !validUUID(beforeID) {
			return nil, errs.New(errs.Invalid, "malformed cursor")
		}
		q = q.Where("(created_at, id) < (?, ?)", beforeAt, beforeID)
	}
	var ms []leaderboard
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list leaderboards", err)
	}
	return toDomains(ms), nil
}

func toDomains(ms []leaderboard) []domain.Leaderboard {
	out := make([]domain.Leaderboard, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out
}

// Save writes the mutable fields guarded by the optimistic version.
func (r *Postgres) Save(ctx context.Context, tx *gorm.DB, lb domain.Leaderboard) error {
	res := tx.WithContext(ctx).Model(&leaderboard{}).
		Where("id = ? AND tenant_id = ? AND version = ? AND deleted_at IS NULL", lb.ID, lb.TenantID, lb.Version).
		Updates(map[string]any{
			"name":        lb.Name,
			"slug":        lb.Slug,
			"description": lb.Description,
			"max_entries": lb.MaxEntries,
			"is_active":   lb.Active,
			"updated_at":  lb.UpdatedAt,
			"version":     gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "save leaderboard", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) SoftDelete(ctx context.Context, tx *gorm.DB, lb domain.Leaderboard, at time.Time) error {
	res := tx.WithContext(ctx).Model(&leaderboard{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", lb.ID, lb.TenantID).
		Updates(map[string]any{"deleted_at": at, "updated_at": at, "version": gorm.Expr("version + 1")})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "delete leaderboard", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Postgres) ActiveByType(ctx context.Context, tenantID, typ string) ([]domain.Leaderboard, error) {
	if !validUUID(tenantID) {
		return nil, nil
	}
	var ms []leaderboard
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND type = ? AND is_active AND deleted_at IS NULL", tenantID, typ).
		Order("id").
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load active leaderboards", err)
	}
	return toDomains(ms), nil
}

// ActiveForActivity returns the tenant's live active activity boards for
// one event type (index ix_leaderboards_activity_event).
func (r *Postgres) ActiveForActivity(ctx context.Context, tenantID, eventType string) ([]domain.Leaderboard, error) {
	if !validUUID(tenantID) || eventType == "" {
		return nil, nil
	}
	var ms []leaderboard
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND type = ? AND config ->> 'event_type' = ? AND is_active AND deleted_at IS NULL",
			tenantID, contracts.TypeActivity, eventType).
		Order("id").
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load activity leaderboards", err)
	}
	return toDomains(ms), nil
}

func (r *Postgres) ActiveBoards(ctx context.Context, afterID string, limit int) ([]domain.Leaderboard, error) {
	q := r.db.WithContext(ctx).Where("is_active AND deleted_at IS NULL").Order("id").Limit(limit)
	if afterID != "" {
		q = q.Where("id > ?", afterID)
	}
	var ms []leaderboard
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "page active leaderboards", err)
	}
	return toDomains(ms), nil
}

func (r *Postgres) LastRun(ctx context.Context, job string) (time.Time, error) {
	var m reconcileMarker
	err := r.db.WithContext(ctx).First(&m, "job = ?", job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, errs.Wrap(errs.Internal, "load reconcile marker", err)
	}
	return m.LastRun.UTC(), nil
}

func (r *Postgres) MarkRun(ctx context.Context, job string, at time.Time) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "job"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_run"}),
		}).
		Create(&reconcileMarker{Job: job, LastRun: at}).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "mark reconcile run", err)
	}
	return nil
}
