package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	badgescontracts "levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/missions/internal/domain"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// Progress outcomes.
const (
	OutcomeApplied   = "applied"
	OutcomeDuplicate = "duplicate"
	OutcomeRejected  = "rejected"
)

// maxStartRaces bounds find-or-start retries when a concurrent request
// opened the period's attempt between our lookup and our insert.
const maxStartRaces = 3

// ProgressInput is one progress command, from HTTP or the job.
type ProgressInput struct {
	IdempotencyKey string
	TenantID       string
	PlayerID       string
	MissionID      string
	Increment      int64
	Source         effect.Source
}

// ProgressResult is the outcome. Rejections are results, not errors.
type ProgressResult struct {
	Outcome    string
	Reason     string // effect.Reason* when rejected
	Attempt    domain.Attempt
	HasAttempt bool
	Completed  bool // this command completed the attempt
}

// HandleProgressJob consumes job.missions.progress. Malformed commands are
// errs.Invalid (immediate DLQ); business rejections are recorded, published
// as missions.progress_rejected.v1, and acked.
func (s *Service) HandleProgressJob(ctx context.Context, cmd contracts.ProgressCmdV1) error {
	if cmd.IdempotencyKey == "" || !isUUID(cmd.TenantID) || !isUUID(cmd.PlayerID) || !isUUID(cmd.MissionID) {
		return errs.New(errs.Invalid, "progress command needs idempotency_key and uuid tenant_id, player_id, mission_id")
	}
	if cmd.Increment <= 0 {
		return errs.New(errs.Invalid, "progress command increment must be > 0")
	}
	_, err := s.applyProgress(ctx, ProgressInput{
		IdempotencyKey: cmd.IdempotencyKey,
		TenantID:       cmd.TenantID,
		PlayerID:       cmd.PlayerID,
		MissionID:      cmd.MissionID,
		Increment:      cmd.Increment,
		Source:         cmd.Source,
	})
	return err
}

// ManualProgress is POST /missions/{id}/progress. idempotencyKey is the
// client's Idempotency-Key header ("" → a fresh key, no dedupe); it is
// namespaced "manual:" so it can never collide with rule-derived keys.
// Rejections are recorded and published like the job's, then returned as
// coded errors.
func (s *Service) ManualProgress(ctx context.Context, missionID, playerID string, increment int64, idempotencyKey string) (ProgressResult, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return ProgressResult{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermProgress, nil); err != nil {
		return ProgressResult{}, err
	}
	if increment <= 0 {
		return ProgressResult{}, domain.ErrNonPositiveIncrease
	}
	if _, err := s.repo.MissionByID(ctx, p.TenantID, missionID); err != nil {
		return ProgressResult{}, err
	}
	key := "manual:" + idempotencyKey
	if idempotencyKey == "" {
		key = "manual:" + id.NewID()
	}
	res, err := s.applyProgress(ctx, ProgressInput{
		IdempotencyKey: key,
		TenantID:       p.TenantID,
		PlayerID:       playerID,
		MissionID:      missionID,
		Increment:      increment,
		Source:         effect.Source{Kind: effect.SourceManual, ID: p.UserID},
	})
	if err != nil {
		return ProgressResult{}, err
	}
	switch res.Outcome {
	case OutcomeRejected:
		return res, rejectionError(res.Reason)
	case OutcomeDuplicate:
		return s.replayDuplicate(ctx, p.TenantID, key, res)
	}
	return res, nil
}

// replayDuplicate answers a repeated Idempotency-Key with the recorded
// outcome instead of applying the increment again.
func (s *Service) replayDuplicate(ctx context.Context, tenantID, key string, res ProgressResult) (ProgressResult, error) {
	ev, ok, err := s.repo.ProgressEventByKey(ctx, tenantID, key)
	if err != nil || !ok {
		return res, err
	}
	if ev.Status == domain.ProgressRejected {
		return res, rejectionError(ev.Reason)
	}
	if ev.AttemptID != "" {
		a, err := s.repo.AttemptByID(ctx, tenantID, ev.AttemptID)
		if err != nil {
			return res, err
		}
		res.Attempt, res.HasAttempt = a, true
	}
	return res, nil
}

func rejectionError(reason string) error {
	switch reason {
	case effect.ReasonPlayerNotFound:
		return domain.ErrPlayerNotFound
	case effect.ReasonPlayerInactive:
		return domain.ErrPlayerInactive
	case effect.ReasonTargetNotFound:
		return domain.ErrMissionNotFound
	case effect.ReasonLimitReached:
		return domain.ErrLimitReached
	default:
		return domain.ErrNotAvailable
	}
}

