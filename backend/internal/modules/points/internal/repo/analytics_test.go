package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/points/internal/app"
	"levelup/internal/shared/id"
)

func TestSummaryDistributionAndDailySQL(t *testing.T) {
	svc, r, _, tenant := newService(t)
	ctx := context.Background()
	a, b, c := id.NewID(), id.NewID(), id.NewID()
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, a, "a1", 1000)))
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, b, "b1", 10)))
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, c, "c1", 501)))
	require.NoError(t, svc.HandleDebit(ctx, debit(tenant, a, "a2", 400)))
	// Another tenant's rows never leak into the aggregates.
	require.NoError(t, svc.HandleCredit(ctx, credit(id.NewID(), a, "x1", 99999)))
	// An old credit falls outside the trailing window and the daily range.
	old := time.Now().UTC().AddDate(0, 0, -45)
	require.NoError(t, sharedDB.Exec(
		`UPDATE points_svc.ledger_entries SET created_at = ? WHERE tenant_id = ? AND idempotency_key = 'c1'`,
		old, tenant).Error)

	s, err := r.Summary(ctx, tenant, time.Now().UTC().Add(-30*24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, app.WalletSummary{
		OpenWallets: 3, TotalBalance: 1111, LifetimeEarned: 1511, LifetimeSpent: 400,
		CreditedLast30d: 1010, DebitedLast30d: 400,
	}, s)

	// Balances 600, 10, 501: lo 10, width ceil(591/10)=60.
	bs, err := r.BalanceDistribution(ctx, tenant, 10)
	require.NoError(t, err)
	require.Len(t, bs, 10)
	require.Equal(t, app.BalanceBucket{From: 10, To: 69, Players: 1}, bs[0])
	require.Equal(t, app.BalanceBucket{From: 490, To: 549, Players: 1}, bs[8])
	require.Equal(t, app.BalanceBucket{From: 550, To: 609, Players: 1}, bs[9])
	var total int64
	for _, x := range bs {
		total += x.Players
	}
	require.Equal(t, int64(3), total)

	empty, err := r.BalanceDistribution(ctx, id.NewID(), 10)
	require.NoError(t, err)
	require.Nil(t, empty)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	rows, err := r.DailyTotals(ctx, tenant, today.AddDate(0, 0, -7), today.AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Equal(t, []app.DailyTotal{{Day: today, Credited: 1010, Debited: 400}}, rows)

	rows, err = r.DailyTotals(ctx, tenant, old.Truncate(24*time.Hour), old.Truncate(24*time.Hour).AddDate(0, 0, 1))
	require.NoError(t, err)
	require.Equal(t, []app.DailyTotal{{Day: old.Truncate(24 * time.Hour), Credited: 501}}, rows)
}

func TestDistributionSingleBalance(t *testing.T) {
	svc, r, _, tenant := newService(t)
	ctx := context.Background()
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, id.NewID(), "a", 5)))
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, id.NewID(), "b", 5)))
	bs, err := r.BalanceDistribution(ctx, tenant, 10)
	require.NoError(t, err)
	require.Equal(t, app.BalanceBucket{From: 5, To: 5, Players: 2}, bs[0])
	require.Equal(t, app.BalanceBucket{From: 14, To: 14}, bs[9])
}
