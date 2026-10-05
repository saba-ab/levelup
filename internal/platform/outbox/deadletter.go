package outbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	amqp "github.com/rabbitmq/amqp091-go"

	"myapp/internal/shared/errs"
)

// DeadLetterSource says which side parked a row: the dispatcher (a row the
// broker refused too many times) or a consumer (a delivery the handler could
// not process and the loop sent to the broker DLQ).
type DeadLetterSource string

const (
	SourceDispatcher DeadLetterSource = "dispatcher"
	SourceConsumer   DeadLetterSource = "consumer"
)

type DeadLetter struct {
	ID         int64
	Source     DeadLetterSource
	EventID    string
	Topic      string
	Queue      string // consumer rows only: the queue the delivery came from
	Payload    []byte
	Headers    []byte
	Error      string
	Attempts   int
	OccurredAt time.Time
	DeadAt     time.Time
	ReplayedAt *time.Time
}

// DeadLetters is the R23 table, outbox_svc.dead_letters: the one queryable
// place for everything that could not be delivered, from either side of the
// broker. Dispatcher rows are replayable from here; consumer rows are a
// mirror of the broker DLQ, which stays the durable park for them.
type DeadLetters struct {
	pool   *pgxpool.Pool
	parked *prometheus.CounterVec
}

// NewDeadLetters registers outbox_dead_letters_total{source} on reg when
// given. One instance per process: a second registration panics.
func NewDeadLetters(pool *pgxpool.Pool, reg *prometheus.Registry) *DeadLetters {
	d := &DeadLetters{
		pool: pool,
		parked: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbox_dead_letters_total",
			Help: "Rows parked in outbox_svc.dead_letters. Alert on any positive rate (R23).",
		}, []string{"source"}),
	}
	if reg != nil {
		reg.MustRegister(d.parked)
	}
	return d
}

// execer is satisfied by *pgxpool.Pool and pgx.Tx, so parking can join the
// dispatcher's tick transaction or run standalone from a consumer.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Park records one dead letter outside any transaction.
func (d *DeadLetters) Park(ctx context.Context, dl DeadLetter) error {
	return d.park(ctx, d.pool, dl)
}