// applyProgress is flow F4. One transaction:
//  1. insert the progress event ON CONFLICT DO NOTHING: a redelivered key
//     inserts nothing and returns before any side effect (duplicates
//     serialize on the unique index, so this holds under concurrency too);
//  2. reject (recorded + missions.progress_rejected.v1) when the player is
//     missing/inactive, the mission is missing/inactive/out of window, or
//     the player hit the completion limit;
//  3. find-or-start the period's open attempt under a row lock;
//  4. progress = min(progress + increment, target); on reaching the target
//     the attempt moves in_progress → completed exactly once and the
//     completion fact plus reward commands ride the same transaction.
func (s *Service) applyProgress(ctx context.Context, in ProgressInput) (ProgressResult, error) {
	// The port call happens before the transaction: never hold a tx open
	// across another module.
	playerReason, err := s.playerReason(ctx, in.TenantID, in.PlayerID)
	if err != nil {
		return ProgressResult{}, err
	}
	now := s.clock.Now()
	ev := domain.ProgressEvent{
		ID:             id.NewID(),
		TenantID:       in.TenantID,
		IdempotencyKey: in.IdempotencyKey,
		MissionID:      in.MissionID,
		PlayerID:       in.PlayerID,
		Increment:      in.Increment,
		Status:         domain.ProgressApplied,
		SourceKind:     in.Source.Kind,
		SourceID:       in.Source.ID,
		CreatedAt:      now,
	}

	var res ProgressResult
	err = s.tx(ctx, func(tx *gorm.DB) error {
		res = ProgressResult{}
		inserted, err := s.repo.InsertProgressEvent(ctx, tx, ev)
		if err != nil {
			return err
		}
		if !inserted {
			res.Outcome = OutcomeDuplicate
			return nil
		}
		reject := func(reason string) error {
			res.Outcome, res.Reason = OutcomeRejected, reason
			ev.Status, ev.Reason = domain.ProgressRejected, reason
			if err := s.repo.FinishProgressEvent(ctx, tx, ev); err != nil {
				return err
			}
			return s.outbox.Publish(ctx, tx, contracts.TopicProgressRejected, contracts.ProgressRejectedV1{
				IdempotencyKey: in.IdempotencyKey,
				TenantID:       in.TenantID,
				PlayerID:       in.PlayerID,
				MissionID:      in.MissionID,
				Reason:         reason,
				Source:         in.Source,
				At:             now,
			})
		}
		if playerReason != "" {
			return reject(playerReason)
		}
		m, err := s.repo.MissionInTx(ctx, tx, in.TenantID, in.MissionID, false)
		if errs.KindOf(err) == errs.NotFound {
			return reject(effect.ReasonTargetNotFound)
		}
		if err != nil {
			return err
		}
		if !m.Available(now) {
			return reject(effect.ReasonTargetInactive)
		}
		a, err := s.findOrStart(ctx, tx, m, in.PlayerID, now)
		if errors.Is(err, domain.ErrLimitReached) {
			return reject(effect.ReasonLimitReached)
		}
		if err != nil {
			return err
		}
		completedNow, err := a.AddProgress(in.Increment, now)
		if err != nil {
			return err
		}
		if err := s.repo.SaveAttempt(ctx, tx, a); err != nil {
			return err
		}
		a.Version++
		updated := attemptV1(a, now)
		updated.IdempotencyKey = in.IdempotencyKey
		if err := s.outbox.Publish(ctx, tx, contracts.TopicProgressUpdated, updated); err != nil {
			return err
		}
		if completedNow {
			if err := s.publishCompletion(ctx, tx, m, a, in.Source.ActivityID, now); err != nil {
				return err
			}
		}
		ev.AttemptID = a.ID
		if err := s.repo.FinishProgressEvent(ctx, tx, ev); err != nil {
			return err
		}
		res = ProgressResult{Outcome: OutcomeApplied, Attempt: a, HasAttempt: true, Completed: completedNow}
		return nil
	})
	if err != nil {
		return ProgressResult{}, err
	}
	return res, nil
}

