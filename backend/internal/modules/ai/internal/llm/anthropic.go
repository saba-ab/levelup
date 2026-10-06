// Package llm is ai's Anthropic Messages API client over plain net/http
// (no SDK dependency, ADR-0009). It sends one structured-output request
// (output_config.format json_schema) and maps provider failures onto errs
// kinds with contracts.Code* codes.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"levelup/internal/modules/ai/contracts"
	"levelup/internal/modules/ai/internal/app"
	"levelup/internal/modules/ai/internal/domain"
	"levelup/internal/shared/errs"
)

const (
	apiVersion   = "2023-06-01"
	fallbackBeta = "server-side-fallback-2026-07-01"
	maxBodyBytes = 4 << 20
	maxRetryWait = 5 * time.Second
)

// Options configure the client.
type Options struct {
	APIKey    string
	BaseURL   string // default https://api.anthropic.com
	Model     string
	MaxTokens int
	Effort    string // low | medium | high | xhigh | max; "" = model default
	// RefusalFallback opts into the server-side refusal fallback
	// (fallbacks: "default"), which re-runs a declined request on a model
	// chosen by refusal category inside the same call.
	RefusalFallback bool
	Timeout         time.Duration
	MaxRetries      int // retries of 429/5xx/529/connection errors
	HTTP            *http.Client
}

type Client struct {
	opt  Options
	http *http.Client
}

var _ app.Generator = (*Client)(nil)

func New(opt Options) *Client {
	if opt.BaseURL == "" {
		opt.BaseURL = "https://api.anthropic.com"
	}
	opt.BaseURL = strings.TrimRight(opt.BaseURL, "/")
	if opt.Timeout <= 0 {
		opt.Timeout = 60 * time.Second
	}
	hc := opt.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: opt.Timeout}
	}
	return &Client{opt: opt, http: hc}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type outputFormat struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema"`
}

type outputConfig struct {
	Effort string       `json:"effort,omitempty"`
	Format outputFormat `json:"format"`
}

type request struct {
	Model        string       `json:"model"`
	MaxTokens    int          `json:"max_tokens"`
	System       string       `json:"system"`
	Messages     []message    `json:"messages"`
	OutputConfig outputConfig `json:"output_config"`
	Fallbacks    string       `json:"fallbacks,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type response struct {
	Model       string         `json:"model"`
	Content     []contentBlock `json:"content"`
	StopReason  string         `json:"stop_reason"`
	StopDetails *struct {
		Category *string `json:"category"`
	} `json:"stop_details"`
	Usage struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Generate sends one request and returns the model's JSON text.
func (c *Client) Generate(ctx context.Context, in app.GenerateRequest) (app.GenerateResult, error) {
	body := request{
		Model:     c.opt.Model,
		MaxTokens: c.opt.MaxTokens,
		System:    in.System,
		Messages:  []message{{Role: "user", Content: in.User}},
		OutputConfig: outputConfig{
			Effort: c.opt.Effort,
			Format: outputFormat{Type: "json_schema", Schema: in.Schema},
		},
	}
	if c.opt.RefusalFallback {
		body.Fallbacks = "default"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return app.GenerateResult{}, errs.Wrap(errs.Internal, "encode anthropic request", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.opt.MaxRetries; attempt++ {
		res, wait, err := c.do(ctx, payload)
		if err == nil || wait < 0 {
			return res, err
		}
		lastErr = err
		if attempt == c.opt.MaxRetries || !sleep(ctx, wait) {
			break
		}
	}
	return app.GenerateResult{}, lastErr
}

// do performs one attempt. wait >= 0 means the failure is retryable after
// wait; wait < 0 means final (success or permanent failure).
func (c *Client) do(ctx context.Context, payload []byte) (app.GenerateResult, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opt.BaseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return app.GenerateResult{}, -1, errs.Wrap(errs.Internal, "build anthropic request", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.opt.APIKey)
	req.Header.Set("anthropic-version", apiVersion)
	if c.opt.RefusalFallback {
		req.Header.Set("anthropic-beta", fallbackBeta)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return app.GenerateResult{}, -1, unavailable("anthropic request cancelled or timed out", err)
		}
		return app.GenerateResult{}, 500 * time.Millisecond, unavailable("anthropic request failed", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return app.GenerateResult{}, 500 * time.Millisecond, unavailable("read anthropic response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return app.GenerateResult{}, retryAfter(resp), statusError(resp.StatusCode, raw)
	}

	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return app.GenerateResult{}, -1, errs.WithCode(errs.Wrap(errs.Unavailable, "decode anthropic response", err), contracts.CodeBadOutput)
	}
	out := app.GenerateResult{
		Model:        r.Model,
		InputTokens:  r.Usage.InputTokens + r.Usage.CacheCreationInputTokens + r.Usage.CacheReadInputTokens,
		OutputTokens: r.Usage.OutputTokens,
	}
	switch r.StopReason {
	case "refusal":
		return out, -1, domain.ErrRefused
	case "max_tokens":
		return out, -1, domain.ErrTruncated
	}
	var text strings.Builder
	for _, b := range r.Content {
		if b.Type == "text" {
			text.WriteString(b.Text)
		}
	}
	if text.Len() == 0 {
		return out, -1, domain.ErrBadOutput
	}
	out.JSON = []byte(text.String())
	return out, -1, nil
}

func unavailable(msg string, err error) error {
	return errs.WithCode(errs.Wrap(errs.Unavailable, msg, err), contracts.CodeUnavailable)
}

// statusError maps a non-200 response. The provider's message goes into the
// wrapped cause (logged), never into a 5xx body.
func statusError(status int, raw []byte) error {
	var ae apiError
	_ = json.Unmarshal(raw, &ae)
	cause := fmt.Errorf("anthropic %d %s: %s", status, ae.Error.Type, ae.Error.Message)
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden, status == http.StatusNotFound:
		// Bad key, no access, or an unknown model: the deployment is misconfigured.
		return errs.WithCode(errs.Wrap(errs.Unavailable, "AI provider rejected the configuration", cause), contracts.CodeNotConfigured)
	case status == http.StatusRequestEntityTooLarge:
		return errs.WithCode(errs.Wrap(errs.Invalid, "the request context is too large", cause), contracts.CodeInvalidContext)
	case retryable(status):
		return errs.WithCode(errs.Wrap(errs.Unavailable, "AI provider is temporarily unavailable", cause), contracts.CodeUnavailable)
	default:
		return errs.WithCode(errs.Wrap(errs.Internal, "AI provider rejected the request", cause), contracts.CodeUpstreamError)
	}
}

func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status == 529 || status >= 500
}

func retryAfter(resp *http.Response) time.Duration {
	if !retryable(resp.StatusCode) {
		return -1
	}
	if s := resp.Header.Get("retry-after"); s != "" {
		if secs, err := strconv.ParseFloat(s, 64); err == nil {
			d := time.Duration(secs * float64(time.Second))
			if d > maxRetryWait {
				return -1 // not worth holding the request open
			}
			return d
		}
	}
	return 500 * time.Millisecond
}

func sleep(ctx context.Context, d time.Duration) bool {
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) < d {
		return false
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
