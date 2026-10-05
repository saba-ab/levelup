package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/points/contracts"
	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/money"
)

// maxKeyLen bounds idempotency keys from any source.
const maxKeyLen = 255

// errDebitPending: the refund arrived before the debit it reverses
// (delivery is unordered). Unavailable climbs the retry ladder; a debit
// that never arrives parks the refund in the DLQ (human path).
var errDebitPending = errs.New(errs.Unavailable, "debit to refund is not settled yet")

// HandleCredit consumes job.points.credit. Business rejections are
// recorded and published as points.credit_rejected.v1 and return nil;
// malformed commands are errs.Invalid (immediate DLQ); anything else is
// transient and retried.
func (s *Service) HandleCredit(ctx context.Context, cmd contracts.CreditCmdV1) error {
	if err := validateCommand(cmd.IdempotencyKey, cmd.TenantID, cmd.PlayerID, cmd.Amount); err != nil {
		return err
	}
	if err := domain.CheckKind(domain.Credit, cmd.Kind); err != nil {
		return errs.Wrap(errs.Invalid, "credit command kind "+cmd.Kind, err)
	}
	return s.applyCommand(ctx, moveReq{
		TenantID: cmd.TenantID,
		PlayerID: cmd.PlayerID,
		Dir:      domain.Credit,
		Amount:   money.Amount(cmd.Amount),
		Spec: domain.EntrySpec{
			IdempotencyKey: cmd.IdempotencyKey,
			Kind:           cmd.Kind,
			Source:         cmd.Source,
			Description:    cmd.Description,
			OccurredAt:     cmd.OccurredAt,
		},
		RecordRejections: true,
	})
}

// HandleDebit consumes job.points.debit. Insufficient balance is a result
// (points.debit_rejected.v1), never an error.
func (s *Service) HandleDebit(ctx context.Context, cmd contracts.DebitCmdV1) error {
	if err := validateCommand(cmd.IdempotencyKey, cmd.TenantID, cmd.PlayerID, cmd.Amount); err != nil {
		return err
	}
	if err := domain.CheckKind(domain.Debit, cmd.Kind); err != nil {
		return errs.Wrap(errs.Invalid, "debit command kind "+cmd.Kind, err)
	}
	return s.applyCommand(ctx, moveReq{
		TenantID: cmd.TenantID,
		PlayerID: cmd.PlayerID,
		Dir:      domain.Debit,
		Amount:   money.Amount(cmd.Amount),
		Spec: domain.EntrySpec{
			IdempotencyKey: cmd.IdempotencyKey,
			Kind:           cmd.Kind,
			Source:         cmd.Source,
			Description:    cmd.Description,
			OccurredAt:     cmd.OccurredAt,
		},
		RecordRejections: true,
	})
}

func (s *Service) applyCommand(ctx context.Context, req moveReq) error {
	settled, err := s.isSettled(ctx, req.TenantID, req.Spec.IdempotencyKey)
	if err != nil || settled {
		return err
	}

	command := domain.CommandCredit
	if req.Dir == domain.Debit {
		command = domain.CommandDebit
	}
	p, err := s.player(ctx, req.TenantID, req.PlayerID)
	switch {
	case errors.Is(err, domain.ErrPlayerNotFound):
		return s.rejectOutsideWallet(ctx, s.newRejection(req.TenantID, req.Spec.IdempotencyKey, command,
			req.PlayerID, req.Amount, effect.ReasonPlayerNotFound, 0, req.Spec.Source))
	case err != nil:
		return err
	case !p.Active:
		return s.rejectOutsideWallet(ctx, s.newRejection(req.TenantID, req.Spec.IdempotencyKey, command,
			req.PlayerID, req.Amount, effect.ReasonPlayerInactive, 0, req.Spec.Source))
	}

	res, err := s.move(ctx, req)
	if err != nil {
		return err
	}
	if res.Rejection != nil && !res.Replay {
		s.log.Info("points command rejected",
			zap.String("tenant_id", req.TenantID), zap.String("player_id", req.PlayerID),
			zap.String("idempotency_key", req.Spec.IdempotencyKey), zap.String("reason", res.Rejection.Reason))
	}
	return nil
}

