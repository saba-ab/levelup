package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"levelup/internal/modules/progression/internal/app"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/id"
	"levelup/internal/shared/validate"
)

// router serves the handler with nil repository and ports: every case
// below is decided before either is reached.
func router(principal *authz.Principal) http.Handler {
	svc := app.NewService(nil, nil, nil, nil, nil, clock.NewFake(time.Now()), nil, app.Options{})
	r := chi.NewRouter()
	if principal != nil {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req.WithContext(authz.Into(req.Context(), *principal)))
			})
		})
	}
	NewHandler(svc, validate.New()).Mount(r)
	return r
}

func get(h http.Handler, path string) (*httptest.ResponseRecorder, httpx.Problem) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var p httpx.Problem
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	return rec, p
}

func TestBatchProgressRequiresAuth(t *testing.T) {
	rec, _ := get(router(nil), "/progress?player_ids="+id.NewID())
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestBatchProgressValidatesIDs(t *testing.T) {
	h := router(&authz.Principal{UserID: id.NewID(), TenantID: id.NewID()})

	rec, prob := get(h, "/progress")
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "player_ids")

	rec, prob = get(h, "/progress?player_ids=nope")
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "player_ids")

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = id.NewID()
	}
	rec, prob = get(h, "/progress?player_ids="+strings.Join(ids, ","))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "too_many_ids", prob.Code)
}
