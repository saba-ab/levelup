package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/ai/internal/app"
	"levelup/internal/modules/ai/internal/domain"
	"levelup/internal/shared/errs"
)

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

// Reserve is one atomic upsert: the first request of the day inserts the
// row; later ones increment it only while requests < limit, so concurrent
// requests can never overshoot the cap.
func (r *Postgres) Reserve(ctx context.Context, tx *gorm.DB, tenantID string, day time.Time, limit int, at time.Time) (int64, bool, error) {
	if !isUUID(tenantID) {
		return 0, false, errs.New(errs.Invalid, "tenant_id must be a uuid")
	}
	row := aiUsage{TenantID: tenantID, Day: domain.DayOf(day), Requests: 1, CreatedAt: at, UpdatedAt: at}
	conflict := clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "day"}},
		DoUpdates: clause.Assignments(map[string]any{
			"requests":   gorm.Expr("ai_usages.requests + 1"),
			"updated_at": at,
		}),
	}
	if limit > 0 {
		conflict.Where = clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "ai_usages.requests < ?", Vars: []any{limit}},
		}}
	}
	res := tx.WithContext(ctx).
		Clauses(conflict, clause.Returning{Columns: []clause.Column{{Name: "requests"}}}).
		Create(&row)
	if res.Error != nil {
		return 0, false, errs.Wrap(errs.Internal, "reserve ai request", res.Error)
	}
	if res.RowsAffected == 0 {
		return int64(limit), false, nil
	}
	return row.Requests, true, nil
}

func (r *Postgres) AddTokens(ctx context.Context, tx *gorm.DB, tenantID string, day time.Time, input, output int64, at time.Time) error {
	if !isUUID(tenantID) {
		return errs.New(errs.Invalid, "tenant_id must be a uuid")
	}
	row := aiUsage{TenantID: tenantID, Day: domain.DayOf(day), InputTokens: input, OutputTokens: output, CreatedAt: at, UpdatedAt: at}
	err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "day"}},
		DoUpdates: clause.Assignments(map[string]any{
			"input_tokens":  gorm.Expr("ai_usages.input_tokens + ?", input),
			"output_tokens": gorm.Expr("ai_usages.output_tokens + ?", output),
			"updated_at":    at,
		}),
	}).Create(&row).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "record ai tokens", err)
	}
	return nil
}

// UsageBetween returns the tenant's usage rows for days in [from, to],
// newest first.
func (r *Postgres) UsageBetween(ctx context.Context, tenantID string, from, to time.Time) ([]domain.UsageDay, error) {
	if !isUUID(tenantID) {
		return []domain.UsageDay{}, nil
	}
	var rows []aiUsage
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND day BETWEEN ? AND ?", tenantID, domain.DayOf(from), domain.DayOf(to)).
		Order("day DESC").
		Find(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "list ai usage", err)
	}
	out := make([]domain.UsageDay, len(rows))
	for i, m := range rows {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	if !isUUID(tenantID) {
		return errs.New(errs.Invalid, "tenant_id must be a uuid")
	}
	if err := tx.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(&aiUsage{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge tenant ai usage", err)
	}
	return nil
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil && len(s) == 36
}
