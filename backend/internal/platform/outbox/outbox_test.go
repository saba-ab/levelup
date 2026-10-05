package outbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	outboxmigrations "levelup/internal/platform/outbox/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/platform/rabbit"
	"levelup/internal/platform/rabbit/rabbittest"
)

func setupOutbox(t *testing.T) (*gorm.DB, *pgxpool.Pool) {
	t.Helper()
	dsn := pgtest.DSN(t)
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "outbox", outboxmigrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 8)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	gdb, err := db.GormBase(nil)
	require.NoError(t, err)
	return gdb, db.Writer()
}

func unpublishedCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM outbox_svc.outbox_events WHERE published_at IS NULL`).Scan(&n))
	return n
}

// R4: publishing inside a tx and rolling back leaves no event.
func TestPublishRollsBackWithTransaction(t *testing.T) {
	gdb, pool := setupOutbox(t)
	store := outbox.NewPostgres(clock.System())
	ctx := context.Background()

	boom := errors.New("boom")
	err := postgres.InTx(ctx, gdb, func(tx *gorm.DB) error {
		if err := store.Publish(ctx, tx, "t.rollback.v1", map[string]string{"k": "v"}); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)
	require.Zero(t, unpublishedCount(t, pool), "rolled-back tx must leave no event (R4)")
}

func TestPublishCapturesLiveTraceContext(t *testing.T) {
	gdb, pool := setupOutbox(t)
	store := outbox.NewPostgres(clock.System())

	otel.SetTextMapPropagator(propagation.TraceContext{})
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	require.NoError(t, postgres.InTx(ctx, gdb, func(tx *gorm.DB) error {
		return store.Publish(ctx, tx, "t.trace.v1", map[string]string{"k": "v"})
	}))

	var headers []byte
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT headers FROM outbox_svc.outbox_events WHERE topic = 't.trace.v1'`).Scan(&headers))
	var h struct {
		TraceCtx map[string]string `json:"trace_ctx"`
	}
	require.NoError(t, json.Unmarshal(headers, &h))
	require.Contains(t, h.TraceCtx["traceparent"], span.SpanContext().TraceID().String(),
		"trace context must be captured at publish time, inside the request (R39)")
}

// R4: two dispatcher replicas never double-deliver the same row.
func TestTwoDispatchersDeliverExactlyOnce(t *testing.T) {
	gdb, pool := setupOutbox(t)
	store := outbox.NewPostgres(clock.System())
	ctx := context.Background()

	eps := rabbittest.Get(t)
	conn, err := rabbit.Dial(eps.AMQPURL, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	topo := rabbit.NewTopology(conn, rabbit.MgmtConfig{
		URL: eps.MgmtURL, User: eps.User, Password: eps.Password,
	}, zap.NewNop())
	require.NoError(t, topo.DeclareCore(ctx))
	require.NoError(t, topo.DeclareEventQueue(ctx, "probe", "t.race.v1"))

	// Count deliveries per event_id straight off the queue.
	ch, err := conn.Channel()
	require.NoError(t, err)
	require.NoError(t, ch.Qos(500, 0, false))
	deliveries, err := ch.Consume(rabbit.EventQueue("probe", "t.race.v1"), "", false, false, false, false, nil)
	require.NoError(t, err)

	var mu sync.Mutex
	seen := map[string]int{}
	go func() {
		for d := range deliveries {
			var env bus.Envelope
			_ = json.Unmarshal(d.Body, &env)
			mu.Lock()
			seen[env.EventID]++
			mu.Unlock()
			_ = d.Ack(false)
		}
	}()

	const total = 500
	require.NoError(t, postgres.InTx(ctx, gdb, func(tx *gorm.DB) error {
		for i := 0; i < total; i++ {
			if err := store.Publish(ctx, tx, "t.race.v1", map[string]int{"i": i}); err != nil {
				return err
			}
		}
		return nil
	}))

	d1 := outbox.NewDispatcher(pool, conn, zap.NewNop(), nil)
	d2 := outbox.NewDispatcher(pool, conn, zap.NewNop(), nil)
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	for _, d := range []*outbox.Dispatcher{d1, d2} {
		wg.Add(1)
		go func() { defer wg.Done(); _ = d.Run(runCtx) }()
	}

	require.Eventually(t, func() bool { return unpublishedCount(t, pool) == 0 },
		30*time.Second, 100*time.Millisecond, "outbox must drain")
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(seen) == total
	}, 30*time.Second, 100*time.Millisecond, "all events must arrive")

	cancel()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	for id, n := range seen {
		require.Equal(t, 1, n, "event %s delivered %d times — SKIP LOCKED must prevent double delivery (R4)", id, n)
	}
}

func TestReplayClearsPublishedAt(t *testing.T) {
	gdb, pool := setupOutbox(t)
	store := outbox.NewPostgres(clock.System())
	ctx := context.Background()

	require.NoError(t, postgres.InTx(ctx, gdb, func(tx *gorm.DB) error {
		return store.Publish(ctx, tx, "t.replay.v1", map[string]string{"k": "v"})
	}))
	_, err := pool.Exec(ctx, `UPDATE outbox_svc.outbox_events SET published_at = now() WHERE topic = 't.replay.v1'`)
	require.NoError(t, err)

	d := outbox.NewDispatcher(pool, nil, zap.NewNop(), nil)
	n, err := d.Replay(ctx, time.Now().Add(-time.Hour), "t.replay.v1")
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.Equal(t, 1, unpublishedCount(t, pool), "replayed rows drain on the next tick (R49)")
}
