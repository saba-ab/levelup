package outbox

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	"myapp/internal/platform/bus"
	"myapp/internal/platform/rabbit"
	"myapp/internal/shared/errs"
)

const (
	defaultBatch       = 100
	defaultMaxAttempts = 10
	idleSleepMin       = 50 * time.Millisecond
	idleSleepMax       = 1 * time.Second
	retentionDays      = 7
)

// Dispatcher drains outbox_svc.outbox_events to RabbitMQ. Safe to run N
// replicas with zero coordination: FOR UPDATE SKIP LOCKED partitions the
// unpublished set between them (PRD §8). It never marks a row published
// before the broker CONFIRMS it (R34): every failure path leaves
// published_at NULL and the row drains on a later tick — with backoff after
// a nack, and after the attempt cap by way of dead_letters, where a row an
// operator replays re-enters the outbox (R23). Nothing is ever dropped.
type Dispatcher struct {
	pool        *pgxpool.Pool // WRITER — SKIP LOCKED needs the primary
	conn        *rabbit.Conn
	log         *zap.Logger
	batch       int
	maxAttempts int
	dead        *DeadLetters

	unpublishedAge prometheus.Gauge
	published      prometheus.Counter
	failures       prometheus.Counter
}

type Option func(*Dispatcher)

// WithMaxPublishAttempts caps how many times the broker may nack one row
// before it is parked in dead_letters (R23). Default defaultMaxAttempts.
func WithMaxPublishAttempts(n int) Option {
	return func(d *Dispatcher) {
		if n > 0 {
			d.maxAttempts = n
		}
	}
}

func NewDispatcher(pool *pgxpool.Pool, conn *rabbit.Conn, log *zap.Logger, reg *prometheus.Registry, opts ...Option) *Dispatcher {
	d := &Dispatcher{
		pool:        pool,
		conn:        conn,
		log:         log,
		batch:       defaultBatch,
		maxAttempts: defaultMaxAttempts,
		dead:        NewDeadLetters(pool, reg),
		unpublishedAge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_unpublished_age_seconds",
			Help: "Age of the oldest unpublished row. The single best health signal in the system; alert at 60s (R40).",
		}),
		published: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "outbox_published_total", Help: "Rows confirmed and marked published.",
		}),
		failures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "outbox_publish_failures_total", Help: "Publish attempts that failed or were nacked.",
		}),
	}
	if reg != nil {
		reg.MustRegister(d.unpublishedAge, d.published, d.failures)
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// DeadLetters exposes the R23 table for the replay and list subcommands.
func (d *Dispatcher) DeadLetters() *DeadLetters { return d.dead }

// Run polls until ctx ends. Exponential idle backoff keeps an empty outbox
// cheap; a busy one is drained batch after batch with no sleep.
func (d *Dispatcher) Run(ctx context.Context) error {
	sleep := idleSleepMin
	lastPrune := time.Time{}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		n, err := d.Tick(ctx)
		switch {
		case err != nil:
			d.failures.Inc()
			d.log.Warn("dispatch tick failed", zap.Error(err))
			sleep = idleSleepMax
		case n > 0:
			sleep = idleSleepMin
		default:
			sleep = min(sleep*2, idleSleepMax)
		}
		d.observeAge(ctx)

		if time.Since(lastPrune) > time.Hour {
			d.prune(ctx)
			lastPrune = time.Now()
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(sleep):
		}
	}
}

type row struct {
	ID         int64
	EventID    string
	Topic      string
	Payload    []byte
	Headers    []byte
	OccurredAt time.Time
	Attempts   int
}

// refusal is one row the broker refused in this batch, or one that cannot
// be encoded. The tick records the attempt with backoff, or parks it.
type refusal struct {
	row       row
	cause     error
	permanent bool
}

