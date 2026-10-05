package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/platform/authz"
)

// MaxBatchItems caps POST /activities/batch.
const MaxBatchItems = 100

// IngestCmd is one activity as reported by a tenant system.
type IngestCmd struct {
	EventID          string
	EventType        string
	PlayerExternalID string
	OccurredAt       *time.Time
	Properties       map[string]any
	Context          map[string]any
}

// IngestResult is the stored activity and whether the event_id had been
// seen before (no new event was published for a duplicate).
type IngestResult struct {
	Activity  domain.Activity
	Duplicate bool
}

// BatchItem is the per-item outcome of a batch: exactly one of Result and
// Err is meaningful.
type BatchItem struct {
	Result IngestResult
	Err    error
}

// Ingest stores one activity. The hot path is one transaction holding the
// idempotent insert and, only when a row was written, the outbox row for
// activity.received.v1.
func (s *Service) Ingest(ctx context.Context, cmd IngestCmd) (IngestResult, error) {
	items, err := s.ingest(ctx, []IngestCmd{cmd})
	if err != nil {
		return IngestResult{}, err
	}
	if items[0].Err != nil {
		return IngestResult{}, items[0].Err
	}
	return items[0].Result, nil
}

// IngestBatch stores up to MaxBatchItems activities in one transaction.
// Invalid items fail individually; the error return is reserved for
// failures of the whole request (authz, infrastructure).
func (s *Service) IngestBatch(ctx context.Context, cmds []IngestCmd) ([]BatchItem, error) {
	switch {
	case len(cmds) == 0:
		return nil, domain.ErrEmptyBatch
	case len(cmds) > MaxBatchItems:
		return nil, domain.ErrTooManyItems
	}
	return s.ingest(ctx, cmds)
}

func (s *Service) ingest(ctx context.Context, cmds []IngestCmd) ([]BatchItem, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermIngest, nil); err != nil {
		return nil, err
	}

	now := s.clock.Now()
	items := make([]BatchItem, len(cmds))
	acts := make([]domain.Activity, len(cmds))
	for i, c := range cmds {
		in := domain.NewInput{
			TenantID:         p.TenantID,
			EventID:          c.EventID,
			EventType:        c.EventType,
			PlayerExternalID: c.PlayerExternalID,
			Properties:       c.Properties,
			Context:          c.Context,
		}
		if c.OccurredAt != nil {
			in.OccurredAt = *c.OccurredAt
		}
		acts[i], items[i].Err = domain.NewActivity(in, now, s.cfg.Limits)
	}

	if err := s.checkEventTypes(ctx, p.TenantID, acts, items); err != nil {
		return nil, err
	}
	if err := s.resolvePlayers(ctx, p.TenantID, acts, items); err != nil {
		return nil, err
	}

	err = s.tx(ctx, func(tx *gorm.DB) error {
		inserted := make([]bool, len(acts))
		var dupes []string
		for i, a := range acts {
			if items[i].Err != nil {
				continue
			}
			ok, err := s.repo.Insert(ctx, tx, a)
			if err != nil {
				return err
			}
			if !ok {
				dupes = append(dupes, a.EventID)
				continue
			}
			inserted[i] = true
			items[i].Result = IngestResult{Activity: a}
			if err := s.outbox.Publish(ctx, tx, contracts.TopicReceived, s.toReceived(a)); err != nil {
				return err
			}
		}
		if len(dupes) == 0 {
			return nil
		}
		// A conflicting event_id: hand back the stored row, publish nothing.
		// Reading through tx also covers one event_id twice in a batch.
		stored, err := s.repo.ByEventIDs(ctx, tx, p.TenantID, dupes)
		if err != nil {
			return err
		}
		for i, a := range acts {
			if items[i].Err != nil || inserted[i] {
				continue
			}
			existing, ok := stored[a.EventID]
			if !ok {
				return domain.ErrDuplicateVanished
			}
			items[i].Result = IngestResult{Activity: existing, Duplicate: true}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// checkEventTypes enforces ACTIVITY_REQUIRE_KNOWN_EVENT_TYPE with one
// batched port call.
func (s *Service) checkEventTypes(ctx context.Context, tenantID string, acts []domain.Activity, items []BatchItem) error {
	if !s.cfg.RequireKnownEventType {
		return nil
	}
	slugs := uniqueValid(acts, items, func(a domain.Activity) string { return a.EventType })
	if len(slugs) == 0 {
		return nil
	}
	known, err := s.eventTypes.BySlugs(ctx, tenantID, slugs)
	if err != nil {
		return err
	}
	for i, a := range acts {
		if items[i].Err != nil {
			continue
		}
		t, ok := known[a.EventType]
		switch {
		case !ok:
			items[i].Err = domain.ErrUnknownEventType
		case !t.Active:
			items[i].Err = domain.ErrInactiveEventType
		}
	}
	return nil
}

// resolvePlayers fills PlayerID for players that already exist. Unknown
// players are not an ingest error: rules decides (rejects) them, unless
// auto-creation is on and a player owner reacts to the flag on the event.
func (s *Service) resolvePlayers(ctx context.Context, tenantID string, acts []domain.Activity, items []BatchItem) error {
	extIDs := uniqueValid(acts, items, func(a domain.Activity) string { return a.PlayerExternalID })
	if len(extIDs) == 0 {
		return nil
	}
	found, err := s.players.ByExternalIDs(ctx, tenantID, extIDs)
	if err != nil {
		return err
	}
	for i := range acts {
		if items[i].Err != nil {
			continue
		}
		if pl, ok := found[acts[i].PlayerExternalID]; ok {
			acts[i].PlayerID = pl.ID
		}
	}
	return nil
}

func uniqueValid(acts []domain.Activity, items []BatchItem, key func(domain.Activity) string) []string {
	seen := map[string]bool{}
	var out []string
	for i, a := range acts {
		k := key(a)
		if items[i].Err != nil || k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}
