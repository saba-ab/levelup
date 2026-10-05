package rabbit

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"levelup/internal/shared/errs"
)

// ErrDeadLetter tells the consume loop a delivery can never succeed
// (undecodable body, unknown job): skip the retry ladder, park it now.
var ErrDeadLetter = errors.New("dead-letter this delivery")

const attemptHeader = "x-attempt"

// DeadLetterSink is told about every delivery the loop parks in the broker
// DLQ, right before the nack (R23). The outbox dead-letter table implements
// it; the broker stays the durable park, the sink is the queryable mirror.
// attempts counts every try including the first.
type DeadLetterSink interface {
	ParkDelivery(ctx context.Context, queue string, d amqp.Delivery, attempts int, cause error) error
}

type ConsumeOption func(*consumeOpts)

type consumeOpts struct {
	sink DeadLetterSink
}

func WithDeadLetterSink(s DeadLetterSink) ConsumeOption {
	return func(o *consumeOpts) { o.sink = s }
}

// Consume runs the shared consumer loop for one queue until ctx ends:
// explicit prefetch, manual ack after success, bounded retry ladder, DLQ,
// channel reconnect with backoff (PRD §7.9.2 Layer 3). Both event and job
// consumers run through here — the semantics exist exactly once.
func Consume(ctx context.Context, conn *Conn, queue string, prefetchN int, log *zap.Logger,
	handle func(ctx context.Context, d amqp.Delivery) error, opts ...ConsumeOption) {

	var o consumeOpts
	for _, opt := range opts {
		opt(&o)
	}

	backoff := time.Second
	for ctx.Err() == nil {
		err := consumeOnce(ctx, conn, queue, prefetchN, log, handle, o)
		if err != nil && ctx.Err() == nil {
			log.Warn("consumer channel lost, reconnecting", zap.String("queue", queue), zap.Error(err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, 30*time.Second)
			}
			continue
		}
		backoff = time.Second
	}
}

func consumeOnce(ctx context.Context, conn *Conn, queue string, prefetchN int, log *zap.Logger,
	handle func(ctx context.Context, d amqp.Delivery) error, o consumeOpts) error {

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if err := ch.Qos(prefetchN, 0, false); err != nil {
		return errs.Wrap(errs.Unavailable, "qos", err)
	}
	deliveries, err := ch.Consume(queue, "", false /* manual ack */, false, false, false, nil)
	if err != nil {
		return errs.Wrap(errs.Unavailable, "consume "+queue, err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return errs.New(errs.Unavailable, "delivery channel closed")
			}
			settle(ctx, ch, queue, log, o.sink, d, safeHandle(ctx, handle, d))
		}
	}
}

// safeHandle wraps the handler in panic recovery: an unrecovered panic in a
// consumer goroutine takes the whole worker down (PRD §7.10).
func safeHandle(ctx context.Context, handle func(context.Context, amqp.Delivery) error, d amqp.Delivery) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errs.New(errs.Internal, "handler panicked")
		}
	}()
	return handle(ctx, d)
}

// settle acks success, parks the unfixable, and climbs the retry ladder for
// the rest — bounded, then DLQ (R35).
func settle(ctx context.Context, ch *amqp.Channel, queue string, log *zap.Logger, sink DeadLetterSink, d amqp.Delivery, herr error) {
	if herr == nil {
		_ = d.Ack(false)
		return
	}
	attempt := attemptOf(d)

	// Unfixable by retrying: explicit dead-letter sentinel, or a payload the
	// handler judged invalid — it will be exactly as invalid in 5 seconds.
	if errors.Is(herr, ErrDeadLetter) || errs.KindOf(herr) == errs.Invalid {
		log.Error("unprocessable delivery — dead-lettering",
			zap.String("queue", queue), zap.String("message_id", d.MessageId), zap.Error(herr))
		park(ctx, log, sink, queue, d, attempt+1, herr)
		return
	}

	log.Warn("handler failed", zap.String("queue", queue),
		zap.String("message_id", d.MessageId), zap.Int("attempt", attempt), zap.Error(herr))

	if attempt >= len(RetryTiers) {
		park(ctx, log, sink, queue, d, attempt+1, herr) // ladder exhausted → per-queue DLX → DLQ
		return
	}

	// Republish into the tier queue; TTL expiry dead-letters it back onto
	// the main queue. If the republish fails, park the original: parked is
	// safe, lost is not.
	headers := amqp.Table{}
	maps.Copy(headers, d.Headers)
	headers[attemptHeader] = int32(attempt + 1)

	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	perr := ch.PublishWithContext(pubCtx, "", queue+".retry."+RetryTiers[attempt].Suffix, false, false,
		amqp.Publishing{
			ContentType:  d.ContentType,
			DeliveryMode: amqp.Persistent,
			MessageId:    d.MessageId,
			Priority:     d.Priority,
			Headers:      headers,
			Body:         d.Body,
		})
	if perr != nil {
		log.Error("retry republish failed — dead-lettering", zap.Error(perr))
		park(ctx, log, sink, queue, d, attempt+1, errors.Join(herr, perr))
		return
	}
	_ = d.Ack(false)
}

// park sends a delivery to the broker DLQ, telling the sink first so the
// dead-letter table mirrors what the broker holds (R23). A sink failure is
// logged and never blocks the nack: parked in the broker is the guarantee,
// the table is the convenience.
func park(ctx context.Context, log *zap.Logger, sink DeadLetterSink, queue string, d amqp.Delivery, attempts int, cause error) {
	if sink != nil {
		if err := sink.ParkDelivery(ctx, queue, d, attempts, cause); err != nil {
			log.Error("dead-letter sink failed — parking in broker only",
				zap.String("queue", queue), zap.String("message_id", d.MessageId), zap.Error(err))
		}
	}
	_ = d.Nack(false, false)
}

func attemptOf(d amqp.Delivery) int {
	switch v := d.Headers[attemptHeader].(type) {
	case int32:
		return int(v)
	case int64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

// ReplayDLQ shovels parked messages back onto their source queue (R49).
// Returns how many were moved. Uses basic.get so it stops cleanly when the
// DLQ is empty.
func ReplayDLQ(ctx context.Context, conn *Conn, sourceQueue string) (int, error) {
	ch, err := conn.Channel()
	if err != nil {
		return 0, err
	}
	defer ch.Close()

	moved := 0
	for ctx.Err() == nil {
		d, ok, err := ch.Get(sourceQueue+".dlq", false)
		if err != nil {
			return moved, errs.Wrap(errs.Unavailable, "dlq get", err)
		}
		if !ok {
			return moved, nil
		}
		headers := amqp.Table{}
		maps.Copy(headers, d.Headers)
		delete(headers, attemptHeader) // fresh ladder after the fix
		if err := ch.PublishWithContext(ctx, "", sourceQueue, false, false, amqp.Publishing{
			ContentType:  d.ContentType,
			DeliveryMode: amqp.Persistent,
			MessageId:    d.MessageId,
			Headers:      headers,
			Body:         d.Body,
		}); err != nil {
			_ = d.Nack(false, true) // leave it parked
			return moved, errs.Wrap(errs.Unavailable, "dlq republish", err)
		}
		_ = d.Ack(false)
		moved++
	}
	return moved, ctx.Err()
}