func (d *DeadLetters) park(ctx context.Context, q execer, dl DeadLetter) error {
	var eventID, queue *string
	if dl.EventID != "" {
		eventID = &dl.EventID
	}
	if dl.Queue != "" {
		queue = &dl.Queue
	}
	headers := string(dl.Headers)
	if headers == "" {
		headers = "{}"
	}
	_, err := q.Exec(ctx, `
		INSERT INTO outbox_svc.dead_letters
			(source, event_id, topic, queue, payload, headers, error, attempts, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		string(dl.Source), eventID, dl.Topic, queue, string(dl.Payload), headers,
		dl.Error, dl.Attempts, dl.OccurredAt)
	if err != nil {
		return errs.Wrap(errs.Internal, "park dead letter", err)
	}
	d.parked.WithLabelValues(string(dl.Source)).Inc()
	return nil
}

// ParkDelivery satisfies rabbit.DeadLetterSink: called by the consumer loop
// right before it nacks a delivery to the broker DLQ. Job payloads are not
// necessarily JSON, so a non-JSON body is wrapped rather than rejected.
func (d *DeadLetters) ParkDelivery(ctx context.Context, queue string, del amqp.Delivery, attempts int, cause error) error {
	payload := del.Body
	if !json.Valid(payload) {
		payload, _ = json.Marshal(map[string]string{
			"raw_base64": base64.StdEncoding.EncodeToString(del.Body),
		})
	}
	headers, err := json.Marshal(del.Headers)
	if err != nil {
		headers = []byte("{}")
	}
	occurred := del.Timestamp
	if occurred.IsZero() {
		occurred = time.Now()
	}
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	return d.park(ctx, d.pool, DeadLetter{
		Source:     SourceConsumer,
		EventID:    del.MessageId,
		Topic:      del.RoutingKey,
		Queue:      queue,
		Payload:    payload,
		Headers:    headers,
		Error:      msg,
		Attempts:   attempts,
		OccurredAt: occurred,
	})
}

const deadLetterColumns = `id, source, event_id, topic, queue, payload, headers, error, attempts, occurred_at, dead_at, replayed_at`

// List returns rows not yet replayed, newest first.
func (d *DeadLetters) List(ctx context.Context, limit int) ([]DeadLetter, error) {
	return d.query(ctx, `
		SELECT `+deadLetterColumns+` FROM outbox_svc.dead_letters
		WHERE replayed_at IS NULL
		ORDER BY dead_at DESC, id DESC
		LIMIT $1`, limit)
}

// ListAll includes replayed rows, for audit.
func (d *DeadLetters) ListAll(ctx context.Context, limit int) ([]DeadLetter, error) {
	return d.query(ctx, `
		SELECT `+deadLetterColumns+` FROM outbox_svc.dead_letters
		ORDER BY dead_at DESC, id DESC
		LIMIT $1`, limit)
}

func (d *DeadLetters) query(ctx context.Context, sql string, args ...any) ([]DeadLetter, error) {
	rows, err := d.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "list dead letters", err)
	}
	defer rows.Close()

	var out []DeadLetter
	for rows.Next() {
		var (
			dl             DeadLetter
			source         string
			eventID, queue *string
		)
		if err := rows.Scan(&dl.ID, &source, &eventID, &dl.Topic, &queue, &dl.Payload, &dl.Headers,
			&dl.Error, &dl.Attempts, &dl.OccurredAt, &dl.DeadAt, &dl.ReplayedAt); err != nil {
			return nil, errs.Wrap(errs.Internal, "scan dead letter", err)
		}
		dl.Source = DeadLetterSource(source)
		if eventID != nil {
			dl.EventID = *eventID
		}
		if queue != nil {
			dl.Queue = *queue
		}
		out = append(out, dl)
	}
	return out, rows.Err()
}

// Replay puts one dispatcher row back into outbox_events with a fresh
// attempt count and marks it replayed. Returns false when the row is
// missing or already replayed. Consumer rows are refused: the broker DLQ
// holds them, and task rabbit:replay is the path.
func (d *DeadLetters) Replay(ctx context.Context, id int64) (bool, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, errs.Wrap(errs.Unavailable, "dead letter replay begin", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		source, topic, payload, headers string
		eventID                         *string
		occurredAt                      time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT source, event_id, topic, payload::text, headers::text, occurred_at
		FROM outbox_svc.dead_letters
		WHERE id = $1 AND replayed_at IS NULL
		FOR UPDATE`, id).Scan(&source, &eventID, &topic, &payload, &headers, &occurredAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, errs.Wrap(errs.Internal, "dead letter lookup", err)
	}
	if DeadLetterSource(source) != SourceDispatcher {
		return false, errs.New(errs.Invalid,
			"consumer dead letters are parked in the broker DLQ: replay with task rabbit:replay QUEUE=<queue>")
	}
	if eventID == nil {
		return false, errs.New(errs.Internal, "dispatcher dead letter without event_id")
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_svc.outbox_events (event_id, topic, payload, headers, occurred_at)
		VALUES ($1::uuid, $2, $3, $4, $5)`, *eventID, topic, payload, headers, occurredAt); err != nil {
		return false, errs.Wrap(errs.Internal, "dead letter reinsert", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE outbox_svc.dead_letters SET replayed_at = now() WHERE id = $1`, id); err != nil {
		return false, errs.Wrap(errs.Internal, "dead letter mark replayed", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, errs.Wrap(errs.Internal, "dead letter replay commit", err)
	}
	return true, nil
}

// ReplayAll replays every unreplayed dispatcher row, optionally for one
// topic. topic == "" means all topics.
func (d *DeadLetters) ReplayAll(ctx context.Context, topic string) (int64, error) {
	tag, err := d.pool.Exec(ctx, `
		WITH picked AS (
			SELECT id, event_id, topic, payload, headers, occurred_at
			FROM outbox_svc.dead_letters
			WHERE replayed_at IS NULL AND source = 'dispatcher' AND event_id IS NOT NULL
			  AND ($1 = '' OR topic = $1)
			FOR UPDATE SKIP LOCKED
		), reinserted AS (
			INSERT INTO outbox_svc.outbox_events (event_id, topic, payload, headers, occurred_at)
			SELECT event_id::uuid, topic, payload, headers, occurred_at FROM picked
		)
		UPDATE outbox_svc.dead_letters SET replayed_at = now()
		WHERE id IN (SELECT id FROM picked)`, topic)
	if err != nil {
		return 0, errs.Wrap(errs.Internal, "dead letter replay all", err)
	}
	return tag.RowsAffected(), nil
}

// Prune deletes replayed rows older than the window. Unreplayed rows are
// never pruned: they are the ones somebody still has to look at.
func (d *DeadLetters) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := d.pool.Exec(ctx, `
		DELETE FROM outbox_svc.dead_letters
		WHERE replayed_at IS NOT NULL AND replayed_at < $1`, time.Now().Add(-olderThan))
	if err != nil {
		return 0, errs.Wrap(errs.Internal, "dead letter prune", err)
	}
	return tag.RowsAffected(), nil
}
