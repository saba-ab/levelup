package httpx_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"myapp/internal/platform/httpx"
	"myapp/internal/shared/errs"
	"myapp/internal/shared/validate"
)

func TestErrorMapsKindsToStatuses(t *testing.T) {
	cases := map[errs.Kind]int{
		errs.Invalid:          422,
		errs.NotFound:         404,
		errs.AlreadyExists:    409,
		errs.Conflict:         409,
		errs.PermissionDenied: 403,
		errs.Unauthenticated:  401,
		errs.Unavailable:      503,
		errs.Internal:         500,
		errs.Unknown:          500,
	}
	for kind, want := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/x", nil)
		httpx.Error(rec, req, errs.New(kind, "boom"))

		require.Equal(t, want, rec.Code, kind.String())
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

		var p httpx.Problem
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
		require.Equal(t, want, p.Status)
	}
}

func TestErrorHidesInternalDetail(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	httpx.Error(rec, req, errs.Wrap(errs.Internal, "pg password leaked here", nil))

	var p httpx.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	require.NotContains(t, p.Detail, "leaked", "5xx detail must not echo internals")
}

func TestErrorRendersValidationFields(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/x", nil)
	err := errs.WithFields(errs.New(errs.Invalid, "validation failed"),
		map[string]string{"amount": "must be greater than 0"})
	httpx.Error(rec, req, err)

	var p httpx.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	require.Equal(t, "must be greater than 0", p.Errors["amount"])
}

type decodeTarget struct {
	Amount   int64  `json:"amount" validate:"required,gt=0"`
	Currency string `json:"currency" validate:"required,iso4217"`
}

func TestDecodeRejectsMalformedJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/x", strings.NewReader("{not json"))
	var dst decodeTarget
	err := httpx.Decode(req, &dst, validate.New())
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestDecodeValidates(t *testing.T) {
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"amount":-1,"currency":"USD"}`))
	var dst decodeTarget
	err := httpx.Decode(req, &dst, validate.New())
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	require.Contains(t, errs.FieldsOf(err), "amount")
}

func TestDecodeHappyPath(t *testing.T) {
	req := httptest.NewRequest("POST", "/x", strings.NewReader(`{"amount":5,"currency":"USD"}`))
	var dst decodeTarget
	require.NoError(t, httpx.Decode(req, &dst, validate.New()))
	require.Equal(t, int64(5), dst.Amount)
}

func newStack(t *testing.T) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	mw := httpx.BaseMiddleware(zap.NewNop(), noop.NewTracerProvider().Tracer("t"), prometheus.NewRegistry())
	r.Use(mw...)
	return r
}

func TestRecoverTurnsPanicIntoProblem(t *testing.T) {
	r := newStack(t)
	r.Get("/boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/boom", nil))

	require.Equal(t, 500, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

func TestEveryHandlerGetsADeadline(t *testing.T) {
	r := newStack(t)
	var hadDeadline bool
	r.Get("/probe", func(w http.ResponseWriter, req *http.Request) {
		_, hadDeadline = req.Context().Deadline()
		w.WriteHeader(204)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/probe", nil))
	require.True(t, hadDeadline, "no handler may run without a context deadline (R10)")
}

func TestRequestIDIsSetAndEchoed(t *testing.T) {
	r := newStack(t)
	r.Get("/probe", func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(204) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/probe", nil))
	require.NotEmpty(t, rec.Header().Get("X-Request-Id"))
}

func TestServerTimeoutsConfigured(t *testing.T) {
	srv := httpx.NewServer(":0", http.NotFoundHandler())
	require.Equal(t, 5*time.Second, srv.ReadHeaderTimeout, "slowloris guard (R10)")
	require.NotZero(t, srv.ReadTimeout)
	require.NotZero(t, srv.WriteTimeout)
	require.NotZero(t, srv.IdleTimeout)
}
