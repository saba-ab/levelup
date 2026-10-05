package outbox

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"

	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

func jsonb(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// Postgres is the production Store: it writes the event into the caller's
// transaction, so the state change and the fact commit or roll back together
// (PRD §4 P3 — the entire point of the outbox).
type Postgres struct {
	clock clock.Clock
}

func NewPostgres(c clock.Clock) *Postgres { return &Postgres{clock: c} }

// Publish stamps the envelope HERE — identity, occurred_at, and the caller's
// live trace context (R39). By dispatcher time the request's trace is gone;
// this is the only moment it can be captured.
func (p *Postgres) Publish(ctx context.Context, tx *gorm.DB, topic string, payload any) error {
	if tx == nil {
		return errs.New(errs.Internal, "outbox.Publish requires the caller's transaction")
	}
	ctx, span := otel.Tracer("levelup/outbox").Start(ctx, "outbox.publish "+topic)
	defer span.End()

	env, err := bus.NewEnvelope(ctx, p.clock, topic, payload)
	if err != nil {
		return err
	}
	headers := map[string]any{"trace_ctx": env.TraceCtx}

	err = tx.WithContext(ctx).Exec(
		`INSERT INTO outbox_svc.outbox_events (event_id, topic, payload, headers, occurred_at)
		 VALUES (?, ?, ?, ?, ?)`,
		env.EventID, env.Topic, string(env.Payload), jsonb(headers), env.OccurredAt,
	).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "outbox insert", err)
	}
	return nil
}
