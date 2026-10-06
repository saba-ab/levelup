package app

import (
	"context"
	"strings"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/missions/internal/domain"
	"levelup/internal/modules/missions/internal/ports"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// DefaultMaxCausationDepth matches rules' default: internal activities
// derived deeper than this never progress missions (loop guard).
const DefaultMaxCausationDepth = 3

// AutoKey is the idempotency key of the progress one activity makes on one
// mission for one player: a redelivered activity derives the same key and
// the progress_events ledger turns it into a no-op.
func AutoKey(activityID, missionID, playerID string) string {
	return id.Derive("mission_auto", activityID, missionID, playerID)
}

// AutoResult reports what one activity did.
type AutoResult struct {
	Matched  int // missions whose criteria matched
	Applied  int // progress applied
	Complete int // attempts this activity completed
}

// HandleActivity is the activity.received.v1 subscriber. Every active,
// in-window mission whose criteria (event_type + where) match the activity
// progresses the player's attempt — started when needed — by the criteria's
// increment, through the same path as job.missions.progress (limits,
// periods, completion and rewards). Unknown or inactive players and
// business rejections (limit reached, mission closed) are silent no-ops:
// rules owns rejections of activities. Malformed payloads are errs.Invalid;
// transient failures are returned so the delivery is retried, and the
// missions already applied are then duplicates.
func (s *Service) HandleActivity(ctx context.Context, ev activitycontracts.ReceivedV1) (AutoResult, error) {
	activityID := strings.TrimSpace(ev.ActivityID)
	eventType := strings.TrimSpace(ev.EventType)
	if !isUUID(ev.TenantID) || activityID == "" || eventType == "" {
		return AutoResult{}, errs.New(errs.Invalid, "activity.received.v1 needs uuid tenant_id, activity_id and event_type")
	}
	if ev.PlayerID == "" && strings.TrimSpace(ev.PlayerExternalID) == "" {
		return AutoResult{}, errs.New(errs.Invalid, "activity.received.v1 needs player_id or player_external_id")
	}
	if ev.CausationDepth > s.maxDepth {
		return AutoResult{}, nil
	}

	candidates, err := s.repo.AutoMissions(ctx, ev.TenantID, eventType)
	if err != nil {
		return AutoResult{}, err
	}
	now := s.clock.Now()
	type match struct {
		mission   domain.Mission
		increment int64
	}
	var matches []match
	for _, m := range candidates {
		if !m.Available(now) || selfTriggered(m, eventType, ev.Properties) {
			continue
		}
		c, err := domain.ParseCriteria(m.Criteria)
		if err != nil || !c.Matches(eventType, ev.Properties) {
			continue // legacy criteria that predate the grammar stay manual
		}
		inc, ok := c.IncrementFor(ev.Properties)
		if !ok {
			continue
		}
		matches = append(matches, match{mission: m, increment: inc})
	}
	if len(matches) == 0 {
		return AutoResult{}, nil
	}

	player, found, err := s.activityPlayer(ctx, ev)
	if err != nil {
		return AutoResult{}, err
	}
	if !found && ev.AutoCreatePlayer {
		// The player module creates this player from the same event; retry
		// until it exists instead of dropping the player's first activities.
		return AutoResult{}, errs.New(errs.Unavailable, "player not created yet for auto-create activity "+activityID+"; retrying")
	}
	if !found || !player.Active {
		return AutoResult{}, nil
	}

	out := AutoResult{Matched: len(matches)}
	for _, mt := range matches {
		res, err := s.applyProgress(ctx, ProgressInput{
			IdempotencyKey: AutoKey(activityID, mt.mission.ID, player.ID),
			TenantID:       ev.TenantID,
			PlayerID:       player.ID,
			MissionID:      mt.mission.ID,
			Increment:      mt.increment,
			Source:         effect.Source{Kind: effect.SourceMission, ID: mt.mission.ID, ActivityID: activityID},
			PlayerChecked:  true,
			Quiet:          true,
		})
		if err != nil {
			return out, err
		}
		if res.Outcome == OutcomeApplied {
			out.Applied++
			if res.Completed {
				out.Complete++
			}
		}
	}
	return out, nil
}

// selfTriggered stops a mission from progressing on its own completion
// (internal mission_completed activities carry properties.mission_id).
func selfTriggered(m domain.Mission, eventType string, props map[string]any) bool {
	if eventType != activitycontracts.EventTypeMissionCompleted {
		return false
	}
	completed, _ := props["mission_id"].(string)
	return completed == m.ID
}

// activityPlayer resolves the activity's player: by PlayerID when ingest
// resolved it, else by the tenant's external id.
func (s *Service) activityPlayer(ctx context.Context, ev activitycontracts.ReceivedV1) (ports.PlayerSnapshot, bool, error) {
	if ev.PlayerID != "" {
		got, err := s.players.PlayersByIDs(ctx, ev.TenantID, []string{ev.PlayerID})
		if err != nil {
			return ports.PlayerSnapshot{}, false, err
		}
		p, ok := got[ev.PlayerID]
		return p, ok && p.TenantID == ev.TenantID, nil
	}
	ext := strings.TrimSpace(ev.PlayerExternalID)
	got, err := s.players.PlayersByExternalIDs(ctx, ev.TenantID, []string{ext})
	if err != nil {
		return ports.PlayerSnapshot{}, false, err
	}
	p, ok := got[ext]
	return p, ok && p.TenantID == ev.TenantID, nil
}
