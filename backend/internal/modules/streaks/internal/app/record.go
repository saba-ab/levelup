package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/streaks/contracts"
	"levelup/internal/modules/streaks/internal/domain"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// Record outcomes.
const (
	OutcomeRecorded  = "recorded"  // a new period bucket was inserted
	OutcomeNoop      = "noop"      // the period was already recorded
	OutcomeDuplicate = "duplicate" // this idempotency key was already handled
	OutcomeRejected  = "rejected"  // business rejection (job path only)
)

// RecordResult is what a record command did.
type RecordResult struct {
	Outcome           string
	Reason            string // set when rejected
	PeriodStart       time.Time
	PlayerStreak      domain.PlayerStreak
	MilestonesReached []int
}

// RecordInput is the HTTP variant of a record command.
type RecordInput struct {
	StreakID       string
	PlayerID       string
	OccurredAt     *time.Time
	IdempotencyKey string // from the Idempotency-Key header; optional
}

// Record records activity through HTTP. Unlike the job path, business
// failures are errors (404 / 409) and nothing is written for them.
func (s *Service) Record(ctx context.Context, in RecordInput) (RecordResult, error) {
	p, err := s.guard(ctx, contracts.PermRecord, nil)
	if err != nil {
		return RecordResult{}, err
	}
	st, err := s.repo.StreakByID(ctx, p.TenantID, in.StreakID)
	if err != nil {
		return RecordResult{}, err
	}
	if !st.Active {
		return RecordResult{}, domain.ErrStreakInactive
	}
	pl, err := s.player(ctx, p.TenantID, in.PlayerID)
	if err != nil {
		return RecordResult{}, err
	}
	if !pl.Active {
		return RecordResult{}, domain.ErrPlayerInactive
	}
	key := id.NewID()
	if k := strings.TrimSpace(in.IdempotencyKey); k != "" {
		key = id.Derive("streak_http_record", p.TenantID, k)
	}
	cmd := contracts.RecordCmdV1{
		IdempotencyKey: key,
		TenantID:       p.TenantID,
		PlayerID:       in.PlayerID,
		StreakID:       st.ID,
		Source:         effect.Source{Kind: effect.SourceManual, ID: p.UserID},
	}
	if in.OccurredAt != nil {
		cmd.OccurredAt = *in.OccurredAt
	}
	res, err := s.apply(ctx, cmd, st)
	if err != nil {
		return RecordResult{}, err
	}
	if res.Outcome == OutcomeDuplicate {
		// Replayed key: report the current state.
		rows, err := s.repo.PlayerStreaksByPlayers(ctx, p.TenantID, []string{in.PlayerID})
		if err != nil {
			return RecordResult{}, err
		}
		for _, r := range rows {
			if r.StreakID == st.ID {
				res.PlayerStreak = r
			}
		}
	}
	return res, nil
}

// HandleRecord applies a job.streaks.record command. Malformed commands are
// errs.Invalid (DLQ); business rejections are published as
// streaks.record_rejected.v1 and return nil; a redelivery is a no-op.
func (s *Service) HandleRecord(ctx context.Context, cmd contracts.RecordCmdV1) (RecordResult, error) {
	if cmd.TenantID == "" || cmd.PlayerID == "" || cmd.IdempotencyKey == "" ||
		(cmd.StreakID == "" && cmd.ActivityKey == "") {
		return RecordResult{}, errs.New(errs.Invalid,
			"record command needs tenant_id, player_id, idempotency_key and streak_id or activity_key")
	}
	done, err := s.repo.RequestExists(ctx, cmd.TenantID, cmd.IdempotencyKey)
	if err != nil {
		return RecordResult{}, err
	}
	if done {
		return RecordResult{Outcome: OutcomeDuplicate}, nil
	}

	var st domain.Streak
	if cmd.StreakID != "" {
		st, err = s.repo.StreakByID(ctx, cmd.TenantID, cmd.StreakID)
	} else {
		st, err = s.repo.StreakByActivityKey(ctx, cmd.TenantID, cmd.ActivityKey)
	}
	switch {
	case errors.Is(err, domain.ErrStreakNotFound):
		return s.reject(ctx, cmd, contracts.ReasonStreakNotFound)
	case err != nil:
		return RecordResult{}, err
	case cmd.ActivityKey != "" && st.ActivityKey != cmd.ActivityKey:
		return s.reject(ctx, cmd, contracts.ReasonStreakNotFound)
	case !st.Active:
		return s.reject(ctx, cmd, contracts.ReasonStreakInactive)
	}

	pl, err := s.player(ctx, cmd.TenantID, cmd.PlayerID)
	switch {
	case errors.Is(err, domain.ErrPlayerNotFound):
		return s.reject(ctx, cmd, effect.ReasonPlayerNotFound)
	case err != nil:
		return RecordResult{}, err
	case !pl.Active:
		return s.reject(ctx, cmd, effect.ReasonPlayerInactive)
	}
	return s.apply(ctx, cmd, st)
}

