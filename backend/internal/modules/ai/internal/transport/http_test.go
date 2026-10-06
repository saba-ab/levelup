package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/ai/contracts"
	"levelup/internal/modules/ai/internal/app"
	"levelup/internal/modules/ai/internal/domain"
	"levelup/internal/modules/ai/internal/llm"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/validate"
)

const (
	tenantA = "0192f0a4-0000-7000-8000-00000000000a"
	badgeID = "0192f0a4-0000-7000-8000-000000000001"
)

// memRepo is an in-memory usage store with the SQL cap semantics.
type memRepo struct {
	mu   sync.Mutex
	rows map[string]domain.UsageDay
}

func (m *memRepo) key(t string, d time.Time) string { return t + d.Format(time.DateOnly) }

func (m *memRepo) Reserve(_ context.Context, _ *gorm.DB, tenantID string, day time.Time, limit int, _ time.Time) (int64, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(tenantID, day)
	row := m.rows[k]
	if limit > 0 && row.Requests >= int64(limit) {
		return row.Requests, false, nil
	}
	row.Day, row.Requests = domain.DayOf(day), row.Requests+1
	m.rows[k] = row
	return row.Requests, true, nil
}

func (m *memRepo) AddTokens(_ context.Context, _ *gorm.DB, tenantID string, day time.Time, in, out int64, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(tenantID, day)
	row := m.rows[k]
	row.Day, row.InputTokens, row.OutputTokens = domain.DayOf(day), row.InputTokens+in, row.OutputTokens+out
	m.rows[k] = row
	return nil
}

func (m *memRepo) UsageBetween(_ context.Context, tenantID string, from, to time.Time) ([]domain.UsageDay, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.UsageDay
	for d := to; !d.Before(from); d = d.AddDate(0, 0, -1) {
		if row, ok := m.rows[m.key(tenantID, d)]; ok {
			out = append(out, row)
		}
	}
	return out, nil
}

func (m *memRepo) PurgeTenant(context.Context, *gorm.DB, string) error { return nil }

// fakeAnthropic answers /v1/messages with a canned structured-output
// response; status overrides it with an error.
type fakeAnthropic struct {
	srv    *httptest.Server
	calls  atomic.Int32
	status int
	text   string
}

func newFakeAnthropic(t *testing.T, text string) *fakeAnthropic {
	t.Helper()
	f := &fakeAnthropic{status: http.StatusOK, text: text}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.Header().Set("content-type", "application/json")
		if f.status != http.StatusOK {
			w.WriteHeader(f.status)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
			return
		}
		resp, _ := json.Marshal(map[string]any{
			"model":       body["model"],
			"content":     []any{map[string]any{"type": "text", "text": f.text}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1200, "output_tokens": 400},
		})
		_, _ = w.Write(resp)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

type allowAll struct{}

func (allowAll) Authorize(context.Context, authz.Principal, authz.Permission, any) error { return nil }

func newRouter(t *testing.T, f *fakeAnthropic, enabled bool, limit int) http.Handler {
	t.Helper()
	gen := llm.New(llm.Options{
		APIKey: "sk-test", BaseURL: f.srv.URL, Model: "claude-sonnet-5-5", MaxTokens: 4096,
		Timeout: 5 * time.Second, MaxRetries: 0,
	})
	svc := app.NewService(&memRepo{rows: map[string]domain.UsageDay{}}, gen, allowAll{}, nil,
		clock.NewFake(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)), nil,
		app.Settings{Enabled: enabled, Model: "claude-sonnet-5-5", DailyLimit: limit})
	svc.SetTxRunner(func(ctx context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) })
	h := NewHandler(svc, validate.New())
	h.now = func() time.Time { return time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC) }
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Header.Get("X-Test-Anon") == "" {
				req = req.WithContext(authz.Into(req.Context(), authz.Principal{UserID: "u1", TenantID: tenantA}))
			}
			next.ServeHTTP(w, req)
		})
	})
	h.Mount(r)
	return r
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	var p struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))
	return p.Code
}

const ruleOutput = `{"drafts":[
 {"name":"Purchase points","description":"10 points per purchase","trigger_event":"purchase_completed","priority":0,
  "conditions":{"all":[{"source":"trigger","field":"amount","operator":"gt","value":0}],"any":null},
  "actions":[{"type":"credit_points","amount":10,"description":null,"badge_id":null,"mission_id":null,"increment":null,"reward_id":null,"activity_key":null},
             {"type":"award_badge","amount":null,"description":null,"badge_id":"` + badgeID + `","mission_id":null,"increment":null,"reward_id":null,"activity_key":null}],
  "limits":{"max_per_player":null,"max_per_player_per_day":5,"max_per_player_per_week":null,"cooldown_seconds":null}},
 {"name":"Hallucinated","description":"","trigger_event":"purchase_completed","priority":0,"conditions":null,
  "actions":[{"type":"grant_reward","amount":null,"description":null,"badge_id":null,"mission_id":null,"increment":null,
              "reward_id":"0192f0a4-0000-7000-8000-0000000000ff","activity_key":null}],"limits":null}]}`

