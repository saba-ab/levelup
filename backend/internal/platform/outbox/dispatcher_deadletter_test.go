package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/rabbit"
	"levelup/internal/platform/rabbit/rabbittest"
)

const (
	stallTopic = "t.stall.v1"
	stallQueue = "evt.stall.t.stall.v1"
	flowTopic  = "t.flow.v1"
)

// stallStack declares a queue for stallTopic that holds exactly one message
// and nacks the next publish, plus a normal queue for flowTopic. The stall
// queue is classic on purpose: classic queues enforce x-max-length exactly,
// which makes the nack deterministic; nothing in this package calls
// Topology.Verify.
func stallStack(t *testing.T) *rabbit.Conn {
	t.Helper()
	ctx := context.Background()
	eps := rabbittest.Get(t)
	conn, err := rabbit.Dial(eps.AMQPURL, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	topo := rabbit.NewTopology(conn, rabbit.MgmtConfig{
		URL: eps.MgmtURL, User: eps.User, Password: eps.Password,
	}, zap.NewNop())
	require.NoError(t, topo.DeclareCore(ctx))
	require.NoError(t, topo.DeclareEventQueue(ctx, "flow", flowTopic))

	ch, err := conn.Channel()
	require.NoError(t, err)
	defer ch.Close()
	_, err = ch.QueueDelete(stallQueue, false, false, false)
	require.NoError(t, err)
	_, err = ch.QueueDeclare(stallQueue, true, false, false, false, amqp.Table{
		"x-max-length": int32(1),
		"x-overflow":   "reject-publish",
	})
	require.NoError(t, err)
	require.NoError(t, ch.QueueBind(stallQueue, stallTopic, rabbit.ExchangeEvents, false, nil))
	return conn
}

func publishOne(t *testing.T, gdb *gorm.DB, topic string) {
	t.Helper()
	store := outbox.NewPostgres(clock.System())
	require.NoError(t, postgres.InTx(context.Background(), gdb, func(tx *gorm.DB) error {
		return store.Publish(context.Background(), tx, topic, map[string]string{"k": "v"})
	}))
}

type unpublishedRow struct {
	Topic     string
	Attempts  int
	InBackoff bool
}

func unpublishedRows(t *testing.T, pool *pgxpool.Pool) []unpublishedRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT topic, attempts, next_attempt_at > now()
		FROM outbox_svc.outbox_events WHERE published_at IS NULL ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()
	var out []unpublishedRow
	for rows.Next() {
		var r unpublishedRow
		require.NoError(t, rows.Scan(&r.Topic, &r.Attempts, &r.InBackoff))
		out = append(out, r)
	}
	return out
}

// R23: one topic's full queue must not block every other topic's rows. The
// nacked row is retried later with backoff; the rest of the batch publishes.
func TestNackedRowBacksOffWithoutStallingOtherTopics(t *testing.T) {
	gdb, pool := setupOutbox(t)
	resetOutbox(t, pool)
	conn := stallStack(t)
	ctx := context.Background()

	publishOne(t, gdb, stallTopic) // fills the one-slot queue
	publishOne(t, gdb, stallTopic) // nacked
	publishOne(t, gdb, flowTopic)  // must not be stuck behind the nack

	d := outbox.NewDispatcher(pool, conn, zap.NewNop(), nil)
	n, err := d.Tick(ctx)
	require.NoError(t, err, "a per-message nack is not a tick failure")
	require.Equal(t, 2, n, "the first stall row and the flow row publish")

	left := unpublishedRows(t, pool)
	require.Equal(t, []unpublishedRow{{Topic: stallTopic, Attempts: 1, InBackoff: true}}, left)

	n, err = d.Tick(ctx)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Equal(t, left, unpublishedRows(t, pool), "a row in backoff is not retried on the next tick")
}

// R23: after the attempt cap the row leaves outbox_events for dead_letters,
// and replaying it puts it back once the queue has room.
func TestRowExhaustingPublishAttemptsIsDeadLettered(t *testing.T) {
	gdb, pool := setupOutbox(t)
	resetOutbox(t, pool)
	conn := stallStack(t)
	ctx := context.Background()

	publishOne(t, gdb, stallTopic)
	publishOne(t, gdb, stallTopic)

	d := outbox.NewDispatcher(pool, conn, zap.NewNop(), nil, outbox.WithMaxPublishAttempts(1))
	n, err := d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Empty(t, unpublishedRows(t, pool), "the exhausted row is no longer in the outbox")

	parked, err := d.DeadLetters().List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, parked, 1)
	require.Equal(t, outbox.SourceDispatcher, parked[0].Source)
	require.Equal(t, stallTopic, parked[0].Topic)
	require.Equal(t, 1, parked[0].Attempts)
	require.Contains(t, parked[0].Error, "nack")

	// Make room, replay, and the row publishes on the next tick.
	ch, err := conn.Channel()
	require.NoError(t, err)
	_, err = ch.QueuePurge(stallQueue, false)
	require.NoError(t, err)
	ch.Close()

	replayed, err := d.DeadLetters().Replay(ctx, parked[0].ID)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Len(t, unpublishedRows(t, pool), 1)

	n, err = d.Tick(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Empty(t, unpublishedRows(t, pool))
	require.Eventually(t, func() bool {
		l, err := d.DeadLetters().List(ctx, 10)
		return err == nil && len(l) == 0
	}, time.Second, 50*time.Millisecond)
}
