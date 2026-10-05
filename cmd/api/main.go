// Command api is the HTTP server binary.
//
//	@title			myapp API
//	@version		1.0
//	@description	Modular monolith blueprint API.
//	@BasePath		/api/v1
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"myapp/internal/app"
	"myapp/internal/config"
)

func main() {
	if err := run(); err != nil {
		// Boot failures print the reason (with the env key named, R15)
		// before the process dies — no listener, no half-started state.
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

	return a.RunAPI(ctx)
}
