package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"levelup/internal/platform/authz"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/validate"
)

func decodeUpdate(t *testing.T, body string) (UpdateEventTypeReq, error) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
	var req UpdateEventTypeReq
	err := httpx.Decode(r, &req, validate.New())
	return req, err
}

func TestUpdateReqDistinguishesAbsentFromNull(t *testing.T) {
	req, err := decodeUpdate(t, `{"name":"x"}`)
	require.NoError(t, err)
	p, err := req.toPatch()
	require.NoError(t, err)
	require.Nil(t, p.CategoryID, "absent category leaves it untouched")
	require.False(t, p.SetSchema, "absent schema leaves it untouched")
	require.Nil(t, p.Description)

	req, err = decodeUpdate(t, `{"category_id":null,"property_schema":null,"description":""}`)
	require.NoError(t, err)
	p, err = req.toPatch()
	require.NoError(t, err)
	require.NotNil(t, p.CategoryID)
	require.Empty(t, *p.CategoryID, "null clears the category")
	require.True(t, p.SetSchema)
	require.Nil(t, p.PropertySchema, "null clears the schema")
	require.Empty(t, *p.Description)

	req, err = decodeUpdate(t, `{"category_id":"0199a000-0000-7000-8000-0000000000ca","property_schema":{"type":"object"}}`)
	require.NoError(t, err)
	p, err = req.toPatch()
	require.NoError(t, err)
	require.Equal(t, "0199a000-0000-7000-8000-0000000000ca", *p.CategoryID)
	require.Equal(t, map[string]any{"type": "object"}, p.PropertySchema)
}

func TestUpdateReqShapeErrors(t *testing.T) {
	cases := map[string]string{
		"category_id":     `{"category_id":"nope"}`,
		"property_schema": `{"property_schema":[1,2]}`,
	}
	for field, body := range cases {
		req, err := decodeUpdate(t, body)
		require.NoError(t, err)
		_, err = req.toPatch()
		require.Equal(t, errs.Invalid, errs.KindOf(err))
		require.Contains(t, errs.FieldsOf(err), field)
	}

	_, err := decodeUpdate(t, `{"name":""}`)
	require.Equal(t, errs.Invalid, errs.KindOf(err), "empty name fails min=1")
	_, err = decodeUpdate(t, `{"tenant_id":"0199a000-0000-7000-8000-00000000000a"}`)
	require.Equal(t, errs.Invalid, errs.KindOf(err), "tenant_id is not an accepted field")
}

func TestCreateReqRejectsTenantAndPredefinedFields(t *testing.T) {
	for _, body := range []string{
		`{"name":"x","tenant_id":"0199a000-0000-7000-8000-00000000000a"}`,
		`{"name":"x","is_predefined":true}`,
		`{"slug":"x"}`,
		`{"name":"x","category_id":"nope"}`,
	} {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		var req CreateEventTypeReq
		err := httpx.Decode(r, &req, validate.New())
		require.Equal(t, errs.Invalid, errs.KindOf(err), body)
	}
}

func TestParseListQuery(t *testing.T) {
	parse := func(qs string) error {
		_, err := parseListQuery(httptest.NewRequest(http.MethodGet, "/events?"+qs, nil))
		return err
	}
	q, err := parseListQuery(httptest.NewRequest(http.MethodGet, "/events", nil))
	require.NoError(t, err)
	require.True(t, q.IncludeGlobal, "include_global defaults to true")
	require.Nil(t, q.Active)
	require.Zero(t, q.Limit)

	q, err = parseListQuery(httptest.NewRequest(http.MethodGet,
		"/events?include_global=false&active=1&limit=10&category=commerce&search=buy&cursor=abc", nil))
	require.NoError(t, err)
	require.False(t, q.IncludeGlobal)
	require.True(t, *q.Active)
	require.Equal(t, 10, q.Limit)
	require.Equal(t, "commerce", q.Category)
	require.Equal(t, "buy", q.Search)
	require.Equal(t, "abc", q.Cursor)

	// Laravel coerced garbage booleans to false; here they are 422.
	for _, bad := range []string{"active=foo", "include_global=maybe", "active=", "limit=0", "limit=101", "limit=x"} {
		require.Equal(t, errs.Invalid, errs.KindOf(parse(bad)), bad)
	}
}

func newRouter() http.Handler {
	r := chi.NewRouter()
	NewHandler(nil, validate.New()).Mount(r)
	return r
}

func TestRoutesRequireAuth(t *testing.T) {
	router := newRouter()
	for _, path := range []string{"/events", "/event-categories", "/platform/event-types", "/platform/event-categories"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusUnauthorized, rec.Code, path)
	}
}

func TestMalformedIDIsNotFound(t *testing.T) {
	router := newRouter()
	ctx := authz.Into(context.Background(), authz.Principal{UserID: "u", TenantID: "t", RoleIDs: []int64{4}})
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/events/42"},
		{http.MethodDelete, "/events/not-a-uuid"},
		{http.MethodGet, "/platform/event-types/42"},
		{http.MethodDelete, "/platform/event-categories/42"},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil).WithContext(ctx))
		require.Equal(t, http.StatusNotFound, rec.Code, tc.path)
	}
}
