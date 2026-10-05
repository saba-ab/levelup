package jobs_test

import (
	"context"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"levelup/internal/platform/jobs"
	"levelup/internal/platform/rabbit"
	"levelup/internal/platform/rabbit/rabbittest"
	"levelup/internal/shared/errs"
)

type recordingSink struct {
	mu    sync.Mutex
	parks []parkedJob
}

type parkedJob struct {
	queue    string
	attempts int
	cause    error
}

func (s *recordingSink) ParkDelivery(_ context.Context, queue string, _ amqp.Delivery, attempts int, cause error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.parks = append(s.parks, parkedJob{queue: queue, attempts: attempts, cause: cause})
	return nil
}

func (s *recordingSink) snapshot() []parkedJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]parkedJob(nil), s.parks...)
}

// R23: a job that exhausts the retry ladder is reported to the sink once,
// with every attempt counted, when the loop parks it in the DLQ.
func TestJobExhaustingLadderIsReportedToSink(t *testing.T) {
	eps := rabbittest.Get(t)
	conn, err := rabbit.Dial(eps.AMQPURL, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	topo := rabbit.NewTopology(conn, rabbit.MgmtConfig{
		URL: eps.MgmtURL, User: eps.User, Password: eps.Password,
	}, zap.NewNop())
	ctx := t.Context()
	require.NoError(t, topo.DeclareCore(ctx))
	require.NoError(t, topo.DeclareJobQueue(ctx, "t.sinkjob"))

	sink := &recordingSink{}
	q := jobs.NewRabbitQueue(conn, zap.NewNop())
	w := jobs.NewWorker(conn, zap.NewNop(), jobs.WithDeadLetterSink(sink))
	go w.Run(ctx, []jobs.Job{{
		Name: "t.sinkjob",
		Run: func(context.Context, []byte) error {
			return errs.New(errs.Internal, "still broken")
		},
	}})

	require.NoError(t, q.Enqueue(ctx, "t.sinkjob", []byte("work")))

	require.Eventually(t, func() bool { return len(sink.snapshot()) == 1 },
		30*time.Second, 100*time.Millisecond, "parked once after the ladder")

	got := sink.snapshot()[0]
	require.Equal(t, rabbit.JobQueue("t.sinkjob"), got.queue)
	require.Equal(t, 1+len(rabbit.RetryTiers), got.attempts, "first try plus every retry tier")
	require.ErrorContains(t, got.cause, "still broken")
}
