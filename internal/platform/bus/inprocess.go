package bus

import (
	"context"
	"errors"
	"sync"
)

// InProcess is a synchronous test double (PRD §5: "NOT a prod path"). It
// exists so unit tests and the Phase-1 skeleton can wire subscriptions
// without a broker. Delivery semantics deliberately mirror production:
// every group receives every matching event.
type InProcess struct {
	mu   sync.RWMutex
	subs []Subscription
}

func NewInProcess() *InProcess { return &InProcess{} }

func (b *InProcess) Subscribe(_ context.Context, s Subscription) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs = append(b.subs, s)
	return nil
}

func (b *InProcess) Publish(ctx context.Context, e Envelope) error {
	b.mu.RLock()
	subs := make([]Subscription, len(b.subs))
	copy(subs, b.subs)
	b.mu.RUnlock()

	var errsAll []error
	for _, s := range subs {
		if s.Topic != e.Topic {
			continue
		}
		if err := s.Handler(e.ExtractTrace(ctx), e); err != nil {
			errsAll = append(errsAll, err)
		}
	}
	return errors.Join(errsAll...)
}
