package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	badgescontracts "levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

type captured struct {
	header http.Header
	body   []byte
}

func receiver(t *testing.T, status int, respBody string) (*httptest.Server, chan captured) {
	t.Helper()
	got := make(chan captured, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- captured{header: r.Header.Clone(), body: b}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestHTTPSenderSignsAndRecords(t *testing.T) {
	srv, got := receiver(t, http.StatusOK, strings.Repeat("x", 3000))
	s := NewHTTPSender(domain.URLPolicy{AllowInsecure: true}, 2*time.Second, "LevelUp-Webhooks/test")
	at := time.Now().UTC()
	body := []byte(`{"event":"badges.awarded","data":{}}`)

	res := s.Send(context.Background(), Request{URL: srv.URL + "/hook", Event: "badges.awarded", DeliveryID: "d-1", Secret: "whsec_abc", Body: body, At: at})
	require.True(t, res.OK(), res.Error())
	require.Equal(t, 200, res.StatusCode)
	require.Len(t, res.Body, domain.ResponseSnippetLimit, "1KB response snippet")
	require.GreaterOrEqual(t, res.LatencyMS, int64(0))

	c := <-got
	require.Equal(t, body, c.body)
	require.Equal(t, "application/json", c.header.Get("Content-Type"))
	require.Equal(t, "LevelUp-Webhooks/test", c.header.Get("User-Agent"))
	require.Equal(t, "badges.awarded", c.header.Get("LevelUp-Event"))
	require.Equal(t, "d-1", c.header.Get("LevelUp-Delivery"))
	sig := c.header.Get("LevelUp-Signature")
	require.True(t, strings.HasPrefix(sig, "t="))
	require.True(t, domain.VerifySignature("whsec_abc", sig, c.body, time.Minute, time.Now()))
	require.False(t, domain.VerifySignature("whsec_other", sig, c.body, time.Minute, time.Now()))
}

func TestHTTPSenderNon2xxAndRedirectsFail(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { targetHits.Add(1) }))
	t.Cleanup(target.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)
	s := NewHTTPSender(domain.URLPolicy{AllowInsecure: true}, 2*time.Second, "ua")

	res := s.Send(context.Background(), Request{URL: redirect.URL, Body: []byte(`{}`), At: time.Now()})
	require.False(t, res.OK())
	require.Equal(t, http.StatusTemporaryRedirect, res.StatusCode)
	require.Zero(t, targetHits.Load(), "redirects are never followed")

	srv, _ := receiver(t, http.StatusInternalServerError, "boom")
	res = s.Send(context.Background(), Request{URL: srv.URL, Body: []byte(`{}`), At: time.Now()})
	require.False(t, res.OK())
	require.Equal(t, 500, res.StatusCode)
	require.Equal(t, "boom", res.Body)
}

func TestHTTPSenderTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	s := NewHTTPSender(domain.URLPolicy{AllowInsecure: true}, 150*time.Millisecond, "ua")

	res := s.Send(context.Background(), Request{URL: srv.URL, Body: []byte(`{}`), At: time.Now()})
	require.False(t, res.OK())
	require.Zero(t, res.StatusCode)
	require.Equal(t, "timeout", res.Err)
}

// The connect-time SSRF guard: a name that resolves to loopback is refused
// even though the URL itself passed validation.
func TestHTTPSenderBlocksPrivateDestinationsAtDialTime(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	s := NewHTTPSender(domain.URLPolicy{}, time.Second, "ua")

	port := srv.URL[strings.LastIndex(srv.URL, ":"):]
	for _, u := range []string{srv.URL, "http://localhost" + port} {
		res := s.Send(context.Background(), Request{URL: u, Body: []byte(`{}`), At: time.Now()})
		require.False(t, res.OK(), u)
		require.Contains(t, res.Err, "blocked destination", u)
	}
	require.Zero(t, hits.Load())
}

// End to end through the service: fan-out → job → real HTTP POST to an
// httptest receiver that verifies the signature with the endpoint secret.
func TestDeliveryEndToEndAgainstHTTPTestServer(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusServiceUnavailable)
	got := make(chan captured, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- captured{header: r.Header.Clone(), body: b}
		w.WriteHeader(int(status.Load()))
		_, _ = io.WriteString(w, `{"received":true}`)
	}))
	t.Cleanup(srv.Close)

	h := newHarness(t, allPerms())
	policy := domain.URLPolicy{AllowInsecure: true}
	h.svc.opts.Policy = policy
	h.svc.sender = NewHTTPSender(policy, 2*time.Second, "LevelUp-Webhooks/1.0")
	// Real wall clock: the receiver checks the signature timestamp.
	h.svc.clock = clock.System()

	e, err := h.svc.CreateEndpoint(ctxFor(tenantA), domain.NewEndpointInput{
		URL: srv.URL + "/levelup", EventTypes: []string{"badges.awarded"}, Active: true,
	})
	require.NoError(t, err)
	require.NoError(t, h.svc.HandleFact(context.Background(), fact(badgescontracts.TopicAwarded, "env-42", tenantA)))
	cmd := lastCmd(t, h.ob)

	// First attempt: receiver down → retryable.
	err = h.svc.HandleDeliver(context.Background(), cmd)
	require.Equal(t, errs.Unavailable, errs.KindOf(err))
	<-got

	// Second attempt succeeds.
	status.Store(http.StatusOK)
	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	c := <-got
	require.True(t, domain.VerifySignature(e.Secret, c.header.Get(domain.HeaderSignature), c.body, time.Minute, time.Now()))
	require.Equal(t, "badges.awarded", c.header.Get(domain.HeaderEvent))
	require.Equal(t, cmd.DeliveryID, c.header.Get(domain.HeaderDelivery))
	require.Contains(t, string(c.body), `"event_id":"env-42"`)

	d := h.delivery(cmd.DeliveryID)
	require.Equal(t, domain.StatusSucceeded, d.Status)
	require.Equal(t, 2, d.Attempts)
	require.Equal(t, 200, *d.ResponseStatus)
	require.Equal(t, `{"received":true}`, d.ResponseBody)
	require.NotNil(t, d.LatencyMS)

	// Synchronous test event against the same receiver.
	td, err := h.svc.SendTest(ctxFor(tenantA), e.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusSucceeded, td.Status)
	c = <-got
	require.Equal(t, domain.TestEvent, c.header.Get(domain.HeaderEvent))
	require.True(t, domain.VerifySignature(e.Secret, c.header.Get(domain.HeaderSignature), c.body, time.Minute, time.Now()))
}
