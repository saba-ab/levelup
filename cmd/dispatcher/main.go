// Command dispatcher is the outbox → RabbitMQ publisher (PRD §5): the ONLY
// production publisher to the broker. Safe at N replicas (SKIP LOCKED).
//
// Subcommands:
//
//	(none)        run the poller
//	replay        --from=RFC3339 [--topic=t]   clear published_at so rows republish (R49)
//	dead-letters  [--limit=n] [--all]          list rows parked in outbox_svc.dead_letters (R23)
//	replay-dead   --id=n | --all [--topic=t]   put dispatcher dead letters back into the outbox (R23)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"myapp/internal/app"
	"myapp/internal/config"
	"myapp/internal/platform/httpx"
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
	log := a.P.Tel.Log

	conn, err := rabbit.Dial(cfg.Rabbit.URL, log, a.P.Tel.Registry)
	if err != nil {
		return err
	}
	defer conn.Close()

	d := outbox.NewDispatcher(a.P.DB.Writer(), conn, log, a.P.Tel.Registry,
		outbox.WithMaxPublishAttempts(cfg.Outbox.MaxPublishAttempts))

	if len(args) > 0 {
		switch args[0] {
		case "replay":
			return replay(ctx, d, args[1:])
		case "dead-letters":
			return listDeadLetters(ctx, d, args[1:])
		case "replay-dead":
			return replayDeadLetters(ctx, d, args[1:])
		}
	}

	// Declare + verify before the first publish: a message routed to a
	// nonexistent binding is silently dropped (R45 refusal included).
	if err := a.DeclareTopology(ctx, a.NewTopology(conn)); err != nil {
		return err
	}

	adm := httpx.NewServer(cfg.HTTP.AdminAddr, a.P.Tel.AdminHandler())
	go func() { _ = adm.ListenAndServe() }()
	defer func() {
		drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpx.Shutdown(drain, adm)
	}()

	log.Info("dispatcher running")
	return d.Run(ctx)
}

func replay(ctx context.Context, d *outbox.Dispatcher, args []string) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fromRaw := fs.String("from", "", "RFC3339 lower bound on occurred_at (required)")
	topic := fs.String("topic", "", "restrict to one topic (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fromRaw == "" {
		return fmt.Errorf("replay: --from is required (RFC3339)")
	}
	from, err := time.Parse(time.RFC3339, *fromRaw)
	if err != nil {
		return fmt.Errorf("replay: bad --from: %w", err)
	}
	n, err := d.Replay(ctx, from, *topic)
	if err != nil {
		return err
	}
	fmt.Printf("replay: %d rows marked unpublished — a running dispatcher will republish them\n", n)
	return nil
}

func listDeadLetters(ctx context.Context, d *outbox.Dispatcher, args []string) error {
	fs := flag.NewFlagSet("dead-letters", flag.ContinueOnError)
	limit := fs.Int("limit", 50, "maximum rows to print")
	all := fs.Bool("all", false, "include rows already replayed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var (
		rows []outbox.DeadLetter
		err  error
	)
	if *all {
		rows, err = d.DeadLetters().ListAll(ctx, *limit)
	} else {
		rows, err = d.DeadLetters().List(ctx, *limit)
	}
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		fmt.Println("dead-letters: none")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tSOURCE\tTOPIC\tQUEUE\tATTEMPTS\tDEAD_AT\tREPLAYED_AT\tERROR"); err != nil {
		return err
	}
	for _, r := range rows {
		replayed := "-"
		if r.ReplayedAt != nil {
			replayed = r.ReplayedAt.Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			r.ID, r.Source, r.Topic, r.Queue, r.Attempts, r.DeadAt.Format(time.RFC3339), replayed, r.Error); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func replayDeadLetters(ctx context.Context, d *outbox.Dispatcher, args []string) error {
	fs := flag.NewFlagSet("replay-dead", flag.ContinueOnError)
	id := fs.Int64("id", 0, "one dead letter id")
	all := fs.Bool("all", false, "every unreplayed dispatcher row")
	topic := fs.String("topic", "", "with --all: restrict to one topic")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch {
	case *id != 0 && *all:
		return fmt.Errorf("replay-dead: --id and --all are exclusive")
	case *id != 0:
		ok, err := d.DeadLetters().Replay(ctx, *id)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Printf("replay-dead: row %d not found or already replayed\n", *id)
			return nil
		}
		fmt.Printf("replay-dead: row %d is back in the outbox — a running dispatcher will publish it\n", *id)
		return nil
	case *all:
		n, err := d.DeadLetters().ReplayAll(ctx, *topic)
		if err != nil {
			return err
		}
		fmt.Printf("replay-dead: %d rows back in the outbox\n", n)
		return nil
	default:
		return fmt.Errorf("replay-dead: --id=<n> or --all is required")
	}
}
