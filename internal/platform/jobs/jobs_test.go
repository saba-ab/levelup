package jobs_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"myapp/internal/platform/jobs"
	"myapp/internal/platform/rabbit"
	"myapp/internal/platform/rabbit/rabbittest"
	"myapp/internal/shared/errs"
)

func init() {
	rabbit.RetryTiers = []rabbit.RetryTier{
		{Suffix: "100ms", TTL: 100 * time.Millisecond},
		{Suffix: "200ms", TTL: 200 * time.Millisecond},
	}
}

func setup(t *testing.T) (*jobs.RabbitQueue, *jobs.Worker, *rabbit.Topology) {
	t.Helper()
	eps := rabbittest.Get(t)
	conn, err := rabbit.Dial(eps.AMQPURL, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	topo := rabbit.NewTopology(conn, rabbit.MgmtConfig{
		URL: eps.MgmtURL, User: eps.User, Password: eps.Password,
	}, zap.NewNop())
	require.NoError(t, topo.DeclareCore(context.Background()))
	return jobs.NewRabbitQueue(conn, zap.NewNop()), jobs.NewWorker(conn, zap.NewNop()), topo
}

func TestEnqueueThenWorkerRuns(t *testing.T) {
	q, w, topo := setup(t)
	ctx := t.Context()
	require.NoError(t, topo.DeclareJobQueue(ctx, "t.simple"))

	var ran atomic.Int64
	go w.Run(ctx, []jobs.Job{{
		Name: "t.simple",
		Run: func(_ context.Context, payload []byte) error {
			require.Equal(t, "work", string(payload))
			ran.Add(1)
			return nil
		},
	}})

	require.NoError(t, q.Enqueue(ctx, "t.simple", []byte("work")))
	require.Eventually(t, func() bool { return ran.Load() == 1 }, 15*time.Second, 50*time.Millisecond)
}

func TestPanickingJobIsRecoveredRetriedThenParked(t *testing.T) {
	q, w, topo := setup(t)
	ctx := t.Context()
	require.NoError(t, topo.DeclareJobQueue(ctx, "t.panics"))

	var attempts atomic.Int64
	go w.Run(ctx, []jobs.Job{{
		Name: "t.panics",
		Run: func(context.Context, []byte) error {
			attempts.Add(1)
			panic("job exploded")
		},
	}})

	require.NoError(t, q.Enqueue(ctx, "t.panics", nil))

	wantAttempts := int64(1 + len(rabbit.RetryTiers))
	require.Eventually(t, func() bool { return attempts.Load() == wantAttempts },
		30*time.Second, 100*time.Millisecond,
		"panic must be recovered and retried, not crash the worker (PRD §7.10)")

	require.Eventually(t, func() bool {
		depths, err := topo.QueueDepths(ctx)
		return err == nil && depths[rabbit.JobQueue("t.panics")+".dlq"] == 1
	}, 30*time.Second, 200*time.Millisecond, "then parked in the DLQ")
}

func TestPriorityJobOvertakesBacklog(t *testing.T) {
	q, w, topo := setup(t)
	ctx := t.Context()
	require.NoError(t, topo.DeclareJobQueue(ctx, "t.prio"))

	// Fill the priority queue BEFORE any consumer exists, then start the
	// worker: the broker delivers by priority for already-queued messages.
	for range 10 {
		require.NoError(t, q.Enqueue(ctx, "t.prio", []byte("low"), jobs.WithPriority(1)))
	}
	require.NoError(t, q.Enqueue(ctx, "t.prio", []byte("urgent"), jobs.WithPriority(9)))

	var mu sync.Mutex
	var order []string
	go w.Run(ctx, []jobs.Job{{
		Name: "t.prio",
		Run: func(_ context.Context, payload []byte) error {
			mu.Lock()
			order = append(order, string(payload))
			mu.Unlock()
			return nil
		},
	}})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) == 11
	}, 15*time.Second, 50*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, order[:priorityWindow], "urgent",
		"the p9 job must overtake the p1 backlog (PRD §7.9)")
}

// priorityWindow: the urgent job must land within the first prefetch batch.
const priorityWindow = 5

func TestReplayDLQ(t *testing.T) {
	q, w, topo := setup(t)
	ctx := t.Context()
	require.NoError(t, topo.DeclareJobQueue(ctx, "t.replay"))

	eps := rabbittest.Get(t)
	conn, err := rabbit.Dial(eps.AMQPURL, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	var healthy atomic.Bool
	var succeeded atomic.Int64
	go w.Run(ctx, []jobs.Job{{
		Name: "t.replay",
		Run: func(context.Context, []byte) error {
			if !healthy.Load() {
				return errs.New(errs.Internal, "downstream broken")
			}
			succeeded.Add(1)
			return nil
		},
	}})

	require.NoError(t, q.Enqueue(ctx, "t.replay", nil))
	require.Eventually(t, func() bool {
		depths, err := topo.QueueDepths(ctx)
		return err == nil && depths[rabbit.JobQueue("t.replay")+".dlq"] == 1
	}, 30*time.Second, 200*time.Millisecond, "job parks after bounded retries")

	// Fix the downstream, then shovel the DLQ back (R49).
	healthy.Store(true)
	moved, err := rabbit.ReplayDLQ(ctx, conn, rabbit.JobQueue("t.replay"))
	require.NoError(t, err)
	require.Equal(t, 1, moved)
	require.Eventually(t, func() bool { return succeeded.Load() == 1 },
		15*time.Second, 50*time.Millisecond, "replayed job must run to success")
}
