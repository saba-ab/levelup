package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	badgescontracts "levelup/internal/modules/badges/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/shared/effect"
)

// Fulfilment: rewards decides what a claim delivers, so rewards issues the
// delivery commands, in the transaction that records the fact justifying
// them (the redeem, or a rewards.grant claim):
//
//	points   → job.points.credit        amount = reward value (whole number), kind "reward"
//	badge    → job.badges.award         badge_reward_id
//	level    → job.progression.grant_xp amount = reward value: a level reward grants that much XP
//	discount, item, custom → no command: the voucher code is the fulfilment
//
// Every command carries contracts.FulfilmentKey(claimID, part), so a
// redelivered grant or a retried transaction never delivers twice; the
// claim's fulfilled_at makes a second fulfilment of the same claim a no-op.

// outboxMsg is one publish, staged so the claim row is written first.
type outboxMsg struct {
	topic   string
	payload any
}

// fulfil stages the fulfilment commands of c and stamps fulfilled_at. It
// writes nothing: the caller persists c and then publishes the returned
// messages in the same tx. r may be nil (purged reward): nothing can be
// delivered then. A command reward without a usable target (no value, no
// badge id) stays unfulfilled.
func (s *Service) fulfil(c *domain.Claim, r *domain.Reward, src *effect.Source, now time.Time) []outboxMsg {
	if c.Fulfilled() || r == nil {
		return nil
	}
	source := effect.Source{Kind: effect.SourceReward, ID: c.ID}
	if src != nil {
		source.ActivityID = src.ActivityID
	}
	var msgs []outboxMsg
	switch r.Type {
	case contracts.TypePoints:
		amount, ok := r.WholeValue()
		if !ok {
			return nil
		}
		msgs = append(msgs, outboxMsg{
			topic: pointscontracts.Topic(pointscontracts.JobCredit),
			payload: pointscontracts.CreditCmdV1{
				IdempotencyKey: contracts.FulfilmentKey(c.ID, contracts.FulfilPoints),
				TenantID:       c.TenantID,
				PlayerID:       c.PlayerID,
				Amount:         amount,
				Kind:           pointscontracts.KindReward,
				Description:    "Reward: " + r.Name,
				Source:         source,
				OccurredAt:     now,
			},
		})
	case contracts.TypeBadge:
		if r.BadgeRewardID == nil {
			return nil
		}
		msgs = append(msgs, outboxMsg{
			topic: badgescontracts.Topic(badgescontracts.JobAward),
			payload: badgescontracts.AwardCmdV1{
				IdempotencyKey: contracts.FulfilmentKey(c.ID, contracts.FulfilBadge),
				TenantID:       c.TenantID,
				PlayerID:       c.PlayerID,
				BadgeID:        *r.BadgeRewardID,
				Source:         source,
				OccurredAt:     now,
			},
		})
	case contracts.TypeLevel:
		amount, ok := r.WholeValue()
		if !ok {
			return nil
		}
		msgs = append(msgs, outboxMsg{
			topic: progressioncontracts.Topic(progressioncontracts.JobGrantXP),
			payload: progressioncontracts.GrantXPCmdV1{
				IdempotencyKey: contracts.FulfilmentKey(c.ID, contracts.FulfilLevel),
				TenantID:       c.TenantID,
				PlayerID:       c.PlayerID,
				Amount:         amount,
				Description:    "Reward: " + r.Name,
				Source:         source,
				OccurredAt:     now,
			},
		})
	}
	// Voucher types are fulfilled by the redeem itself; command types by
	// the command just staged.
	c.MarkFulfilled(now)
	return msgs
}

func (s *Service) publishAll(ctx context.Context, tx *gorm.DB, msgs []outboxMsg) error {
	for _, m := range msgs {
		if err := s.outbox.Publish(ctx, tx, m.topic, m.payload); err != nil {
			return err
		}
	}
	return nil
}
