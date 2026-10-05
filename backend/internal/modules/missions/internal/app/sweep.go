package app

import (
	"context"

	"gorm.io/gorm"

	"levelup/internal/modules/missions/contracts"
	"levelup/internal/shared/errs"
)

// ExpireSweep is the missions.expire_sweep cron (R47). It is reconciling:
// it selects by state ("active/paused and past ends_at", "open and past its
// period or mission over"), never by delta, so a skipped tick heals on the
// next one. The last-run marker records the reconciliation horizon for
// operators; it is advanced only after a fully successful sweep.
//
//  1. missions past ends_at → expired, one missions.expired.v1 each;
//  2. open attempts whose period ended, or whose mission is expired,
//     archived or deleted → expired, one missions.attempt_expired.v1 each.
//
// Each batch is its own transaction with FOR UPDATE SKIP LOCKED, so the
// sweep never blocks live progress for long and two workers never expire
// the same row twice.
func (s *Service) ExpireSweep(ctx context.Context) error {
	if _, err := s.repo.LastRun(ctx, contracts.JobExpireSweep); err != nil {
		return err
	}
	now := s.clock.Now()
	for {
		n, err := s.expireMissionBatch(ctx)
		if err != nil {
			return err
		}
		if n < s.batch {
			break
		}
	}
	for {
		n, err := s.expireAttemptBatch(ctx)
		if err != nil {
			return err
		}
		if n < s.batch {
			break
		}
	}
	return s.repo.MarkRun(ctx, contracts.JobExpireSweep, now)
}

func (s *Service) expireMissionBatch(ctx context.Context) (int, error) {
	now := s.clock.Now()
	var n int
	err := s.tx(ctx, func(tx *gorm.DB) error {
		due, err := s.repo.DueMissions(ctx, tx, now, s.batch)
		if err != nil {
			return err
		}
		n = len(due)
		for _, m := range due {
			if err := m.TransitionTo(contracts.MissionExpired, now); err != nil {
				return errs.Wrap(errs.Internal, "expire mission "+m.ID, err)
			}
			if err := s.repo.SaveMission(ctx, tx, m); err != nil {
				return err
			}
			if err := s.outbox.Publish(ctx, tx, contracts.TopicExpired, changed(m, now)); err != nil {
				return err
			}
		}
		return nil
	})
	return n, err
}

func (s *Service) expireAttemptBatch(ctx context.Context) (int, error) {
	now := s.clock.Now()
	var n int
	err := s.tx(ctx, func(tx *gorm.DB) error {
		due, err := s.repo.DueAttempts(ctx, tx, now, s.batch)
		if err != nil {
			return err
		}
		n = len(due)
		for _, a := range due {
			if err := a.Close(contracts.AttemptExpired, now); err != nil {
				return errs.Wrap(errs.Internal, "expire attempt "+a.ID, err)
			}
			if err := s.repo.SaveAttempt(ctx, tx, a); err != nil {
				return err
			}
			if err := s.outbox.Publish(ctx, tx, contracts.TopicAttemptExpired, attemptV1(a, now)); err != nil {
				return err
			}
		}
		return nil
	})
	return n, err
}

// OnTenantDeleted purges every row of the tenant (replaces the Laravel
// cascades). Idempotent: a redelivery deletes nothing more.
func (s *Service) OnTenantDeleted(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return errs.New(errs.Invalid, "tenant.deleted.v1 without tenant_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, tenantID)
	})
}

// OnPlayerDeleted abandons the player's open attempts. Idempotent by state:
// only in_progress rows change, so a redelivery is a no-op. Completed
// attempts stay as history.
func (s *Service) OnPlayerDeleted(ctx context.Context, tenantID, playerID string) error {
	if tenantID == "" || playerID == "" {
		return errs.New(errs.Invalid, "player.deleted.v1 without tenant_id or player_id")
	}
	now := s.clock.Now()
	return s.tx(ctx, func(tx *gorm.DB) error {
		_, err := s.repo.AbandonOpenAttempts(ctx, tx, tenantID, playerID, now)
		return err
	})
}
