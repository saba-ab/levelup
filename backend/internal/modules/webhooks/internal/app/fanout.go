package app

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// Fact is one consumed event, decoupled from the bus envelope.
type Fact struct {
	Topic      string
	EventID    string // envelope event_id: becomes the webhook event_id
	OccurredAt time.Time
	Payload    json.RawMessage
}

// HandleFact fans a catalogue fact out: one delivery row per matching
// active endpoint of the fact's tenant, plus a job.webhooks.deliver command
// per NEW row, in one transaction.
//
// Idempotent under redelivery (ADR-0012): the delivery insert is ON
// CONFLICT (endpoint_id, event_id) DO NOTHING and only inserted rows
// enqueue a job, so a second delivery of the same fact inserts and
// publishes nothing.
func (s *Service) HandleFact(ctx context.Context, f Fact) error {
	event, ok := domain.EventForTopic(f.Topic)
	if !ok {
		return errs.New(errs.Invalid, "topic "+f.Topic+" is not in the webhook catalogue")
	}
	if f.EventID == "" {
		return errs.New(errs.Invalid, f.Topic+" envelope without event_id")
	}
	var head struct {
		TenantID string `json:"tenant_id"`
	}
	if err := json.Unmarshal(f.Payload, &head); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable "+f.Topic, err)
	}
	if head.TenantID == "" {
		return errs.New(errs.Invalid, f.Topic+" without tenant_id")
	}

	// Fast path outside any transaction: most tenants have no endpoints.
	endpoints, err := s.repo.ActiveEndpointsFor(ctx, head.TenantID, event)
	if err != nil {
		return err
	}
	if len(endpoints) == 0 {
		return nil
	}
	occurred := f.OccurredAt
	if occurred.IsZero() {
		occurred = s.clock.Now()
	}
	body, err := domain.BuildPayload(event, f.EventID, head.TenantID, occurred, f.Payload)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	return s.tx(ctx, func(tx *gorm.DB) error {
		for _, e := range endpoints {
			if !e.Active || !e.Subscribes(event) {
				continue
			}
			d := domain.NewDelivery(head.TenantID, e.ID, f.EventID, event, body, now)
			inserted, err := s.repo.InsertDelivery(ctx, tx, d)
			if err != nil {
				return err
			}
			if !inserted {
				continue
			}
			if err := s.enqueue(ctx, tx, d); err != nil {
				return err
			}
		}
		return nil
	})
}

func newEventID() string { return id.NewID() }

func testData(endpointID string) (json.RawMessage, error) {
	b, err := json.Marshal(map[string]string{
		"endpoint_id": endpointID,
		"message":     "This is a test delivery from LevelUp.",
	})
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "marshal test payload", err)
	}
	return b, nil
}
