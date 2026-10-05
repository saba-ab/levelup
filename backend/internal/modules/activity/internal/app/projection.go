package app

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/activity/internal/domain"
	rulescontracts "levelup/internal/modules/rules/contracts"
	"levelup/internal/shared/errs"
)

// ApplyDecision projects rules.decision_made.v1 onto the activity. It is
// a conditional update of a pending row, so redelivery, a replay of the
// same decision, or a late duplicate decision all change nothing.
func (s *Service) ApplyDecision(ctx context.Context, ev rulescontracts.DecisionMadeV1) error {
	if !isUUID(ev.TenantID) || !isUUID(ev.ActivityID) || !isUUID(ev.DecisionID) {
		return domain.ErrMalformedDecision
	}
	now := s.clock.Now()
	decidedAt := ev.At
	if decidedAt.IsZero() {
		decidedAt = now
	}
	status := domain.StatusDecided
	if ev.Outcome == rulescontracts.OutcomeRejected {
		status = domain.StatusRejected
	}
	a := domain.Activity{
		ID:         ev.ActivityID,
		TenantID:   ev.TenantID,
		PlayerID:   ev.PlayerID,
		Status:     status,
		DecisionID: ev.DecisionID,
		Outcome:    ev.Outcome,
		Reason:     ev.Reason,
		DecidedAt:  decidedAt.UTC(),
		UpdatedAt:  now,
	}
	if a.PlayerID != "" && !isUUID(a.PlayerID) {
		a.PlayerID = ""
	}

	var applied bool
	err := s.tx(ctx, func(tx *gorm.DB) error {
		var err error
		applied, err = s.repo.ApplyDecision(ctx, tx, a)
		return err
	})
	if err != nil {
		return err
	}
	if !applied {
		s.log.Debug("decision not applied: activity already settled or gone",
			zap.String("activity_id", ev.ActivityID), zap.String("decision_id", ev.DecisionID))
	}
	return nil
}

// InternalTrigger is a fact from another module turned into an activity
// (doc 06 §11.9).
type InternalTrigger struct {
	TenantID      string
	PlayerID      string
	EventType     string
	SourceTopic   string
	SourceEventID string // envelope event_id: the dedupe key
	// ParentActivityID, when the source fact names the activity that
	// caused it, lets causation depth grow along a chain.
	ParentActivityID string
	Properties       map[string]any
	OccurredAt       time.Time
}

// RecordInternal stores an internal activity with event_id
// "sys:"+source event id, so redelivery is a no-op, and publishes
// activity.received.v1 only for a new row. Event-type strictness does not
// apply: these types are system-defined.
func (s *Service) RecordInternal(ctx context.Context, t InternalTrigger) error {
	if t.TenantID == "" || t.PlayerID == "" || t.SourceEventID == "" {
		return domain.ErrMalformedTrigger
	}

	depth := 1
	if t.ParentActivityID != "" && isUUID(t.ParentActivityID) {
		parent, err := s.repo.ByID(ctx, t.TenantID, t.ParentActivityID)
		switch {
		case err == nil:
			depth = parent.CausationDepth + 1
		case errs.KindOf(err) != errs.NotFound:
			return err
		}
	}

	extID := ""
	found, err := s.players.ByIDs(ctx, t.TenantID, []string{t.PlayerID})
	if err != nil {
		return err
	}
	if pl, ok := found[t.PlayerID]; ok {
		extID = pl.ExternalID
	}

	a, err := domain.NewActivity(domain.NewInput{
		TenantID:         t.TenantID,
		EventID:          contracts.InternalEventIDPrefix + t.SourceEventID,
		EventType:        t.EventType,
		PlayerExternalID: extID,
		PlayerID:         t.PlayerID,
		Properties:       t.Properties,
		Context: map[string]any{
			"source_topic":    t.SourceTopic,
			"source_event_id": t.SourceEventID,
		},
		OccurredAt:     t.OccurredAt,
		CausationDepth: depth,
		SourceEventID:  t.SourceEventID,
	}, s.clock.Now(), domain.Limits{MaxPayloadBytes: s.cfg.Limits.MaxPayloadBytes})
	if err != nil {
		return err
	}

	return s.tx(ctx, func(tx *gorm.DB) error {
		inserted, err := s.repo.Insert(ctx, tx, a)
		if err != nil || !inserted {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicReceived, s.toReceived(a))
	})
}

// PurgeTenant deletes every activity of a deleted tenant. Idempotent: a
// second run deletes nothing.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	if !isUUID(tenantID) {
		return domain.ErrMalformedTenantPurge
	}
	var n int64
	err := s.tx(ctx, func(tx *gorm.DB) error {
		var err error
		n, err = s.repo.DeleteTenant(ctx, tx, tenantID)
		return err
	})
	if err != nil {
		return err
	}
	s.log.Info("purged tenant activities", zap.String("tenant_id", tenantID), zap.Int64("rows", n))
	return nil
}
