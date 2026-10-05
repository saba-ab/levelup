package idempotency_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"myapp/internal/platform/idempotency"
	idmigrations "myapp/internal/platform/idempotency/migrations"
	"myapp/internal/platform/postgres"
	"myapp/internal/platform/postgres/pgtest"
	"myapp/internal/shared/id"
)

func setup(t *testing.T) (*idempotency.Store, *httptest.Server, *atomic.Int64) {
	t.Helper()
	dsn := pgtest.DSN(t)
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "idempotency", idmigrations.FS))

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	store := idempotency.NewStore(pool)

	var handled atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /charge", func(w http.ResponseWriter, r *http.Request) {
		handled.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"charged":true,"n":`+string(rune('0'+handled.Load()))+`}`)
	})

	srv := httptest.NewServer(idempotency.Middleware(store, zap.NewNop())(mux))
	t.Cleanup(srv.Close)
	return store, srv, &handled
}

func post(t *testing.T, srv *httptest.Server, key, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest("POST", srv.URL+"/charge", strings.NewReader(body))
	require.NoError(t, err)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, string(b)
}

// R11: replay returns the cached response; the handler runs once.
func TestReplayReturnsStoredResponse(t *testing.T) {
	_, srv, handled := setup(t)
	key := id.NewID()

	first, firstBody := post(t, srv, key, `{"amount":100}`)
	require.Equal(t, http.StatusCreated, first.StatusCode)

	second, secondBody := post(t, srv, key, `{"amount":100}`)
	require.Equal(t, http.StatusCreated, second.StatusCode)
	require.Equal(t, firstBody, secondBody, "replay must be byte-identical")
	require.Equal(t, "true", second.Header.Get("Idempotent-Replay"))
	require.EqualValues(t, 1, handled.Load(), "the charge must happen once (R11)")
}

// R11: same key, different body → 422.
func TestConflictingBodyRejected(t *testing.T) {
	_, srv, handled := setup(t)
	key := id.NewID()

	resp, _ := post(t, srv, key, `{"amount":100}`)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	resp, _ = post(t, srv, key, `{"amount":999}`)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	require.EqualValues(t, 1, handled.Load())
}

func TestNoKeyBypasses(t *testing.T) {
	_, srv, handled := setup(t)
	post(t, srv, "", `{"amount":1}`)
	post(t, srv, "", `{"amount":1}`)
	require.EqualValues(t, 2, handled.Load(), "no key → no dedupe, by design")
}

func TestInFlightConflict(t *testing.T) {
	store, srv, _ := setup(t)
	key := id.NewID()

	// Simulate a concurrent identical request holding the claim.
	state, _, err := store.Claim(context.Background(), key, hashOf("POST /charge", `{"amount":5}`))
	require.NoError(t, err)
	require.Equal(t, idempotency.Claimed, state)

	resp, _ := post(t, srv, key, `{"amount":5}`)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func hashOf(routeLine, body string) []byte {
	h := idempotency.HashRequest(routeLine, []byte(body))
	return h
}
