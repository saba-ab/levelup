package app

import (
	"context"
	"strings"

	"gorm.io/gorm"

	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/shared/errs"
)

// Settlement (tx2 of the saga). Every handler is guarded by the claim's
// status under its row lock, so redelivered or reordered outcome events
// (ADR-0012) are no-ops: whichever arrives first wins.

var (
	debitPrefix  = contracts.DebitKey("")
	refundPrefix = contracts.RefundKey("")
)

// OnDebited handles points.debited.v1. Debits that are not a reward claim's
// payment are ignored.
func (s *Service) OnDebited(ctx context.Context, ev pointscontracts.LedgerMovedV1) error {
	claimID, ok := strings.CutPrefix(ev.IdempotencyKey, debitPrefix)
	if !ok {
		return nil
	}
	if ev.TenantID == "" || claimID == "" {
		return errs.New(errs.Invalid, "points.debited.v1 for a reward claim without tenant or claim id")
	}
	return s.settleDebited(ctx, ev.TenantID, claimID)
}

// OnDebitRejected handles points.debit_rejected.v1.
func (s *Service) OnDebitRejected(ctx context.Context, ev pointscontracts.MoveRejectedV1) error {
	claimID, ok := strings.CutPrefix(ev.IdempotencyKey, debitPrefix)
	if !ok {
		return nil
	}
	if ev.TenantID == "" || claimID == "" {
		return errs.New(errs.Invalid, "points.debit_rejected.v1 for a reward claim without tenant or claim id")
	}
	return s.settleRejected(ctx, ev.TenantID, claimID, ev.Reason)
}

// OnRefunded handles points.refunded.v1.
func (s *Service) OnRefunded(ctx context.Context, ev pointscontracts.LedgerMovedV1) error {
	claimID, ok := strings.CutPrefix(ev.IdempotencyKey, refundPrefix)
	if !ok {
		return nil
	}
	if ev.TenantID == "" || claimID == "" {
		return errs.New(errs.Invalid, "points.refunded.v1 for a reward claim without tenant or claim id")
	}
	return s.settleRefunded(ctx, ev.TenantID, claimID)
}

// settleDebited: pending_payment → claimed; cancelled (late debit) →
// refund_pending + job.points.refund; anything else is a no-op.
func (s *Service) settleDebited(ctx context.Context, tenantID, claimID string) error {
	return s.withCodeRetry(func() error {
		return s.tx(ctx, func(tx *gorm.DB) error {
			c, err := s.repo.ClaimForUpdate(ctx, tx, tenantID, claimID)
			if isNotFound(err) {
				return nil // purged tenant or a key we never issued
			}
			if err != nil {
				return err
			}
			now := s.clock.Now()
			switch c.Status {
			case contracts.ClaimPendingPayment:
				r, err := s.repo.RewardByID(ctx, tenantID, c.RewardID, true)
				if err != nil && !isNotFound(err) {
					return err
				}
				if err := c.MarkPaid(r, s.code, now); err != nil {
					return err
				}
				if err := s.repo.SaveClaim(ctx, tx, c); err != nil {
					return err
				}
				var rp *domain.Reward
				if r.ID != "" {
					rp = &r
				}
				return s.outbox.Publish(ctx, tx, contracts.TopicClaimed, claimEvent(c, rp, "", nil, now))
			case contracts.ClaimCancelled:
				if !c.Paid() {
					return nil
				}
				if err := c.LateDebit(now); err != nil {
					return err
				}
				if err := s.repo.SaveClaim(ctx, tx, c); err != nil {
					return err
				}
				return s.publishRefund(ctx, tx, c, "late_debit_after_cancel")
			default:
				return nil
			}
		})
	})
}

// settleRejected: pending_payment → rejected, stock released.
func (s *Service) settleRejected(ctx context.Context, tenantID, claimID, reason string) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		c, err := s.repo.ClaimForUpdate(ctx, tx, tenantID, claimID)
		if isNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if c.Status != contracts.ClaimPendingPayment {
			return nil
		}
		now := s.clock.Now()
		if err := c.MarkRejected(reason, now); err != nil {
			return err
		}
		if err := s.releaseStock(ctx, tx, tenantID, c.RewardID); err != nil {
			return err
		}
		if err := s.repo.SaveClaim(ctx, tx, c); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicClaimRejected, claimEvent(c, nil, reason, nil, now))
	})
}

// settleRefunded: refund_pending → refunded.
func (s *Service) settleRefunded(ctx context.Context, tenantID, claimID string) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		c, err := s.repo.ClaimForUpdate(ctx, tx, tenantID, claimID)
		if isNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if c.Status != contracts.ClaimRefundPending {
			return nil
		}
		if err := c.MarkRefunded(s.clock.Now()); err != nil {
			return err
		}
		return s.repo.SaveClaim(ctx, tx, c)
	})
}
