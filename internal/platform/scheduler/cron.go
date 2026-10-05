// Package scheduler is the singleton cron (PRD §7.10): it ENQUEUES and never
// executes. The actual work runs on cmd/worker, which scales horizontally;
// the scheduler stays stateless and tiny. Deployed at replicas:1, but the
// per-entry Redis lock is still mandatory — replicas:1 is a lie during a
// rolling deploy, when old and new pods overlap for seconds.
package scheduler

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"myapp/internal/platform/jobs"
	"myapp/internal/platform/redis"
)

const (
	lockTTL = 5 * time.Minute
	// Enqueueing takes milliseconds; the timeout is deliberately far below
	// lockTTL so the safe direction (skip) wins over a lease-extending
	// watchdog (PRD §7.8.3).
	tickTimeout = 30 * time.Second
)

type Scheduler struct {
	cron  *cron.Cron
	locks *redis.Locker
	queue jobs.Queue
	log   *zap.Logger
}

func New(locks *redis.Locker, queue jobs.Queue, log *zap.Logger) *Scheduler {
	return &Scheduler{
		cron: cron.New(
			cron.WithLocation(time.UTC), // "3am daily" must not move twice a year
			cron.WithChain(cron.Recover(cronLogger{log})),
		),
		locks: locks,
		queue: queue,
		log:   log,
	}
}

// Register adds one cron entry that enqueues the job on each tick — if this
// instance wins the tick's lock. The lock key carries the tick timestamp and
// is deliberately never released: releasing right after a millisecond-long
// enqueue would let a peer that wakes 50ms later win the "free" lock and
// enqueue the same tick twice. Per-tick keys expire on their own, and the
// next tick is a fresh key, so interval length never matters.
func (s *Scheduler) Register(j jobs.Job) error {
	if j.Schedule == "" {
		return nil // pure queue consumer; nothing to schedule
	}
	_, err := s.cron.AddFunc(j.Schedule, func() {
		ctx, cancel := context.WithTimeout(context.Background(), tickTimeout)
		defer cancel()

		tick := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
		_, err := s.locks.Acquire(ctx, "lock:cron:"+j.Name+":"+tick, lockTTL)
		if err != nil {
			// Won by a peer, or Redis unreachable: both mean do not run
			// (fail closed, R43). A missed reconciling tick self-heals (R47).
			s.log.Debug("cron tick skipped", zap.String("job", j.Name), zap.Error(err))
			return
		}

		if err := s.queue.Enqueue(ctx, j.Name, nil, jobs.WithPriority(j.Priority)); err != nil {
			s.log.Error("cron enqueue failed", zap.String("job", j.Name), zap.Error(err))
		}
	})
	return err
}

func (s *Scheduler) Start() { s.cron.Start() }

func (s *Scheduler) Stop(ctx context.Context) {
	stopped := s.cron.Stop()
	select {
	case <-stopped.Done():
	case <-ctx.Done():
	}
}

type cronLogger struct{ log *zap.Logger }

func (c cronLogger) Info(msg string, kv ...any) { c.log.Sugar().Infow(msg, kv...) }
func (c cronLogger) Error(err error, msg string, kv ...any) {
	c.log.Sugar().Errorw(msg, append(kv, "error", err)...)
}
