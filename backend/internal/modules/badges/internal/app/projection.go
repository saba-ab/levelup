package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/modules/badges/internal/ports"
	missionscontracts "levelup/internal/modules/missions/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	streakscontracts "levelup/internal/modules/streaks/contracts"
	"levelup/internal/platform/bus"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
)

// The requirements engine. Badges keeps its own per-player projection
// (badge_player_stats + badge_player_activity_counts) fed by other modules'
// facts, then evaluates the tenant's active badges whose requirements read
// the metric that changed, and awards the ones now met through the normal
// award path under contracts.AutoAwardKey(player, badge): exactly once per
// (player, badge), whatever the redeliveries or concurrent evaluations.
//
// Idempotency of the projection (ADR-0012, redelivered and reordered):
//   - lifetime_points is absolute (points' lifetime_earned): the newest
//     ledger time wins, so an old event arriving late changes nothing;
//   - streak_days and level only ever rise (GREATEST);
//   - missions_completed, badges_earned and activity counts are increments,
//     guarded by a dedupe key in applied_events written in the same tx.
//
// Evaluation runs after the projection commits and also when the fact was
// already applied, so a handler retried after a failed award finishes the
// job on redelivery.

// StatsUpdate is one change to a player's projection. Zero values change
// nothing.
type StatsUpdate struct {
	// LifetimePoints is applied only when LifetimeAt is newer than the
	// stored one (absolute value, newest wins).
	LifetimePoints *int64
	LifetimeAt     time.Time
	// MissionsCompleted and BadgesEarned are increments.
	MissionsCompleted int64
	BadgesEarned      int64
	// MaxStreak and Level raise the stored value, never lower it.
	MaxStreak int64
	Level     int64
	// ActivityType, when set, adds one to that type's activity count.
	ActivityType string
	At           time.Time
}

// Dedupe key prefixes in applied_events.
const (
	keyMissionCompleted = "missions.completed:"
	keyBadgeAwarded     = "badges.awarded:"
	keyActivityReceived = "activity.received:"
)

// OnPointsCredited handles points.credited.v1: lifetime_points.
func (s *Service) OnPointsCredited(ctx context.Context, e bus.Envelope) error {
	var ev pointscontracts.LedgerMovedV1
	if err := decodeEvent(e, &ev); err != nil {
		return err
	}
	if err := requireIDs(e.Topic, ev.TenantID, ev.PlayerID); err != nil {
		return err
	}
	at := firstTime(ev.At, ev.OccurredAt, e.OccurredAt)
	lifetime := ev.LifetimeEarned
	return s.project(ctx, ev.TenantID, ev.PlayerID, "", StatsUpdate{LifetimePoints: &lifetime, LifetimeAt: at},
		domain.MetricLifetimePoints, "", ev.Source.ActivityID)
}

// OnMissionCompleted handles missions.completed.v1: missions_completed + 1
// per attempt.
func (s *Service) OnMissionCompleted(ctx context.Context, e bus.Envelope) error {
	var ev missionscontracts.CompletedV1
	if err := decodeEvent(e, &ev); err != nil {
		return err
	}
	if err := requireIDs(e.Topic, ev.TenantID, ev.PlayerID); err != nil {
		return err
	}
	key, err := dedupeKey(e.Topic, keyMissionCompleted, ev.AttemptID, e.EventID)
	if err != nil {
		return err
	}
	return s.project(ctx, ev.TenantID, ev.PlayerID, key,
		StatsUpdate{MissionsCompleted: 1}, domain.MetricMissionsCompleted, "", ev.ActivityID)
}

// OnStreakActivity handles streaks.activity_recorded.v1: streak_days is the
// highest current_count seen on any of the player's streaks.
func (s *Service) OnStreakActivity(ctx context.Context, e bus.Envelope) error {
	var ev streakscontracts.ActivityRecordedV1
	if err := decodeEvent(e, &ev); err != nil {
		return err
	}
	if err := requireIDs(e.Topic, ev.TenantID, ev.PlayerID); err != nil {
		return err
	}
	return s.project(ctx, ev.TenantID, ev.PlayerID, "", StatsUpdate{MaxStreak: int64(ev.CurrentCount)},
		domain.MetricStreakDays, "", ev.Source.ActivityID)
}

