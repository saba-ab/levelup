package transport_test

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

	"levelup/internal/modules/player/contracts"
	"levelup/internal/modules/player/internal/app"
	"levelup/internal/modules/player/internal/app/apptest"
	"levelup/internal/modules/player/internal/transport"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/validate"
)

const tenantA = "0198d000-0000-7000-8000-00000000000a"

type harness struct {
	router chi.Router
	ob     *apptest.Outbox
	tenant string
	perms  apptest.AllowKeys
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	state := &apptest.TxState{}
	repo := apptest.NewRepo(state)
	ob := &apptest.Outbox{Tx: state}
	h := &harness{ob: ob, tenant: tenantA, perms: apptest.AllowKeys{
		contracts.PermViewAny.Key(): true, contracts.PermView.Key(): true,
		contracts.PermCreate.Key(): true, contracts.PermUpdate.Key(): true, contracts.PermDelete.Key(): true,
	}}
	svc := app.NewService(repo, ob, h.perms, nil, clock.NewFake(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)), 100).
		WithTxRunner(apptest.Runner(state))
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Header.Get("Authorization") != "" {
				req = req.WithContext(authz.Into(req.Context(), authz.Principal{
					UserID: "0198d000-0000-7000-8000-0000000000a1", TenantID: h.tenant,
				}))
			}
			next.ServeHTTP(w, req)
		})
	})
	transport.NewHandler(svc, validate.New()).Mount(r)
	h.router = r
	return h
}

func (h *harness) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer x")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), rec.Body.String())
	return v
}

func TestUnauthenticatedIs401(t *testing.T) {
	h := newHarness(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/players", nil)
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestCreateGetPatchFlow(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/players",
		`{"external_id":"ext/1","display_name":"Neo","email":"neo@x.io","attributes":{"tier":"gold","age":30}}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	created := decode[transport.PlayerResp](t, rec)
	require.Equal(t, tenantA, created.TenantID)
	require.Equal(t, "ext/1", created.ExternalID)
	require.True(t, created.IsActive)

	rec = h.do(t, http.MethodPost, "/players", `{"external_id":"ext/1"}`)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, contracts.CodeExternalIDTaken, decode[httpx.Problem](t, rec).Code)

	rec = h.do(t, http.MethodGet, "/players/by-external-id/ext%2F1", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, created.ID, decode[transport.PlayerResp](t, rec).ID)

	rec = h.do(t, http.MethodPatch, "/players/"+created.ID, `{"email":"trinity@x.io","attributes":{"age":null}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	patched := decode[transport.PlayerResp](t, rec)
	require.Equal(t, "trinity@x.io", *patched.Email)
	require.Equal(t, "Neo", *patched.DisplayName, "omitted field stays untouched")
	require.Equal(t, map[string]any{"tier": "gold"}, patched.Attributes)

	rec = h.do(t, http.MethodPost, "/players/"+created.ID+"/deactivate", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, decode[transport.PlayerResp](t, rec).IsActive)

	rec = h.do(t, http.MethodPost, "/players/"+created.ID+"/activate", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, decode[transport.PlayerResp](t, rec).IsActive)

	rec = h.do(t, http.MethodDelete, "/players/"+created.ID, "")
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = h.do(t, http.MethodGet, "/players/"+created.ID, "")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, contracts.CodePlayerNotFound, decode[httpx.Problem](t, rec).Code)

	require.Equal(t, []string{
		contracts.TopicPlayerCreated, contracts.TopicPlayerUpdated, contracts.TopicPlayerDeactivated,
		contracts.TopicPlayerActivated, contracts.TopicPlayerDeleted,
	}, h.ob.Topics())
}

func TestCrossTenantGetIs404(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/players", `{"external_id":"mine"}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	id := decode[transport.PlayerResp](t, rec).ID

	h.tenant = "0198d000-0000-7000-8000-00000000000b"
	require.Equal(t, http.StatusNotFound, h.do(t, http.MethodGet, "/players/"+id, "").Code)
	require.Equal(t, http.StatusNotFound, h.do(t, http.MethodPatch, "/players/"+id, `{"display_name":"x"}`).Code)
	require.Equal(t, http.StatusNotFound, h.do(t, http.MethodDelete, "/players/"+id, "").Code)
}

func TestDeleteWithoutAdminPermIs403(t *testing.T) {
	h := newHarness(t)
	rec := h.do(t, http.MethodPost, "/players", `{"external_id":"x"}`)
	id := decode[transport.PlayerResp](t, rec).ID
	delete(h.perms, contracts.PermDelete.Key())
	require.Equal(t, http.StatusForbidden, h.do(t, http.MethodDelete, "/players/"+id, "").Code)
}

func TestValidationIs422(t *testing.T) {
	h := newHarness(t)
	cases := map[string]string{
		"missing external_id":   `{"display_name":"x"}`,
		"bad email":             `{"external_id":"x","email":"nope"}`,
		"attributes not object": `{"external_id":"x","attributes":[1,2]}`,
		"unknown field":         `{"external_id":"x","tenant_id":"evil"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := h.do(t, http.MethodPost, "/players", body)
			require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
		})
	}
}

func TestListEnvelopeAndQueryValidation(t *testing.T) {
	h := newHarness(t)
	for _, ext := range []string{"a", "b", "c"} {
		require.Equal(t, http.StatusCreated, h.do(t, http.MethodPost, "/players", `{"external_id":"`+ext+`"}`).Code)
	}
	rec := h.do(t, http.MethodGet, "/players?limit=2", "")
	require.Equal(t, http.StatusOK, rec.Code)
	page := decode[transport.ListResp](t, rec)
	require.Len(t, page.Data, 2)
	require.NotEmpty(t, page.NextCursor)

	rec = h.do(t, http.MethodGet, "/players?limit=2&cursor="+page.NextCursor, "")
	page = decode[transport.ListResp](t, rec)
	require.Len(t, page.Data, 1)
	require.Empty(t, page.NextCursor)
	require.Contains(t, rec.Body.String(), `"next_cursor":""`)

	require.Equal(t, http.StatusUnprocessableEntity, h.do(t, http.MethodGet, "/players?is_active=maybe", "").Code)
	require.Equal(t, http.StatusUnprocessableEntity, h.do(t, http.MethodGet, "/players?limit=-1", "").Code)
	require.Equal(t, http.StatusOK, h.do(t, http.MethodGet, "/players?is_active=false&search=a", "").Code)
}
