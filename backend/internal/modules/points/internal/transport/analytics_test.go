package transport

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/platform/authz"
	"levelup/internal/shared/id"
)

func TestAnalyticsRoutesRequireAuth(t *testing.T) {
	h := router(nil)
	for _, path := range []string{"/wallets?player_ids=" + id.NewID(), "/wallets/summary", "/wallets/distribution", "/wallets/daily"} {
		rec, _ := do(h, http.MethodGet, path, "", nil)
		require.Equal(t, http.StatusUnauthorized, rec.Code, path)
	}
}

func TestBatchWalletsValidatesIDs(t *testing.T) {
	p := authz.Principal{UserID: id.NewID(), TenantID: id.NewID()}
	h := router(&p)

	rec, prob := do(h, http.MethodGet, "/wallets", "", nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "player_ids")

	rec, prob = do(h, http.MethodGet, "/wallets?player_ids="+id.NewID()+",nope", "", nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "player_ids")

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = id.NewID()
	}
	rec, prob = do(h, http.MethodGet, "/wallets?player_ids="+strings.Join(ids, ","), "", nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "too_many_ids", prob.Code)
}

func TestDailyValidatesDates(t *testing.T) {
	p := authz.Principal{UserID: id.NewID(), TenantID: id.NewID()}
	h := router(&p)

	rec, prob := do(h, http.MethodGet, "/wallets/daily?from=yesterday", "", nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, prob.Errors, "from")

	rec, prob = do(h, http.MethodGet, "/wallets/daily?from=2026-10-05&to=2026-10-01", "", nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t, "invalid_range", prob.Code)
}