// findOrStart returns the period's open attempt, locked, or starts one.
// Starting respects max_completions_per_player and once-per-period for
// daily/weekly missions, and publishes missions.started.v1.
func (s *Service) findOrStart(ctx context.Context, tx *gorm.DB, m domain.Mission, playerID string, now time.Time) (domain.Attempt, error) {
	periodKey, _ := domain.PeriodFor(m.Type, now)
	for range maxStartRaces {
		a, found, err := s.repo.OpenAttemptForUpdate(ctx, tx, m.TenantID, m.ID, playerID, periodKey)
		if err != nil {
			return domain.Attempt{}, err
		}
		if found {
			return a, nil
		}
		started, ok, err := s.startLocked(ctx, tx, m, playerID, periodKey, now)
		if err != nil {
			return domain.Attempt{}, err
		}
		if ok {
			return started, nil
		}
		// A concurrent request opened the attempt first: lock that one.
	}
	return domain.Attempt{}, errs.New(errs.Unavailable, "mission attempt contended, retry")
}

// startLocked inserts a fresh attempt; ok=false when another open attempt
// for the period won the race.
func (s *Service) startLocked(ctx context.Context, tx *gorm.DB, m domain.Mission, playerID, periodKey string, now time.Time) (domain.Attempt, bool, error) {
	completed, periodCompleted, err := s.repo.AttemptCounts(ctx, tx, m.TenantID, m.ID, playerID, periodKey)
	if err != nil {
		return domain.Attempt{}, false, err
	}
	if err := m.CanStart(completed, periodCompleted); err != nil {
		return domain.Attempt{}, false, err
	}
	a := domain.StartAttempt(m, playerID, now)
	inserted, err := s.repo.InsertAttempt(ctx, tx, a)
	if err != nil || !inserted {
		return domain.Attempt{}, false, err
	}
	if err := s.outbox.Publish(ctx, tx, contracts.TopicStarted, attemptV1(a, now)); err != nil {
		return domain.Attempt{}, false, err
	}
	return a, true, nil
}

// publishCompletion records the completion fact and the reward commands in
// the transaction that completed the attempt. Keys derive from the attempt
// id, so a replayed completion can never pay twice downstream.
func (s *Service) publishCompletion(ctx context.Context, tx *gorm.DB, m domain.Mission, a domain.Attempt, activityID string, now time.Time) error {
	src := effect.Source{Kind: effect.SourceMission, ID: a.ID, ActivityID: activityID}
	if err := s.outbox.Publish(ctx, tx, contracts.TopicCompleted, contracts.CompletedV1{
		AttemptID:    a.ID,
		TenantID:     a.TenantID,
		PlayerID:     a.PlayerID,
		MissionID:    m.ID,
		MissionSlug:  m.Slug,
		PointsReward: m.PointsReward,
		XPReward:     m.XPReward,
		BadgeID:      m.BadgeRewardID,
		ActivityID:   activityID,
		At:           now,
	}); err != nil {
		return err
	}
	description := "Completed mission: " + m.Name
	if m.PointsReward > 0 {
		if err := s.outbox.Publish(ctx, tx, pointscontracts.Topic(pointscontracts.JobCredit), pointscontracts.CreditCmdV1{
			IdempotencyKey: id.Derive("mission_completion", a.ID, "points"),
			TenantID:       a.TenantID,
			PlayerID:       a.PlayerID,
			Amount:         m.PointsReward,
			Kind:           pointscontracts.KindReward,
			Description:    description,
			Source:         src,
			OccurredAt:     now,
		}); err != nil {
			return err
		}
	}
	if m.XPReward > 0 {
		if err := s.outbox.Publish(ctx, tx, progressioncontracts.Topic(progressioncontracts.JobGrantXP), progressioncontracts.GrantXPCmdV1{
			IdempotencyKey: id.Derive("mission_completion", a.ID, "xp"),
			TenantID:       a.TenantID,
			PlayerID:       a.PlayerID,
			Amount:         m.XPReward,
			Description:    description,
			Source:         src,
			OccurredAt:     now,
		}); err != nil {
			return err
		}
	}
	if m.BadgeRewardID != "" {
		if err := s.outbox.Publish(ctx, tx, badgescontracts.Topic(badgescontracts.JobAward), badgescontracts.AwardCmdV1{
			IdempotencyKey: id.Derive("mission_completion", a.ID, "badge"),
			TenantID:       a.TenantID,
			PlayerID:       a.PlayerID,
			BadgeID:        m.BadgeRewardID,
			Source:         src,
			OccurredAt:     now,
		}); err != nil {
			return err
		}
	}
	return nil
}

