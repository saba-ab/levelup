package app

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/modules/rewards/internal/ports"
)

// Job names of the reconciling crons.
const (
	JobClaimsReconcile = "rewards.claims_reconcile"
	JobClaimsExpire    = "rewards.claims_expire"
)

// maxSweepRounds bounds one run; the next tick continues where this left off.
const maxSweepRounds = 20

// ReconcileClaims is rewards.claims_reconcile (every minute). It is
// reconciling by state, so a skipped tick leaves no gap:
//   - pending_payment past hold_expires_at: ask points by DebitKey —
//     applied → claimed, rejected → rejected, nothing → cancelled (stock
//     released; a later debit is refunded by OnDebited);
//   - paid cancelled claims touched since the last run (minus a lookback):
//     a debit that landed without its event reaching us → refund;
//   - refund_pending: the refund applied without its event → refunded.
//
// Per-row failures do not stop the sweep; they are joined and returned so
// the job is retried.
func (s *Service) ReconcileClaims(ctx context.Context) error {
	started := s.clock.Now()
	last, err := s.repo.LastRun(ctx, JobClaimsReconcile)
	if err != nil {
		return err
	}
	var errsOut []error

	seen := map[string]bool{}
	for range maxSweepRounds {
		due, err := s.repo.DuePendingClaims(ctx, s.clock.Now(), s.cfg.SweepBatchSize)
		if err != nil {
			return err
		}
		progressed := false
		for _, c := range due {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			progressed = true
			if err := s.reconcilePending(ctx, c); err != nil {
				errsOut = append(errsOut, err)
			}
		}
		if len(due) < s.cfg.SweepBatchSize || !progressed {
			break
		}
	}

	cancelled, err := s.repo.PaidCancelledSince(ctx, last.Add(-s.cfg.LateDebitLookback), s.cfg.SweepBatchSize)
	if err != nil {
		return err
	}
	for _, c := range cancelled {
		out, found, err := s.points.OutcomeByKey(ctx, c.TenantID, c.DebitKey)
		if err != nil {
			errsOut = append(errsOut, err)
			continue
		}
		if found && out.Status == ports.PaymentApplied {
			if err := s.settleDebited(ctx, c.TenantID, c.ID); err != nil {
				errsOut = append(errsOut, err)
			}
		}
	}

	refunds, err := s.repo.RefundPendingClaims(ctx, s.cfg.SweepBatchSize)
	if err != nil {
		return err
	}
	for _, c := range refunds {
		out, found, err := s.points.OutcomeByKey(ctx, c.TenantID, contracts.RefundKey(c.ID))
		if err != nil {
			errsOut = append(errsOut, err)
			continue
		}
		if found && out.Status == ports.PaymentApplied {
			if err := s.settleRefunded(ctx, c.TenantID, c.ID); err != nil {
				errsOut = append(errsOut, err)
			}
		}
	}

	if len(errsOut) > 0 {
		return errors.Join(errsOut...)
	}
	return s.repo.MarkRun(ctx, JobClaimsReconcile, started)
}

func (s *Service) reconcilePending(ctx context.Context, c domain.Claim) error {
	out, found, err := s.points.OutcomeByKey(ctx, c.TenantID, c.DebitKey)
	if err != nil {
		return err
	}
	switch {
	case found && out.Status == ports.PaymentApplied:
		return s.settleDebited(ctx, c.TenantID, c.ID)
	case found && out.Status == ports.PaymentRejected:
		return s.settleRejected(ctx, c.TenantID, c.ID, out.Reason)
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		locked, err := s.repo.ClaimForUpdate(ctx, tx, c.TenantID, c.ID)
		if isNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if locked.Status != contracts.ClaimPendingPayment ||
			locked.HoldExpiresAt == nil || s.clock.Now().Before(*locked.HoldExpiresAt) {
			return nil // settled meanwhile
		}
		return s.cancelLocked(ctx, tx, &locked, contracts.CancelHoldExpired)
	})
}

// ExpireClaims is rewards.claims_expire (hourly, reconciling by state):
// claimed claims past expires_at become expired (stock stays consumed);
// rewards past end_at become expired.
func (s *Service) ExpireClaims(ctx context.Context) error {
	var errsOut []error
	seen := map[string]bool{}
	for range maxSweepRounds {
		due, err := s.repo.DueExpiringClaims(ctx, s.clock.Now(), s.cfg.SweepBatchSize)
		if err != nil {
			return err
		}
		progressed := false
		for _, c := range due {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			progressed = true
			if err := s.expireClaim(ctx, c); err != nil {
				errsOut = append(errsOut, err)
			}
		}
		if len(due) < s.cfg.SweepBatchSize || !progressed {
			break
		}
	}

	ended, err := s.repo.EndedRewards(ctx, s.clock.Now(), s.cfg.SweepBatchSize)
	if err != nil {
		return err
	}
	for _, r := range ended {
		err := s.tx(ctx, func(tx *gorm.DB) error {
			locked, err := s.repo.RewardForUpdate(ctx, tx, r.TenantID, r.ID, false)
			if isNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !locked.ExpireIfEnded(s.clock.Now()) {
				return nil
			}
			return s.repo.SaveReward(ctx, tx, locked)
		})
		if err != nil {
			errsOut = append(errsOut, err)
		}
	}
	if len(errsOut) > 0 {
		return errors.Join(errsOut...)
	}
	return s.repo.MarkRun(ctx, JobClaimsExpire, s.clock.Now())
}

func (s *Service) expireClaim(ctx context.Context, c domain.Claim) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		locked, err := s.repo.ClaimForUpdate(ctx, tx, c.TenantID, c.ID)
		if isNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if err := locked.Expire(now); err != nil {
			return nil // redeemed or cancelled meanwhile
		}
		if err := s.repo.SaveClaim(ctx, tx, locked); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicClaimExpired, claimEvent(locked, nil, "", nil, now))
	})
}
