package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/modules/badges/internal/ports"
	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/platform/jobs"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// AwardCmd is one award request, from HTTP or from job.badges.award.
type AwardCmd struct {
	TenantID       string
	PlayerID       string
	BadgeID        string
	IdempotencyKey string
	Source         effect.Source
	AwardedBy      string
	OccurredAt     time.Time
}

// AwardOutcome is the settled result. Replay is true when the key was
// already recorded: nothing was written or published by this call.
type AwardOutcome struct {
	Award       domain.Award
	PlayerBadge domain.PlayerBadge
	Replay      bool
}

// errKeyRace aborts a transaction whose award insert lost the
// (tenant_id, idempotency_key) race to a concurrent delivery of the same
// command; the caller then reads the winner's row and reports a replay.
var errKeyRace = errors.New("badges: idempotency key recorded concurrently")

// PointsCreditKey is the deterministic idempotency key of the points
// credit an applied award issues: a redelivered award never credits twice,
// and the key is derivable from the award id alone.
func PointsCreditKey(awardID string) string { return id.Derive("badge_award", awardID, "points") }

// ManualAwardKey namespaces HTTP awards: the Idempotency-Key header when
// present (so the ledger stays protected after the middleware's replay
// cache expires), otherwise a fresh key (no idempotency, parity).
func ManualAwardKey(header string) string {
	if header == "" {
		return "manual:" + id.NewID()
	}
	return "manual:" + header
}

// AwardManually is POST /badges/{id}/award. Badge and player are validated
// first (404 badge_not_found / player_not_found, nothing recorded); the
// remaining business rejections are recorded and published like any job
// rejection and then surfaced as 409 with a code.
func (s *Service) AwardManually(ctx context.Context, badgeID, playerID, idempotencyHeader string) (AwardOutcome, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermAward, nil)
	if err != nil {
		return AwardOutcome{}, err
	}
	if _, err := s.repo.BadgeByID(ctx, p.TenantID, badgeID); err != nil {
		return AwardOutcome{}, err
	}
	snap, err := s.requirePlayer(ctx, p.TenantID, playerID)
	if err != nil {
		return AwardOutcome{}, err
	}
	out, err := s.apply(ctx, AwardCmd{
		TenantID:       p.TenantID,
		PlayerID:       playerID,
		BadgeID:        badgeID,
		IdempotencyKey: ManualAwardKey(idempotencyHeader),
		Source:         effect.Source{Kind: effect.SourceManual, ID: p.UserID},
		AwardedBy:      p.UserID,
		OccurredAt:     s.now(),
	}, &snap)
	if err != nil {
		return AwardOutcome{}, err
	}
	if !out.Award.Applied() {
		return out, rejectionError(out.Award.Reason)
	}
	return out, nil
}

func rejectionError(reason string) error {
	switch reason {
	case effect.ReasonAlreadyEarned:
		return domain.ErrAlreadyEarned
	case effect.ReasonLimitReached:
		return domain.ErrMaxAwardsReached
	case effect.ReasonTargetInactive:
		return domain.ErrBadgeInactive
	case effect.ReasonTargetNotFound:
		return domain.ErrBadgeNotFound
	case effect.ReasonPlayerNotFound:
		return domain.ErrPlayerNotFound
	case effect.ReasonPlayerInactive:
		return domain.ErrPlayerInactive
	default:
		return errs.WithCode(errs.New(errs.Conflict, "award rejected: "+reason), "award_rejected")
	}
}

// HandleAwardJob consumes job.badges.award. Malformed payloads are
// errs.Invalid (immediate DLQ); business rejections are published as
// badges.award_rejected.v1 and acked (nil); transient errors are returned
// for the retry ladder; a redelivery is a silent no-op.
func (s *Service) HandleAwardJob(ctx context.Context, body []byte) error {
	var cmd contracts.AwardCmdV1
	if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
		return err
	}
	if err := validateAwardCmd(cmd); err != nil {
		return err
	}
	occurred := cmd.OccurredAt.UTC()
	if occurred.IsZero() {
		occurred = s.now()
	}
	_, err := s.apply(ctx, AwardCmd{
		TenantID:       cmd.TenantID,
		PlayerID:       cmd.PlayerID,
		BadgeID:        cmd.BadgeID,
		IdempotencyKey: cmd.IdempotencyKey,
		Source:         cmd.Source,
		OccurredAt:     occurred,
	}, nil)
	return err
}

