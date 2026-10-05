package bus

import (
	"context"
	"encoding/json"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// Envelope is the wire shape of every event (PRD §7.11). TraceCtx carries the
// W3C traceparent so one trace spans HTTP → outbox → dispatcher → consumer.
type Envelope struct {
	EventID    string            `json:"event_id"`
	Topic      string            `json:"topic"`
	OccurredAt time.Time         `json:"occurred_at"`
	TraceCtx   map[string]string `json:"trace_ctx,omitempty"`
	Payload    json.RawMessage   `json:"payload"`
}

// NewEnvelope stamps identity, time, and the caller's trace context. It must
// be called where the fact happens (inside the transaction that records it),
// not in the dispatcher — by then the request's trace is gone.
func NewEnvelope(ctx context.Context, c clock.Clock, topic string, payload any) (Envelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, errs.Wrap(errs.Internal, "marshal event payload", err)
	}

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	return Envelope{
		EventID:    id.NewID(),
		Topic:      topic,
		OccurredAt: c.Now(),
		TraceCtx:   carrier,
		Payload:    body,
	}, nil
}

// ExtractTrace returns ctx joined to the trace the envelope carries.
func (e Envelope) ExtractTrace(ctx context.Context) context.Context {
	if len(e.TraceCtx) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(e.TraceCtx))
}
