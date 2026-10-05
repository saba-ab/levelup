package app

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	badgescontracts "levelup/internal/modules/badges/contracts"
	missionscontracts "levelup/internal/modules/missions/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	rewardscontracts "levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rules/internal/domain"
	streakscontracts "levelup/internal/modules/streaks/contracts"
	"levelup/internal/shared/errs"
)

// OutcomeTopics maps every outcome fact rules settles effects from to the
// status it settles to.
var OutcomeTopics = map[string]string{
	pointscontracts.TopicCredited:           domain.EffectApplied,
	pointscontracts.TopicCreditRejected:     domain.EffectRejected,
	progressioncontracts.TopicXPGained:      domain.EffectApplied,
	progressioncontracts.TopicGrantRejected: domain.EffectRejected,
	badgescontracts.TopicAwarded:            domain.EffectApplied,
	badgescontracts.TopicAwardRejected:      domain.EffectRejected,
	streakscontracts.TopicActivityRecorded:  domain.EffectApplied,
	streakscontracts.TopicRecordRejected:    domain.EffectRejected,
	missionscontracts.TopicProgressUpdated:  domain.EffectApplied,
	missionscontracts.TopicCompleted:        domain.EffectApplied,
	missionscontracts.TopicProgressRejected: domain.EffectRejected,
	rewardscontracts.TopicClaimed:           domain.EffectApplied,
	rewardscontracts.TopicClaimRejected:     domain.EffectRejected,
}

// outcomeFact is the common subset of every outcome payload. Payloads
// without an idempotency_key (missions.completed.v1, and missions.started.v1
// or rewards claims that no rule caused) are ignored: the effect settles on
// the fact that does carry its key (missions.progress_updated.v1).
type outcomeFact struct {
	IdempotencyKey string `json:"idempotency_key"`
	TenantID       string `json:"tenant_id"`
	Reason         string `json:"reason"`
}

// Settle marks the effect keyed by the fact's idempotency key applied or
// rejected (G16: a badge already earned is a settled rejection, not a
// failure). Idempotent: only a requested effect moves, so redelivery and
// reordering change nothing; unknown keys (effects not issued by rules,
// e.g. mission bonus credits) are ignored.
func (s *Service) Settle(ctx context.Context, topic string, payload []byte) error {
	status, ok := OutcomeTopics[topic]
	if !ok {
		return errs.New(errs.Invalid, "rules does not settle "+topic)
	}
	var f outcomeFact
	if err := json.Unmarshal(payload, &f); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable "+topic, err)
	}
	if f.IdempotencyKey == "" || f.TenantID == "" {
		return nil
	}
	reason := ""
	if status == domain.EffectRejected {
		reason = f.Reason
		if reason == "" {
			reason = "rejected"
		}
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		_, err := s.repo.SettleEffect(ctx, tx, f.TenantID, f.IdempotencyKey, status, reason, s.clock.Now())
		return err
	})
}

// ReconcileEffects re-publishes the exact command of every effect still
// requested after PendingAfter (R47: state-based, so a skipped tick leaves
// no gap). Targets dedupe on the idempotency key, so a re-publish is safe.
func (s *Service) ReconcileEffects(ctx context.Context) error {
	now := s.clock.Now()
	for range 20 { // bounded batches per run
		pending, err := s.repo.PendingEffects(ctx, now.Add(-s.cfg.PendingAfter), s.cfg.MaxAttempts, 500)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
		err = s.tx(ctx, func(tx *gorm.DB) error {
			ids := make([]string, len(pending))
			for i, e := range pending {
				ids[i] = e.ID
				if err := s.outbox.Publish(ctx, tx, e.Target, json.RawMessage(e.Command)); err != nil {
					return err
				}
			}
			return s.repo.MarkEffectsRetried(ctx, tx, ids, now)
		})
		if err != nil {
			return err
		}
		if len(pending) < 500 {
			return nil
		}
	}
	return nil
}

// PurgeTenant deletes every rules row of a deleted tenant (R65). Idempotent.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string, _ time.Time) error {
	if tenantID == "" {
		return errs.New(errs.Invalid, "tenant.deleted.v1: tenant_id is required")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, tenantID)
	})
}
