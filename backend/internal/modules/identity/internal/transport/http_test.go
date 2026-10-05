package transport

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"levelup/internal/shared/validate"
)

// Shape validation rejects before the service runs (nil service is never
// reached), and protected groups refuse anonymous callers.
func TestRequestShapeValidation(t *testing.T) {
	r := chi.NewRouter()
	NewHandler(nil, validate.New()).Mount(r)

	for name, tc := range map[string]struct {
		method, path, body string
		want               int
	}{
		"register password over 72":   {"POST", "/auth/register", `{"tenant_name":"A","email":"a@example.com","password":"` + strings.Repeat("x", 73) + `"}`, 422},
		"register short password":     {"POST", "/auth/register", `{"tenant_name":"A","email":"a@example.com","password":"short"}`, 422},
		"register mismatched confirm": {"POST", "/auth/register", `{"tenant_name":"A","email":"a@example.com","password":"long-enough","password_confirmation":"other-value"}`, 422},
		"register unknown field":      {"POST", "/auth/register", `{"tenant_id":"x","tenant_name":"A","email":"a@example.com","password":"long-enough"}`, 422},
		"login missing password":      {"POST", "/auth/login", `{"email":"a@example.com"}`, 422},
		"refresh missing token":       {"POST", "/auth/refresh", `{}`, 422},
		"me anonymous":                {"GET", "/auth/me", ``, 401},
		"users anonymous":             {"GET", "/users", ``, 401},
		"tenant anonymous":            {"GET", "/tenant", ``, 401},
		"platform anonymous":          {"GET", "/platform/tenants", ``, 401},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, tc.want, rec.Code, name+": "+rec.Body.String())
		require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"), name)
	}
}
