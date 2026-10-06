package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/ai/contracts"
	"levelup/internal/modules/ai/internal/app"
	"levelup/internal/shared/errs"
)

// fakeAnthropic is an httptest Messages API. handle returns status and body
// for the n-th call (0-based); the last request body is kept.
type fakeAnthropic struct {
	srv     *httptest.Server
	calls   atomic.Int32
	lastReq map[string]any
	lastHdr http.Header
}

func newFake(t *testing.T, handle func(n int) (int, http.Header, string)) *fakeAnthropic {
	t.Helper()
	f := &fakeAnthropic{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(f.calls.Add(1)) - 1
		require.Equal(t, "/v1/messages", r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		f.lastReq = map[string]any{}
		_ = json.Unmarshal(b, &f.lastReq)
		f.lastHdr = r.Header.Clone()
		status, hdr, body := handle(n)
		for k, v := range hdr {
			w.Header()[k] = v
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAnthropic) client(opts ...func(*Options)) *Client {
	o := Options{
		APIKey: "sk-test", BaseURL: f.srv.URL, Model: "claude-sonnet-5-5", MaxTokens: 4096,
		Effort: "low", RefusalFallback: true, Timeout: 5 * time.Second, MaxRetries: 1,
	}
	for _, fn := range opts {
		fn(&o)
	}
	return New(o)
}

func okBody(text string) string {
	b, _ := json.Marshal(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-sonnet-5-5",
		"content": []any{
			map[string]any{"type": "thinking", "thinking": "", "signature": "sig"},
			map[string]any{"type": "text", "text": text},
		},
		"stop_reason": "end_turn", "stop_details": nil,
		"usage": map[string]any{"input_tokens": 100, "output_tokens": 50, "cache_read_input_tokens": 20, "cache_creation_input_tokens": 0},
	})
	return string(b)
}

func errBody(typ, msg string) string {
	return `{"type":"error","error":{"type":"` + typ + `","message":"` + msg + `"}}`
}

var schema = map[string]any{"type": "object", "properties": map[string]any{"drafts": map[string]any{"type": "array"}}}

func gen(t *testing.T, c *Client) (app.GenerateResult, error) {
	t.Helper()
	return c.Generate(context.Background(), app.GenerateRequest{System: "sys", User: "usr", Schema: schema})
}

func TestGenerateSendsStructuredOutputRequest(t *testing.T) {
	f := newFake(t, func(int) (int, http.Header, string) { return 200, nil, okBody(`{"drafts":[]}`) })
	res, err := gen(t, f.client())
	require.NoError(t, err)
	require.JSONEq(t, `{"drafts":[]}`, string(res.JSON), "thinking blocks are skipped, text is returned")
	require.Equal(t, "claude-sonnet-5-5", res.Model)
	require.Equal(t, int64(120), res.InputTokens, "cache reads count as input")
	require.Equal(t, int64(50), res.OutputTokens)

	require.Equal(t, "sk-test", f.lastHdr.Get("x-api-key"))
	require.Equal(t, "2023-06-01", f.lastHdr.Get("anthropic-version"))
	require.Equal(t, "server-side-fallback-2026-07-01", f.lastHdr.Get("anthropic-beta"))
	require.Equal(t, "claude-sonnet-5-5", f.lastReq["model"])
	require.EqualValues(t, 4096, f.lastReq["max_tokens"])
	require.Equal(t, "sys", f.lastReq["system"])
	require.Equal(t, "default", f.lastReq["fallbacks"])
	require.Equal(t, []any{map[string]any{"role": "user", "content": "usr"}}, f.lastReq["messages"])
	oc := f.lastReq["output_config"].(map[string]any)
	require.Equal(t, "low", oc["effort"])
	format := oc["format"].(map[string]any)
	require.Equal(t, "json_schema", format["type"])
	require.Equal(t, "object", format["schema"].(map[string]any)["type"])
	require.NotContains(t, f.lastReq, "tool_choice", "forced tool use 400s on current models")
	require.NotContains(t, f.lastReq, "thinking")
}

func TestGenerateWithoutFallbackOmitsBetaHeader(t *testing.T) {
	f := newFake(t, func(int) (int, http.Header, string) { return 200, nil, okBody(`{}`) })
	_, err := gen(t, f.client(func(o *Options) { o.RefusalFallback = false; o.Effort = "" }))
	require.NoError(t, err)
	require.Empty(t, f.lastHdr.Get("anthropic-beta"))
	require.NotContains(t, f.lastReq, "fallbacks")
	require.NotContains(t, f.lastReq["output_config"], "effort")
}

func TestGenerateStopReasons(t *testing.T) {
	for _, tc := range []struct {
		stop string
		code string
		kind errs.Kind
	}{
		{"refusal", contracts.CodeRefused, errs.Invalid},
		{"max_tokens", contracts.CodeOutputTruncated, errs.Invalid},
	} {
		t.Run(tc.stop, func(t *testing.T) {
			f := newFake(t, func(int) (int, http.Header, string) {
				return 200, nil, `{"model":"m","content":[{"type":"text","text":"{\"dra"}],"stop_reason":"` + tc.stop + `",
					"usage":{"input_tokens":10,"output_tokens":7}}`
			})
			res, err := gen(t, f.client())
			require.Equal(t, tc.kind, errs.KindOf(err))
			require.Equal(t, tc.code, errs.CodeOf(err))
			require.Equal(t, int64(7), res.OutputTokens, "usage is reported with the error so it is accounted")
			require.EqualValues(t, 1, f.calls.Load(), "not retried")
		})
	}
}

func TestGenerateNoTextIsBadOutput(t *testing.T) {
	f := newFake(t, func(int) (int, http.Header, string) {
		return 200, nil, `{"model":"m","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
	})
	_, err := gen(t, f.client())
	require.Equal(t, contracts.CodeBadOutput, errs.CodeOf(err))
}

func TestGenerateErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		typ    string
		kind   errs.Kind
		code   string
		calls  int32
	}{
		{"bad key", 401, "authentication_error", errs.Unavailable, contracts.CodeNotConfigured, 1},
		{"no access", 403, "permission_error", errs.Unavailable, contracts.CodeNotConfigured, 1},
		{"unknown model", 404, "not_found_error", errs.Unavailable, contracts.CodeNotConfigured, 1},
		{"bad request", 400, "invalid_request_error", errs.Internal, contracts.CodeUpstreamError, 1},
		{"too large", 413, "request_too_large", errs.Invalid, contracts.CodeInvalidContext, 1},
		{"rate limited", 429, "rate_limit_error", errs.Unavailable, contracts.CodeUnavailable, 2},
		{"server error", 500, "api_error", errs.Unavailable, contracts.CodeUnavailable, 2},
		{"overloaded", 529, "overloaded_error", errs.Unavailable, contracts.CodeUnavailable, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t, func(int) (int, http.Header, string) {
				return tc.status, http.Header{"Retry-After": {"0"}}, errBody(tc.typ, "boom")
			})
			_, err := gen(t, f.client())
			require.Error(t, err)
			require.Equal(t, tc.kind, errs.KindOf(err))
			require.Equal(t, tc.code, errs.CodeOf(err))
			require.Contains(t, err.Error(), "boom", "provider message kept in the cause for logs")
			require.Equal(t, tc.calls, f.calls.Load())
		})
	}
}

func TestGenerateRetriesTransientThenSucceeds(t *testing.T) {
	f := newFake(t, func(n int) (int, http.Header, string) {
		if n == 0 {
			return 529, http.Header{"Retry-After": {"0"}}, errBody("overloaded_error", "busy")
		}
		return 200, nil, okBody(`{"drafts":[]}`)
	})
	res, err := gen(t, f.client())
	require.NoError(t, err)
	require.NotEmpty(t, res.JSON)
	require.EqualValues(t, 2, f.calls.Load())
}

func TestGenerateLongRetryAfterIsNotWaited(t *testing.T) {
	f := newFake(t, func(int) (int, http.Header, string) {
		return 429, http.Header{"Retry-After": {"60"}}, errBody("rate_limit_error", "slow down")
	})
	_, err := gen(t, f.client())
	require.Equal(t, contracts.CodeUnavailable, errs.CodeOf(err))
	require.EqualValues(t, 1, f.calls.Load())
}

func TestGenerateTimeoutIsUnavailable(t *testing.T) {
	f := newFake(t, func(int) (int, http.Header, string) {
		time.Sleep(300 * time.Millisecond)
		return 200, nil, okBody(`{}`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := f.client().Generate(ctx, app.GenerateRequest{Schema: schema})
	require.Equal(t, errs.Unavailable, errs.KindOf(err))
	require.Equal(t, contracts.CodeUnavailable, errs.CodeOf(err))
}
