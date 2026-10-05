package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"levelup/internal/modules/program/internal/app"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/validate"
)

func decodePatch(t *testing.T, body string) (PatchReq, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	var req PatchReq
	err := httpx.Decode(r, &req, validate.New())
	return req, err
}

func TestPatchRejectsStatus(t *testing.T) {
	_, err := decodePatch(t, `{"name":"x","status":"active"}`)
	require.Equal(t, errs.Invalid, errs.KindOf(err), "status is not editable through PATCH")
}

func TestPatchDistinguishesAbsentFromNull(t *testing.T) {
	req, err := decodePatch(t, `{"name":"New","description":null,"ends_at":"2026-12-31T23:59:59Z"}`)
	require.NoError(t, err)
	p := req.toPatch()
	require.Equal(t, "New", *p.Name)
	require.Nil(t, p.Slug)
	require.True(t, p.Description.Set, "explicit null clears")
	require.Nil(t, p.Description.Value)
	require.False(t, p.StartsAt.Set, "absent stays untouched")
	require.True(t, p.EndsAt.Set)
	require.Equal(t, 2026, p.EndsAt.Value.Year())
	require.Nil(t, p.Settings)

	req, err = decodePatch(t, `{}`)
	require.NoError(t, err)
	p = req.toPatch()
	require.False(t, p.Description.Set || p.StartsAt.Set || p.EndsAt.Set)
	require.Nil(t, p.Name)
}

func TestPatchShapeValidation(t *testing.T) {
	_, err := decodePatch(t, `{"name":""}`)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	_, err = decodePatch(t, `{"starts_at":"not a time"}`)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestEnrollReqRequiresUUID(t *testing.T) {
	for _, body := range []string{`{}`, `{"player_id":"12"}`} {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		var req EnrollReq
		require.Equal(t, errs.Invalid, errs.KindOf(httpx.Decode(r, &req, validate.New())), body)
	}
}

// router mounts the real handler over a service with no repository: only
// requests that are rejected before reaching the service may be sent.
func router() http.Handler {
	svc := app.NewService(nil, nil, nil, authz.AllowAll{}, nil, clock.System(), 0)
	r := chi.NewRouter()
	NewHandler(svc, validate.New()).Mount(r)
	return r
}

func serve(t *testing.T, method, path string, authed bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	if authed {
		req = req.WithContext(authz.Into(context.Background(), authz.Principal{UserID: "u", TenantID: "t"}))
	}
	rec := httptest.NewRecorder()
	router().ServeHTTP(rec, req)
	return rec
}

func TestRoutesRequireAuth(t *testing.T) {
	require.Equal(t, http.StatusUnauthorized, serve(t, http.MethodGet, "/programs", false).Code)
	require.Equal(t, http.StatusUnauthorized, serve(t, http.MethodPost, "/programs/x/activate", false).Code)
}

func TestNonUUIDIdsAreNotFound(t *testing.T) {
	rec := serve(t, http.MethodGet, "/programs/42", true)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"program_not_found"`)

	rec = serve(t, http.MethodDelete, "/programs/0198d000-0000-7000-8000-000000000001/players/abc", true)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"player_not_found"`)
}

func TestLimitValidation(t *testing.T) {
	rec := serve(t, http.MethodGet, "/programs?limit=-1", true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	rec = serve(t, http.MethodGet, "/programs?limit=abc", true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}
