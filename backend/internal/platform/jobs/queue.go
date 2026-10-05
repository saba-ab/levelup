// Package jobs carries WORK: commands with exactly one consumer, retries,
// and optional priority (PRD §7.9). Distinct from bus events by design —
// collapsing them produces an API where half the parameters are meaningless.
package jobs

import "context"

// Job declares background work a module owns (PRD §6 Module.Jobs).
//   - Schedule != "": cmd/scheduler enqueues on that cron spec (UTC). The
//     scheduler never executes — cmd/worker does (PRD §7.10).
//   - Schedule == "": pure queue consumer; something else enqueues.
type Job struct {
	Name     string
	Schedule string
	Priority uint8 // >0 routes via the classic priority queue (documented exception, PRD §7.9)
	Run      func(ctx context.Context, payload []byte) error
}

type EnqueueOpt func(*EnqueueOpts)

type EnqueueOpts struct {
	Priority uint8
}

func WithPriority(p uint8) EnqueueOpt { return func(o *EnqueueOpts) { o.Priority = p } }

// Queue enqueues work. Direct Enqueue is fire-and-forget by definition: if
// the broker is down the work is gone. State-change-triggered jobs must go
// through the outbox instead (PRD §7.9.2 Layer 4, R46).
type Queue interface {
	Enqueue(ctx context.Context, name string, payload []byte, opts ...EnqueueOpt) error
}
