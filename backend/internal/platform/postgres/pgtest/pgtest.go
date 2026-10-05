// Package pgtest starts one throwaway Postgres container per test package.
// Only test files import it; guard every caller with testing.Short so
// `task test` never needs Docker (R13).
package pgtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

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
		if admin := os.Getenv("PGTEST_DSN"); admin != "" {
			dsn, err = freshDatabase(admin)
			return
		}
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

// freshDatabase is the no-Docker path: PGTEST_DSN points at a server where
// the user may CREATE DATABASE, and every test package gets its own empty
// database (packages run in parallel, so they must not share one). The
// databases are named pgtest_<random> and are left behind for inspection;
// `task test:pg:clean` drops them.
func freshDatabase(admin string) (string, error) {
	u, err := url.Parse(admin)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	name := "pgtest_" + hex.EncodeToString(buf)

	db, err := sql.Open("pgx", admin)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := db.Exec("CREATE DATABASE " + name); err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}
