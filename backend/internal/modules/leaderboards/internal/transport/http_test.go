package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/leaderboards/internal/app"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/modules/leaderboards/internal/repo"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/validate"
)

const tenant = "0198d000-0000-7000-8000-00000000000a"

// stubRepo implements only what these HTTP tests reach; anything else
// panics through the nil embedded interface.
type stubRepo struct {
	app.Repository
	boards map[string]domain.Leaderboard
}

func (s *stubRepo) Create(_ context.Context, _ *gorm.DB, lb domain.Leaderboard) error {
	s.boards[lb.ID] = lb
	return nil
}

func (s *stubRepo) ByID(_ context.Context, tenantID, id string) (domain.Leaderboard, error) {
	b, ok := s.boards[id]
	if !ok || b.TenantID != tenantID || b.DeletedAt != nil {
		return domain.Leaderboard{}, domain.ErrNotFound
	}
	return b, nil
}

func (s *stubRepo) Save(_ context.Context, _ *gorm.DB, lb domain.Leaderboard) error {
	s.boards[lb.ID] = lb
	return nil
}

func (s *stubRepo) SoftDelete(_ context.Context, _ *gorm.DB, lb domain.Leaderboard, at time.Time) error {
	lb.DeletedAt = &at
	s.boards[lb.ID] = lb
	return nil
}

func (s *stubRepo) Periods(context.Context, string, string, bool) ([]domain.Period, error) {
	return nil, nil
}

func (s *stubRepo) RangeByPosition(context.Context, domain.Leaderboard, time.Time, int64, int64) ([]domain.Standing, error) {
	return nil, nil
}

type allow struct{}

func (allow) Authorize(context.Context, authz.Principal, authz.Permission, any) error { return nil }

func newServer(t *testing.T) (http.Handler, *stubRepo) {
	t.Helper()
	r := &stubRepo{boards: map[string]domain.Leaderboard{}}
	svc := app.NewService(r, repo.Disabled{}, nil, nil, allow{}, nil,
		clock.NewFake(time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)), nil, app.Settings{})
	svc.SetTxRunner(func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) })
	router := chi.NewRouter()
	NewHandler(svc, validate.New()).Mount(router)
	return router, r
}

func do(h http.Handler, method, path, body string, authed bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if authed {
		req = req.WithContext(authz.Into(req.Context(), authz.Principal{UserID: "u", TenantID: tenant}))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateGetPatchDelete(t *testing.T) {
	h, _ := newServer(t)
	rec := do(h, http.MethodPost, "/leaderboards", `{"name":"Weekly Race","type":"points","reset_frequency":"weekly","tenant_id":"x"}`, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "tenant_id in the body is rejected, never honoured")

	rec = do(h, http.MethodPost, "/leaderboards", `{"name":"Weekly Race","type":"points","reset_frequency":"weekly"}`, true)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created LeaderboardResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, tenant, created.TenantID)
	require.Equal(t, "weekly-race", created.Slug)
	require.True(t, created.IsActive)
	require.Nil(t, created.ProgramID)

	rec = do(h, http.MethodGet, "/leaderboards/"+created.ID, "", true)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = do(h, http.MethodPatch, "/leaderboards/"+created.ID, `{"type":"badges"}`, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "type is immutable")
	rec = do(h, http.MethodPatch, "/leaderboards/"+created.ID, `{"max_entries":0}`, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "an explicit zero is invalid, not ignored")
	rec = do(h, http.MethodPatch, "/leaderboards/"+created.ID, `{"max_entries":5000}`, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	rec = do(h, http.MethodPatch, "/leaderboards/"+created.ID, `{"name":"Renamed","is_active":false}`, true)
	require.Equal(t, http.StatusOK, rec.Code)
	var updated LeaderboardResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	require.Equal(t, "Renamed", updated.Name)
	require.False(t, updated.IsActive)
	require.Equal(t, "weekly", updated.ResetFrequency)

	rec = do(h, http.MethodGet, "/leaderboards/"+created.ID+"/entries?limit=5", "", true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var entries EntriesResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &entries))
	require.NotNil(t, entries.Data)
	require.Equal(t, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), entries.PeriodStart)
	require.NotNil(t, entries.PeriodEnd)

	rec = do(h, http.MethodGet, "/leaderboards/"+created.ID+"/entries?limit=-1", "", true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = do(h, http.MethodDelete, "/leaderboards/"+created.ID, "", true)
	require.Equal(t, http.StatusNoContent, rec.Code)
	rec = do(h, http.MethodGet, "/leaderboards/"+created.ID, "", true)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), domain.CodeNotFound)
}

func TestCreateValidation(t *testing.T) {
	h, _ := newServer(t)
	for _, body := range []string{
		`{"type":"points"}`,
		`{"name":"x","type":"custom"}`,
		`{"name":"x","type":"points","reset_frequency":"hourly"}`,
		`{"name":"x","type":"points","program_id":"nope"}`,
		`{"name":"x","type":"points","metric":"balance","reset_frequency":"daily"}`,
	} {
		rec := do(h, http.MethodPost, "/leaderboards", body, true)
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, body)
	}
}

func TestRoutesRequireAuth(t *testing.T) {
	h, _ := newServer(t)
	rec := do(h, http.MethodGet, "/leaderboards", "", false)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
