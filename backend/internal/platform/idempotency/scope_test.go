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

	"levelup/internal/platform/authz"
	"levelup/internal/platform/idempotency"
	idmigrations "levelup/internal/platform/idempotency/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

// scopedServer injects a principal from X-Test-Tenant, the way authn does in
// production (it runs before the idempotency middleware). /flaky fails with
// 500 on its first call and succeeds afterwards.
func scopedServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	dsn := pgtest.DSN(t)
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "idempotency", idmigrations.FS))
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	var calls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("POST /charge", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		p, _ := authz.From(r.Context())
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, p.TenantID)
	})
	mux.HandleFunc("POST /flaky", func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	withPrincipal := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tid := r.Header.Get("X-Test-Tenant"); tid != "" {
				r = r.WithContext(authz.Into(r.Context(), authz.Principal{UserID: "u-" + tid, TenantID: tid}))
			}
			next.ServeHTTP(w, r)
		})
	}
	srv := httptest.NewServer(withPrincipal(idempotency.Middleware(idempotency.NewStore(pool), zap.NewNop())(mux)))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func send(t *testing.T, srv *httptest.Server, path, tenant, key string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(`{}`))
	require.NoError(t, err)
	req.Header.Set("Idempotency-Key", key)
	if tenant != "" {
		req.Header.Set("X-Test-Tenant", tenant)
	}
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(b)
}

// Two tenants choosing the same key must never see each other's response.
func TestKeysAreScopedPerPrincipal(t *testing.T) {
	srv, calls := scopedServer(t)
	key := "order-" + id.NewID()

	status, body := send(t, srv, "/charge", "tenant-a", key)
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "tenant-a", body)

	status, body = send(t, srv, "/charge", "tenant-b", key)
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, "tenant-b", body, "tenant b must not receive tenant a's stored response")
	require.Equal(t, int64(2), calls.Load())

	_, body = send(t, srv, "/charge", "tenant-a", key)
	require.Equal(t, "tenant-a", body)
	require.Equal(t, int64(2), calls.Load(), "same principal + key replays")
}

// A 5xx is not an answer about the operation: the retry must run again.
func TestServerErrorsAreNotStored(t *testing.T) {
	srv, calls := scopedServer(t)
	key := "retry-" + id.NewID()

	status, _ := send(t, srv, "/flaky", "tenant-a", key)
	require.Equal(t, http.StatusInternalServerError, status)
	status, _ = send(t, srv, "/flaky", "tenant-a", key)
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, int64(2), calls.Load())
}
