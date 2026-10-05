package app

import (
	"context"

	"levelup/internal/modules/points/contracts"
	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

var knownKinds = map[string]bool{
	contracts.KindEarn: true, contracts.KindBonus: true, contracts.KindReward: true,
	contracts.KindAdjustment: true, contracts.KindSpend: true, contracts.KindRedeem: true,
	contracts.KindPenalty: true, contracts.KindExpire: true, contracts.KindTransfer: true,
	contracts.KindRefund: true,
}

// Wallet is GET /players/{id}/wallet. A player whose wallet was never
// opened gets a zero-balance view; a GET never writes (fixes B17).
func (s *Service) Wallet(ctx context.Context, playerID string) (w domain.Wallet, opened bool, err error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Wallet{}, false, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewWallet, nil); err != nil {
		return domain.Wallet{}, false, err
	}
	if _, err := s.player(ctx, p.TenantID, playerID); err != nil {
		return domain.Wallet{}, false, err
	}
	w, ok, err := s.repo.WalletByPlayer(ctx, p.TenantID, playerID)
	if err != nil {
		return domain.Wallet{}, false, err
	}
	if !ok {
		return domain.Wallet{TenantID: p.TenantID, PlayerID: playerID, Active: true}, false, nil
	}
	return w, true, nil
}

// LedgerQuery is the transactions list request.
type LedgerQuery struct {
	Kind      string
	Direction string // "", "credit", "debit"
	Cursor    string
	Limit     int
}

// Ledger is GET /players/{id}/wallet/transactions: newest first, keyset
// cursor over (created_at, id).
func (s *Service) Ledger(ctx context.Context, playerID string, q LedgerQuery) ([]domain.LedgerEntry, string, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewTransactions, nil); err != nil {
		return nil, "", err
	}
	f := LedgerFilter{Kind: q.Kind, Limit: q.Limit}
	if f.Kind != "" && !knownKinds[f.Kind] {
		return nil, "", errs.WithFields(errs.New(errs.Invalid, "unknown kind"), map[string]string{"kind": "unknown kind"})
	}
	switch q.Direction {
	case "":
	case "credit":
		f.Direction = domain.Credit
	case "debit":
		f.Direction = domain.Debit
	default:
		return nil, "", errs.WithFields(errs.New(errs.Invalid, "direction must be credit or debit"),
			map[string]string{"direction": "must be credit or debit"})
	}
	if f.Limit <= 0 {
		f.Limit = defaultPageSize
	}
	if f.Limit > maxPageSize {
		f.Limit = maxPageSize
	}
	if q.Cursor != "" {
		f.BeforeAt, f.BeforeID, err = pagination.DecodeCursor(q.Cursor)
		if err != nil {
			return nil, "", err
		}
	}
	if _, err := s.player(ctx, p.TenantID, playerID); err != nil {
		return nil, "", err
	}

	want := f.Limit
	f.Limit = want + 1
	rows, err := s.repo.ListEntries(ctx, p.TenantID, playerID, f)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > want {
		rows = rows[:want]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

// WalletsByPlayerIDs implements contracts.Reader. Players without a wallet
// are absent from the result.
func (s *Service) WalletsByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) ([]contracts.WalletSnapshot, error) {
	if len(playerIDs) == 0 {
		return nil, nil
	}
	ws, err := s.repo.WalletsByPlayers(ctx, tenantID, playerIDs)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.WalletSnapshot, len(ws))
	for i, w := range ws {
		out[i] = w.Snapshot()
	}
	return out, nil
}

// OutcomeByKey implements contracts.Reader: the settled result of a
// command, applied or rejected. Saga sweeps use it when an outcome event
// never arrived.
func (s *Service) OutcomeByKey(ctx context.Context, tenantID, key string) (contracts.MoveOutcome, bool, error) {
	var out moveResult
	settled, err := s.settledInTx(ctx, nil, tenantID, key, &out)
	if err != nil || !settled {
		return contracts.MoveOutcome{}, false, err
	}
	if out.Rejection != nil {
		return contracts.MoveOutcome{
			IdempotencyKey: key,
			Status:         contracts.OutcomeRejected,
			Reason:         out.Rejection.Reason,
			Amount:         out.Rejection.Amount.Minor(),
		}, true, nil
	}
	return contracts.MoveOutcome{
		IdempotencyKey: key,
		Status:         contracts.OutcomeApplied,
		EntryID:        out.Entry.ID,
		Amount:         out.Entry.Amount.Minor(),
	}, true, nil
}

var _ contracts.Reader = (*Service)(nil)
