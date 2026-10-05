package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/platform/httpx"
)

func corsServer(allowed ...string) http.Handler {
	return httpx.CORS(allowed)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
}

func TestCORSPreflightForAllowedOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/players", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	corsServer("http://localhost:5173").ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code, "preflight never reaches the handler")
	require.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Idempotency-Key")
	require.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "PATCH")
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORSActualRequestExposesHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/players", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	corsServer("http://localhost:5173").ServeHTTP(rec, req)

	require.Equal(t, http.StatusTeapot, rec.Code)
	require.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, rec.Header().Get("Access-Control-Expose-Headers"), "Idempotent-Replay")
}

func TestCORSUnknownOriginGetsNoGrant(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/players", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "DELETE")
	rec := httptest.NewRecorder()
	corsServer("http://localhost:5173").ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORSWildcardAndNoOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://anything.example")
	rec := httptest.NewRecorder()
	corsServer("*").ServeHTTP(rec, req)
	require.Equal(t, "https://anything.example", rec.Header().Get("Access-Control-Allow-Origin"))

	rec = httptest.NewRecorder()
	corsServer("http://localhost:5173").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"), "same-origin and server calls are untouched")
	require.Equal(t, http.StatusTeapot, rec.Code)
}
