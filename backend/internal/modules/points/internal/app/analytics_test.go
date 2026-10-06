package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

func TestSummaryAggregatesTenantAndTrailingWindow(t *testing.T) {
	repo, ob, pl := newFakeRepo(), &fakeOutbox{}, players()
	svc, clk := newTestService(repo, pl, ob, allPerms)
	ctx := context.Background()

	require.NoError(t, svc.HandleCredit(ctx, creditCmd("old", alice, 1000)))
	clk.Advance(40 * 24 * time.Hour)
	require.NoError(t, svc.HandleCredit(ctx, creditCmd("c1", alice, 100)))
	require.NoError(t, svc.HandleCredit(ctx, creditCmd("c2", bob, 50)))
	require.NoError(t, svc.HandleDebit(ctx, debitCmd("d1", alice, 30)))

	s, err := svc.Summary(asAdmin())
	require.NoError(t, err)
	require.Equal(t, WalletSummary{
		OpenWallets: 2, TotalBalance: 1120, LifetimeEarned: 1150, LifetimeSpent: 30,
		CreditedLast30d: 150, DebitedLast30d: 30,
	}, s)
}

func TestAnalyticsNeedPermissionAndTenant(t *testing.T) {
	f := newFixture(t, allowKeys{})
	_, err := f.svc.Summary(asAdmin())
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = f.svc.Distribution(asAdmin())
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = f.svc.WalletsFor(asAdmin(), []string{alice})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = f.svc.Daily(asAdmin(), time.Time{}, time.Time{})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	f = newFixture(t, allPerms)
	noTenant := authz.Into(context.Background(), authz.Principal{UserID: adminID})
	_, err = f.svc.Summary(noTenant)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestWalletsForBatchesInRequestOrderWithZeroViews(t *testing.T) {
	f := newFixture(t, allPerms)
	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("c1", alice, 70)))
	ghost := id.Derive("player", "ghost")

	views, err := f.svc.WalletsFor(asAdmin(), []string{bob, alice, foreign, ghost, alice})
	require.NoError(t, err)
	require.Len(t, views, 2, "unknown and foreign players are omitted, duplicates collapsed")
	require.Equal(t, bob, views[0].Wallet.PlayerID)
	require.False(t, views[0].Opened)
	require.Empty(t, views[0].Wallet.ID)
	require.True(t, views[0].Wallet.Active)
	require.Equal(t, alice, views[1].Wallet.PlayerID)
	require.True(t, views[1].Opened)
	require.Equal(t, int64(70), views[1].Wallet.Balance.Minor())
	require.Len(t, f.repo.wallets, 1, "a batch read never opens a wallet")
}

func TestWalletsForCapsBatch(t *testing.T) {
	f := newFixture(t, allPerms)
	ids := make([]string, MaxBatchPlayers+1)
	for i := range ids {
		ids[i] = id.Derive("player", fmt.Sprint(i))
	}
	_, err := f.svc.WalletsFor(asAdmin(), ids)
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	views, err := f.svc.WalletsFor(asAdmin(), nil)
	require.NoError(t, err)
	require.Empty(t, views)
}

func TestDistributionBucketsBalances(t *testing.T) {
	f := newFixture(t, allPerms)
	got, err := f.svc.Distribution(asAdmin())
	require.NoError(t, err)
	require.Empty(t, got, "no wallets, no buckets")

	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("c1", alice, 99)))
	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("c2", bob, 0+1)))
	got, err = f.svc.Distribution(asAdmin())
	require.NoError(t, err)
	require.Len(t, got, DistributionBuckets)
	require.Equal(t, BalanceBucket{From: 1, To: 10, Players: 1}, got[0])
	require.Equal(t, BalanceBucket{From: 91, To: 100, Players: 1}, got[9])
}

func TestDailyZeroFillsAndValidatesRange(t *testing.T) {
	repo, ob, pl := newFakeRepo(), &fakeOutbox{}, players()
	svc, clk := newTestService(repo, pl, ob, allPerms)
	ctx := context.Background()
	require.NoError(t, svc.HandleCredit(ctx, creditCmd("c1", alice, 100)))
	require.NoError(t, svc.HandleDebit(ctx, debitCmd("d1", alice, 40)))
	clk.Advance(48 * time.Hour)
	require.NoError(t, svc.HandleCredit(ctx, creditCmd("c2", alice, 5)))

	day0 := truncDay(testNow)
	rows, err := svc.Daily(asAdmin(), day0, day0.AddDate(0, 0, 2))
	require.NoError(t, err)
	require.Equal(t, []DailyTotal{
		{Day: day0, Credited: 100, Debited: 40},
		{Day: day0.AddDate(0, 0, 1)},
		{Day: day0.AddDate(0, 0, 2), Credited: 5},
	}, rows)

	rows, err = svc.Daily(asAdmin(), time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Len(t, rows, 30, "default window is the last 30 days")
	require.Equal(t, truncDay(clk.Now()), rows[29].Day)

	_, err = svc.Daily(asAdmin(), day0.AddDate(0, 0, 1), day0)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	_, err = svc.Daily(asAdmin(), day0.AddDate(-2, 0, 0), day0)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}
