package app

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"levelup/internal/modules/points/contracts"
	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
	"levelup/internal/shared/money"
)

// errKeyRace aborts a transaction whose ledger insert hit the idempotency
// unique index after the in-tx check passed (a concurrent writer with the
// same key on another wallet). The caller reloads the settled outcome.
var errKeyRace = errs.New(errs.Conflict, "idempotency key settled concurrently")

// moveReq is one single-wallet credit or debit.
type moveReq struct {
	TenantID string
	PlayerID string
	Dir      domain.Direction
	Amount   money.Amount
	Spec     domain.EntrySpec
	// RecordRejections turns business rejections into a stored rejection
	// plus a *_rejected.v1 fact (job path). False: return the error and
	// record nothing (HTTP path).
	RecordRejections bool
}

// moveResult is the settled outcome of a key: an entry or a rejection.
type moveResult struct {
	Entry     domain.LedgerEntry
	Wallet    domain.Wallet
	Rejection *domain.Rejection
	// Replay is true when the key was already settled before this call:
	// nothing was written and nothing was published.
	Replay bool
}

// move applies one movement in one transaction: open the wallet if needed,
// lock it, settle-by-key, mutate, append the ledger row, save with the
// version guard, and publish — all or nothing.
func (s *Service) move(ctx context.Context, req moveReq) (moveResult, error) {
	var out moveResult
	err := s.tx(ctx, func(tx *gorm.DB) error {
		out = moveResult{}
		if err := s.ensureWallet(ctx, tx, req.TenantID, req.PlayerID); err != nil {
			return err
		}
		w, err := s.repo.WalletForUpdate(ctx, tx, req.TenantID, req.PlayerID)
		if err != nil {
			return err
		}
		settled, err := s.settledInTx(ctx, tx, req.TenantID, req.Spec.IdempotencyKey, &out)
		if err != nil || settled {
			return err
		}

		now := s.clock.Now()
		var before, after money.Amount
		if req.Dir == domain.Credit {
			before, after, err = w.Credit(req.Amount, now)
		} else {
			before, after, err = w.Debit(req.Amount, now)
		}
		if err != nil {
			reason, isRejection := rejectionReason(err)
			if !isRejection || !req.RecordRejections {
				if errors.Is(err, domain.ErrInsufficientBalance) {
					return errs.WithCode(errs.Wrap(errs.Invalid,
						fmt.Sprintf("requested %d, available %d", req.Amount.Minor(), w.Balance.Minor()), err),
						"insufficient_balance")
				}
				return err
			}
			cmd := domain.CommandCredit
			if req.Dir == domain.Debit {
				cmd = domain.CommandDebit
			}
			rej := s.newRejection(req.TenantID, req.Spec.IdempotencyKey, cmd, req.PlayerID, req.Amount, reason, w.Balance, req.Spec.Source)
			out.Rejection = &rej
			out.Wallet = w
			return s.recordRejection(ctx, tx, rej)
		}

		entry, err := domain.NewEntry(w, req.Dir, req.Amount, before, after, req.Spec, now)
		if err != nil {
			return err
		}
		if err := s.appendAndSave(ctx, tx, []domain.LedgerEntry{entry}, &w); err != nil {
			return err
		}
		out.Entry, out.Wallet = entry, w
		topic := contracts.TopicCredited
		if req.Dir == domain.Debit {
			topic = contracts.TopicDebited
		}
		return s.outbox.Publish(ctx, tx, topic, movedPayload(entry, w))
	})
	if errors.Is(err, errKeyRace) {
		return s.settledOutside(ctx, req.TenantID, req.Spec.IdempotencyKey)
	}
	if err != nil {
		return moveResult{}, err
	}
	return out, nil
}

// ensureWallet opens the wallet idempotently and publishes
// points.wallet_opened.v1 only when this call inserted it.
func (s *Service) ensureWallet(ctx context.Context, tx *gorm.DB, tenantID, playerID string) error {
	w := domain.NewWallet(tenantID, playerID, s.clock.Now())
	created, err := s.repo.EnsureWallet(ctx, tx, w)
	if err != nil || !created {
		return err
	}
	return s.outbox.Publish(ctx, tx, contracts.TopicWalletOpened, contracts.WalletOpenedV1{
		WalletID: w.ID,
		TenantID: w.TenantID,
		PlayerID: w.PlayerID,
		At:       w.CreatedAt,
	})
}

