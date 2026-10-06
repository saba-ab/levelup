package app

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/modules/leaderboards/internal/ports"
	"levelup/internal/shared/errs"
)

// rankOp is a read-model write deferred until after commit.
type rankOp struct {
	period   domain.Period
	playerID string
	op       domain.Op
	delta    int64 // for increments
	score    int64 // for sets: the committed score
}

// ApplyFact projects one upstream fact onto every matching active board of
// the tenant. Idempotent per (event, board) through applied_events in the
// same transaction as the score upsert; increments commute, so delivery
// order does not matter (ADR-0012). Redis is touched after commit only.
func (s *Service) ApplyFact(ctx context.Context, f domain.Fact) error {
	if f.EventID == "" || f.TenantID == "" || f.PlayerID == "" {
		return errs.New(errs.Invalid, "fact needs event_id, tenant_id and player_id")
	}
	typ := f.Kind.BoardType()
	if typ == "" {
		return errs.New(errs.Invalid, "unknown fact kind "+string(f.Kind))
	}
	if f.At.IsZero() {
		return errs.New(errs.Invalid, "fact needs an occurrence time")
	}
	var boards []domain.Leaderboard
	var err error
	if f.Kind == domain.FactActivity {
		if f.EventType == "" {
			return errs.New(errs.Invalid, "activity fact needs an event type")
		}
		boards, err = s.repo.ActiveForActivity(ctx, f.TenantID, f.EventType)
	} else {
		boards, err = s.repo.ActiveByType(ctx, f.TenantID, typ)
	}
	if err != nil {
		return err
	}
	type planned struct {
		board domain.Leaderboard
		op    domain.Op
	}
	var plan []planned
	for _, b := range boards {
		if op, ok := domain.Contribution(b, f); ok {
			plan = append(plan, planned{board: b, op: op})
		}
	}
	if len(plan) == 0 {
		return nil
	}

	now := s.clock.Now()
	var after []rankOp
	err = s.tx(ctx, func(tx *gorm.DB) error {
		after = after[:0]
		hidden, err := s.repo.IsHidden(ctx, tx, f.TenantID, f.PlayerID)
		if err != nil {
			return err
		}
		for _, pl := range plan {
			if pl.board.ProgramID != "" {
				member, err := s.repo.IsMember(ctx, tx, pl.board.ProgramID, f.PlayerID)
				if err != nil {
					return err
				}
				if !member {
					continue // program boards count enrolled players only
				}
			}
			period := domain.PeriodOf(pl.board, f.At)
			res, err := s.repo.ApplyScore(ctx, tx, ScoreChange{
				EventID: f.EventID, Period: period, PlayerID: f.PlayerID,
				Op: pl.op, At: f.At, Now: now,
			})
			if err != nil {
				return err
			}
			if !res.Applied || !res.Changed || hidden {
				continue
			}
			after = append(after, rankOp{period: period, playerID: f.PlayerID, op: pl.op, delta: pl.op.Value, score: res.Score})
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.applyRankOps(ctx, after)
	return nil
}

// ApplyActivity is the activity.received.v1 entry point. An activity whose
// player was not resolved at ingest cannot be attributed: it is acked with
// no effect rather than dead-lettered. A malformed player id can never
// succeed and is errs.Invalid.
func (s *Service) ApplyActivity(ctx context.Context, f domain.Fact) error {
	if f.Kind != domain.FactActivity {
		return errs.New(errs.Invalid, "not an activity fact")
	}
	if f.PlayerID == "" {
		r, ok := s.players.(ports.ExternalIDResolver)
		if !f.AutoCreatePlayer || f.PlayerExternalID == "" || !ok {
			return nil
		}
		id, found, err := r.IDByExternalID(ctx, f.TenantID, f.PlayerExternalID)
		if err != nil {
			return err
		}
		if !found {
			return errs.New(errs.Unavailable, "player not created yet for auto-create activity; retrying")
		}
		f.PlayerID = id
	}
	if _, err := uuid.Parse(f.PlayerID); err != nil {
		return errs.New(errs.Invalid, "activity player_id is not a uuid")
	}
	return s.ApplyFact(ctx, f)
}

// applyRankOps mirrors committed score changes into redis. Failures are
// logged, never returned: Postgres already holds the truth and the rebuild
// job heals the read model.
func (s *Service) applyRankOps(ctx context.Context, ops []rankOp) {
	for _, o := range ops {
		var err error
		switch o.op.Kind {
		case domain.OpIncrement:
			err = s.ranks.Increment(ctx, o.period, o.playerID, o.delta)
		case domain.OpSet:
			err = s.ranks.Set(ctx, o.period, o.playerID, o.score)
		}
		if err != nil {
			s.log.Warn("leaderboard read-model write failed; rebuild will heal",
				zap.String("leaderboard_id", o.period.LeaderboardID), zap.Error(err))
		}
	}
}

// SetPlayerActive projects player.activated/deactivated.v1. Last writer by
// event time wins, so a stale redelivery cannot flip the state back.
func (s *Service) SetPlayerActive(ctx context.Context, tenantID, playerID string, active bool, at time.Time) error {
	if tenantID == "" || playerID == "" {
		return errs.New(errs.Invalid, "player status needs tenant_id and player_id")
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.SetPlayerStatus(ctx, tx, tenantID, playerID, active, at)
	}); err != nil {
		return err
	}
	return s.syncPlayer(ctx, tenantID, playerID)
}

// PlayerDeleted projects player.deleted.v1: the player is hidden for good.
func (s *Service) PlayerDeleted(ctx context.Context, tenantID, playerID string, at time.Time) error {
	if tenantID == "" || playerID == "" {
		return errs.New(errs.Invalid, "player deletion needs tenant_id and player_id")
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.MarkPlayerDeleted(ctx, tx, tenantID, playerID, at)
	}); err != nil {
		return err
	}
	return s.syncPlayer(ctx, tenantID, playerID)
}

// SetMembership projects program.player_enrolled/unenrolled.v1.
func (s *Service) SetMembership(ctx context.Context, tenantID, programID, playerID string, enrolled bool, at time.Time) error {
	if tenantID == "" || programID == "" || playerID == "" {
		return errs.New(errs.Invalid, "enrolment needs tenant_id, program_id and player_id")
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.SetMembership(ctx, tx, tenantID, programID, playerID, enrolled, at)
	}); err != nil {
		return err
	}
	return s.syncPlayer(ctx, tenantID, playerID)
}

