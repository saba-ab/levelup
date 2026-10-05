package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/points/contracts"
	"levelup/internal/modules/points/internal/app"
	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/shared/errs"
)

// reconcileMarker → points_svc.reconcile_markers (single row, id 1).
type reconcileMarker struct {
	ID      int `gorm:"primaryKey"`
	LastRun time.Time
}

var _ app.Reconciler = (*Postgres)(nil)

func (r *Postgres) LastRun(ctx context.Context) (time.Time, error) {
	var m reconcileMarker
	err := r.db.WithContext(ctx).First(&m, "id = 1").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil // never ran: sweep everything
	}
	if err != nil {
		return time.Time{}, errs.Wrap(errs.Internal, "load reconcile marker", err)
	}
	return m.LastRun.UTC(), nil
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

// driftRow is the scan target of the drift query.
type driftRow struct {
	WalletID       string
	TenantID       string
	PlayerID       string
	Balance        int64
	LedgerBalance  int64
	LifetimeEarned int64
	LedgerEarned   int64
	LifetimeSpent  int64
	LedgerSpent    int64
	ChainBreaks    int64
}

// driftSQL checks every wallet touched since the marker (updated, or with a
// ledger entry created) against its full ledger. Refunds are credits that
// undo spending, so they lower the expected lifetime_spent instead of
// raising lifetime_earned. Chain breaks: an entry whose balance_before is
// not the previous entry's balance_after, ordered by wallet_version.
const driftSQL = `
WITH touched AS (
    SELECT id FROM points_svc.wallets WHERE updated_at >= @since
    UNION
    SELECT DISTINCT wallet_id FROM points_svc.ledger_entries WHERE created_at >= @since
),
sums AS (
    SELECT l.wallet_id,
           COALESCE(SUM(l.direction * l.amount), 0) AS ledger_balance,
           COALESCE(SUM(l.amount) FILTER (WHERE l.direction = 1 AND l.kind <> @refund), 0) AS ledger_earned,
           COALESCE(SUM(l.amount) FILTER (WHERE l.direction = -1), 0)
             - COALESCE(SUM(l.amount) FILTER (WHERE l.kind = @refund), 0) AS ledger_spent
    FROM points_svc.ledger_entries l
    WHERE l.wallet_id IN (SELECT id FROM touched)
    GROUP BY l.wallet_id
),
chain AS (
    SELECT wallet_id, COUNT(*) AS chain_breaks
    FROM (
        SELECT wallet_id, balance_before,
               LAG(balance_after) OVER (PARTITION BY wallet_id ORDER BY wallet_version, created_at, id) AS prev_after
        FROM points_svc.ledger_entries
        WHERE wallet_id IN (SELECT id FROM touched)
    ) ordered
    WHERE prev_after IS NOT NULL AND prev_after <> balance_before
    GROUP BY wallet_id
)
SELECT w.id AS wallet_id, w.tenant_id, w.player_id,
       w.balance, COALESCE(s.ledger_balance, 0) AS ledger_balance,
       w.lifetime_earned, COALESCE(s.ledger_earned, 0) AS ledger_earned,
       w.lifetime_spent, COALESCE(s.ledger_spent, 0) AS ledger_spent,
       COALESCE(c.chain_breaks, 0) AS chain_breaks
FROM points_svc.wallets w
JOIN touched t ON t.id = w.id
LEFT JOIN sums s ON s.wallet_id = w.id
LEFT JOIN chain c ON c.wallet_id = w.id
WHERE w.balance <> COALESCE(s.ledger_balance, 0)
   OR w.lifetime_earned <> COALESCE(s.ledger_earned, 0)
   OR w.lifetime_spent <> COALESCE(s.ledger_spent, 0)
   OR COALESCE(c.chain_breaks, 0) > 0
ORDER BY w.id`

func (r *Postgres) FindDrift(ctx context.Context, since time.Time) ([]domain.Drift, error) {
	var rows []driftRow
	err := r.db.WithContext(ctx).Raw(driftSQL, map[string]any{
		"since":  since,
		"refund": contracts.KindRefund,
	}).Scan(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "reconcile drift query", err)
	}
	out := make([]domain.Drift, len(rows))
	for i, d := range rows {
		out[i] = domain.Drift(d)
	}
	return out, nil
}
