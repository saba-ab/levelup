package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	badgescontracts "levelup/internal/modules/badges/contracts"
	missionscontracts "levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/domain"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	rewardscontracts "levelup/internal/modules/rewards/contracts"
	streakscontracts "levelup/internal/modules/streaks/contracts"
	"levelup/internal/platform/bus"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// Fact is a decoded trigger event: who it is about and the template data.
type Fact struct {
	TenantID string
	PlayerID string
	Data     domain.Data
}

// Decoder turns one topic's payload into a Fact.
type Decoder func(payload json.RawMessage) (Fact, error)

// TriggerTopic binds a trigger to the versioned topic that carries it.
type TriggerTopic struct {
	Trigger string
	Topic   string
	Decode  Decoder
}

func decodeInto[T any](payload json.RawMessage, topic string) (T, error) {
	var v T
	if err := json.Unmarshal(payload, &v); err != nil {
		return v, errs.Wrap(errs.Invalid, "undecodable "+topic, err)
	}
	return v, nil
}

// TriggerTopics is every subscription that can fan out notifications.
func TriggerTopics() []TriggerTopic {
	return []TriggerTopic{
		{contracts.TriggerBadgeAwarded, badgescontracts.TopicAwarded, func(raw json.RawMessage) (Fact, error) {
			ev, err := decodeInto[badgescontracts.AwardedV1](raw, badgescontracts.TopicAwarded)
			return Fact{TenantID: ev.TenantID, PlayerID: ev.PlayerID, Data: domain.Data{Badge: domain.BadgeData{
				ID: ev.BadgeID, Slug: ev.BadgeSlug, Name: ev.BadgeSlug, Tier: ev.Tier, Category: ev.Category,
				EarnedCount: ev.EarnedCount, PointsValue: ev.PointsValue,
			}}}, err
		}},
		{contracts.TriggerLevelReached, progressioncontracts.TopicLevelReached, func(raw json.RawMessage) (Fact, error) {
			ev, err := decodeInto[progressioncontracts.LevelReachedV1](raw, progressioncontracts.TopicLevelReached)
			return Fact{TenantID: ev.TenantID, PlayerID: ev.PlayerID, Data: domain.Data{Level: domain.LevelData{
				ID: ev.LevelID, Number: ev.LevelNumber, Name: ev.LevelName, TotalXP: ev.TotalXP, PointsReward: ev.PointsReward,
			}}}, err
		}},
		{contracts.TriggerMissionCompleted, missionscontracts.TopicCompleted, func(raw json.RawMessage) (Fact, error) {
			ev, err := decodeInto[missionscontracts.CompletedV1](raw, missionscontracts.TopicCompleted)
			return Fact{TenantID: ev.TenantID, PlayerID: ev.PlayerID, Data: domain.Data{Mission: domain.MissionData{
				ID: ev.MissionID, Slug: ev.MissionSlug, Name: ev.MissionSlug, PointsReward: ev.PointsReward, XPReward: ev.XPReward,
			}}}, err
		}},
		{contracts.TriggerStreakMilestone, streakscontracts.TopicMilestoneReached, func(raw json.RawMessage) (Fact, error) {
			ev, err := decodeInto[streakscontracts.MilestoneReachedV1](raw, streakscontracts.TopicMilestoneReached)
			return Fact{TenantID: ev.TenantID, PlayerID: ev.PlayerID, Data: domain.Data{Streak: domain.StreakData{
				ID: ev.StreakID, Milestone: ev.Milestone, BonusPoints: ev.BonusPoints,
			}}}, err
		}},
		{contracts.TriggerStreakBroken, streakscontracts.TopicBroken, func(raw json.RawMessage) (Fact, error) {
			ev, err := decodeInto[streakscontracts.BrokenV1](raw, streakscontracts.TopicBroken)
			return Fact{TenantID: ev.TenantID, PlayerID: ev.PlayerID, Data: domain.Data{Streak: domain.StreakData{
				ID: ev.StreakID, FinalCount: ev.FinalCount,
			}}}, err
		}},
		{contracts.TriggerRewardClaimed, rewardscontracts.TopicClaimed, func(raw json.RawMessage) (Fact, error) {
			ev, err := decodeInto[rewardscontracts.ClaimV1](raw, rewardscontracts.TopicClaimed)
			return Fact{TenantID: ev.TenantID, PlayerID: ev.PlayerID, Data: domain.Data{Reward: domain.RewardData{
				ID: ev.RewardID, Slug: ev.RewardSlug, Name: ev.RewardSlug, Type: ev.RewardType, Code: ev.Code,
				Value: ev.Value, PointsCost: ev.PointsCost,
			}}}, err
		}},
		{contracts.TriggerPointsCredited, pointscontracts.TopicCredited, func(raw json.RawMessage) (Fact, error) {
			ev, err := decodeInto[pointscontracts.LedgerMovedV1](raw, pointscontracts.TopicCredited)
			return Fact{TenantID: ev.TenantID, PlayerID: ev.PlayerID, Data: domain.Data{Points: domain.PointsData{
				Amount: ev.Amount, Balance: ev.BalanceAfter, LifetimeEarned: ev.LifetimeEarned, Kind: ev.Kind,
			}}}, err
		}},
	}
}

