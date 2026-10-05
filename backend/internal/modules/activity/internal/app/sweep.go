package app

import (
	"context"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/activity/contracts"
)

// StuckSweepJob is the cron job name and its reconcile-marker key.
const StuckSweepJob = "activity.stuck_sweep"

// SweepStuck re-publishes activity.received.v1 for activities still
// pending StuckAfter after they arrived (or after their last re-publish).
// Rules' decisions are idempotent on activity_id, so a re-publish of an
// activity that was in fact decided is harmless. It is reconciling: every
// run looks at current state, never at a delta, so a skipped tick leaves
// no gap (R47). Each row is re-published at most MaxRepublishes times;
// rows at the cap stay pending and are reported for a human.
func (s *Service) SweepStuck(ctx context.Context) error {
	lastRun, err := s.repo.LastRun(ctx, StuckSweepJob)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	q := StuckQuery{
		ReceivedBefore: now.Add(-s.cfg.StuckAfter),
		MaxRepublishes: s.cfg.MaxRepublishes,
		Limit:          s.cfg.SweepBatch,
	}

	var republished int
	err = s.tx(ctx, func(tx *gorm.DB) error {
		rows, err := s.repo.LockStuck(ctx, tx, q)
		if err != nil || len(rows) == 0 {
			return err
		}
		ids := make([]string, len(rows))
		for i, a := range rows {
			ids[i] = a.ID
			if err := s.outbox.Publish(ctx, tx, contracts.TopicReceived, s.toReceived(a)); err != nil {
				return err
			}
		}
		republished = len(rows)
		return s.repo.MarkRepublished(ctx, tx, ids, now)
	})
	if err != nil {
		return err
	}

	if republished > 0 {
		if s.republished != nil {
			s.republished.Add(float64(republished))
		}
		s.log.Warn("re-published stuck activities",
			zap.Int("count", republished), zap.Time("previous_run", lastRun))
	}

	exhausted, err := s.repo.CountExhausted(ctx, q.ReceivedBefore, q.MaxRepublishes)
	if err != nil {
		return err
	}
	if exhausted > 0 {
		s.log.Error("activities still pending after the re-publish cap; inspect rules consumer and DLQ",
			zap.Int64("count", exhausted), zap.Int("max_republishes", q.MaxRepublishes))
	}
	return s.repo.MarkRun(ctx, StuckSweepJob, now)
}