// Tick claims one batch, publishes with confirms, marks only confirmed rows.
// Rows in backoff after a nack are left for a later tick. Exported for tests
// and for the replay path.
func (d *Dispatcher) Tick(ctx context.Context) (int, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return 0, errs.Wrap(errs.Unavailable, "outbox begin", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, event_id, topic, payload, headers, occurred_at, attempts
		FROM outbox_svc.outbox_events
		WHERE published_at IS NULL AND next_attempt_at <= now()
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, d.batch)
	if err != nil {
		return 0, errs.Wrap(errs.Internal, "outbox select", err)
	}
	batch, err := pgx.CollectRows(rows, pgx.RowToStructByPos[row])
	if err != nil {
		return 0, errs.Wrap(errs.Internal, "outbox scan", err)
	}
	if len(batch) == 0 {
		return 0, tx.Commit(ctx)
	}

	confirmed, refused, err := d.publishBatch(ctx, batch)
	if len(confirmed) > 0 {
		// Mark what the broker accepted even if a later row failed —
		// never re-deliver what was already confirmed just because a
		// neighbour was nacked.
		if _, uerr := tx.Exec(ctx, `
			UPDATE outbox_svc.outbox_events
			SET published_at = now()
			WHERE id = ANY($1)`, confirmed); uerr != nil {
			return 0, errs.Wrap(errs.Internal, "outbox mark published", uerr)
		}
	}
	for _, r := range refused {
		if rerr := d.recordRefusal(ctx, tx, r); rerr != nil {
			return 0, rerr
		}
	}
	if len(confirmed)+len(refused) > 0 {
		if cerr := tx.Commit(ctx); cerr != nil {
			return 0, errs.Wrap(errs.Internal, "outbox commit", cerr)
		}
		d.published.Add(float64(len(confirmed)))
	}
	return len(confirmed), err
}

