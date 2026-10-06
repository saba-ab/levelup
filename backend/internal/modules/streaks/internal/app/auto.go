package app

import (
	"context"
	"errors"
	"strings"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/streaks/contracts"
	"levelup/internal/modules/streaks/internal/domain"
	"levelup/internal/modules/streaks/internal/ports"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// AutoKey is the idempotency key of the automatic record an activity makes
// for one streak: a redelivered activity derives the same key and the
// record_requests row turns it into a no-op.
func AutoKey(activityID, streakID string) string {
	return id.Derive("streak_auto", activityID, streakID)
}

// HandleActivity is the activity.received.v1 subscriber. The active,
// auto-recording streak whose activity_key equals the event type records the
// activity's period for its player, through the same path as a record
// command (period buckets, milestones, points). Nothing to do — no such
// streak, opted out, inactive, unknown or inactive player — is a silent
// no-op: rules owns rejections of activities. Malformed payloads are
// errs.Invalid (DLQ); transient failures are returned for a retry.
func (s *Service) HandleActivity(ctx context.Context, ev activitycontracts.ReceivedV1) (RecordResult, error) {
	activityID := strings.TrimSpace(ev.ActivityID)
	eventType := strings.TrimSpace(ev.EventType)
	if ev.TenantID == "" || activityID == "" || eventType == "" {
		return RecordResult{}, errs.New(errs.Invalid, "activity.received.v1 needs tenant_id, activity_id and event_type")
	}
	if ev.PlayerID == "" && strings.TrimSpace(ev.PlayerExternalID) == "" {
		return RecordResult{}, errs.New(errs.Invalid, "activity.received.v1 needs player_id or player_external_id")
	}

	st, err := s.repo.StreakByActivityKey(ctx, ev.TenantID, eventType)
	switch {
	case errors.Is(err, domain.ErrStreakNotFound):
		return RecordResult{Outcome: OutcomeNoop}, nil
	case err != nil:
		return RecordResult{}, err
	case st.ActivityKey != eventType || !st.Active || !st.AutoRecord:
		return RecordResult{Outcome: OutcomeNoop}, nil
	}

	key := AutoKey(activityID, st.ID)
	done, err := s.repo.RequestExists(ctx, ev.TenantID, key)
	if err != nil {
		return RecordResult{}, err
	}
	if done {
		return RecordResult{Outcome: OutcomeDuplicate}, nil
	}

	pl, found, err := s.activityPlayer(ctx, ev)
	if err != nil {
		return RecordResult{}, err
	}
	if !found && ev.AutoCreatePlayer {
		// The player module creates this player from the same event; retry
		// until it exists instead of dropping the player's first activities.
		return RecordResult{}, errs.New(errs.Unavailable, "player not created yet for auto-create activity "+activityID+"; retrying")
	}
	if !found || !pl.Active {
		return RecordResult{Outcome: OutcomeNoop}, nil
	}

	return s.apply(ctx, contracts.RecordCmdV1{
		IdempotencyKey: key,
		TenantID:       ev.TenantID,
		PlayerID:       pl.ID,
		StreakID:       st.ID,
		ActivityKey:    st.ActivityKey,
		Source:         effect.Source{Kind: effect.SourceStreak, ID: st.ID, ActivityID: activityID},
		OccurredAt:     ev.OccurredAt,
	}, st)
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