// HandleRefund consumes job.points.refund: reverses the debit recorded
// under DebitIdempotencyKey, exactly once (unique reversal_of), into the
// same wallet. The player is not re-checked: a refund only returns points
// that an already-validated debit took.
func (s *Service) HandleRefund(ctx context.Context, cmd contracts.RefundCmdV1) error {
	if err := validateKeyAndTenant(cmd.IdempotencyKey, cmd.TenantID); err != nil {
		return err
	}
	if strings.TrimSpace(cmd.DebitIdempotencyKey) == "" || len(cmd.DebitIdempotencyKey) > maxKeyLen {
		return errs.New(errs.Invalid, "refund command needs a debit_idempotency_key")
	}
	if cmd.DebitIdempotencyKey == cmd.IdempotencyKey {
		return errs.New(errs.Invalid, "refund key must differ from the debit key")
	}
	settled, err := s.isSettled(ctx, cmd.TenantID, cmd.IdempotencyKey)
	if err != nil || settled {
		return err
	}

	return s.tx(ctx, func(tx *gorm.DB) error {
		debit, ok, err := s.repo.EntryByKey(ctx, tx, cmd.TenantID, cmd.DebitIdempotencyKey)
		if err != nil {
			return err
		}
		if !ok {
			rej, rejected, err := s.repo.RejectionByKey(ctx, tx, cmd.TenantID, cmd.DebitIdempotencyKey)
			if err != nil {
				return err
			}
			if !rejected {
				return errDebitPending
			}
			// The debit never applied: nothing to give back.
			return s.recordRejection(ctx, tx, s.newRejection(cmd.TenantID, cmd.IdempotencyKey, domain.CommandRefund,
				rej.PlayerID, rej.Amount, effect.ReasonTargetNotFound, 0, cmd.Source))
		}
		if debit.Direction != domain.Debit || debit.Kind == contracts.KindTransfer {
			return errs.New(errs.Invalid, "refund target "+cmd.DebitIdempotencyKey+" is not a refundable debit")
		}

		w, err := s.repo.WalletForUpdate(ctx, tx, cmd.TenantID, debit.PlayerID)
		if err != nil {
			return err
		}
		var out moveResult
		if settled, err := s.settledInTx(ctx, tx, cmd.TenantID, cmd.IdempotencyKey, &out); err != nil || settled {
			return err
		}
		if _, refunded, err := s.repo.RefundOf(ctx, tx, cmd.TenantID, debit.ID); err != nil {
			return err
		} else if refunded {
			return s.recordRejection(ctx, tx, s.newRejection(cmd.TenantID, cmd.IdempotencyKey, domain.CommandRefund,
				debit.PlayerID, debit.Amount, contracts.ReasonAlreadyRefunded, w.Balance, cmd.Source))
		}

		now := s.clock.Now()
		before, after, err := w.Refund(debit.Amount, now)
		if err != nil {
			if reason, ok := rejectionReason(err); ok {
				return s.recordRejection(ctx, tx, s.newRejection(cmd.TenantID, cmd.IdempotencyKey, domain.CommandRefund,
					debit.PlayerID, debit.Amount, reason, w.Balance, cmd.Source))
			}
			return err
		}
		entry, err := domain.NewEntry(w, domain.Credit, debit.Amount, before, after, domain.EntrySpec{
			IdempotencyKey: cmd.IdempotencyKey,
			Kind:           contracts.KindRefund,
			Source:         cmd.Source,
			Description:    cmd.Reason,
			ReversalOf:     debit.ID,
			OccurredAt:     cmd.OccurredAt,
		}, now)
		if err != nil {
			return err
		}
		if err := s.appendAndSave(ctx, tx, []domain.LedgerEntry{entry}, &w); err != nil {
			if errors.Is(err, errKeyRace) {
				return errs.New(errs.Conflict, "refund key settled concurrently, retry")
			}
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicRefunded, movedPayload(entry, w))
	})
}

// isSettled is the cheap pre-check: a redelivered command whose key already
// has an outcome does nothing, calls nothing and publishes nothing.
func (s *Service) isSettled(ctx context.Context, tenantID, key string) (bool, error) {
	var out moveResult
	return s.settledInTx(ctx, nil, tenantID, key, &out)
}

func validateCommand(key, tenantID, playerID string, amount int64) error {
	if err := validateKeyAndTenant(key, tenantID); err != nil {
		return err
	}
	if !isUUID(playerID) {
		return errs.New(errs.Invalid, "command player_id must be a uuid")
	}
	if amount <= 0 {
		return errs.Wrap(errs.Invalid, "command amount", domain.ErrNonPositiveAmount)
	}
	return nil
}

func validateKeyAndTenant(key, tenantID string) error {
	if strings.TrimSpace(key) == "" || len(key) > maxKeyLen {
		return errs.New(errs.Invalid, "command needs an idempotency_key of at most 255 characters")
	}
	if !isUUID(tenantID) {
		return errs.New(errs.Invalid, "command tenant_id must be a uuid")
	}
	return nil
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// OpenForPlayer is the player.created.v1 entry point: an idempotent upsert
// that publishes points.wallet_opened.v1 only when it inserted.
func (s *Service) OpenForPlayer(ctx context.Context, tenantID, playerID string) error {
	if !isUUID(tenantID) || !isUUID(playerID) {
		return errs.New(errs.Invalid, "player.created.v1 needs uuid tenant_id and player_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.ensureWallet(ctx, tx, tenantID, playerID)
	})
}

// PurgeTenant is the tenant.deleted.v1 entry point; idempotent.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	if !isUUID(tenantID) {
		return errs.New(errs.Invalid, "tenant.deleted.v1 needs a uuid tenant_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, tenantID)
	})
}

// Reconcile verifies every wallet touched since the last successful run:
// balance == Σ ledger, lifetime counters, and the balance_before/after
// chain. Drift is logged and counted, never auto-fixed (human path).
func (s *Service) Reconcile(ctx context.Context, rec Reconciler) error {
	since, err := rec.LastRun(ctx)
	if err != nil {
		return err
	}
	startedAt := s.clock.Now()
	// Overlap one minute so a transaction committing across the marker
	// boundary is never skipped (rows are stamped before commit).
	if !since.IsZero() {
		since = since.Add(-time.Minute)
	}
	drifts, err := rec.FindDrift(ctx, since)
	if err != nil {
		return err
	}
	for _, d := range drifts {
		s.log.Error("points ledger drift",
			zap.String("wallet_id", d.WalletID), zap.String("tenant_id", d.TenantID), zap.String("player_id", d.PlayerID),
			zap.Int64("balance", d.Balance), zap.Int64("ledger_balance", d.LedgerBalance),
			zap.Int64("lifetime_earned", d.LifetimeEarned), zap.Int64("ledger_earned", d.LedgerEarned),
			zap.Int64("lifetime_spent", d.LifetimeSpent), zap.Int64("ledger_spent", d.LedgerSpent),
			zap.Int64("chain_breaks", d.ChainBreaks))
		if s.drift != nil {
			s.drift.Inc()
		}
	}
	s.log.Info("points reconcile finished", zap.Time("since", since), zap.Int("drifting_wallets", len(drifts)))
	return rec.MarkRun(ctx, startedAt)
}
