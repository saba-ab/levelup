package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"

	"levelup/internal/modules/webhooks/internal/domain"
)

// HTTPSender POSTs deliveries. Its client enforces the SSRF policy at
// connect time (see domain.URLPolicy), ignores proxy settings, never
// follows redirects (a 3xx is a failed attempt) and times out after
// Timeout.
type HTTPSender struct {
	client    *http.Client
	userAgent string
}

// errBlockedDestination is returned by the dialer's Control hook.
var errBlockedDestination = errors.New("destination address is not allowed")

func NewHTTPSender(policy domain.URLPolicy, timeout time.Duration, userAgent string) *HTTPSender {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		// Control runs after DNS resolution, right before connect, for
		// every address tried: the resolve-time SSRF check.
		Control: func(_, address string, _ syscall.RawConn) error {
			if !policy.AllowDial(address) {
				return errBlockedDestination
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy:                 nil, // a proxy would hide the real destination from Control
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
	}
	return &HTTPSender{
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		userAgent: userAgent,
	}
}

var _ Sender = (*HTTPSender)(nil)

func (h *HTTPSender) Send(ctx context.Context, r Request) domain.AttemptResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(r.Body))
	if err != nil {
		return domain.AttemptResult{Err: "invalid request: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", h.userAgent)
	req.Header.Set(domain.HeaderEvent, r.Event)
	req.Header.Set(domain.HeaderDelivery, r.DeliveryID)
	req.Header.Set(domain.HeaderSignature, domain.SignatureHeader(r.Secret, r.At, r.Body))

	start := time.Now()
	resp, err := h.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return domain.AttemptResult{LatencyMS: latency, Err: describe(err)}
	}
	defer resp.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, domain.ResponseSnippetLimit))
	// Drain a bounded tail so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return domain.AttemptResult{StatusCode: resp.StatusCode, LatencyMS: latency, Body: string(snippet)}
}

func describe(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, errBlockedDestination):
		return "blocked destination: the URL resolves to a private or reserved address"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	default:
		return "request failed: " + err.Error()
	}
}