func validateAwardCmd(cmd contracts.AwardCmdV1) error {
	fields := map[string]string{}
	if cmd.IdempotencyKey == "" {
		fields["idempotency_key"] = "is required"
	}
	for name, v := range map[string]string{"tenant_id": cmd.TenantID, "player_id": cmd.PlayerID, "badge_id": cmd.BadgeID} {
		if _, err := uuid.Parse(v); err != nil {
			fields[name] = "must be a valid UUID"
		}
	}
	if len(fields) > 0 {
		return errs.WithFields(errs.New(errs.Invalid, "invalid badges.award command"), fields)
	}
	return nil
}

// apply is the single award path. snap, when non-nil, is a player already
// resolved by the caller.
func (s *Service) apply(ctx context.Context, cmd AwardCmd, snap *ports.PlayerSnapshot) (AwardOutcome, error) {
	// Cheap redelivery short-circuit before touching the player provider.
	if a, found, err := s.repo.AwardByKey(ctx, cmd.TenantID, cmd.IdempotencyKey); err != nil {
		return AwardOutcome{}, err
	} else if found {
		return s.replay(ctx, a)
	}

	if snap == nil {
		got, found, err := s.players.ByID(ctx, cmd.TenantID, cmd.PlayerID)
		if err != nil {
			return AwardOutcome{}, err // transient: retry ladder
		}
		if !found || got.TenantID != cmd.TenantID {
			return s.rejectStandalone(ctx, cmd, effect.ReasonPlayerNotFound)
		}
		snap = &got
	}
	if !snap.Active {
		return s.rejectStandalone(ctx, cmd, effect.ReasonPlayerInactive)
	}

	var (
		out      AwardOutcome
		replayed *domain.Award
	)
	err := s.tx(ctx, func(tx *gorm.DB) error {
		if a, found, err := s.repo.AwardByKeyTx(ctx, tx, cmd.TenantID, cmd.IdempotencyKey); err != nil {
			return err
		} else if found {
			replayed = &a
			return nil
		}

		badge, err := s.repo.BadgeByIDTx(ctx, tx, cmd.TenantID, cmd.BadgeID)
		if errs.KindOf(err) == errs.NotFound {
			return s.recordRejection(ctx, tx, cmd, effect.ReasonTargetNotFound, &out)
		}
		if err != nil {
			return err
		}
		// Checked before insert-or-lock so a rejected first award never
		// leaves a placeholder holding behind.
		if reason := badge.CanAward(0); reason != "" {
			return s.recordRejection(ctx, tx, cmd, reason, &out)
		}

		now := s.now()
		pb, err := s.repo.LockOrCreatePlayerBadge(ctx, tx,
			domain.NewPlayerBadge(cmd.TenantID, cmd.PlayerID, cmd.BadgeID, now))
		if err != nil {
			return err
		}
		if reason := badge.CanAward(pb.EarnedCount); reason != "" {
			return s.recordRejection(ctx, tx, cmd, reason, &out)
		}

		isFirst := pb.Award(now)
		award := domain.Award{
			ID:             id.NewID(),
			TenantID:       cmd.TenantID,
			PlayerID:       cmd.PlayerID,
			BadgeID:        cmd.BadgeID,
			PlayerBadgeID:  pb.ID,
			IdempotencyKey: cmd.IdempotencyKey,
			Source:         cmd.Source,
			AwardedBy:      cmd.AwardedBy,
			EarnedCount:    pb.EarnedCount,
			OccurredAt:     cmd.OccurredAt,
			Status:         domain.AwardApplied,
			CreatedAt:      now,
		}
		inserted, err := s.repo.InsertAward(ctx, tx, award)
		if err != nil {
			return err
		}
		if !inserted {
			return errKeyRace
		}
		if err := s.repo.SavePlayerBadge(ctx, tx, pb); err != nil {
			return err
		}
		pb.Version++

		if err := s.outbox.Publish(ctx, tx, contracts.TopicAwarded, contracts.AwardedV1{
			AwardID:            award.ID,
			IdempotencyKey:     award.IdempotencyKey,
			TenantID:           award.TenantID,
			PlayerID:           award.PlayerID,
			BadgeID:            badge.ID,
			BadgeSlug:          badge.Slug,
			EarnedCount:        pb.EarnedCount,
			PointsValue:        badge.PointsValue,
			Source:             award.Source,
			OccurredAt:         award.OccurredAt,
			At:                 now,
			PlayerBadgeID:      pb.ID,
			Tier:               string(badge.Tier),
			Category:           string(badge.Category),
			IsFirstEarn:        isFirst,
			AwardedBy:          award.AwardedBy,
			PlayerBadgeVersion: pb.Version,
		}); err != nil {
			return err
		}
		// Rule 3: badges decides the bonus, so badges issues the credit, in
		// the same tx as the fact that justifies it.
		if badge.PointsValue > 0 {
			if err := s.outbox.Publish(ctx, tx, pointscontracts.Topic(pointscontracts.JobCredit), pointscontracts.CreditCmdV1{
				IdempotencyKey: PointsCreditKey(award.ID),
				TenantID:       award.TenantID,
				PlayerID:       award.PlayerID,
				Amount:         badge.PointsValue,
				Kind:           pointscontracts.KindBonus,
				Description:    "Earned badge: " + badge.Name,
				Source:         effect.Source{Kind: effect.SourceBadge, ID: award.ID, ActivityID: cmd.Source.ActivityID},
				OccurredAt:     award.OccurredAt,
			}); err != nil {
				return err
			}
		}
		out = AwardOutcome{Award: award, PlayerBadge: pb}
		return nil
	})
	switch {
	case errors.Is(err, errKeyRace):
		return s.replayByKey(ctx, cmd)
	case err != nil:
		return AwardOutcome{}, err
	case replayed != nil:
		return s.replay(ctx, *replayed)
	}
	return out, nil
}

