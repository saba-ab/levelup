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

	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/validate"
)

func TestRoutesRequireAuth(t *testing.T) {
	r := chi.NewRouter()
	NewHandler(nil, validate.New()).Mount(r)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/webhooks"},
		{http.MethodPost, "/webhooks"},
		{http.MethodGet, "/webhooks/event-types"},
		{http.MethodGet, "/webhooks/deliveries"},
		{http.MethodGet, "/webhooks/deliveries/x"},
		{http.MethodPost, "/webhooks/deliveries/x/redeliver"},
		{http.MethodGet, "/webhooks/x"},
		{http.MethodPatch, "/webhooks/x"},
		{http.MethodDelete, "/webhooks/x"},
		{http.MethodPost, "/webhooks/x/rotate-secret"},
		{http.MethodPost, "/webhooks/x/test"},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}")))
		require.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", tc.method, tc.path)
	}
}

func TestCreateReqValidation(t *testing.T) {
	val := validate.New()
	require.NoError(t, val.Struct(CreateEndpointReq{URL: "https://x.example", EventTypes: []string{"*"}}))
	require.Error(t, val.Struct(CreateEndpointReq{URL: "https://x.example"}), "event_types required")
	require.Error(t, val.Struct(CreateEndpointReq{EventTypes: []string{"*"}}), "url required")
	require.Error(t, val.Struct(CreateEndpointReq{URL: "https://x.example", EventTypes: []string{""}}))
	empty := []string{}
	require.Error(t, val.Struct(UpdateEndpointReq{EventTypes: &empty}))
	require.NoError(t, val.Struct(UpdateEndpointReq{}))
}

func TestMalformedPathIDIsNotFound(t *testing.T) {
	for _, tc := range []struct {
		param string
		err   error
		code  string
	}{
		{"id", domain.ErrEndpointNotFound, domain.CodeEndpointNotFound},
		{"deliveryID", domain.ErrDeliveryNotFound, domain.CodeDeliveryNotFound},
	} {
		req := httptest.NewRequest(http.MethodGet, "/webhooks/42", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add(tc.param, "42")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		_, err := pathUUID(req, tc.param, tc.err)
		rec := httptest.NewRecorder()
		httpx.Error(rec, req, err)
		require.Equal(t, http.StatusNotFound, rec.Code)
		require.Contains(t, rec.Body.String(), `"code":"`+tc.code+`"`)
	}
}

func TestResponsesNeverLeakSecretOutsideCreateAndRotate(t *testing.T) {
	e := domain.Endpoint{ID: "e1", Secret: "whsec_secret", EventTypes: []string{"*"}, CreatedAt: time.Now()}
	plain, err := json.Marshal(toEndpointResp(e))
	require.NoError(t, err)
	require.NotContains(t, string(plain), "whsec_secret")
	withSecret, err := json.Marshal(toEndpointWithSecret(e))
	require.NoError(t, err)
	require.Contains(t, string(withSecret), `"secret":"whsec_secret"`)

	d := domain.Delivery{ID: "d1", Payload: []byte(`{"event":"x"}`)}
	list, err := json.Marshal(toDeliveryResp(d, false))
	require.NoError(t, err)
	require.NotContains(t, string(list), "payload")
	one, err := json.Marshal(toDeliveryResp(d, true))
	require.NoError(t, err)
	require.Contains(t, string(one), `"payload":{"event":"x"}`)
}