// recordRefusal is the R23 policy: a refused row backs off and is retried,
// until the attempt cap moves it into dead_letters in the same transaction
// that removes it from the outbox. Nothing is lost either way; the row is
// either still here or parked where an operator can see and replay it.
func (d *Dispatcher) recordRefusal(ctx context.Context, tx pgx.Tx, r refusal) error {
	attempts := r.row.Attempts + 1
	d.failures.Inc()

	if r.permanent || attempts >= d.maxAttempts {
		if err := d.dead.park(ctx, tx, DeadLetter{
			Source:     SourceDispatcher,
			EventID:    r.row.EventID,
			Topic:      r.row.Topic,
			Payload:    r.row.Payload,
			Headers:    r.row.Headers,
			Error:      r.cause.Error(),
			Attempts:   attempts,
			OccurredAt: r.row.OccurredAt,
		}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM outbox_svc.outbox_events WHERE id = $1`, r.row.ID); err != nil {
			return errs.Wrap(errs.Internal, "outbox remove dead-lettered row", err)
		}
		d.log.Error("outbox row dead-lettered",
			zap.Int64("id", r.row.ID), zap.String("event_id", r.row.EventID),
			zap.String("topic", r.row.Topic), zap.Int("attempts", attempts), zap.Error(r.cause))
		return nil
	}

	delay := publishBackoff(attempts)
	if _, err := tx.Exec(ctx, `
		UPDATE outbox_svc.outbox_events
		SET attempts = $2, next_attempt_at = now() + make_interval(secs => $3)
		WHERE id = $1`, r.row.ID, attempts, delay.Seconds()); err != nil {
		return errs.Wrap(errs.Internal, "outbox record refusal", err)
	}
	d.log.Warn("outbox row refused by broker, backing off",
		zap.Int64("id", r.row.ID), zap.String("topic", r.row.Topic),
		zap.Int("attempts", attempts), zap.Duration("retry_in", delay), zap.Error(r.cause))
	return nil
}

// publishBackoff doubles from one second and caps at one minute: ten
// attempts span roughly four minutes before a row is parked.
func publishBackoff(attempts int) time.Duration {
	shift := min(attempts-1, 6)
	return min(time.Second<<shift, time.Minute)
}

// publishBatch awaits a broker confirm per message (PRD §7.9.2 Layer 2).
// Without confirms, basic.publish is fire-and-forget and a "published" row
// may never have reached the broker — silent, permanent loss.
//
// Three outcomes per row: confirmed; refused (a per-message nack, or a row
// that cannot be encoded) which is recorded and does not stop the batch; or
// a channel-level failure, which aborts the batch because every row after
// it would fail the same way and nothing is marked.
func (d *Dispatcher) publishBatch(ctx context.Context, batch []row) (confirmed []int64, refused []refusal, err error) {
	ch, err := d.conn.Channel()
	if err != nil {
		return nil, nil, err
	}
	defer ch.Close()
	if err := ch.Confirm(false); err != nil {
		return nil, nil, errs.Wrap(errs.Unavailable, "confirm mode", err)
	}

	tracer := otel.Tracer("myapp/outbox")
	confirmed = make([]int64, 0, len(batch))
	for _, ev := range batch {
		env := bus.Envelope{
			EventID:    ev.EventID,
			Topic:      ev.Topic,
			OccurredAt: ev.OccurredAt,
			TraceCtx:   traceFromHeaders(ev.Headers),
			Payload:    ev.Payload,
		}
		// The dispatch span joins the ORIGINATING request's trace via the
		// stored context — this hop is otherwise invisible (R39).
		_, span := tracer.Start(env.ExtractTrace(ctx), "outbox.dispatch "+ev.Topic)

		body, err := json.Marshal(env)
		if err != nil {
			// Will be exactly as unencodable next tick: park it now.
			span.RecordError(err)
			span.End()
			refused = append(refused, refusal{row: ev,
				cause: errs.Wrap(errs.Invalid, "encode envelope", err), permanent: true})
			continue
		}

		exchange, routingKey := routeFor(ev.Topic)
		pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second) // bounded: a blocked broker must not stall forever (R48)
		dc, err := ch.PublishWithDeferredConfirmWithContext(pubCtx, exchange, routingKey, false, false,
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				MessageId:    ev.EventID,
				Timestamp:    ev.OccurredAt,
				Body:         body,
			})
		if err != nil {
			cancel()
			span.RecordError(err)
			span.End()
			return confirmed, refused, errs.Wrap(errs.Unavailable, "publish", err)
		}
		ok, err := dc.WaitContext(pubCtx)
		cancel()
		switch {
		case err != nil:
			// Unknown outcome (timeout, channel gone): leave the remainder
			// for the next tick rather than guess.
			span.RecordError(err)
			span.End()
			return confirmed, refused, errs.Wrap(errs.Unavailable, "await confirm", err)
		case !ok:
			// basic.nack is per message — a full queue with reject-publish
			// (R48). Note it and carry on so one topic cannot stall the
			// others (R23).
			nerr := errs.New(errs.Unavailable, "nacked by broker")
			span.RecordError(nerr)
			span.End()
			refused = append(refused, refusal{row: ev, cause: nerr})
			continue
		}
		span.End()
		confirmed = append(confirmed, ev.ID)
	}
	return confirmed, refused, nil
}

// routeFor: state-change-triggered jobs route through the outbox as topics
// prefixed "job." (R46) and land on the jobs exchange; everything else is an
// event on the topic exchange.
func routeFor(topic string) (exchange, routingKey string) {
	if name, ok := strings.CutPrefix(topic, "job."); ok {
		return rabbit.ExchangeJobs, name
	}
	return rabbit.ExchangeEvents, topic
}

func traceFromHeaders(raw []byte) map[string]string {
	var h struct {
		TraceCtx map[string]string `json:"trace_ctx"`
	}
	if json.Unmarshal(raw, &h) != nil {
		return nil
	}
	return h.TraceCtx
}

func (d *Dispatcher) observeAge(ctx context.Context) {
	var age *float64
	err := d.pool.QueryRow(ctx, `
		SELECT EXTRACT(EPOCH FROM (now() - min(occurred_at)))
		FROM outbox_svc.outbox_events WHERE published_at IS NULL`).Scan(&age)
	if err != nil || age == nil {
		d.unpublishedAge.Set(0)
		return
	}
	d.unpublishedAge.Set(*age)
}

// prune drops rows published more than 7 days ago. The window is what makes
// consumer bugs recoverable by replay instead of archaeology (PRD §7.9.2).
func (d *Dispatcher) prune(ctx context.Context) {
	tag, err := d.pool.Exec(ctx, `
		DELETE FROM outbox_svc.outbox_events
		WHERE published_at < now() - make_interval(days => $1)`, retentionDays)
	if err != nil {
		d.log.Warn("outbox prune failed", zap.Error(err))
		return
	}
	if tag.RowsAffected() > 0 {
		d.log.Info("outbox pruned", zap.Int64("rows", tag.RowsAffected()))
	}
	// Replayed dead letters age out on the same window; unreplayed ones
	// are kept until somebody deals with them.
	n, err := d.dead.Prune(ctx, retentionDays*24*time.Hour)
	if err != nil {
		d.log.Warn("dead letter prune failed", zap.Error(err))
		return
	}
	if n > 0 {
		d.log.Info("dead letters pruned", zap.Int64("rows", n))
	}
}

// Replay clears published_at for matching rows so the poller republishes
// them (R49). topic == "" replays every topic in the window.
func (d *Dispatcher) Replay(ctx context.Context, from time.Time, topic string) (int64, error) {
	tag, err := d.pool.Exec(ctx, `
		UPDATE outbox_svc.outbox_events
		SET published_at = NULL
		WHERE occurred_at >= $1
		  AND published_at IS NOT NULL
		  AND ($2 = '' OR topic = $2)`, from, topic)
	if err != nil {
		return 0, errs.Wrap(errs.Internal, "outbox replay", err)
	}
	return tag.RowsAffected(), nil
}
