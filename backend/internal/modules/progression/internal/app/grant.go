package app

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	badgescontracts "levelup/internal/modules/badges/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// GrantResult is the outcome of one grant. Replayed means the idempotency
// key was already applied and nothing changed; Rejected carries an
// effect.Reason* when the command was refused as a business result.
type GrantResult struct {
	Grant         domain.XPGrant
	Progress      domain.Progress
	LevelsReached []domain.Level
	Replayed      bool
	Rejected      string
}

// ManualGrant is the HTTP request to grant XP to a player.
type ManualGrant struct {
	PlayerID       string
	Amount         int64
	Description    string
	IdempotencyKey string // the client's Idempotency-Key header, optional
}

// GrantXP is the HTTP path: authorize, validate the player (404 / 409),
// then apply. The key is "manual:"+header so a retried request after the
// middleware cache expired is still a ledger no-op.
func (s *Service) GrantXP(ctx context.Context, req ManualGrant) (GrantResult, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return GrantResult{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermGrantXP, nil); err != nil {
		return GrantResult{}, err
	}
	if err := s.requireActivePlayer(ctx, p.TenantID, req.PlayerID); err != nil {
		return GrantResult{}, err
	}
	key := id.NewID()
	if req.IdempotencyKey != "" {
		key = "manual:" + req.IdempotencyKey
	}
	g, err := domain.NewXPGrant(domain.GrantSpec{
		TenantID:       p.TenantID,
		PlayerID:       req.PlayerID,
		IdempotencyKey: key,
		Amount:         req.Amount,
		Description:    req.Description,
		SourceKind:     effect.SourceManual,
		SourceID:       p.UserID,
		CreatedBy:      p.UserID,
	}, s.clock.Now())
	if err != nil {
		return GrantResult{}, err
	}
	return s.apply(ctx, g)
}

// HandleGrantXP consumes job.progression.grant_xp. Malformed commands are
// errs.Invalid (immediate DLQ); an unknown or inactive player is a
// rejection fact, not an error.
func (s *Service) HandleGrantXP(ctx context.Context, cmd contracts.GrantXPCmdV1) (GrantResult, error) {
	g, err := domain.NewXPGrant(domain.GrantSpec{
		TenantID:       cmd.TenantID,
		PlayerID:       cmd.PlayerID,
		IdempotencyKey: cmd.IdempotencyKey,
		Amount:         cmd.Amount,
		Description:    cmd.Description,
		SourceKind:     cmd.Source.Kind,
		SourceID:       cmd.Source.ID,
		ActivityID:     cmd.Source.ActivityID,
		OccurredAt:     cmd.OccurredAt,
	}, s.clock.Now())
	if err != nil {
		return GrantResult{}, errs.Wrap(errs.Invalid, "invalid grant_xp command", err)
	}

	player, err := s.players.ByID(ctx, cmd.TenantID, cmd.PlayerID)
	switch {
	case errs.KindOf(err) == errs.NotFound:
		return s.reject(ctx, cmd, effect.ReasonPlayerNotFound)
	case err != nil:
		return GrantResult{}, err
	case !player.Active:
		return s.reject(ctx, cmd, effect.ReasonPlayerInactive)
	}
	return s.apply(ctx, g)
}

func (s *Service) reject(ctx context.Context, cmd contracts.GrantXPCmdV1, reason string) (GrantResult, error) {
	now := s.clock.Now()
	err := s.tx(ctx, func(tx *gorm.DB) error {
		inserted, err := s.repo.InsertRejection(ctx, tx, domain.GrantRejection{
			TenantID:       cmd.TenantID,
			IdempotencyKey: cmd.IdempotencyKey,
			PlayerID:       cmd.PlayerID,
			Reason:         reason,
			CreatedAt:      now,
		})
		if err != nil || !inserted {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicGrantRejected, contracts.GrantRejectedV1{
			IdempotencyKey: cmd.IdempotencyKey,
			TenantID:       cmd.TenantID,
			PlayerID:       cmd.PlayerID,
			Amount:         cmd.Amount,
			Reason:         reason,
			Source:         cmd.Source,
			At:             now,
		})
	})
	if err != nil {
		return GrantResult{}, err
	}
	return GrantResult{Rejected: reason}, nil
}

