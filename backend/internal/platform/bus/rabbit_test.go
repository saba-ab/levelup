package bus_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/rabbit"
	"levelup/internal/platform/rabbit/rabbittest"
	"levelup/internal/platform/redis"
	"levelup/internal/platform/redis/redistest"
	"levelup/internal/shared/errs"
)

// Short ladder so the poison test does not sit through 5s/30s/2m of real
// backoff. Set before any queue declaration in this package.
func init() {
	rabbit.RetryTiers = []rabbit.RetryTier{
		{Suffix: "100ms", TTL: 100 * time.Millisecond},
		{Suffix: "200ms", TTL: 200 * time.Millisecond},
	}
}

func rabbitBus(t *testing.T) (*bus.Rabbit, *rabbit.Topology) {
	t.Helper()
	eps := rabbittest.Get(t)
	conn, err := rabbit.Dial(eps.AMQPURL, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	topo := rabbit.NewTopology(conn, rabbit.MgmtConfig{
		URL: eps.MgmtURL, User: eps.User, Password: eps.Password,
	}, zap.NewNop())
	require.NoError(t, topo.DeclareCore(context.Background()))
	return bus.NewRabbit(conn, zap.NewNop()), topo
}

func envelope(t *testing.T, topic string) bus.Envelope {
	t.Helper()
	env, err := bus.NewEnvelope(context.Background(), clock.System(), topic, map[string]string{"k": "v"})
	require.NoError(t, err)
	return env
}

func TestRabbitDeliverHappyPath(t *testing.T) {
	b, topo := rabbitBus(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, topo.DeclareEventQueue(ctx, "happy", "t.happy.v1"))

	var got atomic.Int64
	require.NoError(t, b.Subscribe(ctx, bus.Subscription{
		Topic: "t.happy.v1", Group: "happy",
		Handler: func(_ context.Context, e bus.Envelope) error {
			got.Add(1)
			return nil
		},
	}))

	require.NoError(t, b.Publish(ctx, envelope(t, "t.happy.v1")))
	require.Eventually(t, func() bool { return got.Load() == 1 }, 15*time.Second, 50*time.Millisecond)
}

// R35: a poison message reaches the DLQ within bounded attempts and does not
// starve the queue.
func TestPoisonMessageParksInDLQAndQueueKeepsFlowing(t *testing.T) {
	b, topo := rabbitBus(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, topo.DeclareEventQueue(ctx, "poison", "t.poison.v1"))

	var attempts, goodDelivered atomic.Int64
	require.NoError(t, b.Subscribe(ctx, bus.Subscription{
		Topic: "t.poison.v1", Group: "poison",
		Handler: func(_ context.Context, e bus.Envelope) error {
			if string(e.Payload) == `"poison"` {
				attempts.Add(1)
				return errs.New(errs.Internal, "cannot process")
			}
			goodDelivered.Add(1)
			return nil
		},
	}))

	poison, err := bus.NewEnvelope(ctx, clock.System(), "t.poison.v1", "poison")
	require.NoError(t, err)
	require.NoError(t, b.Publish(ctx, poison))
	require.NoError(t, b.Publish(ctx, envelope(t, "t.poison.v1")))

	require.Eventually(t, func() bool { return goodDelivered.Load() == 1 },
		15*time.Second, 50*time.Millisecond, "poison must not starve the queue (R35)")

	// 1 initial + len(RetryTiers) retries, then DLQ.
	wantAttempts := int64(1 + len(rabbit.RetryTiers))
	require.Eventually(t, func() bool { return attempts.Load() == wantAttempts },
		30*time.Second, 100*time.Millisecond, "bounded retries then park")

	queue := rabbit.EventQueue("poison", "t.poison.v1")
	require.Eventually(t, func() bool {
		depths, err := topo.QueueDepths(ctx)
		return err == nil && depths[queue+".dlq"] == 1 && depths[queue] == 0
	}, 30*time.Second, 200*time.Millisecond, "poison message must be parked in the DLQ")

	// And it stays parked: no further attempts.
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, wantAttempts, attempts.Load())
}

func TestDedupeSkipsSecondDelivery(t *testing.T) {
	b, topo := rabbitBus(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, topo.DeclareEventQueue(ctx, "dedup", "t.dedup.v1"))

	core := redis.NewCore(redistest.Addr(t))
	var handled atomic.Int64
	require.NoError(t, b.Subscribe(ctx, bus.Subscription{
		Topic: "t.dedup.v1", Group: "dedup",
		Handler: bus.WithDedupe(core, "dedup", func(context.Context, bus.Envelope) error {
			handled.Add(1)
			return nil
		}),
	}))

	env := envelope(t, "t.dedup.v1")
	require.NoError(t, b.Publish(ctx, env))
	require.NoError(t, b.Publish(ctx, env), "same event_id twice — dispatcher crash between confirm and update (PRD §7.9.3)")

	require.Eventually(t, func() bool { return handled.Load() >= 1 }, 15*time.Second, 50*time.Millisecond)
	time.Sleep(time.Second)
	require.EqualValues(t, 1, handled.Load(), "consumer dedupes on event_id (R35)")
}
