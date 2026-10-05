package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"

	"myapp/internal/platform/outbox"
	"myapp/internal/shared/errs"
	"myapp/internal/shared/id"
)

// resetOutbox empties both tables: the package shares one database, and
// these tests assert on counts.
func resetOutbox(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`TRUNCATE outbox_svc.dead_letters, outbox_svc.outbox_events`)
	require.NoError(t, err)
}

// R23: a dead letter parked by the dispatcher is visible in SQL and can be
// put back into the outbox for another attempt.
func TestDeadLettersParkListReplay(t *testing.T) {
	_, pool := setupOutbox(t)
	resetOutbox(t, pool)
	dls := outbox.NewDeadLetters(pool, nil)
	ctx := context.Background()

	eventID := id.NewID()
	require.NoError(t, dls.Park(ctx, outbox.DeadLetter{
		Source:     outbox.SourceDispatcher,
		EventID:    eventID,
		Topic:      "t.dead.v1",
		Payload:    []byte(`{"k":"v"}`),
		Headers:    []byte(`{}`),
		Error:      "nacked by broker",
		Attempts:   10,
		OccurredAt: time.Now().UTC(),
	}))

	list, err := dls.List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, outbox.SourceDispatcher, list[0].Source)
	require.Equal(t, "t.dead.v1", list[0].Topic)
	require.Equal(t, "nacked by broker", list[0].Error)
	require.Equal(t, 10, list[0].Attempts)
	require.Nil(t, list[0].ReplayedAt)

	replayed, err := dls.Replay(ctx, list[0].ID)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, 1, unpublishedCount(t, pool), "replay re-inserts the row for the dispatcher")

	var attempts int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT attempts FROM outbox_svc.outbox_events WHERE event_id = $1`, eventID).Scan(&attempts))
	require.Zero(t, attempts, "a replayed row starts a fresh attempt count")

	list, err = dls.List(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, list, "replayed rows leave the list")

	again, err := dls.Replay(ctx, replayed1ID(t, dls, ctx))
	require.NoError(t, err)
	require.False(t, again, "a second replay of the same row is a no-op")
}

// replayed1ID returns the id of the single row in the table, replayed or not.
func replayed1ID(t *testing.T, dls *outbox.DeadLetters, ctx context.Context) int64 {
	t.Helper()
	all, err := dls.ListAll(ctx, 10)
	require.NoError(t, err)
	require.Len(t, all, 1)
	return all[0].ID
}

// Consumer-side rows are a queryable mirror of the broker DLQ; the broker
// stays the durable park, so replaying them from the table is refused.
func TestDeadLettersParkDeliveryIsNotReplayable(t *testing.T) {
	_, pool := setupOutbox(t)
	resetOutbox(t, pool)
	dls := outbox.NewDeadLetters(pool, nil)
	ctx := context.Background()

	d := amqp.Delivery{
		MessageId:  id.NewID(),
		RoutingKey: "t.poison.v1",
		Body:       []byte(`{"event_id":"x"}`),
		Headers:    amqp.Table{"x-attempt": int32(3)},
	}
	require.NoError(t, dls.ParkDelivery(ctx, "evt.g.t.poison.v1", d, 3, errors.New("cannot process")))

	list, err := dls.List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, outbox.SourceConsumer, list[0].Source)
	require.Equal(t, "evt.g.t.poison.v1", list[0].Queue)
	require.Equal(t, "t.poison.v1", list[0].Topic)
	require.Equal(t, 3, list[0].Attempts)
	require.Contains(t, list[0].Error, "cannot process")

	_, err = dls.Replay(ctx, list[0].ID)
	require.Equal(t, errs.Invalid, errs.KindOf(err), "consumer rows replay from the broker DLQ, not the table")
	require.Zero(t, unpublishedCount(t, pool))
}

// ReplayAll is the bulk path behind `task outbox:deadletter:replay ALL=true`:
// dispatcher rows only, optionally narrowed to one topic.
func TestDeadLettersReplayAllHonoursTopicAndSkipsConsumerRows(t *testing.T) {
	_, pool := setupOutbox(t)
	resetOutbox(t, pool)
	dls := outbox.NewDeadLetters(pool, nil)
	ctx := context.Background()

	park := func(topic string) {
		require.NoError(t, dls.Park(ctx, outbox.DeadLetter{
			Source: outbox.SourceDispatcher, EventID: id.NewID(), Topic: topic,
			Payload: []byte(`{}`), Headers: []byte(`{}`), Error: "x", Attempts: 1, OccurredAt: time.Now(),
		}))
	}
	park("t.a.v1")
	park("t.a.v1")
	park("t.b.v1")
	require.NoError(t, dls.ParkDelivery(ctx, "evt.g.t.a.v1",
		amqp.Delivery{MessageId: id.NewID(), RoutingKey: "t.a.v1", Body: []byte(`{}`)}, 1, errors.New("poison")))

	n, err := dls.ReplayAll(ctx, "t.a.v1")
	require.NoError(t, err)
	require.EqualValues(t, 2, n, "both dispatcher rows for the topic, not the consumer row")
	require.Equal(t, 2, unpublishedCount(t, pool))

	n, err = dls.ReplayAll(ctx, "")
	require.NoError(t, err)
	require.EqualValues(t, 1, n, "empty topic means every remaining dispatcher row")
	require.Equal(t, 3, unpublishedCount(t, pool))

	left, err := dls.List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, left, 1)
	require.Equal(t, outbox.SourceConsumer, left[0].Source, "consumer rows are never replayed from the table")
}

// Unreplayed rows are kept forever; replayed rows age out with the outbox.
func TestDeadLettersPruneRemovesOnlyReplayedRows(t *testing.T) {
	_, pool := setupOutbox(t)
	resetOutbox(t, pool)
	dls := outbox.NewDeadLetters(pool, nil)
	ctx := context.Background()

	for range 2 {
		require.NoError(t, dls.Park(ctx, outbox.DeadLetter{
			Source: outbox.SourceDispatcher, EventID: id.NewID(), Topic: "t.prune.v1",
			Payload: []byte(`{}`), Headers: []byte(`{}`), Error: "x", Attempts: 1, OccurredAt: time.Now(),
		}))
	}
	list, err := dls.List(ctx, 10)
	require.NoError(t, err)
	require.Len(t, list, 2)

	_, err = dls.Replay(ctx, list[0].ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE outbox_svc.dead_letters SET replayed_at = now() - interval '8 days' WHERE id = $1`, list[0].ID)
	require.NoError(t, err)

	n, err := dls.Prune(ctx, 7*24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	all, err := dls.ListAll(ctx, 10)
	require.NoError(t, err)
	require.Len(t, all, 1, "the unreplayed row is never pruned")
	require.Nil(t, all[0].ReplayedAt)
}
