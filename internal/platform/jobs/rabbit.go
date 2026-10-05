package jobs

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"myapp/internal/platform/rabbit"
	"myapp/internal/shared/errs"
)

const jobNameHeader = "x-job"

// RabbitQueue is the production Queue (direct exchange `jobs`). Reminder of
// the contract (PRD §7.9.2 Layer 4): a direct Enqueue is fire-and-forget —
// state-change-triggered work routes through the outbox as topic
// "job.<name>" instead (R46).
type RabbitQueue struct {
	conn *rabbit.Conn
	log  *zap.Logger
}

func NewRabbitQueue(conn *rabbit.Conn, log *zap.Logger) *RabbitQueue {
	return &RabbitQueue{conn: conn, log: log}
}

func (q *RabbitQueue) Enqueue(ctx context.Context, name string, payload []byte, opts ...EnqueueOpt) error {
	var o EnqueueOpts
	for _, opt := range opts {
		opt(&o)
	}

	routingKey := name
	if o.Priority > 0 {
		// The single classic-queue exception (PRD §7.9): everything with a
		// priority shares job.priority; the worker dispatches by header.
		routingKey = "priority"
	}

	ch, err := q.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if err := ch.Confirm(false); err != nil {
		return errs.Wrap(errs.Unavailable, "confirm mode", err)
	}
	dc, err := ch.PublishWithDeferredConfirmWithContext(ctx, rabbit.ExchangeJobs, routingKey, false, false,
		amqp.Publishing{
			ContentType:  "application/octet-stream",
			DeliveryMode: amqp.Persistent,
			Priority:     o.Priority,
			Headers:      amqp.Table{jobNameHeader: name},
			Body:         payload,
		})
	if err != nil {
		return errs.Wrap(errs.Unavailable, "enqueue "+name, err)
	}
	if ok, err := dc.WaitContext(ctx); err != nil || !ok {
		return errs.Wrap(errs.Unavailable, "await confirm for "+name, err)
	}
	return nil
}
