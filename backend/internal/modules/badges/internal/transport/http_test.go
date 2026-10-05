package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"levelup/internal/platform/httpx"
	"levelup/internal/shared/validate"
)

func TestOptionalDistinguishesAbsentNullAndValue(t *testing.T) {
	var absent, null, set UpdateBadgeReq
	require.NoError(t, json.Unmarshal([]byte(`{}`), &absent))
	require.NoError(t, json.Unmarshal([]byte(`{"max_awards":null,"requirements":null}`), &null))
	require.NoError(t, json.Unmarshal([]byte(`{"max_awards":4,"requirements":{"a":1}}`), &set))

	require.False(t, absent.patch().MaxAwardsSet)
	require.False(t, absent.patch().RequirementsSet)
	require.True(t, null.patch().MaxAwardsSet)
	require.Nil(t, null.patch().MaxAwards)
	require.True(t, null.patch().RequirementsSet)
	require.Nil(t, null.patch().Requirements)
	require.Equal(t, 4, *set.patch().MaxAwards)
	require.Equal(t, map[string]any{"a": 1.0}, set.patch().Requirements)
}

func TestCreateReqValidationAndDefaults(t *testing.T) {
	val := validate.New()
	bad := CreateBadgeReq{Name: "x", Tier: "mythic", Category: "skill"}
	require.Error(t, val.Struct(bad))
	neg := int64(-1)
	require.Error(t, val.Struct(CreateBadgeReq{Name: "x", Tier: "gold", Category: "skill", PointsValue: &neg}))
	ok := CreateBadgeReq{Name: "x", Tier: "gold", Category: "skill"}
	require.NoError(t, val.Struct(ok))
	require.True(t, ok.params().Active, "is_active defaults to true")
	require.Nil(t, ok.params().PointsValue, "omitted points → tier default in the domain")
}

// Routes: unauthenticated requests are 401 and the player sub-route
// coexists with another module's /players subtree.
func TestRoutesRequireAuthAndCoexistWithPlayersSubtree(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/players", func(r chi.Router) {
		r.Get("/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	})
	NewHandler(nil, validate.New()).Mount(r)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/badges"},
		{http.MethodPost, "/badges"},
		{http.MethodGet, "/badges/x"},
		{http.MethodPatch, "/badges/x"},
		{http.MethodDelete, "/badges/x"},
		{http.MethodPost, "/badges/x/award"},
		{http.MethodDelete, "/badges/x/players/y"},
		{http.MethodGet, "/players/y/badges"},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}")))
		require.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", tc.method, tc.path)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/players/y", nil))
	require.Equal(t, http.StatusTeapot, rec.Code, "the player module's routes are untouched")
}

func TestMalformedPathIDIsNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/badges/42", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "42")
	req = req.WithContext(chiCtx(req, rctx))
	_, err := pathUUID(req, "id", errMalformedBadgeID)
	rec := httptest.NewRecorder()
	httpx.Error(rec, req, err)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"badge_not_found"`)
}
