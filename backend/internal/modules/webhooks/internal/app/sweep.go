package app

import (
	"context"

	"gorm.io/gorm"

	"levelup/internal/modules/webhooks/contracts"
	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/shared/errs"
)

// SweepResult counts what one retry sweep did.
type SweepResult struct {
	Requeued int
	Failed   int
	Disabled int
}

// RetrySweep is the webhooks.retry_sweep cron. Reconciling (R47): it
// selects by STATE, so a skipped or failed tick is healed by the next one,
// and records the last successful run as a marker.
//
//   - Pending deliveries untouched for StaleAfter (lost job, crashed worker,
//     parked message) are re-enqueued with a fresh ladder, until the cycle
//     has used MaxAttempts; then they are failed and counted against the
//     endpoint.
//   - Active endpoints whose failure streak reached the threshold (e.g. a
//     disable that raced an admin edit) are disabled.
func (s *Service) RetrySweep(ctx context.Context) (SweepResult, error) {
	var out SweepResult
	if _, err := s.repo.LastRun(ctx, contracts.JobRetrySweep); err != nil {
		return out, err
	}
	now := s.clock.Now()
	before := now.Add(-s.opts.StaleAfter)

	stale, err := s.repo.StalePending(ctx, before, now, sweepBatchSize)
	if err != nil {
		return out, err
	}
	for _, row := range stale {
		err := s.tx(ctx, func(tx *gorm.DB) error {
			d, err := s.repo.DeliveryForUpdate(ctx, tx, row.TenantID, row.ID)
			if err != nil {
				if errs.KindOf(err) == errs.NotFound {
					return nil
				}
				return err
			}
			if d.Status != domain.StatusPending || (d.LeaseUntil != nil && d.LeaseUntil.After(now)) {
				return nil
			}
			if d.CycleAttempts >= s.opts.Retry.MaxAttempts {
				out.Failed++
				return s.failFinal(ctx, tx, d, domain.ErrTextAttemptsExceeded)
			}
			d.EnqueuedAt = now
			d.LeaseUntil = nil
			d.UpdatedAt = now
			if err := s.repo.SaveDelivery(ctx, tx, d); err != nil {
				return err
			}
			out.Requeued++
			return s.enqueue(ctx, tx, d)
		})
		if err != nil {
			return out, err
		}
	}

	over, err := s.repo.ActiveEndpointsOverThreshold(ctx, s.opts.DisableAfterFailures, sweepBatchSize)
	if err != nil {
		return out, err
	}
	for _, e := range over {
		if err := s.tx(ctx, func(tx *gorm.DB) error {
			return s.disable(ctx, tx, e, e.ConsecutiveFailures)
		}); err != nil {
			return out, err
		}
		out.Disabled++
	}

	if err := s.repo.MarkRun(ctx, contracts.JobRetrySweep, now); err != nil {
		return out, err
	}
	return out, nil
}

// PurgeTenant handles tenant.deleted.v1: every endpoint and delivery of the
// tenant goes. Idempotent: deleting nothing is success.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return errs.New(errs.Invalid, "tenant.deleted.v1 without tenant_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, tenantID)
	})
}
