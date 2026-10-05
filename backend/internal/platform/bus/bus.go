// Package bus carries EVENTS: facts that already happened, fanned out to many
// independent consumer groups (PRD §7.9). It is not for work distribution —
// that is jobs.Queue. Modules never publish here directly: the outbox
// dispatcher is the only production publisher (PRD §4 P3).
package bus

import "context"

type Handler func(ctx context.Context, e Envelope) error

// Subscription is a module's declaration that it consumes a topic. The
// composition root wires these; the module never touches transport (PRD §6).
type Subscription struct {
	Topic   string
	Group   string // consumer group, usually the module name
	Handler Handler
}

type Bus interface {
	Publish(ctx context.Context, e Envelope) error
	Subscribe(ctx context.Context, s Subscription) error
}
