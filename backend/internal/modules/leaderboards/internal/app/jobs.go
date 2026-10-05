package app

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/modules/leaderboards/internal/domain"
)

// Job names and reconcile-marker keys.
const (
	JobRollover    = "leaderboards.rollover"
	JobRebuild     = "leaderboards.rebuild"
	JobPruneEvents = "leaderboards.applied_events_prune"

	TopN           = 10 // entries carried by leaderboards.period_closed.v1
	duePageSize    = 200
	maxDuePages    = 50
	boardsPageSize = 200
)

// Rollover closes every period that ended (plus the grace window) and has
// not been closed yet: snapshot + leaderboards.period_closed.v1, exactly
// once per period. Reconciling by construction: it sweeps all unclosed
// periods, so a skipped tick leaves no gap (R47).
func (s *Service) Rollover(ctx context.Context) error {
	now := s.clock.Now()
	cutoff := now.Add(-s.settings.CloseGrace)
	var failures []error
	for range maxDuePages {
		due, err := s.repo.DuePeriods(ctx, cutoff, duePageSize)
		if err != nil {
			return err
		}
		progressed := 0
		for _, p := range due {
			if err := s.closePeriod(ctx, p); err != nil {
				failures = append(failures, err)
				s.log.Warn("close leaderboard period", zap.String("leaderboard_id", p.LeaderboardID), zap.Error(err))
				continue
			}
			progressed++
		}
		if len(due) < duePageSize || progressed == 0 {
			break
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	return s.repo.MarkRun(ctx, JobRollover, now)
}

func (s *Service) closePeriod(ctx context.Context, p domain.Period) error {
	lb, err := s.repo.ByID(ctx, p.TenantID, p.LeaderboardID)
	if err != nil {
		return err
	}
	at := s.clock.Now()
	return s.tx(ctx, func(tx *gorm.DB) error {
		closed, top, err := s.repo.ClosePeriod(ctx, tx, lb, p, at, s.settings.SnapshotLimit, TopN)
		if err != nil || !closed {
			return err
		}
		ev := contracts.PeriodClosedV1{
			LeaderboardID: lb.ID,
			TenantID:      lb.TenantID,
			PeriodStart:   p.Start,
			PeriodEnd:     p.End,
			Top:           make([]contracts.TopEntryV1, 0, TopN),
			At:            at,
		}
		for _, st := range top {
			ev.Top = append(ev.Top, contracts.TopEntryV1{PlayerID: st.PlayerID, Rank: int(st.Rank), Score: st.Score})
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicPeriodClosed, ev)
	})
}

// RebuildAll reloads the read model of every active board from Postgres
// (R64: flush redis → rebuild → identical ranks).
func (s *Service) RebuildAll(ctx context.Context) error {
	now := s.clock.Now()
	after := ""
	var failures []error
	for {
		boards, err := s.repo.ActiveBoards(ctx, after, boardsPageSize)
		if err != nil {
			return err
		}
		for _, b := range boards {
			if _, err := s.rebuildBoard(ctx, b); err != nil {
				failures = append(failures, err)
				s.log.Warn("rebuild leaderboard", zap.String("leaderboard_id", b.ID), zap.Error(err))
			}
		}
		if len(boards) < boardsPageSize {
			break
		}
		after = boards[len(boards)-1].ID
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	return s.repo.MarkRun(ctx, JobRebuild, now)
}

// PruneAppliedEvents drops idempotency rows older than the retention window:
// far beyond any redelivery the broker's retry ladder can produce.
func (s *Service) PruneAppliedEvents(ctx context.Context) error {
	now := s.clock.Now()
	n, err := s.repo.PruneAppliedEvents(ctx, now.Add(-s.settings.AppliedRetention))
	if err != nil {
		return err
	}
	s.log.Debug("pruned applied events", zap.Int64("rows", n))
	return s.repo.MarkRun(ctx, JobPruneEvents, now)
}
