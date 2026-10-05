package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/progression/internal/app"
	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/shared/errs"
)

func (r *Postgres) LastRun(ctx context.Context) (time.Time, error) {
	var m reconcileMarker
	err := r.db.WithContext(ctx).First(&m, "id = 1").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil // never ran: sweep everything
	}
	if err != nil {
		return time.Time{}, errs.Wrap(errs.Internal, "load reconcile marker", err)
	}
	return m.LastRun, nil
}

func (r *Postgres) MarkRun(ctx context.Context, at time.Time) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_run"}),
		}).
		Create(&reconcileMarker{ID: 1, LastRun: at}).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "mark reconcile run", err)
	}
	return nil
}

// XPDrift compares each touched progress row with the sum of its ledger.
// Every grant bumps updated_at in the same transaction, so updated_at >=
// since covers every player that gained XP since the marker.
func (r *Postgres) XPDrift(ctx context.Context, since time.Time) ([]app.XPDrift, error) {
	sql := `SELECT p.tenant_id, p.player_id, p.total_xp, s.ledger_xp
FROM ` + r.table("playerProgress") + ` p
CROSS JOIN LATERAL (
    SELECT COALESCE(SUM(g.amount), 0)::BIGINT AS ledger_xp
    FROM ` + r.table("xpGrant") + ` g
    WHERE g.tenant_id = p.tenant_id AND g.player_id = p.player_id
) s
WHERE p.updated_at >= ? AND p.total_xp <> s.ledger_xp
ORDER BY p.tenant_id, p.player_id`
	var rows []struct {
		TenantID string
		PlayerID string
		TotalXP  int64 `gorm:"column:total_xp"`
		LedgerXP int64 `gorm:"column:ledger_xp"`
	}
	if err := r.db.WithContext(ctx).Raw(sql, since).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "reconcile xp drift", err)
	}
	out := make([]app.XPDrift, len(rows))
	for i, row := range rows {
		out[i] = app.XPDrift{TenantID: row.TenantID, PlayerID: row.PlayerID, TotalXP: row.TotalXP, LedgerXP: row.LedgerXP}
	}
	return out, nil
}

func (r *Postgres) ProgressCandidates(ctx context.Context, since time.Time, afterTenant, afterPlayer string, limit int) ([]domain.Progress, error) {
	// Soft-deleted levels count as ladder changes, hence Unscoped.
	changedLadders := r.db.Unscoped().Model(&level{}).Select("tenant_id").Where("updated_at >= ?", since)
	q := r.db.WithContext(ctx).
		Where("updated_at >= ? OR tenant_id IN (?)", since, changedLadders).
		Order("tenant_id ASC, player_id ASC").
		Limit(limit)
	if afterTenant != "" {
		q = q.Where("(tenant_id, player_id) > (?, ?)", afterTenant, afterPlayer)
	}
	var ms []playerProgress
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "reconcile candidates", err)
	}
	out := make([]domain.Progress, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}
