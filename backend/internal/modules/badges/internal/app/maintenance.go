package app

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/platform/bus"
	"levelup/internal/shared/errs"
)

// OnTenantDeleted purges every badges row of the tenant. Idempotent: a
// redelivery deletes nothing more.
func (s *Service) OnTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	if _, err := uuid.Parse(ev.TenantID); err != nil {
		return errs.Wrap(errs.Invalid, "tenant.deleted.v1 without a valid tenant_id", err)
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, ev.TenantID)
	})
}

// Reconcile sweeps holdings touched since the last successful run and
// reports drift (earned_count != applied awards, earned_count > max_awards)
// as a log line and a metric. It never repairs: drift means a bug, and a
// human decides (R47).
func (s *Service) Reconcile(ctx context.Context, rec Reconciler) error {
	since, err := rec.LastRun(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	drifts, err := rec.DriftSince(ctx, since)
	if err != nil {
		return err
	}
	for _, d := range drifts {
		for _, check := range d.Checks() {
			if s.drift != nil {
				s.drift.WithLabelValues(check).Inc()
			}
			s.log.Warn("badges reconcile drift",
				zap.String("check", check),
				zap.String("tenant_id", d.TenantID),
				zap.String("player_badge_id", d.PlayerBadgeID),
				zap.String("player_id", d.PlayerID),
				zap.String("badge_id", d.BadgeID),
				zap.Int("earned_count", d.EarnedCount),
				zap.Int("applied_awards", d.AppliedAwards))
		}
	}
	return rec.MarkRun(ctx, now)
}
