// Command scheduler is the singleton cron (PRD §7.10). DEPLOY AT replicas=1.
// It only enqueues — execution happens on cmd/worker — and every entry takes
// a per-tick Redis lock, because replicas:1 is a lie during a rolling deploy.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"levelup/internal/app"
	"levelup/internal/config"
	"levelup/internal/platform/httpx"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/rabbit"
	"levelup/internal/platform/redis"
	"levelup/internal/platform/scheduler"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
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

	if err := a.DeclareTopology(ctx, a.NewTopology(conn)); err != nil {
		return err
	}

	sched := scheduler.New(
		redis.NewLocker(a.P.RedisCore, log),
		jobs.NewRabbitQueue(conn, log),
		log,
	)
	entries := 0
	for _, m := range a.Modules {
		for _, j := range m.Jobs() {
			if j.Schedule == "" {
				continue
			}
			if err := sched.Register(j); err != nil {
				return fmt.Errorf("register cron %q: %w", j.Name, err)
			}
			entries++
		}
	}

	adm := httpx.NewServer(cfg.HTTP.AdminAddr, a.P.Tel.AdminHandler())
	go func() { _ = adm.ListenAndServe() }()
	defer func() {
		drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpx.Shutdown(drain, adm)
	}()

	sched.Start()
	log.Sugar().Infof("scheduler running: %d cron entries (singleton — do not scale)", entries)
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sched.Stop(stopCtx)
	return nil
}