func (s *Service) reject(ctx context.Context, cmd contracts.RecordCmdV1, reason string) (RecordResult, error) {
	now := s.clock.Now()
	err := s.tx(ctx, func(tx *gorm.DB) error {
		inserted, err := s.repo.InsertRequest(ctx, tx, domain.RecordRequest{
			TenantID:       cmd.TenantID,
			IdempotencyKey: cmd.IdempotencyKey,
			PlayerID:       cmd.PlayerID,
			StreakID:       cmd.StreakID,
			ActivityKey:    cmd.ActivityKey,
			Status:         domain.RequestRejected,
			Reason:         reason,
			CreatedAt:      now,
		})
		if err != nil {
			return err
		}
		if !inserted {
			return errDuplicateRequest
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicRecordRejected, contracts.RecordRejectedV1{
			IdempotencyKey: cmd.IdempotencyKey,
			TenantID:       cmd.TenantID,
			PlayerID:       cmd.PlayerID,
			StreakID:       cmd.StreakID,
			ActivityKey:    cmd.ActivityKey,
			Reason:         reason,
			Source:         cmd.Source,
			At:             now,
		})
	})
	if errors.Is(err, errDuplicateRequest) {
		return RecordResult{Outcome: OutcomeDuplicate}, nil
	}
	if err != nil {
		return RecordResult{}, err
	}
	return RecordResult{Outcome: OutcomeRejected, Reason: reason}, nil
}

