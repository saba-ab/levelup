package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"levelup/internal/modules/analytics/internal/app"
	"levelup/internal/modules/analytics/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
)

const tenant = "0198d000-0000-7000-8000-00000000000a"

// stubRepo answers the read paths with empty data; anything else panics
// through the nil embedded interface.
type stubRepo struct{ app.Repository }

func (stubRepo) Counters(context.Context, string, domain.Range, []string) ([]app.CounterRow, error) {
	return nil, nil
}

func (stubRepo) ActivePlayersByDay(context.Context, string, domain.Range) (map[time.Time]int64, error) {
	return nil, nil
}

func (stubRepo) DistinctActivePlayers(context.Context, string, domain.Range) (int64, error) {
	return 0, nil
}

func (stubRepo) Funnel(_ context.Context, _ string, steps []string, _ domain.Range) ([]int64, error) {
	return make([]int64, len(steps)), nil
}

func router() http.Handler {
	svc := app.NewService(stubRepo{}, authz.AllowAll{}, nil, clock.NewFake(time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)), app.Settings{})
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(authz.Into(req.Context(), authz.Principal{UserID: "u", TenantID: tenant})))
		})
	})
	NewHandler(svc).Mount(r)
	return r
}

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestOverviewShapeAndDefaults(t *testing.T) {
	w := get(t, "/analytics/overview?from=2026-10-01&to=2026-10-07")
	require.Equal(t, http.StatusOK, w.Code)
	var out OverviewResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, "2026-10-01", out.From)
	require.Equal(t, "2026-10-07", out.To)
	require.Len(t, out.Daily, 7)
	require.Equal(t, "2026-10-01", out.Daily[0].Day)
}

func TestBadParamsAre422WithCodes(t *testing.T) {
	cases := map[string]string{
		"/analytics/overview?from=yesterday":                  domain.CodeInvalidRange,
		"/analytics/engagement?from=2024-01-01&to=2026-01-01": domain.CodeRangeTooLarge,
		"/analytics/retention?weeks=x":                        domain.CodeInvalidCohort,
		"/analytics/retention?cohort=month":                   domain.CodeInvalidCohort,
		"/analytics/funnel?steps=only":                        domain.CodeInvalidSteps,
	}
	for path, code := range cases {
		w := get(t, path)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code, path)
		var p struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &p))
		require.Equal(t, code, p.Code, path)
	}
}

func TestFunnelShape(t *testing.T) {
	w := get(t, "/analytics/funnel?steps=a,b")
	require.Equal(t, http.StatusOK, w.Code)
	var out FunnelResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Equal(t, "day", out.Approximation)
	require.Len(t, out.Steps, 2)
}
