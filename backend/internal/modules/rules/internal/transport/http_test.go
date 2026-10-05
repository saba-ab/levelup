package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"levelup/internal/platform/httpx"
	"levelup/internal/shared/validate"
)

func router() http.Handler {
	r := chi.NewRouter()
	NewHandler(nil, validate.New()).Mount(r)
	return r
}

func TestExecuteIsGoneWithSuccessorPointer(t *testing.T) {
	rec := httptest.NewRecorder()
	router().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/rules/execute", strings.NewReader(`{"player_id":7}`)))
	require.Equal(t, http.StatusGone, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Header().Get("Link"), "/api/v1/activities")
	var p httpx.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	require.Equal(t, "endpoint_gone", p.Code)
	require.Contains(t, p.Detail, "POST /api/v1/activities")
}

func TestRoutesRequireAuth(t *testing.T) {
	for _, rt := range []struct{ method, path string }{
		{http.MethodGet, "/rules"}, {http.MethodPost, "/rules"}, {http.MethodGet, "/rules/x"},
		{http.MethodPatch, "/rules/x"}, {http.MethodDelete, "/rules/x"}, {http.MethodPost, "/rules/x/publish"},
		{http.MethodGet, "/rules/x/versions"}, {http.MethodPost, "/rules/x/versions"},
		{http.MethodPost, "/rules/simulate"}, {http.MethodGet, "/rules/decisions"}, {http.MethodGet, "/rules/decisions/x"},
	} {
		rec := httptest.NewRecorder()
		router().ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, strings.NewReader(`{}`)))
		require.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", rt.method, rt.path)
	}
}

func TestNullableStringDistinguishesAbsentFromNull(t *testing.T) {
	var a, b, c UpdateRuleReq
	require.NoError(t, json.Unmarshal([]byte(`{}`), &a))
	require.NoError(t, json.Unmarshal([]byte(`{"description":null}`), &b))
	require.NoError(t, json.Unmarshal([]byte(`{"description":"x","conditions":null}`), &c))
	require.False(t, a.Description.Set)
	require.True(t, b.Description.Set && b.Description.Null)
	require.Equal(t, "x", c.Description.Value)
	require.Nil(t, a.Conditions, "absent conditions stay untouched")
	require.Equal(t, "null", string(c.Conditions), "explicit null clears conditions (always true)")
}