// StartMission is POST /missions/{id}/start: opens the current period's
// attempt. 409 mission_already_started when one is open.
func (s *Service) StartMission(ctx context.Context, missionID, playerID string) (domain.Attempt, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Attempt{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermStart, nil); err != nil {
		return domain.Attempt{}, err
	}
	if _, err := s.repo.MissionByID(ctx, p.TenantID, missionID); err != nil {
		return domain.Attempt{}, err
	}
	if _, err := s.requirePlayer(ctx, p.TenantID, playerID, true); err != nil {
		return domain.Attempt{}, err
	}
	now := s.clock.Now()
	var out domain.Attempt
	err = s.tx(ctx, func(tx *gorm.DB) error {
		m, err := s.repo.MissionInTx(ctx, tx, p.TenantID, missionID, false)
		if err != nil {
			return err
		}
		if !m.Available(now) {
			return domain.ErrNotAvailable
		}
		periodKey, _ := domain.PeriodFor(m.Type, now)
		if _, found, err := s.repo.OpenAttemptForUpdate(ctx, tx, m.TenantID, m.ID, playerID, periodKey); err != nil {
			return err
		} else if found {
			return domain.ErrAlreadyStarted
		}
		a, ok, err := s.startLocked(ctx, tx, m, playerID, periodKey, now)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrAlreadyStarted
		}
		out = a
		return nil
	})
	if err != nil {
		return domain.Attempt{}, err
	}
	return out, nil
}

// CompleteMission is POST /missions/{id}/complete. It completes the open
// attempt only when progress reached the target (else 409
// mission_not_completed). Repeating it after completion returns the
// completed attempt without re-publishing or re-rewarding (R61). No attempt
// at all is 404 mission_not_started (Laravel gave a 500, B1).
func (s *Service) CompleteMission(ctx context.Context, missionID, playerID string) (domain.Attempt, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Attempt{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermComplete, nil); err != nil {
		return domain.Attempt{}, err
	}
	if _, err := s.repo.MissionByID(ctx, p.TenantID, missionID); err != nil {
		return domain.Attempt{}, err
	}
	if _, err := s.requirePlayer(ctx, p.TenantID, playerID, true); err != nil {
		return domain.Attempt{}, err
	}
	now := s.clock.Now()
	var out domain.Attempt
	err = s.tx(ctx, func(tx *gorm.DB) error {
		m, err := s.repo.MissionInTx(ctx, tx, p.TenantID, missionID, false)
		if err != nil {
			return err
		}
		periodKey, _ := domain.PeriodFor(m.Type, now)
		a, found, err := s.repo.OpenAttemptForUpdate(ctx, tx, m.TenantID, m.ID, playerID, periodKey)
		if err != nil {
			return err
		}
		if !found {
			latest, ok, err := s.repo.LatestAttempt(ctx, tx, m.TenantID, m.ID, playerID, periodKey)
			if err != nil {
				return err
			}
			if ok && latest.Status == contracts.AttemptCompleted {
				out = latest
				return nil
			}
			return domain.ErrNotStarted
		}
		if err := a.Complete(now); err != nil {
			return err
		}
		if err := s.repo.SaveAttempt(ctx, tx, a); err != nil {
			return err
		}
		a.Version++
		out = a
		return s.publishCompletion(ctx, tx, m, a, "", now)
	})
	if err != nil {
		return domain.Attempt{}, err
	}
	return out, nil
}

// playerReason maps the player check to a rejection reason ("" = ok).
// Transient lookup failures are errors so the job retries.
func (s *Service) playerReason(ctx context.Context, tenantID, playerID string) (string, error) {
	got, err := s.players.PlayersByIDs(ctx, tenantID, []string{playerID})
	if err != nil {
		return "", err
	}
	snap, ok := got[playerID]
	switch {
	case !ok:
		return effect.ReasonPlayerNotFound, nil
	case !snap.Active:
		return effect.ReasonPlayerInactive, nil
	}
	return "", nil
}

func attemptV1(a domain.Attempt, at time.Time) contracts.AttemptV1 {
	return contracts.AttemptV1{
		AttemptID: a.ID,
		TenantID:  a.TenantID,
		PlayerID:  a.PlayerID,
		MissionID: a.MissionID,
		Progress:  a.Progress,
		Target:    a.Target,
		At:        at,
	}
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