// apply records the grant and its consequences in one transaction:
//  1. ledger row, ON CONFLICT DO NOTHING (conflict → replay, no publish);
//  2. lock the progress row and add the amount;
//  3. re-derive the level from total XP against the active ladder;
//  4. per crossed level, insert level_rewards once and only then publish
//     progression.level_reached.v1 plus the reward commands;
//  5. publish progression.xp_gained.v1.
func (s *Service) apply(ctx context.Context, g domain.XPGrant) (GrantResult, error) {
	now := s.clock.Now()
	var res GrantResult
	err := s.tx(ctx, func(tx *gorm.DB) error {
		res = GrantResult{Grant: g}
		inserted, err := s.repo.InsertGrant(ctx, tx, g)
		if err != nil {
			return err
		}
		if !inserted {
			res.Replayed = true
			if res.Grant, err = s.repo.GrantByKey(ctx, tx, g.TenantID, g.IdempotencyKey); err != nil {
				return err
			}
			return nil
		}

		ladder, err := s.repo.Ladder(ctx, tx, g.TenantID, false)
		if err != nil {
			return err
		}
		if err := s.repo.EnsureProgress(ctx, tx, domain.NewProgress(g.TenantID, g.PlayerID, now)); err != nil {
			return err
		}
		prog, err := s.repo.ProgressForUpdate(ctx, tx, g.TenantID, g.PlayerID)
		if err != nil {
			return err
		}
		crossed, err := prog.Gain(g.Amount, ladder, now)
		if err != nil {
			return err
		}
		if err := s.repo.SaveProgress(ctx, tx, prog); err != nil {
			return err
		}
		prog.Version++
		res.Progress = prog

		for _, lvl := range crossed {
			reached, err := s.reachLevel(ctx, tx, g, prog, lvl, now)
			if err != nil {
				return err
			}
			if reached {
				res.LevelsReached = append(res.LevelsReached, lvl)
			}
		}

		return s.outbox.Publish(ctx, tx, contracts.TopicXPGained, contracts.XPGainedV1{
			GrantID:        g.ID,
			IdempotencyKey: g.IdempotencyKey,
			TenantID:       g.TenantID,
			PlayerID:       g.PlayerID,
			Amount:         g.Amount,
			TotalXP:        prog.TotalXP,
			LevelNumber:    prog.LevelNumber,
			Source:         effect.Source{Kind: g.SourceKind, ID: g.SourceID, ActivityID: g.ActivityID},
			OccurredAt:     g.OccurredAt,
			At:             now,
		})
	})
	if err != nil {
		return GrantResult{}, err
	}
	if res.Replayed {
		// Report the current state, read after commit.
		got, err := s.repo.ProgressByPlayers(ctx, g.TenantID, []string{g.PlayerID})
		if err != nil {
			return GrantResult{}, err
		}
		res.Progress = got[g.PlayerID]
	}
	return res, nil
}

// reachLevel records (player, level) once and, only when this call
// inserted it, publishes the fact and issues the reward commands. Laravel
// never awarded badge_reward_id (B13); progression decides the effect, so
// progression issues the command (00 §3 rule 3).
func (s *Service) reachLevel(ctx context.Context, tx *gorm.DB, g domain.XPGrant, prog domain.Progress,
	lvl domain.Level, at time.Time) (bool, error) {
	reward := domain.NewLevelReward(g.TenantID, g.PlayerID, lvl.ID, at)
	inserted, err := s.repo.InsertLevelReward(ctx, tx, reward)
	if err != nil || !inserted {
		return false, err
	}
	if err := s.outbox.Publish(ctx, tx, contracts.TopicLevelReached, contracts.LevelReachedV1{
		TenantID:      g.TenantID,
		PlayerID:      g.PlayerID,
		LevelID:       lvl.ID,
		LevelNumber:   lvl.Number,
		LevelName:     lvl.Name,
		TotalXP:       prog.TotalXP,
		LevelRewardID: reward.ID,
		PointsReward:  lvl.PointsReward,
		BadgeRewardID: lvl.BadgeRewardID,
		ActivityID:    g.ActivityID,
		At:            at,
	}); err != nil {
		return false, err
	}
	source := effect.Source{Kind: effect.SourceLevel, ID: reward.ID, ActivityID: g.ActivityID}
	if lvl.PointsReward > 0 {
		if err := s.outbox.Publish(ctx, tx, pointscontracts.Topic(pointscontracts.JobCredit), pointscontracts.CreditCmdV1{
			IdempotencyKey: id.Derive("level_reward", g.PlayerID, lvl.ID, "points"),
			TenantID:       g.TenantID,
			PlayerID:       g.PlayerID,
			Amount:         lvl.PointsReward,
			Kind:           pointscontracts.KindBonus,
			Description:    fmt.Sprintf("Reached level %d: %s", lvl.Number, lvl.Name),
			Source:         source,
			OccurredAt:     at,
		}); err != nil {
			return false, err
		}
	}
	if lvl.BadgeRewardID != "" {
		if err := s.outbox.Publish(ctx, tx, badgescontracts.Topic(badgescontracts.JobAward), badgescontracts.AwardCmdV1{
			IdempotencyKey: id.Derive("level_reward", g.PlayerID, lvl.ID, "badge"),
			TenantID:       g.TenantID,
			PlayerID:       g.PlayerID,
			BadgeID:        lvl.BadgeRewardID,
			Source:         source,
			OccurredAt:     at,
		}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Service) requireActivePlayer(ctx context.Context, tenantID, playerID string) error {
	pl, err := s.players.ByID(ctx, tenantID, playerID)
	switch {
	case errs.KindOf(err) == errs.NotFound:
		return domain.ErrPlayerNotFound
	case err != nil:
		return err
	case !pl.Active:
		return domain.ErrPlayerInactive
	}
	return nil
}

func (s *Service) requirePlayer(ctx context.Context, tenantID, playerID string) error {
	_, err := s.players.ByID(ctx, tenantID, playerID)
	if errs.KindOf(err) == errs.NotFound {
		return domain.ErrPlayerNotFound
	}
	return err
}
