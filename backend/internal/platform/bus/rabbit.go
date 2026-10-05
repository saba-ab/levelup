package bus

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	"levelup/internal/platform/rabbit"
	"levelup/internal/shared/errs"
)

// prefetch is explicit always: the unlimited default lets one consumer
// buffer the whole queue while the others idle (PRD §7.9.2).
const prefetch = 32

// Rabbit is the production Bus. In the serving path only its CONSUME side is
// used — the outbox dispatcher is the only publisher (PRD §4 P3); Publish
// exists for tests and tooling.
type Rabbit struct {
	conn *rabbit.Conn
	log  *zap.Logger
	sink rabbit.DeadLetterSink
}

type Option func(*Rabbit)

// WithDeadLetterSink mirrors every parked delivery into the sink (R23).
func WithDeadLetterSink(s rabbit.DeadLetterSink) Option {
	return func(b *Rabbit) { b.sink = s }
}

func NewRabbit(conn *rabbit.Conn, log *zap.Logger, opts ...Option) *Rabbit {
	b := &Rabbit{conn: conn, log: log}
	for _, o := range opts {
		o(b)
	}
	return b
}

func (b *Rabbit) consumeOpts() []rabbit.ConsumeOption {
	if b.sink == nil {
		return nil
	}
	return []rabbit.ConsumeOption{rabbit.WithDeadLetterSink(b.sink)}
}

func (b *Rabbit) Publish(ctx context.Context, e Envelope) error {
	ch, err := b.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if err := ch.Confirm(false); err != nil {
		return errs.Wrap(errs.Unavailable, "confirm mode", err)
	}
	body, err := json.Marshal(e)
	if err != nil {
		return errs.Wrap(errs.Internal, "marshal envelope", err)
	}
	dc, err := ch.PublishWithDeferredConfirmWithContext(ctx, rabbit.ExchangeEvents, e.Topic, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    e.EventID,
			Body:         body,
		})
	if err != nil {
		return errs.Wrap(errs.Unavailable, "publish", err)
	}
	if ok, err := dc.WaitContext(ctx); err != nil || !ok {
		return errs.Wrap(errs.Unavailable, "await confirm", err)
	}
	return nil
}

// Subscribe consumes evt.<group>.<topic> until ctx ends, through the shared
// consumer loop (manual ack, bounded retry, DLQ — rabbit.Consume). Each
// delivery joins the trace the envelope carries and opens a consumer span,
// so one trace ID runs HTTP → outbox → dispatcher → consumer (R39).
func (b *Rabbit) Subscribe(ctx context.Context, s Subscription) error {
	queue := rabbit.EventQueue(s.Group, s.Topic)
	tracer := otel.Tracer("levelup/bus")
	go rabbit.Consume(ctx, b.conn, queue, prefetch, b.log,
		func(ctx context.Context, d amqp.Delivery) error {
			var env Envelope
			if err := json.Unmarshal(d.Body, &env); err != nil {
				return rabbit.ErrDeadLetter // no retry can fix an undecodable body
			}
			hctx, span := tracer.Start(env.ExtractTrace(ctx), "consume "+env.Topic)
			defer span.End()
			err := s.Handler(hctx, env)
			if err != nil {
				span.RecordError(err)
			}
			return err
		}, b.consumeOpts()...)
	return nil
}
