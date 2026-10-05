package scheduler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"levelup/internal/platform/jobs"
	"levelup/internal/platform/redis"
	"levelup/internal/platform/redis/redistest"
	"levelup/internal/platform/scheduler"
)

type countingQueue struct {
	mu       sync.Mutex
	enqueues []time.Time
}

func (q *countingQueue) Enqueue(_ context.Context, _ string, _ []byte, _ ...jobs.EnqueueOpt) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.enqueues = append(q.enqueues, time.Now())
	return nil
}

func (q *countingQueue) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.enqueues)
}

// R36: two scheduler instances, one enqueue per tick — never two.
func TestTwoSchedulersEnqueueOncePerTick(t *testing.T) {
	core := redis.NewCore(redistest.Addr(t))
	queue := &countingQueue{}

	job := jobs.Job{Name: "probe.tick", Schedule: "@every 1s"}

	var scheds []*scheduler.Scheduler
	for range 2 {
		s := scheduler.New(redis.NewLocker(core, zap.NewNop()), queue, zap.NewNop())
		require.NoError(t, s.Register(job))
		s.Start()
		scheds = append(scheds, s)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for _, s := range scheds {
			s.Stop(ctx)
		}
	}()

	// Watch ~4 ticks. The per-entry lock (held ~lockTTL ≫ tick here? no —
	// released right after enqueue) means each tick is won by exactly one
	// instance.
	time.Sleep(4500 * time.Millisecond)

	got := queue.count()
	require.GreaterOrEqual(t, got, 3, "ticks must fire")
	require.LessOrEqual(t, got, 5, "two instances must not double-enqueue (R36): got %d", got)
}

func TestRegisterSkipsPureConsumers(t *testing.T) {
	core := redis.NewCore(redistest.Addr(t))
	s := scheduler.New(redis.NewLocker(core, zap.NewNop()), &countingQueue{}, zap.NewNop())
	require.NoError(t, s.Register(jobs.Job{Name: "no.schedule"}))
}
