package app

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/player/contracts"
	"levelup/internal/modules/player/internal/domain"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// ErrMalformedAutoCreate parks an activity.received.v1 flagged
// auto_create_player that cannot name a player (immediate DLQ).
var ErrMalformedAutoCreate = errs.New(errs.Invalid,
	"auto_create_player needs a UUID tenant_id and activity_id and a player_external_id")

// AutoCreateFromActivity reacts to activity.received.v1. When the activity
// is flagged AutoCreatePlayer and no live player holds its external id in
// the tenant, it creates one (created_by null, the system) and records
// player.created.v1 in the same transaction.
//
// Idempotent under redelivery and reordering: the player id is derived from
// the activity id, so a redelivered event collides on the primary key even
// after the player was deleted, and two activities racing for one unknown
// player collide on the live (tenant_id, external_id) index. A collision
// publishes nothing.
//
// Display hints (activity contracts' convention): display_name comes from
// properties.player_display_name, else context.player_display_name, else
// the external id; email from properties.player_email, else
// context.player_email. A hint that is not a string or fails validation is
// dropped, never fatal.
func (s *Service) AutoCreateFromActivity(ctx context.Context, ev activitycontracts.ReceivedV1) (created bool, err error) {
	if !ev.AutoCreatePlayer {
		return false, nil
	}
	ext := strings.TrimSpace(ev.PlayerExternalID)
	if !isUUID(ev.TenantID) || !isUUID(ev.ActivityID) || ext == "" {
		return false, ErrMalformedAutoCreate
	}

	// Cheap pre-check off the hot write path; the insert below is the
	// race-safe arbiter.
	if _, err := s.repo.ByExternalID(ctx, ev.TenantID, ext); err == nil {
		return false, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}

	pl, err := s.autoPlayer(ev, ext)
	if err != nil {
		return false, err
	}

	err = s.tx(ctx, func(tx *gorm.DB) error {
		inserted, err := s.repo.CreateIfAbsent(ctx, tx, pl)
		if err != nil || !inserted {
			return err
		}
		created = true
		return s.outbox.Publish(ctx, tx, contracts.TopicPlayerCreated, contracts.PlayerCreatedV1{
			PlayerID:         pl.ID,
			TenantID:         pl.TenantID,
			ExternalID:       pl.ExternalID,
			DisplayName:      pl.DisplayName,
			At:               pl.CreatedAt,
			AutoCreated:      true,
			SourceActivityID: ev.ActivityID,
		})
	})
	if err != nil {
		return false, err
	}
	// Activity's ingest lookup may have cached a tombstone for this
	// external id; evict even on a collision (after commit, R44).
	s.repo.Evict(ctx, ev.TenantID, Ref{ID: pl.ID, ExternalID: pl.ExternalID})
	return created, nil
}

// autoPlayer builds the player, dropping display hints that do not pass
// the domain's invariants. Only the external id itself can fail.
func (s *Service) autoPlayer(ev activitycontracts.ReceivedV1, ext string) (domain.Player, error) {
	playerID := id.Derive("player.autocreate", ev.TenantID, ev.ActivityID)
	prof := domain.Profile{
		DisplayName: hint(ev, activitycontracts.PropPlayerDisplayName),
		Email:       hint(ev, activitycontracts.PropPlayerEmail),
	}
	if prof.DisplayName == "" {
		prof.DisplayName = ext
	}
	now := s.clock.Now()
	for {
		pl, err := domain.NewPlayer(playerID, ev.TenantID, ext, prof, "", now)
		switch {
		case err == nil:
			return pl, nil
		case errors.Is(err, domain.ErrEmailInvalid) && prof.Email != "":
			prof.Email = ""
		case errors.Is(err, domain.ErrDisplayNameLong) && prof.DisplayName != ext:
			prof.DisplayName = ext
		default:
			return domain.Player{}, errs.Wrap(errs.Invalid, "auto-create player", err)
		}
	}
}

func hint(ev activitycontracts.ReceivedV1, key string) string {
	for _, src := range []map[string]any{ev.Properties, ev.Context} {
		if v, ok := src[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
