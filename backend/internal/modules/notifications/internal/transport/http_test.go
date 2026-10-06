package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"levelup/internal/modules/notifications/internal/app"
	"levelup/internal/modules/notifications/internal/domain"
	"levelup/internal/shared/validate"
)

// Routes: unauthenticated requests are 401 and the player feed routes
// coexist with another module's /players subtree.
func TestRoutesRequireAuthAndCoexistWithPlayersSubtree(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/players", func(r chi.Router) {
		r.Get("/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	})
	r.With().Get("/players/{playerID}/badges", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	NewHandler(nil, validate.New()).Mount(r)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/notifications/templates"},
		{http.MethodPost, "/notifications/templates"},
		{http.MethodGet, "/notifications/templates/x"},
		{http.MethodPatch, "/notifications/templates/x"},
		{http.MethodDelete, "/notifications/templates/x"},
		{http.MethodPost, "/notifications/templates/x/preview"},
		{http.MethodGet, "/notifications/channels"},
		{http.MethodPatch, "/notifications/channels"},
		{http.MethodGet, "/notifications/history"},
		{http.MethodGet, "/notifications/stats"},
		{http.MethodGet, "/players/y/notifications"},
		{http.MethodPost, "/players/y/notifications/read-all"},
		{http.MethodPost, "/players/y/notifications/z/read"},
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}")))
		require.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", tc.method, tc.path)
	}
	for _, path := range []string{"/players/y", "/players/y/badges"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusTeapot, rec.Code, "the other modules' routes are untouched: %s", path)
	}
}

func TestCreateReqValidation(t *testing.T) {
	val := validate.New()
	ok := CreateTemplateReq{Name: "x", Trigger: "badges.awarded", Channels: []string{"in_app", "email"}, TitleTemplate: "Hi"}
	require.NoError(t, val.Struct(ok))
	require.True(t, ok.params().Active, "is_active defaults to true")

	for name, bad := range map[string]CreateTemplateReq{
		"trigger":  {Name: "x", Trigger: "points.debited", Channels: []string{"in_app"}, TitleTemplate: "Hi"},
		"channel":  {Name: "x", Trigger: "badges.awarded", Channels: []string{"sms"}, TitleTemplate: "Hi"},
		"empty":    {Name: "x", Trigger: "badges.awarded", Channels: []string{}, TitleTemplate: "Hi"},
		"no title": {Name: "x", Trigger: "badges.awarded", Channels: []string{"in_app"}},
	} {
		require.Error(t, val.Struct(bad), name)
	}

	var patch UpdateTemplateReq
	require.NoError(t, json.Unmarshal([]byte(`{"channels":["push"]}`), &patch))
	require.Error(t, val.Struct(patch))
	require.NoError(t, json.Unmarshal([]byte(`{"channels":["email"],"is_active":false}`), &patch))
	require.NoError(t, val.Struct(patch))
	require.Equal(t, []string{"email"}, *patch.patch().Channels)
	require.False(t, *patch.patch().Active)
	require.Nil(t, patch.patch().Name, "omitted stays untouched")

	require.Error(t, val.Struct(UpdateChannelsReq{}), "email object is required")
	evil := "Acme\r\nBcc: x"
	require.Error(t, val.Struct(UpdateChannelsReq{Email: &EmailChannelPatch{FromName: &evil}}))
}

func TestMalformedPathIDIsNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/notifications/templates/42", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "42")
	req = req.WithContext(chiCtx(req, rctx))
	_, err := pathUUID(req, "id", domain.ErrTemplateNotFound)
	require.ErrorIs(t, err, domain.ErrTemplateNotFound)
}

func TestStatsRespShape(t *testing.T) {
	s := app.StatsResult{
		Stats: domain.Aggregate([]domain.StatsRow{
			{TemplateID: "b", Channel: "in_app", Status: "delivered", Count: 1},
			{TemplateID: "a", Channel: "in_app", Status: "delivered", Count: 4, Read: 2},
		}),
		TemplateNames:    map[string]string{"a": "Alpha"},
		TemplateTriggers: map[string]string{"a": "badges.awarded"},
	}
	raw, err := json.Marshal(toStatsResp(s))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	for _, k := range []string{"sent", "delivered", "failed", "pending", "skipped", "read", "by_channel", "by_template", "open_rate"} {
		require.Contains(t, got, k)
	}
	require.Contains(t, got["by_channel"], "email", "both channels always present")
	byTemplate := got["by_template"].([]any)
	require.Equal(t, "Alpha", byTemplate[0].(map[string]any)["name"], "sorted by sent desc")
	require.InDelta(t, 0.4, got["open_rate"], 1e-9)
}
