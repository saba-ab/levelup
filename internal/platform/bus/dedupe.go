package bus

import (
	"context"
	"time"

	"go.uber.org/zap"

	"myapp/internal/platform/redis"
)

const dedupeTTL = 24 * time.Hour

// WithDedupe wraps a handler with event_id dedupe on redis-core (§13 Q6:
// Redis chosen for speed; the durable guarantee stays the handler's own
// idempotency, which must hold regardless — delivery is at-least-once by
// design). Redis being down FAILS OPEN into the handler: processing twice
// is the contract; not processing is a stall.
func WithDedupe(core redis.Core, group string, h Handler) Handler {
	log := zap.L()
	return func(ctx context.Context, e Envelope) error {
		key := "dedupe:" + group + ":" + e.EventID
		ok, err := core.SetNX(ctx, key, 1, dedupeTTL).Result()
		if err != nil {
			log.Warn("dedupe store unreachable — processing anyway", zap.Error(err))
			return h(ctx, e)
		}
		if !ok {
			return nil // duplicate: already handled (or being handled)
		}
		herr := h(ctx, e)
		if herr != nil {
			// Failed handling must not swallow the event on redelivery.
			_ = core.Del(ctx, key).Err()
		}
		return herr
	}
}