// Handler returns the bus handler of one trigger topic.
func (s *Service) Handler(tt TriggerTopic) bus.Handler {
	return func(ctx context.Context, e bus.Envelope) error {
		fact, err := tt.Decode(e.Payload)
		if err != nil {
			return err
		}
		return s.FanOut(ctx, tt.Trigger, e.EventID, fact)
	}
}

// FanOut creates one notification per (active template of the trigger,
// channel). Idempotent under redelivery: rows are keyed by (template_id,
// event_id, channel) and the email job is published only for a row this
// delivery inserted. Reordering is harmless: every fact stands alone.
func (s *Service) FanOut(ctx context.Context, trigger, eventID string, fact Fact) error {
	fields := map[string]string{}
	if _, err := uuid.Parse(fact.TenantID); err != nil {
		fields["tenant_id"] = "must be a valid UUID"
	}
	if _, err := uuid.Parse(fact.PlayerID); err != nil {
		fields["player_id"] = "must be a valid UUID"
	}
	if strings.TrimSpace(eventID) == "" {
		fields["event_id"] = "is required"
	}
	if len(fields) > 0 {
		return errs.WithFields(errs.New(errs.Invalid, trigger+" fact is malformed"), fields)
	}

	templates, err := s.repo.ActiveTemplatesByTrigger(ctx, fact.TenantID, trigger)
	if err != nil || len(templates) == 0 {
		return err
	}
	player, found, err := s.players.ByID(ctx, fact.TenantID, fact.PlayerID)
	if err != nil {
		return err
	}
	if !found || player.TenantID != fact.TenantID || !player.Active {
		s.log.Debug("notification fan-out skipped: player unknown or inactive",
			zap.String("tenant_id", fact.TenantID), zap.String("player_id", fact.PlayerID), zap.String("trigger", trigger))
		return nil
	}
	settings, err := s.channelSettings(ctx, fact.TenantID)
	if err != nil {
		return err
	}

	data := fact.Data
	data.Trigger = trigger
	data.Player = domain.PlayerData{ID: player.ID, ExternalID: player.ExternalID, DisplayName: player.DisplayName}
	now := s.now()

	var rows []domain.Notification
	for _, t := range templates {
		rendered, renderErr := domain.Render(t, data, s.opts.RenderTimeout)
		for _, ch := range t.Channels {
			n := domain.Notification{
				ID: id.NewID(), TenantID: fact.TenantID, TemplateID: t.ID, PlayerID: fact.PlayerID, EventID: eventID,
				Trigger: trigger, Channel: ch, Title: rendered.Title, Body: rendered.Body,
				CreatedAt: now, UpdatedAt: now,
			}
			switch {
			case renderErr != nil:
				n.Fail(contracts.ReasonRenderError, renderErr.Error(), now)
			case ch == contracts.ChannelInApp:
				n.Deliver(now)
			case !settings.EmailEnabled:
				n.Skip(contracts.ReasonEmailDisabled, now)
			case strings.TrimSpace(player.Email) == "":
				n.Skip(contracts.ReasonNoEmail, now)
			default:
				n.Status = contracts.StatusPending
			}
			rows = append(rows, n)
		}
	}

	return s.tx(ctx, func(tx *gorm.DB) error {
		for _, n := range rows {
			inserted, err := s.repo.InsertNotification(ctx, tx, n)
			if err != nil {
				return err
			}
			if !inserted || !n.Pending() {
				continue
			}
			if err := s.outbox.Publish(ctx, tx, contracts.Topic(contracts.JobSendEmail), contracts.SendEmailCmdV1{
				TenantID: n.TenantID, NotificationID: n.ID,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}
