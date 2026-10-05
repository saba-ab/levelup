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

	"levelup/internal/modules/points/internal/app"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/id"
	"levelup/internal/shared/validate"
)

type allowAll struct{}

func (allowAll) Authorize(context.Context, authz.Principal, authz.Permission, any) error { return nil }

// router serves the handler with a service whose repository and player
// port are nil: every case below must be decided before either is reached.
func router(principal *authz.Principal) http.Handler {
	svc := app.NewService(nil, nil, nil, allowAll{}, nil, clock.NewFake(time.Now()), nil, nil)
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

func do(h http.Handler, method, path, body string, headers map[string]string) (*httptest.ResponseRecorder, httpx.Problem) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var p httpx.Problem
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	return rec, p
}

func TestRoutesRequireAuth(t *testing.T) {
	h := router(nil)
	player := id.NewID()
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/players/" + player + "/wallet"},
		{http.MethodPatch, "/players/" + player + "/wallet"},
		{http.MethodGet, "/players/" + player + "/wallet/transactions"},
		{http.MethodPost, "/players/" + player + "/wallet/credit"},
		{http.MethodPost, "/players/" + player + "/wallet/debit"},
		{http.MethodPost, "/wallets/transfer"},
	} {
		rec, _ := do(h, c.method, c.path, "{}", nil)
		require.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", c.method, c.path)
	}
}

func TestCreditValidation(t *testing.T) {
	p := authz.Principal{UserID: id.NewID(), TenantID: id.NewID()}
	h := router(&p)
	player := id.NewID()

	rec, prob := do(h, http.MethodPost, "/players/not-a-uuid/wallet/credit", `{"amount":1,"kind":"bonus"}`, nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "playerID")

	// B6: a debit kind is refused on the credit endpoint.
	rec, prob = do(h, http.MethodPost, "/players/"+player+"/wallet/credit", `{"amount":1,"kind":"penalty"}`,
		map[string]string{"Idempotency-Key": "k"})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "kind")

	rec, prob = do(h, http.MethodPost, "/players/"+player+"/wallet/debit", `{"amount":-5,"kind":"spend"}`,
		map[string]string{"Idempotency-Key": "k"})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "amount")

	rec, prob = do(h, http.MethodPost, "/players/"+player+"/wallet/credit", `{"amount":1,"kind":"bonus"}`, nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "idempotency_key_required", prob.Code)
}

func TestTransferValidation(t *testing.T) {
	p := authz.Principal{UserID: id.NewID(), TenantID: id.NewID()}
	h := router(&p)
	same := id.NewID()

	rec, prob := do(h, http.MethodPost, "/wallets/transfer",
		`{"from_player_id":"`+same+`","to_player_id":"`+same+`","amount":5}`, map[string]string{"Idempotency-Key": "k"})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "to_player_id")

	rec, prob = do(h, http.MethodPost, "/wallets/transfer",
		`{"from_player_id":"`+same+`","to_player_id":"`+id.NewID()+`","amount":5}`, nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "idempotency_key_required", prob.Code)
}

func TestPatchRequiresIsActive(t *testing.T) {
	p := authz.Principal{UserID: id.NewID(), TenantID: id.NewID()}
	rec, prob := do(router(&p), http.MethodPatch, "/players/"+id.NewID()+"/wallet", `{}`, nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "is_active")
}

func TestListRejectsBadLimit(t *testing.T) {
	p := authz.Principal{UserID: id.NewID(), TenantID: id.NewID()}
	rec, prob := do(router(&p), http.MethodGet, "/players/"+id.NewID()+"/wallet/transactions?limit=zero", "", nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "limit")
}