// syncPlayer makes the read model agree with the player's current
// visibility: state-based, so it is idempotent and order-independent.
func (s *Service) syncPlayer(ctx context.Context, tenantID, playerID string) error {
	since := s.clock.Now().Add(-s.settings.ClosedRetention)
	rows, err := s.repo.PlayerScores(ctx, tenantID, playerID, since)
	if err != nil {
		return err
	}
	for _, r := range rows {
		var werr error
		if r.Visible {
			werr = s.ranks.Set(ctx, r.Period, playerID, r.Score)
		} else {
			werr = s.ranks.Remove(ctx, r.Period, playerID)
		}
		if werr != nil {
			s.log.Warn("leaderboard read-model visibility sync failed; rebuild will heal",
				zap.String("leaderboard_id", r.Period.LeaderboardID), zap.Error(werr))
		}
	}
	return nil
}

// PurgeTenant handles tenant.deleted.v1: every row goes, then the read-model
// keys of the tenant's boards (listed from Postgres; never KEYS/SCAN).
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return errs.New(errs.Invalid, "tenant purge needs tenant_id")
	}
	var boards []domain.Leaderboard
	var periods []domain.Period
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		var err error
		boards, periods, err = s.repo.PurgeTenant(ctx, tx, tenantID)
		return err
	}); err != nil {
		return err
	}
	now := s.clock.Now()
	for _, b := range boards {
		periods = append(periods, domain.PeriodOf(b, now))
	}
	if len(periods) == 0 {
		return nil
	}
	if err := s.ranks.Delete(ctx, periods...); err != nil {
		s.log.Warn("drop purged tenant read model; keys expire by TTL", zap.String("tenant_id", tenantID), zap.Error(err))
	}
	return nil
}