const draftBody = `{"kind":"rule","prompt":"10 points per purchase, max 5 a day","count":2,
 "context":{"event_types":[{"slug":"purchase_completed","name":"Purchase"}],"badges":[{"id":"` + badgeID + `","name":"Buyer"}]}}`

func TestDraftEndpointReturnsCreateShapedDrafts(t *testing.T) {
	f := newFakeAnthropic(t, ruleOutput)
	h := newRouter(t, f, true, 10)

	rec := do(t, h, http.MethodPost, "/ai/drafts", draftBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var resp DraftResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "rule", resp.Kind)
	require.Equal(t, "claude-sonnet-5-5", resp.Model)
	require.Len(t, resp.Drafts, 1)
	require.Len(t, resp.Rejected, 1)
	require.Equal(t, 1, resp.Rejected[0].Index)
	require.Equal(t, []string{"actions[0].reward_id: must be the id of a reward listed in context"}, resp.Rejected[0].Reasons)
	require.Equal(t, TokenUsageResp{InputTokens: 1200, OutputTokens: 400}, resp.Usage)
	require.Equal(t, QuotaResp{DailyLimit: 10, Used: 1, Remaining: 9}, resp.Quota)

	draft, err := json.Marshal(resp.Drafts[0])
	require.NoError(t, err)
	require.JSONEq(t, `{
		"name":"Purchase points","description":"10 points per purchase","trigger_event":"purchase_completed","priority":0,
		"conditions":{"all":[{"source":"trigger","field":"amount","operator":"gt","value":0}]},
		"actions":[{"type":"credit_points","amount":10},{"type":"award_badge","badge_id":"`+badgeID+`"}],
		"limits":{"max_per_player_per_day":5}}`, string(draft), "exactly the rules CreateRuleReq body")

	rec = do(t, h, http.MethodGet, "/ai/usage?days=7", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var usage UsageResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &usage))
	require.Equal(t, UsageDayResp{Day: "2026-10-06", Requests: 1, InputTokens: 1200, OutputTokens: 400}, usage.Today)
	require.Equal(t, int64(9), usage.Remaining)
	require.Len(t, usage.Data, 1)
}

func TestDraftEndpointQuotaIs429(t *testing.T) {
	f := newFakeAnthropic(t, `{"drafts":[]}`)
	h := newRouter(t, f, true, 1)
	require.Equal(t, http.StatusOK, do(t, h, http.MethodPost, "/ai/drafts", draftBody).Code)

	rec := do(t, h, http.MethodPost, "/ai/drafts", draftBody)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, contracts.CodeQuotaExceeded, problemCode(t, rec))
	require.Equal(t, "3601", rec.Header().Get("Retry-After"), "until the next UTC midnight")
	require.EqualValues(t, 1, f.calls.Load())
}

func TestDraftEndpointErrors(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		f := newFakeAnthropic(t, `{}`)
		rec := do(t, newRouter(t, f, false, 10), http.MethodPost, "/ai/drafts", draftBody)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, contracts.CodeNotConfigured, problemCode(t, rec))
		require.Zero(t, f.calls.Load())
	})
	t.Run("provider overloaded", func(t *testing.T) {
		f := newFakeAnthropic(t, `{}`)
		f.status = 529
		rec := do(t, newRouter(t, f, true, 10), http.MethodPost, "/ai/drafts", draftBody)
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
		require.Equal(t, contracts.CodeUnavailable, problemCode(t, rec))
		require.NotContains(t, rec.Body.String(), "Overloaded", "5xx detail is not echoed")
	})
	t.Run("validation", func(t *testing.T) {
		f := newFakeAnthropic(t, `{}`)
		h := newRouter(t, f, true, 10)
		for _, body := range []string{
			`{"kind":"streak","prompt":"x"}`,
			`{"kind":"badge","prompt":""}`,
			`{"kind":"badge","prompt":"` + strings.Repeat("a", 2001) + `"}`,
			`{"kind":"badge","prompt":"x","count":6}`,
			`{"kind":"badge","prompt":"x","context":{"badges":[{"id":"nope"}]}}`,
			`{"kind":"badge","prompt":"x","schema":{}}`,
		} {
			rec := do(t, h, http.MethodPost, "/ai/drafts", body)
			require.Equal(t, http.StatusUnprocessableEntity, rec.Code, body)
		}
		require.Zero(t, f.calls.Load())
	})
	t.Run("anonymous", func(t *testing.T) {
		f := newFakeAnthropic(t, `{}`)
		req := httptest.NewRequest(http.MethodGet, "/ai/templates", nil)
		req.Header.Set("X-Test-Anon", "1")
		rec := httptest.NewRecorder()
		newRouter(t, f, true, 10).ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestTemplatesEndpoint(t *testing.T) {
	f := newFakeAnthropic(t, `{}`)
	rec := do(t, newRouter(t, f, false, 10), http.MethodGet, "/ai/templates", "")
	require.Equal(t, http.StatusOK, rec.Code)
	var resp TemplateListResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 10)
	require.Equal(t, TemplateResp{
		ID: "badge-concept", Name: "Badge Concept", Kind: "badge",
		Description:   "One badge with a name, tier, category and unlock criteria from a short idea.",
		ExamplePrompt: "A badge for customers who leave their first product review.", DefaultCount: 1,
	}, resp.Data[0])
}
