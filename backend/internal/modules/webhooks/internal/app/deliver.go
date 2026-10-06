package app

import (
	"context"

	"gorm.io/gorm"

	"levelup/internal/modules/webhooks/contracts"
	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/shared/errs"
)

// HandleDeliver runs job.webhooks.deliver: one attempt for one delivery.
//
//  1. Claim (tx 1): lock the row; skip if it is no longer pending or
//     another attempt holds the lease; fail it if the endpoint is gone or
//     inactive; otherwise count the attempt and take a lease. Commit.
//  2. POST outside any transaction (never call out with a tx open).
//  3. Settle (tx 2): record status code, latency, response snippet. A 2xx
//     succeeds and resets the endpoint's failure streak. A failure is final
//     when the retry ladder or the attempt budget is spent: the delivery is
//     marked failed and the endpoint's failure streak grows, disabling it
//     at the threshold (publishing webhooks.endpoint_disabled.v1 once).
//
// A non-final failure returns errs.Unavailable so the job retry ladder
// tries again; everything else returns nil.
func (s *Service) HandleDeliver(ctx context.Context, cmd contracts.DeliverCmdV1) error {
	if cmd.TenantID == "" || cmd.DeliveryID == "" {
		return errs.New(errs.Invalid, "job.webhooks.deliver without tenant_id or delivery_id")
	}
	var (
		req     Request
		attempt bool
	)
	now := s.clock.Now()
	err := s.tx(ctx, func(tx *gorm.DB) error {
		d, err := s.repo.DeliveryForUpdate(ctx, tx, cmd.TenantID, cmd.DeliveryID)
		if errs.KindOf(err) == errs.NotFound {
			return nil // purged with its tenant or endpoint: nothing to do
		}
		if err != nil {
			return err
		}
		if d.Status != domain.StatusPending || (d.LeaseUntil != nil && d.LeaseUntil.After(now)) {
			return nil // settled, or another attempt is in flight
		}
		e, err := s.repo.EndpointByID(ctx, cmd.TenantID, d.EndpointID)
		if err != nil && errs.KindOf(err) != errs.NotFound {
			return err
		}
		if err != nil || !e.Active {
			d.Fail(domain.ErrTextEndpointInactive, now)
			return s.repo.SaveDelivery(ctx, tx, d)
		}
		if d.CycleAttempts >= s.opts.Retry.MaxAttempts {
			return s.failFinal(ctx, tx, d, domain.ErrTextAttemptsExceeded)
		}
		if err := d.Begin(now, s.opts.Lease); err != nil {
			return err
		}
		if err := s.repo.SaveDelivery(ctx, tx, d); err != nil {
			return err
		}
		attempt = true
		req = Request{URL: e.URL, Event: d.Event, DeliveryID: d.ID, Secret: e.Secret, Body: d.Payload, At: now}
		return nil
	})
	if err != nil || !attempt {
		return err
	}

	res := s.sender.Send(ctx, req)

	retry := false
	err = s.tx(ctx, func(tx *gorm.DB) error {
		d, err := s.repo.DeliveryForUpdate(ctx, tx, cmd.TenantID, cmd.DeliveryID)
		if errs.KindOf(err) == errs.NotFound {
			return nil
		}
		if err != nil {
			return err
		}
		if d.Status != domain.StatusPending {
			return nil
		}
		final := s.opts.Retry.Final(d.CycleAttempts, cmd.BaseAttempt)
		d.Settle(res, final, s.clock.Now())
		if err := s.repo.SaveDelivery(ctx, tx, d); err != nil {
			return err
		}
		switch {
		case res.OK():
			return s.repo.ResetFailures(ctx, tx, d.TenantID, d.EndpointID)
		case final:
			return s.countFailure(ctx, tx, d)
		default:
			retry = true
			return nil
		}
	})
	if err != nil {
		return err
	}
	if retry {
		return errs.WithCode(errs.New(errs.Unavailable, "webhook delivery failed: "+res.Error()), domain.CodeDeliveryFailed)
	}
	return nil
}

// failFinal marks a delivery failed without an attempt and counts it.
func (s *Service) failFinal(ctx context.Context, tx *gorm.DB, d domain.Delivery, reason string) error {
	d.Fail(reason, s.clock.Now())
	if err := s.repo.SaveDelivery(ctx, tx, d); err != nil {
		return err
	}
	return s.countFailure(ctx, tx, d)
}

// countFailure grows the endpoint's streak of consecutive failed deliveries
// and disables it at the threshold.
func (s *Service) countFailure(ctx context.Context, tx *gorm.DB, d domain.Delivery) error {
	n, err := s.repo.IncrementFailures(ctx, tx, d.TenantID, d.EndpointID)
	if err != nil {
		return err
	}
	if n < s.opts.DisableAfterFailures {
		return nil
	}
	e, err := s.repo.EndpointByID(ctx, d.TenantID, d.EndpointID)
	if err != nil {
		if errs.KindOf(err) == errs.NotFound {
			return nil
		}
		return err
	}
	return s.disable(ctx, tx, e, n)
}

func (s *Service) disable(ctx context.Context, tx *gorm.DB, e domain.Endpoint, failures int) error {
	now := s.clock.Now()
	changed, err := s.repo.DisableEndpoint(ctx, tx, e.TenantID, e.ID, contracts.DisabledReasonConsecutiveFailures, now)
	if err != nil || !changed {
		return err
	}
	return s.outbox.Publish(ctx, tx, contracts.TopicEndpointDisabled, contracts.EndpointDisabledV1{
		TenantID:            e.TenantID,
		EndpointID:          e.ID,
		URL:                 e.URL,
		Reason:              contracts.DisabledReasonConsecutiveFailures,
		ConsecutiveFailures: failures,
		At:                  now,
	})
}
