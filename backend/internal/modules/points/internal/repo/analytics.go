package repo

import (
	"context"
	"time"

	"levelup/internal/modules/points/internal/app"
	"levelup/internal/shared/errs"
)

const summarySQL = `
SELECT
    (SELECT COUNT(*) FROM points_svc.wallets WHERE tenant_id = @tenant) AS open_wallets,
    (SELECT COALESCE(SUM(balance), 0) FROM points_svc.wallets WHERE tenant_id = @tenant) AS total_balance,
    (SELECT COALESCE(SUM(lifetime_earned), 0) FROM points_svc.wallets WHERE tenant_id = @tenant) AS lifetime_earned,
    (SELECT COALESCE(SUM(lifetime_spent), 0) FROM points_svc.wallets WHERE tenant_id = @tenant) AS lifetime_spent,
    (SELECT COALESCE(SUM(amount), 0) FROM points_svc.ledger_entries
       WHERE tenant_id = @tenant AND direction = 1 AND created_at >= @since) AS credited_last30d,
    (SELECT COALESCE(SUM(amount), 0) FROM points_svc.ledger_entries
       WHERE tenant_id = @tenant AND direction = -1 AND created_at >= @since) AS debited_last30d`

type summaryRow struct {
	OpenWallets     int64
	TotalBalance    int64
	LifetimeEarned  int64
	LifetimeSpent   int64
	CreditedLast30d int64 `gorm:"column:credited_last30d"`
	DebitedLast30d  int64 `gorm:"column:debited_last30d"`
}

func (r *Postgres) Summary(ctx context.Context, tenantID string, since time.Time) (app.WalletSummary, error) {
	var row summaryRow
	err := r.db.WithContext(ctx).Raw(summarySQL, map[string]any{"tenant": tenantID, "since": since}).Scan(&row).Error
	if err != nil {
		return app.WalletSummary{}, errs.Wrap(errs.Internal, "wallet summary", err)
	}
	return app.WalletSummary(row), nil
}

// distributionSQL sizes the buckets as integers: width = ceil((max-min+1)/n)
// (at least 1), so bucket i covers [lo+i*w, lo+(i+1)*w-1] and the top
// balance always falls inside bucket n (width_bucket's upper bound is
// exclusive and lo+n*w > max).
const distributionSQL = `
WITH b AS (
    SELECT MIN(balance) AS lo, MAX(balance) AS hi
    FROM points_svc.wallets WHERE tenant_id = @tenant
), p AS (
    SELECT lo, GREATEST(CEIL((hi - lo + 1)::numeric / @n), 1)::bigint AS w
    FROM b WHERE lo IS NOT NULL
)
SELECT p.lo, p.w,
       width_bucket(wl.balance, p.lo, p.lo + p.w * @n, @n) AS bucket,
       COUNT(*) AS players
FROM points_svc.wallets wl CROSS JOIN p
WHERE wl.tenant_id = @tenant
GROUP BY p.lo, p.w, bucket`

type bucketRow struct {
	Lo      int64
	W       int64
	Bucket  int
	Players int64
}

func (r *Postgres) BalanceDistribution(ctx context.Context, tenantID string, n int) ([]app.BalanceBucket, error) {
	var rows []bucketRow
	err := r.db.WithContext(ctx).Raw(distributionSQL, map[string]any{"tenant": tenantID, "n": n}).Scan(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "balance distribution", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	lo, w := rows[0].Lo, rows[0].W
	out := make([]app.BalanceBucket, n)
	for i := range out {
		from := lo + int64(i)*w
		out[i] = app.BalanceBucket{From: from, To: from + w - 1}
	}
	for _, row := range rows {
		if row.Bucket >= 1 && row.Bucket <= n {
			out[row.Bucket-1].Players += row.Players
		}
	}
	return out, nil
}

const dailySQL = `
SELECT date_trunc('day', created_at AT TIME ZONE 'UTC') AS day,
       COALESCE(SUM(amount) FILTER (WHERE direction = 1), 0) AS credited,
       COALESCE(SUM(amount) FILTER (WHERE direction = -1), 0) AS debited
FROM points_svc.ledger_entries
WHERE tenant_id = @tenant AND created_at >= @from AND created_at < @to
GROUP BY 1
ORDER BY 1`

type dailyRow struct {
	Day      time.Time
	Credited int64
	Debited  int64
}

func (r *Postgres) DailyTotals(ctx context.Context, tenantID string, from, to time.Time) ([]app.DailyTotal, error) {
	var rows []dailyRow
	err := r.db.WithContext(ctx).Raw(dailySQL, map[string]any{"tenant": tenantID, "from": from, "to": to}).Scan(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "daily totals", err)
	}
	out := make([]app.DailyTotal, len(rows))
	for i, row := range rows {
		d := row.Day
		out[i] = app.DailyTotal{
			Day:      time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC),
			Credited: row.Credited,
			Debited:  row.Debited,
		}
	}
	return out, nil
}