// recordRejection appends a rejected ledger row and publishes
// badges.award_rejected.v1 inside tx.
func (s *Service) recordRejection(ctx context.Context, tx *gorm.DB, cmd AwardCmd, reason string, out *AwardOutcome) error {
	now := s.now()
	award := domain.Award{
		ID:             id.NewID(),
		TenantID:       cmd.TenantID,
		PlayerID:       cmd.PlayerID,
		BadgeID:        cmd.BadgeID,
		IdempotencyKey: cmd.IdempotencyKey,
		Source:         cmd.Source,
		AwardedBy:      cmd.AwardedBy,
		OccurredAt:     cmd.OccurredAt,
		Status:         domain.AwardRejected,
		Reason:         reason,
		CreatedAt:      now,
	}
	inserted, err := s.repo.InsertAward(ctx, tx, award)
	if err != nil {
		return err
	}
	if !inserted {
		return errKeyRace
	}
	if err := s.outbox.Publish(ctx, tx, contracts.TopicAwardRejected, contracts.AwardRejectedV1{
		IdempotencyKey: award.IdempotencyKey,
		TenantID:       award.TenantID,
		PlayerID:       award.PlayerID,
		BadgeID:        award.BadgeID,
		Reason:         reason,
		Source:         award.Source,
		At:             now,
	}); err != nil {
		return err
	}
	*out = AwardOutcome{Award: award}
	return nil
}

// rejectStandalone records a rejection decided before the award tx (player
// checks) in its own tx.
func (s *Service) rejectStandalone(ctx context.Context, cmd AwardCmd, reason string) (AwardOutcome, error) {
	var out AwardOutcome
	err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.recordRejection(ctx, tx, cmd, reason, &out)
	})
	if errors.Is(err, errKeyRace) {
		return s.replayByKey(ctx, cmd)
	}
	if err != nil {
		return AwardOutcome{}, err
	}
	return out, nil
}

func (s *Service) replayByKey(ctx context.Context, cmd AwardCmd) (AwardOutcome, error) {
	a, found, err := s.repo.AwardByKey(ctx, cmd.TenantID, cmd.IdempotencyKey)
	if err != nil {
		return AwardOutcome{}, err
	}
	if !found {
		// The winner rolled back after our conflict: let the ladder retry.
		return AwardOutcome{}, errs.New(errs.Unavailable, "concurrent award with the same key did not settle; retry")
	}
	return s.replay(ctx, a)
}

// replay reports a previously recorded outcome without writing anything.
func (s *Service) replay(ctx context.Context, a domain.Award) (AwardOutcome, error) {
	out := AwardOutcome{Award: a, Replay: true}
	if a.Applied() {
		pb, found, err := s.repo.PlayerBadge(ctx, a.TenantID, a.PlayerID, a.BadgeID)
		if err != nil {
			return AwardOutcome{}, err
		}
		if found {
			out.PlayerBadge = pb
		}
	}
	return out, nil
}
