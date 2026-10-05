package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/activity/internal/app"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/modules/activity/internal/ports"
	"levelup/internal/modules/activity/internal/testfakes"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/validate"
)

const tenantA = "0198d000-0000-7000-8000-00000000000a"

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

type server struct {
	router http.Handler
	ob     *testfakes.Outbox
}

func newServer(t *testing.T) *server {
	t.Helper()
	ob := &testfakes.Outbox{}
	svc := app.NewService(testfakes.NewRepo(),
		&testfakes.Players{ByExt: map[string]ports.PlayerSnapshot{}},
		&testfakes.EventTypes{Types: map[string]ports.EventTypeSnapshot{
			"purchase_completed": {Slug: "purchase_completed", Active: true},
		}},
		ob,
		testfakes.AllowKeys{
			contracts.PermIngest.Key(): true, contracts.PermView.Key(): true, contracts.PermViewAny.Key(): true,
		},
		nil, clock.NewFake(now), nil,
		app.Settings{
			RequireKnownEventType: true,
			Limits:                domain.Limits{MaxAge: 720 * time.Hour, MaxFutureSkew: 5 * time.Minute, MaxPayloadBytes: 32 << 10},
		},
		nil, app.WithTx(testfakes.PassThroughTx))
	r := chi.NewRouter()
	NewHandler(svc, validate.New()).Mount(r)
	return &server{router: r, ob: ob}
}

func (s *server) do(t *testing.T, method, path string, body any, authed bool) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	if authed {
		req = req.WithContext(authz.Into(context.Background(), authz.Principal{UserID: "u1", TenantID: tenantA}))
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func validBody(eventID string) map[string]any {
	return map[string]any{
		"event_id":           eventID,
		"event_type":         "purchase_completed",
		"player_external_id": "ext-1",
		"properties":         map[string]any{"amount": 10},
	}
}

func TestIngestReturns202ThenDuplicate200(t *testing.T) {
	s := newServer(t)

	rec := s.do(t, http.MethodPost, "/activities", validBody("evt-1"), true)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var first IngestResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &first))
	require.NotEmpty(t, first.ActivityID)
	require.Equal(t, "pending", first.Status)
	require.False(t, first.Duplicate)
	require.Nil(t, first.Activity)

	rec = s.do(t, http.MethodPost, "/activities", validBody("evt-1"), true)
	require.Equal(t, http.StatusOK, rec.Code)
	var dup IngestResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dup))
	require.True(t, dup.Duplicate)
	require.Equal(t, first.ActivityID, dup.ActivityID)
	require.NotNil(t, dup.Activity)
	require.Equal(t, "evt-1", dup.Activity.EventID)
	require.Len(t, s.ob.Published, 1)
}

func TestIngestErrorsAreProblemJSONWithCodes(t *testing.T) {
	s := newServer(t)

	body := validBody("evt-1")
	body["event_type"] = "unknown_one"
	rec := s.do(t, http.MethodPost, "/activities", body, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	var p httpx.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	require.Equal(t, "unknown_event_type", p.Code)

	body = validBody("evt-2")
	body["occurred_at"] = now.Add(time.Hour).Format(time.RFC3339)
	rec = s.do(t, http.MethodPost, "/activities", body, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	require.Equal(t, "occurred_at_in_future", p.Code)

	rec = s.do(t, http.MethodPost, "/activities", map[string]any{"event_type": "purchase_completed"}, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	require.Contains(t, p.Errors, "event_id")
	require.Contains(t, p.Errors, "player_external_id")

	rec = s.do(t, http.MethodPost, "/activities", validBody("evt-3"), false)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Empty(t, s.ob.Published)
}

func TestBatchReportsPerItemResults(t *testing.T) {
	s := newServer(t)
	unknown := validBody("evt-b")
	unknown["event_type"] = "unknown_one"
	rec := s.do(t, http.MethodPost, "/activities/batch", map[string]any{"items": []any{
		validBody("evt-a"),
		unknown,
		map[string]any{"event_id": "evt-c", "bogus": true},
		validBody("evt-a"),
	}}, true)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var resp BatchResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Results, 4)
	require.Equal(t, 1, resp.Accepted)
	require.Equal(t, 1, resp.Duplicates)
	require.Equal(t, 2, resp.Failed)

	require.NotEmpty(t, resp.Results[0].ActivityID)
	require.Nil(t, resp.Results[0].Error)
	require.Equal(t, "unknown_event_type", resp.Results[1].Error.Code)
	require.Equal(t, http.StatusUnprocessableEntity, resp.Results[1].Error.Status)
	require.NotNil(t, resp.Results[2].Error, "unknown fields fail the item only")
	require.Equal(t, "evt-c", resp.Results[2].EventID)
	require.True(t, resp.Results[3].Duplicate)
	require.Equal(t, resp.Results[0].ActivityID, resp.Results[3].ActivityID)
	require.Len(t, s.ob.Published, 1)
}

func TestBatchRejectsOversizedRequest(t *testing.T) {
	s := newServer(t)
	items := make([]any, 101)
	for i := range items {
		items[i] = validBody("e")
	}
	rec := s.do(t, http.MethodPost, "/activities/batch", map[string]any{"items": items}, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = s.do(t, http.MethodPost, "/activities/batch", map[string]any{"items": []any{}}, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestGetAndList(t *testing.T) {
	s := newServer(t)
	rec := s.do(t, http.MethodPost, "/activities", validBody("evt-1"), true)
	var created IngestResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = s.do(t, http.MethodGet, "/activities/"+created.ActivityID, nil, true)
	require.Equal(t, http.StatusOK, rec.Code)
	var got ActivityResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, created.ActivityID, got.ID)
	require.Equal(t, "pending", got.Status)
	require.Empty(t, got.DecisionID)

	rec = s.do(t, http.MethodGet, "/activities/0198d000-0000-7000-8000-000000000999", nil, true)
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec = s.do(t, http.MethodGet, "/activities?limit=10&status=pending&event_type=purchase_completed", nil, true)
	require.Equal(t, http.StatusOK, rec.Code)
	var list ListResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	require.Empty(t, list.NextCursor)

	rec = s.do(t, http.MethodGet, "/activities?limit=abc", nil, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	rec = s.do(t, http.MethodGet, "/activities?status=nope", nil, true)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}