// appendAndSave inserts the entries (idempotency belt) then saves every
// wallet they touch with the version guard. Versions are bumped in memory.
func (s *Service) appendAndSave(ctx context.Context, tx *gorm.DB, entries []domain.LedgerEntry, wallets ...*domain.Wallet) error {
	inserted, err := s.repo.InsertEntries(ctx, tx, entries...)
	if err != nil {
		return err
	}
	if !inserted {
		return errKeyRace
	}
	for _, w := range wallets {
		if err := s.repo.SaveWallet(ctx, tx, *w); err != nil {
			return err
		}
		w.Version++
	}
	return nil
}

// settledInTx fills out when key already has an outcome (entry or rejection).
func (s *Service) settledInTx(ctx context.Context, tx *gorm.DB, tenantID, key string, out *moveResult) (bool, error) {
	entry, ok, err := s.repo.EntryByKey(ctx, tx, tenantID, key)
	if err != nil {
		return false, err
	}
	if ok {
		out.Entry, out.Replay = entry, true
		return true, nil
	}
	rej, ok, err := s.repo.RejectionByKey(ctx, tx, tenantID, key)
	if err != nil {
		return false, err
	}
	if ok {
		out.Rejection, out.Replay = &rej, true
		return true, nil
	}
	return false, nil
}

// settledOutside reloads a key's outcome after a lost insert race.
func (s *Service) settledOutside(ctx context.Context, tenantID, key string) (moveResult, error) {
	var out moveResult
	settled, err := s.settledInTx(ctx, nil, tenantID, key, &out)
	if err != nil {
		return moveResult{}, err
	}
	if !settled {
		return moveResult{}, errs.New(errs.Conflict, "idempotency key conflict, retry")
	}
	return out, nil
}

func (s *Service) newRejection(tenantID, key, command, playerID string, amount money.Amount, reason string,
	available money.Amount, src effect.Source) domain.Rejection {
	return domain.Rejection{
		ID:             id.NewID(),
		TenantID:       tenantID,
		IdempotencyKey: key,
		Command:        command,
		PlayerID:       playerID,
		Amount:         amount,
		Reason:         reason,
		Available:      available,
		Source:         src,
		CreatedAt:      s.clock.Now(),
	}
}

// recordRejection stores the rejection and publishes the matching
// *_rejected.v1 fact — only when this call inserted it.
func (s *Service) recordRejection(ctx context.Context, tx *gorm.DB, r domain.Rejection) error {
	inserted, err := s.repo.InsertRejection(ctx, tx, r)
	if err != nil || !inserted {
		return err
	}
	return s.outbox.Publish(ctx, tx, rejectedTopic(r.Command), contracts.MoveRejectedV1{
		IdempotencyKey: r.IdempotencyKey,
		TenantID:       r.TenantID,
		PlayerID:       r.PlayerID,
		Amount:         r.Amount.Minor(),
		Reason:         r.Reason,
		Available:      r.Available.Minor(),
		Source:         r.Source,
		At:             r.CreatedAt,
	})
}

// rejectOutsideWallet records a rejection decided before any wallet is
// touched (player not found / inactive, unknown debit).
func (s *Service) rejectOutsideWallet(ctx context.Context, r domain.Rejection) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.recordRejection(ctx, tx, r)
	})
}

func rejectedTopic(command string) string {
	switch command {
	case domain.CommandDebit:
		return contracts.TopicDebitRejected
	case domain.CommandRefund:
		return contracts.TopicRefundRejected
	default:
		return contracts.TopicCreditRejected
	}
}

// rejectionReason maps a domain error to a business-rejection reason.
func rejectionReason(err error) (string, bool) {
	switch {
	case errors.Is(err, domain.ErrInsufficientBalance):
		return effect.ReasonInsufficientBalance, true
	case errors.Is(err, domain.ErrWalletInactive):
		return effect.ReasonWalletInactive, true
	}
	return "", false
}

func movedPayload(e domain.LedgerEntry, w domain.Wallet) contracts.LedgerMovedV1 {
	return contracts.LedgerMovedV1{
		EntryID:        e.ID,
		IdempotencyKey: e.IdempotencyKey,
		TenantID:       e.TenantID,
		PlayerID:       e.PlayerID,
		WalletID:       e.WalletID,
		Kind:           e.Kind,
		Amount:         e.Amount.Minor(),
		BalanceAfter:   e.BalanceAfter.Minor(),
		LifetimeEarned: w.LifetimeEarned.Minor(),
		Source:         e.Source,
		OccurredAt:     e.OccurredAt,
		At:             e.CreatedAt,
	}
}
