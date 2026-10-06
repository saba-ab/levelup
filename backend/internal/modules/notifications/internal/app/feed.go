package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/domain"
	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/platform/bus"
	"levelup/internal/shared/errs"
)

// FeedPage is one page of a player's in-app feed.
type FeedPage struct {
	Items       []domain.Notification
	More        bool
	UnreadCount int64
}

// PlayerFeed pages a player's in-app notifications newest first.
func (s *Service) PlayerFeed(ctx context.Context, playerID string, f FeedFilter) (FeedPage, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermFeedView)
	if err != nil {
		return FeedPage{}, err
	}
	if _, err := s.requirePlayer(ctx, p.TenantID, playerID); err != nil {
		return FeedPage{}, err
	}
	limit := clampLimit(f.Limit)
	f.Limit = limit + 1
	rows, err := s.repo.ListFeed(ctx, p.TenantID, playerID, f)
	if err != nil {
		return FeedPage{}, err
	}
	out := FeedPage{Items: rows}
	if len(rows) > limit {
		out.Items, out.More = rows[:limit], true
	}
	out.UnreadCount, err = s.repo.CountUnread(ctx, p.TenantID, playerID)
	if err != nil {
		return FeedPage{}, err
	}
	return out, nil
}

// MarkRead marks one in-app notification of the player read. Idempotent:
// a second call keeps the first read_at.
func (s *Service) MarkRead(ctx context.Context, playerID, notificationID string) (domain.Notification, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermFeedMarkRead)
	if err != nil {
		return domain.Notification{}, err
	}
	if _, err := s.requirePlayer(ctx, p.TenantID, playerID); err != nil {
		return domain.Notification{}, err
	}
	var out domain.Notification
	err = s.tx(ctx, func(tx *gorm.DB) error {
		n, found, err := s.repo.MarkRead(ctx, tx, p.TenantID, playerID, notificationID, s.now())
		if err != nil {
			return err
		}
		if !found {
			return domain.ErrNotificationNotFound
		}
		out = n
		return nil
	})
	return out, err
}

// MarkAllRead marks every unread in-app notification of the player read and
// returns how many changed.
func (s *Service) MarkAllRead(ctx context.Context, playerID string) (int64, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermFeedMarkRead)
	if err != nil {
		return 0, err
	}
	if _, err := s.requirePlayer(ctx, p.TenantID, playerID); err != nil {
		return 0, err
	}
	var n int64
	err = s.tx(ctx, func(tx *gorm.DB) error {
		var err error
		n, err = s.repo.MarkAllRead(ctx, tx, p.TenantID, playerID, s.now())
		return err
	})
	return n, err
}

// HistoryPage is one page of the notification history, with the template
// names of the page's rows.
type HistoryPage struct {
	Items         []domain.Notification
	More          bool
	TemplateNames map[string]string
}

// History pages every notification of the tenant newest first.
func (s *Service) History(ctx context.Context, f HistoryFilter) (HistoryPage, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermViewAny)
	if err != nil {
		return HistoryPage{}, err
	}
	limit := clampLimit(f.Limit)
	f.Limit = limit + 1
	rows, err := s.repo.ListHistory(ctx, p.TenantID, f)
	if err != nil {
		return HistoryPage{}, err
	}
	out := HistoryPage{Items: rows}
	if len(rows) > limit {
		out.Items, out.More = rows[:limit], true
	}
	ids := make([]string, 0, len(out.Items))
	for _, n := range out.Items {
		ids = append(ids, n.TemplateID)
	}
	out.TemplateNames, err = s.templateNames(ctx, p.TenantID, ids)
	return out, err
}

func (s *Service) templateNames(ctx context.Context, tenantID string, ids []string) (map[string]string, error) {
	names := map[string]string{}
	if len(ids) == 0 {
		return names, nil
	}
	ts, err := s.repo.TemplatesByIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	for _, t := range ts {
		names[t.ID] = t.Name
	}
	return names, nil
}

// StatsResult is the aggregate with template names.
type StatsResult struct {
	domain.Stats
	TemplateNames    map[string]string
	TemplateTriggers map[string]string
}

// Stats aggregates the tenant's notifications created in [from, to); zero
// bounds are open.
func (s *Service) Stats(ctx context.Context, from, to time.Time) (StatsResult, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermViewAny)
	if err != nil {
		return StatsResult{}, err
	}
	rows, err := s.repo.Stats(ctx, p.TenantID, from, to)
	if err != nil {
		return StatsResult{}, err
	}
	out := StatsResult{Stats: domain.Aggregate(rows), TemplateNames: map[string]string{}, TemplateTriggers: map[string]string{}}
	ids := make([]string, 0, len(out.ByTemplate))
	for tid := range out.ByTemplate {
		ids = append(ids, tid)
	}
	if len(ids) > 0 {
		ts, err := s.repo.TemplatesByIDs(ctx, p.TenantID, ids)
		if err != nil {
			return StatsResult{}, err
		}
		for _, t := range ts {
			out.TemplateNames[t.ID] = t.Name
			out.TemplateTriggers[t.ID] = t.Trigger
		}
	}
	return out, nil
}

// OnTenantDeleted purges every notifications row of the tenant. Idempotent.
func (s *Service) OnTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	if _, err := uuid.Parse(ev.TenantID); err != nil {
		return errs.Wrap(errs.Invalid, "tenant.deleted.v1 without a valid tenant_id", err)
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, ev.TenantID)
	})
}

// OnPlayerDeleted drops the player's notifications (they hold rendered
// personal data). Idempotent.
func (s *Service) OnPlayerDeleted(ctx context.Context, e bus.Envelope) error {
	var ev playercontracts.PlayerDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable player.deleted.v1", err)
	}
	_, terr := uuid.Parse(ev.TenantID)
	_, perr := uuid.Parse(ev.PlayerID)
	if terr != nil || perr != nil {
		return errs.New(errs.Invalid, "player.deleted.v1 without valid tenant_id/player_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgePlayer(ctx, tx, ev.TenantID, ev.PlayerID)
	})
}
