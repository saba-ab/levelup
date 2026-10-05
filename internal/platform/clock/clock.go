// Package clock is the injectable time source. Services take clock.Clock so
// tests control time; time.Now() in business code is a bug.
package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

func System() Clock { return systemClock{} }

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// Fake is a manually-advanced clock for tests.
type Fake struct {
	mu sync.Mutex
	t  time.Time
}

func NewFake(start time.Time) *Fake { return &Fake{t: start} }

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}