// OnLevelReached handles progression.level_reached.v1: the highest level.
func (s *Service) OnLevelReached(ctx context.Context, e bus.Envelope) error {
	var ev progressioncontracts.LevelReachedV1
	if err := decodeEvent(e, &ev); err != nil {
		return err
	}
	if err := requireIDs(e.Topic, ev.TenantID, ev.PlayerID); err != nil {
		return err
	}
	return s.project(ctx, ev.TenantID, ev.PlayerID, "", StatsUpdate{Level: int64(ev.LevelNumber)},
		domain.MetricLevel, "", "")
}

// OnBadgeAwarded handles badges' own badges.awarded.v1: badges_earned + 1
// per applied award (stacks count; revocations do not take it back).
func (s *Service) OnBadgeAwarded(ctx context.Context, e bus.Envelope) error {
	var ev contracts.AwardedV1
	if err := decodeEvent(e, &ev); err != nil {
		return err
	}
	if err := requireIDs(e.Topic, ev.TenantID, ev.PlayerID); err != nil {
		return err
	}
	key, err := dedupeKey(e.Topic, keyBadgeAwarded, ev.AwardID, e.EventID)
	if err != nil {
		return err
	}
	return s.project(ctx, ev.TenantID, ev.PlayerID, key,
		StatsUpdate{BadgesEarned: 1}, domain.MetricBadgesEarned, "", ev.Source.ActivityID)
}

// OnActivityReceived handles activity.received.v1: one more activity of its
// event type. An activity whose player was not resolved at ingest is skipped,
// unless it is flagged auto_create_player: then it waits (retries) for the
// player module to create the player, so first activities still count.
func (s *Service) OnActivityReceived(ctx context.Context, e bus.Envelope) error {
	var ev activitycontracts.ReceivedV1
	if err := decodeEvent(e, &ev); err != nil {
		return err
	}
	if ev.EventType == "" {
		return nil
	}
	if ev.PlayerID == "" {
		id, err := resolveAutoCreated(ctx, s.players, ev)
		if err != nil || id == "" {
			return err
		}
		ev.PlayerID = id
	}
	if err := requireIDs(e.Topic, ev.TenantID, ev.PlayerID); err != nil {
		return err
	}
	key, err := dedupeKey(e.Topic, keyActivityReceived, ev.ActivityID, ev.EventID, e.EventID)
	if err != nil {
		return err
	}
	return s.project(ctx, ev.TenantID, ev.PlayerID, key,
		StatsUpdate{ActivityType: ev.EventType}, domain.MetricActivityCount, ev.EventType, ev.ActivityID)
}

// project applies u once (dedupeKey guards increments) and then evaluates
// the badges reading metric.
func (s *Service) project(ctx context.Context, tenantID, playerID, dedupeKey string, u StatsUpdate,
	metric, eventType, activityID string) error {
	now := s.now()
	u.At = now
	if u.LifetimePoints != nil && u.LifetimeAt.IsZero() {
		u.LifetimeAt = now
	}
	err := s.tx(ctx, func(tx *gorm.DB) error {
		if dedupeKey != "" {
			fresh, err := s.repo.MarkEventApplied(ctx, tx, tenantID, dedupeKey, now)
			if err != nil {
				return err
			}
			if !fresh {
				return nil // redelivery: counted already
			}
		}
		return s.repo.ApplyPlayerStats(ctx, tx, tenantID, playerID, u)
	})
	if err != nil {
		return err
	}
	return s.evaluate(ctx, tenantID, playerID, metric, eventType, activityID)
}