// apply records one period bucket and derives everything from the buckets.
// Points, events and milestones happen ONLY when a new bucket is inserted.
func (s *Service) apply(ctx context.Context, cmd contracts.RecordCmdV1, st domain.Streak) (RecordResult, error) {
	loc, err := s.location(ctx, cmd.TenantID)
	if err != nil {
		return RecordResult{}, err
	}
	now := s.clock.Now()
	occurred := cmd.OccurredAt
	if occurred.IsZero() || occurred.After(now) {
		occurred = now // never bucket into the future (clock skew)
	}
	periodStart := st.Period.Start(occurred, loc)
	nowPeriod := st.Period.Start(now, loc)

	fresh, err := domain.NewPlayerStreak(cmd.TenantID, cmd.PlayerID, st.ID, now)
	if err != nil {
		return RecordResult{}, err
	}

	res := RecordResult{PeriodStart: periodStart}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		inserted, err := s.repo.InsertRequest(ctx, tx, domain.RecordRequest{
			TenantID:       cmd.TenantID,
			IdempotencyKey: cmd.IdempotencyKey,
			PlayerID:       cmd.PlayerID,
			StreakID:       st.ID,
			ActivityKey:    cmd.ActivityKey,
			Status:         domain.RequestApplied,
			CreatedAt:      now,
		})
		if err != nil {
			return err
		}
		if !inserted {
			return errDuplicateRequest
		}
		if err := s.repo.EnsurePlayerStreak(ctx, tx, fresh); err != nil {
			return err
		}
		ps, err := s.repo.PlayerStreakForUpdate(ctx, tx, cmd.TenantID, cmd.PlayerID, st.ID)
		if err != nil {
			return err
		}
		bucket := domain.PeriodBucket{
			ID:             id.NewID(),
			TenantID:       cmd.TenantID,
			PlayerStreakID: ps.ID,
			PeriodStart:    periodStart,
			IdempotencyKey: cmd.IdempotencyKey,
			RecordedAt:     now,
		}
		isNew, err := s.repo.InsertPeriod(ctx, tx, bucket)
		if err != nil {
			return err
		}
		res.PlayerStreak = ps
		if !isNew {
			res.Outcome = OutcomeNoop
			return nil
		}
		res.Outcome = OutcomeRecorded

		starts, err := s.repo.PeriodStarts(ctx, tx, ps.ID)
		if err != nil {
			return err
		}
		change := ps.Recompute(st, starts, nowPeriod, now)
		if err := s.repo.SavePlayerStreak(ctx, tx, ps); err != nil {
			return err
		}
		ps.Version++
		res.PlayerStreak = ps

		if st.PointsPerPeriod > 0 {
			if err := s.outbox.Publish(ctx, tx, pointscontracts.Topic(pointscontracts.JobCredit), pointscontracts.CreditCmdV1{
				IdempotencyKey: id.Derive("streak_period", ps.ID, domain.DateKey(periodStart)),
				TenantID:       cmd.TenantID,
				PlayerID:       cmd.PlayerID,
				Amount:         st.PointsPerPeriod,
				Kind:           pointscontracts.KindEarn,
				Description:    "Streak period: " + st.Name,
				Source:         effect.Source{Kind: effect.SourceStreak, ID: bucket.ID, ActivityID: cmd.Source.ActivityID},
				OccurredAt:     occurred,
			}); err != nil {
				return err
			}
		}
		if err := s.outbox.Publish(ctx, tx, contracts.TopicActivityRecorded, contracts.ActivityRecordedV1{
			IdempotencyKey: cmd.IdempotencyKey,
			TenantID:       cmd.TenantID,
			PlayerID:       cmd.PlayerID,
			StreakID:       st.ID,
			PeriodStart:    periodStart,
			NewPeriod:      true,
			CurrentCount:   ps.CurrentCount,
			LongestCount:   ps.LongestCount,
			Source:         cmd.Source,
			At:             now,
			PlayerStreakID: ps.ID,
			PointsAwarded:  st.PointsPerPeriod,
		}); err != nil {
			return err
		}

		reached, err := s.payMilestones(ctx, tx, cmd, st, ps, change.Runs, periodStart, now)
		if err != nil {
			return err
		}
		res.MilestonesReached = reached

		if change.Lapsed {
			return s.publishBroken(ctx, tx, ps, st, change.BrokenCount, contracts.BrokenReasonLapsed, now)
		}
		return nil
	})
	if errors.Is(err, errDuplicateRequest) {
		return RecordResult{Outcome: OutcomeDuplicate, PeriodStart: periodStart}, nil
	}
	if err != nil {
		return RecordResult{}, err
	}
	return res, nil
}

