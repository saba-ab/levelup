package app

import (
	"context"
	"time"

	"levelup/internal/modules/points/contracts"
	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

const (
	// MaxBatchPlayers caps GET /wallets?player_ids=.
	MaxBatchPlayers = 100
	// DistributionBuckets is the number of histogram buckets.
	DistributionBuckets = 10
	// MaxDailyRange caps GET /wallets/daily, in days (inclusive).
	MaxDailyRange = 366
	// defaultDailyRange is the window when from/to are omitted.
	defaultDailyRange = 30
	summaryWindow     = 30 * 24 * time.Hour
)

// WalletSummary is the tenant-wide wallet roll-up.
type WalletSummary struct {
	OpenWallets     int64
	TotalBalance    int64
	LifetimeEarned  int64
	LifetimeSpent   int64
	CreditedLast30d int64
	DebitedLast30d  int64
}

// BalanceBucket counts the wallets whose balance lies in [From, To]
// (both inclusive integer bounds).
type BalanceBucket struct {
	From    int64
	To      int64
	Players int64
}

// DailyTotal is one UTC day of ledger movement.
type DailyTotal struct {
	Day      time.Time // UTC midnight
	Credited int64
	Debited  int64
}

// WalletView is one player's wallet in a batch read; Opened is false for a
// never-opened wallet (zero counters, empty id), exactly like GET
// /players/{id}/wallet.
type WalletView struct {
	Wallet domain.Wallet
	Opened bool
}

// Summary is GET /wallets/summary.
func (s *Service) Summary(ctx context.Context) (WalletSummary, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return WalletSummary{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewWallet, nil); err != nil {
		return WalletSummary{}, err
	}
	return s.repo.Summary(ctx, p.TenantID, s.clock.Now().UTC().Add(-summaryWindow))
}

// WalletsFor is GET /wallets?player_ids=: one view per known player of the
// caller's tenant, in request order (duplicates collapsed). Unknown and
// foreign players are absent, never an error, so the list never leaks.
func (s *Service) WalletsFor(ctx context.Context, playerIDs []string) ([]WalletView, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewWallet, nil); err != nil {
		return nil, err
	}
	ids := dedupe(playerIDs)
	if len(ids) > MaxBatchPlayers {
		return nil, errs.WithFields(errs.WithCode(errs.New(errs.Invalid, "too many player ids"), "too_many_ids"),
			map[string]string{"player_ids": "at most 100 ids"})
	}
	if len(ids) == 0 {
		return []WalletView{}, nil
	}
	known, err := s.players.PlayersByIDs(ctx, p.TenantID, ids)
	if err != nil {
		return nil, err
	}
	var present []string
	for _, pid := range ids {
		if pl, ok := known[pid]; ok && pl.TenantID == p.TenantID {
			present = append(present, pid)
		}
	}
	if len(present) == 0 {
		return []WalletView{}, nil
	}
	ws, err := s.repo.WalletsByPlayers(ctx, p.TenantID, present)
	if err != nil {
		return nil, err
	}
	byPlayer := make(map[string]domain.Wallet, len(ws))
	for _, w := range ws {
		byPlayer[w.PlayerID] = w
	}
	out := make([]WalletView, 0, len(present))
	for _, pid := range present {
		if w, ok := byPlayer[pid]; ok {
			out = append(out, WalletView{Wallet: w, Opened: true})
			continue
		}
		out = append(out, WalletView{Wallet: domain.Wallet{TenantID: p.TenantID, PlayerID: pid, Active: true}})
	}
	return out, nil
}

// Distribution is GET /wallets/distribution: DistributionBuckets
// equal-width buckets over current balances. A tenant without wallets gets
// an empty list.
func (s *Service) Distribution(ctx context.Context) ([]BalanceBucket, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewWallet, nil); err != nil {
		return nil, err
	}
	b, err := s.repo.BalanceDistribution(ctx, p.TenantID, DistributionBuckets)
	if err != nil {
		return nil, err
	}
	if b == nil {
		b = []BalanceBucket{}
	}
	return b, nil
}

// Daily is GET /wallets/daily: one row per UTC day in [from, to] (both
// inclusive dates), zero-filled. Zero from/to default to the last 30 days
// ending today.
func (s *Service) Daily(ctx context.Context, from, to time.Time) ([]DailyTotal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewTransactions, nil); err != nil {
		return nil, err
	}
	if to.IsZero() {
		to = s.clock.Now().UTC()
	}
	to = truncDay(to)
	if from.IsZero() {
		from = to.AddDate(0, 0, -(defaultDailyRange - 1))
	}
	from = truncDay(from)
	if from.After(to) {
		return nil, errs.WithFields(errs.WithCode(errs.New(errs.Invalid, "from must not be after to"), "invalid_range"),
			map[string]string{"from": "must not be after to"})
	}
	days := int(to.Sub(from)/(24*time.Hour)) + 1
	if days > MaxDailyRange {
		return nil, errs.WithFields(errs.WithCode(errs.New(errs.Invalid, "range exceeds 366 days"), "invalid_range"),
			map[string]string{"from": "range must be at most 366 days"})
	}
	rows, err := s.repo.DailyTotals(ctx, p.TenantID, from, to.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	byDay := make(map[time.Time]DailyTotal, len(rows))
	for _, r := range rows {
		byDay[truncDay(r.Day)] = r
	}
	out := make([]DailyTotal, days)
	for i := range days {
		d := from.AddDate(0, 0, i)
		r := byDay[d]
		out[i] = DailyTotal{Day: d, Credited: r.Credited, Debited: r.Debited}
	}
	return out, nil
}

func truncDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func dedupe(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, v := range ids {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
