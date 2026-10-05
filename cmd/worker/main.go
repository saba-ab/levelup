// Command worker consumes events and jobs for every enabled module (PRD §5).
// Scales horizontally; all retry/DLQ semantics live in the platform consumer.
//
// Subcommands:
//
//	(none)      run consumers
//	replay-dlq  --queue=<source queue>  shovel its DLQ back (R49)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"myapp/internal/app"
	"myapp/internal/config"
	"myapp/internal/platform/bus"
	"myapp/internal/platform/httpx"
	"myapp/internal/platform/jobs"
	"myapp/internal/platform/outbox"
	"myapp/internal/platform/rabbit"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	a, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer a.Close()

	if err := a.VerifyAuthz(); err != nil {
		return err // stale grants must kill the boot, not surface as 403s (R19)
	}
	log := a.P.Tel.Log

	conn, err := rabbit.Dial(cfg.Rabbit.URL, log, a.P.Tel.Registry)
	if err != nil {
		return err
	}
	defer conn.Close()

	if len(args) > 0 && args[0] == "replay-dlq" {
		return replayDLQ(ctx, conn, args[1:])
	}

	if err := a.DeclareTopology(ctx, a.NewTopology(conn)); err != nil {
		return err
	}

	// Events: every module subscription, deduped on event_id per group
	// (delivery is at-least-once by design — PRD §7.9.3).
	// R23: every delivery the consumer loop parks in a broker DLQ is also
	// written to outbox_svc.dead_letters, the one SQL view of both sides.
	dead := outbox.NewDeadLetters(a.P.DB.Writer(), a.P.Tel.Registry)
	rb := bus.NewRabbit(conn, log, bus.WithDeadLetterSink(dead))
	subs := 0
	var allJobs []jobs.Job
	for _, m := range a.Modules {
		for _, s := range m.Subscriptions() {
			s.Handler = bus.WithDedupe(a.P.RedisCore, s.Group, s.Handler)
			if err := rb.Subscribe(ctx, s); err != nil {
				return err
			}
			subs++
		}
		allJobs = append(allJobs, m.Jobs()...)
	}

	adm := httpx.NewServer(cfg.HTTP.AdminAddr, a.P.Tel.AdminHandler())
	go func() { _ = adm.ListenAndServe() }()
	defer func() {
		drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpx.Shutdown(drain, adm)
	}()

	log.Sugar().Infof("worker running: %d subscriptions, %d job types", subs, len(allJobs))
	jobs.NewWorker(conn, log, jobs.WithDeadLetterSink(dead)).Run(ctx, allJobs) // blocks until ctx ends
	return nil
}

func replayDLQ(ctx context.Context, conn *rabbit.Conn, args []string) error {
	fs := flag.NewFlagSet("replay-dlq", flag.ContinueOnError)
	queue := fs.String("queue", "", "source queue whose DLQ to shovel back (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *queue == "" {
		return fmt.Errorf("replay-dlq: --queue is required")
	}
	moved, err := rabbit.ReplayDLQ(ctx, conn, *queue)
	if err != nil {
		return err
	}
	fmt.Printf("replay-dlq: moved %d messages back onto %s\n", moved, *queue)
	return nil
}
