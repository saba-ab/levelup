// Package pgtest starts one throwaway Postgres container per test package.
// Only test files import it; guard every caller with testing.Short so
// `task test` never needs Docker (R13).
package pgtest

import (
	"context"
	"sync"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	once sync.Once
	dsn  string
	err  error
)

// DSN returns a superuser connection string to a package-scoped container.
// The container dies with the test process (Ryuk reaps it).
func DSN(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: needs Docker, skipped with -short")
	}
	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		ctr, e := tcpostgres.Run(ctx, "postgres:17-alpine",
			tcpostgres.WithDatabase("app"),
			tcpostgres.WithUsername("app"),
			tcpostgres.WithPassword("app"),
			tcpostgres.BasicWaitStrategies(),
			tcpostgres.WithSQLDriver("pgx"),
		)
		if e != nil {
			err = e
			return
		}
		dsn, err = ctr.ConnectionString(ctx, "sslmode=disable")
		_ = wait.ForLog // keep the wait import stable across tc versions
	})
	if err != nil {
		t.Fatalf("starting postgres container: %v", err)
	}
	return dsn
}
