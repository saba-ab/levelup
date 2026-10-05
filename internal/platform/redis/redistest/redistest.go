// Package redistest starts one throwaway Redis container per test package.
// Guard every caller with testing.Short (R13: unit tests need no Docker).
package redistest

import (
	"context"
	"sync"
	"testing"
	"time"

	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

var (
	once sync.Once
	addr string
	err  error
)

// Addr returns host:port of a package-scoped Redis 7 container.
func Addr(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: needs Docker, skipped with -short")
	}
	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		ctr, e := tcredis.Run(ctx, "redis:7-alpine")
		if e != nil {
			err = e
			return
		}
		ep, e := ctr.PortEndpoint(ctx, "6379/tcp", "")
		if e != nil {
			err = e
			return
		}
		addr = ep
	})
	if err != nil {
		t.Fatalf("starting redis container: %v", err)
	}
	return addr
}

// Fresh starts a dedicated container the test may stop/kill freely
// (failure-mode tests, R43).
func Fresh(t *testing.T) (addrOut string, stop func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: needs Docker, skipped with -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ctr, e := tcredis.Run(ctx, "redis:7-alpine")
	if e != nil {
		t.Fatalf("starting redis container: %v", e)
	}
	ep, e := ctr.PortEndpoint(ctx, "6379/tcp", "")
	if e != nil {
		t.Fatalf("redis endpoint: %v", e)
	}
	return ep, func() { _ = ctr.Terminate(context.Background()) }
}
