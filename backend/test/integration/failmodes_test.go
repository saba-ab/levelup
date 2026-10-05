package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"levelup/internal/platform/httpx"
	"levelup/internal/platform/redis"
	"levelup/internal/platform/redis/redistest"
)

// R43: rate limiting fails OPEN, locks fail CLOSED — the same outage, two
// deliberate directions (PRD §7.8.3). Whoever writes the error branch first
// otherwise decides this by accident.
func TestFailureDirectionsUnderRedisOutage(t *testing.T) {
	addr, stop := redistest.Fresh(t)
	core := redis.NewCore(addr)

	// A limited endpoint…
	r := chi.NewRouter()
	r.Use(httpx.RateLimit(redis.NewLimiter(core), 10_000, zap.NewNop()))
	r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	srv := httptest.NewServer(r)
	defer srv.Close()

	// …and a lock, both on the same Redis.
	locker := redis.NewLocker(core, zap.NewNop())

	// Healthy baseline: request served, lock acquired.
	resp, err := srv.Client().Get(srv.URL + "/ping")
	require.NoError(t, err)
	require.Equal(t, 204, resp.StatusCode)
	resp.Body.Close()

	lock, err := locker.Acquire(context.Background(), "lock:cron:failmode-baseline", time.Minute)
	require.NoError(t, err)
	_ = lock.Release(context.Background())

	stop() // ---- Redis dies mid-run ----

	// Rate limiting: the request MUST still be served (fail open) — a Redis
	// outage must not 429 the entire API.
	resp, err = srv.Client().Get(srv.URL + "/ping")
	require.NoError(t, err)
	require.Equal(t, 204, resp.StatusCode, "rate limiter must fail OPEN (R43)")
	resp.Body.Close()

	// Locks: acquisition MUST fail (fail closed) — two schedulers running
	// the same job is a correctness incident.
	_, err = locker.Acquire(context.Background(), "lock:cron:failmode-down", time.Minute)
	require.Error(t, err, "lock acquisition must fail CLOSED (R43)")
}
