package jobs

import (
	"context"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"myapp/internal/platform/rabbit"
)

const (
	jobPrefetch = 8
	// Priority queues only order what is already buffered: a high prefetch
	// pulls 100 low-priority messages into the local buffer before the
	// urgent one arrives (PRD §7.9). Small prefetch is the price of
	// priority working at all.
	priorityPrefetch = 5

	defaultJobTimeout = 5 * time.Minute
)

// Worker consumes job queues for every module Job it is given, plus the one
// shared priority queue. Panic recovery, bounded retries and DLQ come from
// the shared consumer loop.
type Worker struct {
	conn *rabbit.Conn
	log  *zap.Logger
	sink rabbit.DeadLetterSink
}

type WorkerOption func(*Worker)

// WithDeadLetterSink mirrors every parked job into the sink (R23).
func WithDeadLetterSink(s rabbit.DeadLetterSink) WorkerOption {
	return func(w *Worker) { w.sink = s }
}

func NewWorker(conn *rabbit.Conn, log *zap.Logger, opts ...WorkerOption) *Worker {
	w := &Worker{conn: conn, log: log}
	for _, o := range opts {
		o(w)
	}
	return w
}

func (w *Worker) consumeOpts() []rabbit.ConsumeOption {
	if w.sink == nil {
		return nil
	}
	return []rabbit.ConsumeOption{rabbit.WithDeadLetterSink(w.sink)}
}

// Run starts one consumer per job queue and the priority consumer, then
// blocks until ctx ends.
func (w *Worker) Run(ctx context.Context, jobs []Job) {
	byName := make(map[string]Job, len(jobs))
	for _, j := range jobs {
		byName[j.Name] = j
		go rabbit.Consume(ctx, w.conn, rabbit.JobQueue(j.Name), jobPrefetch, w.log, w.runner(j), w.consumeOpts()...)
	}

	// Priority consumer dispatches by the x-job header.
	go rabbit.Consume(ctx, w.conn, rabbit.PriorityQueue, priorityPrefetch, w.log,
		func(ctx context.Context, d amqp.Delivery) error {
			name, _ := d.Headers[jobNameHeader].(string)
			j, ok := byName[name]
			if !ok {
				w.log.Error("priority job with no registered handler", zap.String("job", name))
				return rabbit.ErrDeadLetter
			}
			return w.execute(ctx, j, d.Body)
		}, w.consumeOpts()...)

	<-ctx.Done()
}

func (w *Worker) runner(j Job) func(ctx context.Context, d amqp.Delivery) error {
	return func(ctx context.Context, d amqp.Delivery) error {
		return w.execute(ctx, j, d.Body)
	}
}

func (w *Worker) execute(ctx context.Context, j Job, payload []byte) error {
	ctx, cancel := context.WithTimeout(ctx, defaultJobTimeout)
	defer cancel()
	start := time.Now()
	err := j.Run(ctx, payload)
	if err == nil {
		w.log.Info("job done", zap.String("job", j.Name), zap.Duration("took", time.Since(start)))
	}
	return err
}