// payMilestones pays every milestone the run holding the new bucket has
// reached and that was not yet paid for this run. An award recorded under
// any run start inside the run's span counts: when a late bucket merges two
// runs, milestones already paid in either part are not paid again.
func (s *Service) payMilestones(ctx context.Context, tx *gorm.DB, cmd contracts.RecordCmdV1, st domain.Streak,
	ps domain.PlayerStreak, runs []domain.Run, bucket, now time.Time) ([]int, error) {
	run, ok := domain.RunContaining(runs, bucket)
	if !ok {
		return nil, nil // bucket at or before the reset floor
	}
	candidates := st.MilestonesUpTo(run.Length)
	if len(candidates) == 0 {
		return nil, nil
	}
	paid, err := s.repo.AwardedMilestonesBetween(ctx, tx, ps.ID, run.Start, run.End)
	if err != nil {
		return nil, err
	}
	var reached []int
	for _, m := range candidates {
		if paid[m.Count] {
			continue
		}
		award := domain.MilestoneAward{
			ID:             domain.MilestoneAwardID(ps.ID, m.Count, run.Start),
			TenantID:       cmd.TenantID,
			PlayerStreakID: ps.ID,
			Milestone:      m.Count,
			BonusPoints:    m.BonusPoints,
			RunStartedAt:   run.Start,
			AwardedAt:      now,
		}
		inserted, err := s.repo.InsertMilestoneAward(ctx, tx, award)
		if err != nil {
			return nil, err
		}
		if !inserted {
			continue
		}
		reached = append(reached, m.Count)
		if err := s.outbox.Publish(ctx, tx, contracts.TopicMilestoneReached, contracts.MilestoneReachedV1{
			TenantID:       cmd.TenantID,
			PlayerID:       cmd.PlayerID,
			StreakID:       st.ID,
			Milestone:      m.Count,
			BonusPoints:    m.BonusPoints,
			At:             now,
			PlayerStreakID: ps.ID,
			AwardID:        award.ID,
			RunStartedAt:   run.Start,
		}); err != nil {
			return nil, err
		}
		if m.BonusPoints > 0 {
			if err := s.outbox.Publish(ctx, tx, pointscontracts.Topic(pointscontracts.JobCredit), pointscontracts.CreditCmdV1{
				IdempotencyKey: id.Derive("streak_milestone", award.ID),
				TenantID:       cmd.TenantID,
				PlayerID:       cmd.PlayerID,
				Amount:         m.BonusPoints,
				Kind:           pointscontracts.KindBonus,
				Description:    "Streak milestone reached: " + st.Name,
				Source:         effect.Source{Kind: effect.SourceStreak, ID: award.ID, ActivityID: cmd.Source.ActivityID},
				OccurredAt:     now,
			}); err != nil {
				return nil, err
			}
		}
	}
	return reached, nil
}

func (s *Service) publishBroken(ctx context.Context, tx *gorm.DB, ps domain.PlayerStreak, st domain.Streak,
	finalCount int, reason string, now time.Time) error {
	ev := contracts.BrokenV1{
		TenantID:       ps.TenantID,
		PlayerID:       ps.PlayerID,
		StreakID:       st.ID,
		FinalCount:     finalCount,
		At:             now,
		PlayerStreakID: ps.ID,
		Reason:         reason,
	}
	if ps.LastPeriodStart != nil {
		ev.LastPeriod = *ps.LastPeriodStart
	}
	return s.outbox.Publish(ctx, tx, contracts.TopicBroken, ev)
}

// Reset is the admin action: the player's run restarts from zero.
func (s *Service) Reset(ctx context.Context, streakID, playerID string) (domain.PlayerStreak, error) {
	p, err := s.guard(ctx, contracts.PermReset, nil)
	if err != nil {
		return domain.PlayerStreak{}, err
	}
	st, err := s.repo.StreakByID(ctx, p.TenantID, streakID)
	if err != nil {
		return domain.PlayerStreak{}, err
	}
	if _, err := s.player(ctx, p.TenantID, playerID); err != nil {
		return domain.PlayerStreak{}, err
	}
	now := s.clock.Now()
	var out domain.PlayerStreak
	err = s.tx(ctx, func(tx *gorm.DB) error {
		ps, err := s.repo.PlayerStreakForUpdate(ctx, tx, p.TenantID, playerID, st.ID)
		if err != nil {
			return err
		}
		prev := ps.Reset(now)
		if err := s.repo.SavePlayerStreak(ctx, tx, ps); err != nil {
			return err
		}
		ps.Version++
		out = ps
		if prev == 0 {
			return nil // nothing was running: no broken event (fixes S6)
		}
		return s.publishBroken(ctx, tx, ps, st, prev, contracts.BrokenReasonReset, now)
	})
	if err != nil {
		return domain.PlayerStreak{}, err
	}
	return out, nil
}
