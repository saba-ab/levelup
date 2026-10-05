// Command migrate applies platform and module migrations in registry order
// (R3). It connects direct to Postgres (DB_MIGRATE_DSN), never through
// PgBouncer: goose relies on search_path, a session setting.
package main

import (
	"context"
	"fmt"
	"os"

	"myapp/internal/app"
	"myapp/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	cmd := "up"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "up":
		return app.Migrate(context.Background(), cfg)
	default:
		return fmt.Errorf("unknown migrate command %q (supported: up)", cmd)
	}
}
