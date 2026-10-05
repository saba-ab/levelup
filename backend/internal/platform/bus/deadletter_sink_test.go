package bus_test

import (
	"context"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"levelup/internal/platform/bus"
	"levelup/internal/platform/rabbit"
	"levelup/internal/platform/rabbit/rabbittest"
	"levelup/internal/shared/errs"
)

// recordingSink captures what the consumer loop reports when it parks a
// delivery, standing in for the outbox dead-letter table.
type recordingSink struct {
	mu    sync.Mutex
	parks []parked
}

type parked struct {
	queue    string
	msgID    string
	attempts int
	cause    error
}

func (s *recordingSink) ParkDelivery(_ context.Context, queue string, d amqp.Delivery, attempts int, cause error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.parks = append(s.parks, parked{queue: queue, msgID: d.MessageId, attempts: attempts, cause: cause})
	return nil
}

func (s *recordingSink) snapshot() []parked {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]parked(nil), s.parks...)
}

// R23: a delivery the loop sends to the broker DLQ is also reported to the
// sink, once, with the attempt count and the handler's error.
func TestDeadLetteredDeliveryIsReportedToSink(t *testing.T) {
	eps := rabbittest.Get(t)
	conn, err := rabbit.Dial(eps.AMQPURL, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	topo := rabbit.NewTopology(conn, rabbit.MgmtConfig{
		URL: eps.MgmtURL, User: eps.User, Password: eps.Password,
	}, zap.NewNop())
	ctx := t.Context()
	require.NoError(t, topo.DeclareCore(ctx))
	require.NoError(t, topo.DeclareEventQueue(ctx, "sink", "t.sink.v1"))

	sink := &recordingSink{}
	b := bus.NewRabbit(conn, zap.NewNop(), bus.WithDeadLetterSink(sink))
	require.NoError(t, b.Subscribe(ctx, bus.Subscription{
		Topic: "t.sink.v1", Group: "sink",
		Handler: func(context.Context, bus.Envelope) error {
			return errs.New(errs.Invalid, "malformed for good")
		},
	}))

	env := envelope(t, "t.sink.v1")
	require.NoError(t, b.Publish(ctx, env))

	require.Eventually(t, func() bool { return len(sink.snapshot()) == 1 },
		15*time.Second, 50*time.Millisecond, "an Invalid error parks immediately and reports once")

	got := sink.snapshot()[0]
	require.Equal(t, rabbit.EventQueue("sink", "t.sink.v1"), got.queue)
	require.Equal(t, env.EventID, got.msgID)
	require.Equal(t, 1, got.attempts, "parked on the first attempt")
	require.ErrorContains(t, got.cause, "malformed for good")

	time.Sleep(300 * time.Millisecond)
	require.Len(t, sink.snapshot(), 1, "no second report after parking")
}