// evaluate awards every active badge whose requirements read metric, are
// met by the player's stats, and may still be awarded (not held for a
// non-stackable badge, below max_awards for a stackable one). Awards for
// one badge failing do not stop the others; the joined error makes the
// subscription retry, and the auto key makes the retry exactly-once.
func (s *Service) evaluate(ctx context.Context, tenantID, playerID, metric, eventType, activityID string) error {
	badges, err := s.repo.AutoAwardBadges(ctx, tenantID)
	if err != nil {
		return err
	}
	type candidate struct {
		badge domain.Badge
		req   domain.Requirements
	}
	var candidates []candidate
	for _, b := range badges {
		if req, ok := b.AutoRequirements(); ok && req.Uses(metric, eventType) {
			candidates = append(candidates, candidate{b, req})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	stats, err := s.repo.PlayerStats(ctx, tenantID, playerID)
	if err != nil {
		return err
	}
	holdings, err := s.repo.PlayerBadgesByPlayerIDs(ctx, tenantID, []string{playerID})
	if err != nil {
		return err
	}
	earned := make(map[string]int, len(holdings))
	for _, pb := range holdings {
		earned[pb.BadgeID] = pb.EarnedCount
	}

	var (
		snap   *ports.PlayerSnapshot
		failed []error
	)
	for _, c := range candidates {
		if !c.req.Satisfied(stats) || c.badge.CanAward(earned[c.badge.ID]) != "" {
			continue
		}
		if snap == nil {
			got, found, err := s.players.ByID(ctx, tenantID, playerID)
			if err != nil {
				return err
			}
			// An unknown or inactive player is skipped, not rejected: a
			// rejection under the auto key would block the badge forever.
			if !found || got.TenantID != tenantID || !got.Active {
				return nil
			}
			snap = &got
		}
		if _, err := s.apply(ctx, AwardCmd{
			TenantID:       tenantID,
			PlayerID:       playerID,
			BadgeID:        c.badge.ID,
			IdempotencyKey: contracts.AutoAwardKey(playerID, c.badge.ID),
			Source:         effect.Source{Kind: contracts.SourceRequirements, ID: c.badge.ID, ActivityID: activityID},
			OccurredAt:     s.now(),
		}, snap); err != nil {
			failed = append(failed, err)
		}
	}
	return errors.Join(failed...)
}

// PruneAppliedEvents is the badges.prune_applied_events cron: dedupe keys
// older than retention are deleted in batches. Reconciling by age, so a
// skipped run leaves nothing behind.
func (s *Service) PruneAppliedEvents(ctx context.Context, retention time.Duration) error {
	if retention <= 0 {
		return nil
	}
	before := s.now().Add(-retention)
	for range maxPruneRounds {
		n, err := s.repo.PruneAppliedEvents(ctx, before, pruneBatch)
		if err != nil {
			return err
		}
		if n < pruneBatch {
			return nil
		}
	}
	return nil
}

const (
	pruneBatch     = 5000
	maxPruneRounds = 200
)

func decodeEvent(e bus.Envelope, dst any) error {
	if err := json.Unmarshal(e.Payload, dst); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable "+e.Topic, err)
	}
	return nil
}

func requireIDs(topic, tenantID, playerID string) error {
	if _, err := uuid.Parse(tenantID); err != nil {
		return errs.Wrap(errs.Invalid, topic+" without a valid tenant_id", err)
	}
	if _, err := uuid.Parse(playerID); err != nil {
		return errs.Wrap(errs.Invalid, topic+" without a valid player_id", err)
	}
	return nil
}

func firstTime(ts ...time.Time) time.Time {
	for _, t := range ts {
		if !t.IsZero() {
			return t.UTC()
		}
	}
	return time.Time{}
}

// dedupeKey is prefix + the first non-empty natural id (the fact's own id,
// then the envelope's event id). A fact without any id cannot be counted
// exactly once, so it is refused.
func dedupeKey(topic, prefix string, ids ...string) (string, error) {
	for _, v := range ids {
		if v != "" {
			return prefix + v, nil
		}
	}
	return "", errs.New(errs.Invalid, topic+" without an id to deduplicate on")
}

// resolveAutoCreated finds the player of an activity that ingest could not
// resolve. Unflagged activities resolve to "" (skip); flagged ones retry with
// errs.Unavailable until the player module has created the player.
func resolveAutoCreated(ctx context.Context, players ports.PlayerReader, ev activitycontracts.ReceivedV1) (string, error) {
	r, ok := players.(ports.ExternalIDResolver)
	if !ev.AutoCreatePlayer || ev.PlayerExternalID == "" || !ok {
		return "", nil
	}
	id, found, err := r.IDByExternalID(ctx, ev.TenantID, ev.PlayerExternalID)
	if err != nil {
		return "", err
	}
	if !found {
		return "", errs.New(errs.Unavailable, "player not created yet for auto-create activity "+ev.ActivityID+"; retrying")
	}
	return id, nil
}
