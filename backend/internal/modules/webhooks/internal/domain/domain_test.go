package domain

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/shared/errs"
)

var t0 = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func TestSignatureMatchesSpecAndVerifies(t *testing.T) {
	body := []byte(`{"event":"badges.awarded"}`)
	secret := "whsec_test"
	h := SignatureHeader(secret, t0, body)

	// t=<unix>,v1=hex(hmac_sha256(secret, "<unix>.<body>"))
	require.Equal(t, "t=1791288000,v1="+Sign(secret, 1791288000, body), h)
	require.Len(t, Sign(secret, 1, body), 64)
	require.NotEqual(t, Sign(secret, 1, body), Sign(secret, 2, body), "timestamp is signed")

	require.True(t, VerifySignature(secret, h, body, 5*time.Minute, t0.Add(time.Minute)))
	require.False(t, VerifySignature("whsec_other", h, body, 5*time.Minute, t0), "wrong secret")
	require.False(t, VerifySignature(secret, h, []byte(`{"event":"x"}`), 5*time.Minute, t0), "tampered body")
	require.False(t, VerifySignature(secret, h, body, 5*time.Minute, t0.Add(10*time.Minute)), "replayed too late")
	require.False(t, VerifySignature(secret, "garbage", body, 5*time.Minute, t0))
	require.False(t, VerifySignature(secret, "t=abc,v1=00", body, 5*time.Minute, t0))
}

func TestURLPolicyValidate(t *testing.T) {
	strict := URLPolicy{}
	dev := URLPolicy{AllowInsecure: true}
	for _, tc := range []struct {
		name   string
		policy URLPolicy
		url    string
		want   error
	}{
		{"https public", strict, "https://hooks.example.com/levelup?x=1", nil},
		{"https public ip", strict, "https://93.184.216.34/hook", nil},
		{"trimmed", strict, "  https://example.com/h  ", nil},
		{"empty", strict, "", ErrURLRequired},
		{"relative", strict, "/hook", ErrURLMalformed},
		{"no host", strict, "https:///hook", ErrURLMalformed},
		{"credentials", strict, "https://user:pw@example.com/", ErrURLCredentials},
		{"fragment", strict, "https://example.com/#x", ErrURLFragment},
		{"http public", strict, "http://example.com/", ErrInsecureURL},
		{"http public dev", dev, "http://example.com/", ErrInsecureURL},
		{"ftp", dev, "ftp://localhost/", ErrInsecureURL},
		{"localhost strict", strict, "https://localhost/", ErrForbiddenHost},
		{"sub.localhost strict", strict, "https://api.localhost:8443/", ErrForbiddenHost},
		{"http localhost strict", strict, "http://localhost:3000/", ErrInsecureURL},
		{"http localhost dev", dev, "http://localhost:3000/hook", nil},
		{"http 127 dev", dev, "http://127.0.0.1:9000/hook", nil},
		{"http ::1 dev", dev, "http://[::1]:9000/hook", nil},
		{"loopback ip strict", strict, "https://127.0.0.1/", ErrForbiddenHost},
		{"private 10", strict, "https://10.1.2.3/", ErrForbiddenHost},
		{"private 10 dev", dev, "https://10.1.2.3/", ErrForbiddenHost},
		{"private 172", strict, "https://172.20.0.1/", ErrForbiddenHost},
		{"private 192.168", dev, "https://192.168.1.1/", ErrForbiddenHost},
		{"link-local metadata", dev, "https://169.254.169.254/latest/meta-data", ErrForbiddenHost},
		{"cgnat", strict, "https://100.64.0.1/", ErrForbiddenHost},
		{"unspecified", strict, "https://0.0.0.0/", ErrForbiddenHost},
		{"ipv6 ula", strict, "https://[fd00::1]/", ErrForbiddenHost},
		{"ipv6 link-local", strict, "https://[fe80::1]/", ErrForbiddenHost},
		{"ipv4-mapped loopback", strict, "https://[::ffff:127.0.0.1]/", ErrForbiddenHost},
		{"ipv4-mapped private", strict, "https://[::ffff:10.0.0.1]/", ErrForbiddenHost},
		{"decimal ip", strict, "https://2130706433/", ErrForbiddenHost},
		{"short ip", strict, "https://127.1/", ErrForbiddenHost},
		{"hex ip", strict, "https://0x7f.0.0.1/", ErrForbiddenHost},
		{"too long", strict, "https://example.com/" + strings.Repeat("a", 2100), ErrURLTooLong},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.policy.Validate(tc.url)
			if tc.want == nil {
				require.NoError(t, err)
				require.Equal(t, strings.TrimSpace(tc.url), got)
				return
			}
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
		})
	}
}

func TestIsBlockedAddrAndAllowDial(t *testing.T) {
	for _, ip := range []string{
		"127.0.0.1", "10.0.0.1", "172.16.5.4", "192.168.0.1", "169.254.169.254", "100.100.100.200",
		"0.0.0.0", "255.255.255.255", "224.0.0.1", "198.18.0.1", "::1", "::", "fe80::1", "fc00::1",
		"ff02::1", "::ffff:192.168.0.1", "64:ff9b::a00:1", "2002:a00:1::",
	} {
		require.True(t, IsBlockedAddr(netip.MustParseAddr(ip)), ip)
	}
	for _, ip := range []string{"8.8.8.8", "93.184.216.34", "2606:4700:4700::1111", "172.32.0.1"} {
		require.False(t, IsBlockedAddr(netip.MustParseAddr(ip)), ip)
	}

	strict := URLPolicy{}
	dev := URLPolicy{AllowInsecure: true}
	require.True(t, strict.AllowDial("8.8.8.8:443"))
	require.False(t, strict.AllowDial("127.0.0.1:443"))
	require.False(t, strict.AllowDial("[::1]:443"))
	require.False(t, strict.AllowDial("10.0.0.5:443"))
	require.False(t, strict.AllowDial("not-an-ip:443"))
	require.True(t, dev.AllowDial("127.0.0.1:8080"), "dev allows loopback")
	require.True(t, dev.AllowDial("[::1]:8080"))
	require.False(t, dev.AllowDial("10.0.0.5:443"), "dev never allows private ranges")
	require.False(t, dev.AllowDial("169.254.169.254:80"))
}

func TestCatalogueAndEventTypes(t *testing.T) {
	require.Len(t, Catalogue, 14)
	ev, ok := EventForTopic("badges.awarded.v1")
	require.True(t, ok)
	require.Equal(t, "badges.awarded", ev)
	ev, ok = EventForTopic("progression.level_reached.v1")
	require.True(t, ok)
	require.Equal(t, "progression.level_reached", ev)
	_, ok = EventForTopic("points.credit_rejected.v1")
	require.False(t, ok)
	for _, e := range Catalogue {
		require.False(t, strings.HasSuffix(e.Event, ".v1"), e.Event)
		require.NotEmpty(t, e.Description)
	}

	got, err := NormalizeEventTypes([]string{"points.debited", "badges.awarded", "points.debited"})
	require.NoError(t, err)
	require.Equal(t, []string{"badges.awarded", "points.debited"}, got, "deduplicated and sorted")

	got, err = NormalizeEventTypes([]string{"*"})
	require.NoError(t, err)
	require.Equal(t, []string{"*"}, got)

	_, err = NormalizeEventTypes(nil)
	require.ErrorIs(t, err, ErrEventTypesRequired)
	_, err = NormalizeEventTypes([]string{"*", "badges.awarded"})
	require.ErrorIs(t, err, ErrWildcardNotAlone)
	_, err = NormalizeEventTypes([]string{"badges.awarded.v1"})
	require.Equal(t, CodeUnknownEventType, errs.CodeOf(err))
	_, err = NormalizeEventTypes([]string{TestEvent})
	require.Equal(t, CodeUnknownEventType, errs.CodeOf(err), "webhook.test is not subscribable")

	require.True(t, Subscribes([]string{"*"}, "streaks.broken"))
	require.True(t, Subscribes([]string{"streaks.broken"}, "streaks.broken"))
	require.False(t, Subscribes([]string{"streaks.milestone_reached"}, "streaks.broken"))
}

func TestEndpointLifecycle(t *testing.T) {
	_, err := NewEndpoint("", NewEndpointInput{URL: "https://example.com", EventTypes: []string{"*"}}, URLPolicy{}, t0)
	require.ErrorIs(t, err, ErrNoTenant)
	_, err = NewEndpoint("t1", NewEndpointInput{URL: "https://10.0.0.1", EventTypes: []string{"*"}}, URLPolicy{}, t0)
	require.ErrorIs(t, err, ErrForbiddenHost)

	e, err := NewEndpoint("t1", NewEndpointInput{URL: "https://example.com/h", EventTypes: []string{"badges.awarded"}, Active: true}, URLPolicy{}, t0)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(e.Secret, SecretPrefix))
	require.Len(t, e.Secret, len(SecretPrefix)+48)
	old := e.Secret
	require.NoError(t, e.RotateSecret(t0))
	require.NotEqual(t, old, e.Secret)

	bad := "http://example.com"
	require.ErrorIs(t, e.Apply(EndpointPatch{URL: &bad}, URLPolicy{}, t0), ErrInsecureURL)
	require.Equal(t, "https://example.com/h", e.URL, "a failed patch changes nothing")

	// Disabled endpoint re-activated: failure streak cleared.
	e.Active = false
	e.ConsecutiveFailures = 50
	e.DisabledReason = "consecutive_failures"
	e.DisabledAt = &t0
	on := true
	types := []string{"*"}
	require.NoError(t, e.Apply(EndpointPatch{Active: &on, EventTypes: &types}, URLPolicy{}, t0.Add(time.Hour)))
	require.True(t, e.Active)
	require.Zero(t, e.ConsecutiveFailures)
	require.Empty(t, e.DisabledReason)
	require.Nil(t, e.DisabledAt)
	require.Equal(t, []string{"*"}, e.EventTypes)
}

func TestRetryPolicyFinal(t *testing.T) {
	p := RetryPolicy{LadderAttempts: 4, MaxAttempts: 6}
	for _, tc := range []struct {
		cycle, base int
		final       bool
	}{
		{1, 0, false}, {3, 0, false}, {4, 0, true}, // ladder of the fan-out job
		{5, 4, false}, {6, 4, true}, // a sweep re-enqueue hits the budget
		{3, 3, false}, {6, 5, true},
	} {
		require.Equal(t, tc.final, p.Final(tc.cycle, tc.base), "cycle=%d base=%d", tc.cycle, tc.base)
	}
}

func TestDeliveryStateMachine(t *testing.T) {
	d := NewDelivery("t1", "e1", "ev1", "badges.awarded", []byte(`{}`), t0)
	require.Equal(t, StatusPending, d.Status)

	require.NoError(t, d.Begin(t0, 15*time.Second))
	require.Equal(t, 1, d.Attempts)
	require.Equal(t, 1, d.CycleAttempts)
	require.ErrorIs(t, d.Begin(t0.Add(time.Second), 15*time.Second), ErrDeliveryInProgress, "leased")

	d.Settle(AttemptResult{StatusCode: 500, LatencyMS: 12, Body: "boom"}, false, t0.Add(time.Second))
	require.Equal(t, StatusPending, d.Status)
	require.Equal(t, 500, *d.ResponseStatus)
	require.Equal(t, "receiver responded 500", d.LastError)
	require.Nil(t, d.LeaseUntil)

	require.NoError(t, d.Begin(t0.Add(2*time.Second), 15*time.Second))
	d.Settle(AttemptResult{Err: "timeout", LatencyMS: 10000}, true, t0.Add(3*time.Second))
	require.Equal(t, StatusFailed, d.Status)
	require.Nil(t, d.ResponseStatus)
	require.Equal(t, "timeout", d.LastError)
	require.Error(t, d.Begin(t0.Add(4*time.Second), time.Second), "failed is final")

	require.NoError(t, d.Requeue(t0.Add(5*time.Second)))
	require.Equal(t, StatusPending, d.Status)
	require.Zero(t, d.CycleAttempts)
	require.Equal(t, 2, d.Attempts, "total attempts survive a redeliver")

	require.NoError(t, d.Begin(t0.Add(6*time.Second), time.Second))
	d.Settle(AttemptResult{StatusCode: 204, LatencyMS: 5}, false, t0.Add(7*time.Second))
	require.Equal(t, StatusSucceeded, d.Status)
	require.Empty(t, d.LastError)
	require.NotNil(t, d.DeliveredAt)
}

func TestAttemptResultOK(t *testing.T) {
	require.True(t, AttemptResult{StatusCode: 200}.OK())
	require.True(t, AttemptResult{StatusCode: 299}.OK())
	require.False(t, AttemptResult{StatusCode: 302}.OK(), "redirects are failures")
	require.False(t, AttemptResult{StatusCode: 404}.OK())
	require.False(t, AttemptResult{StatusCode: 200, Err: "x"}.OK())
	require.False(t, AttemptResult{}.OK())
}

func TestSnippet(t *testing.T) {
	require.Len(t, Snippet(strings.Repeat("a", 5000)), ResponseSnippetLimit)
	// A multi-byte rune straddling the cut is dropped, not split.
	s := Snippet(strings.Repeat("a", ResponseSnippetLimit-1) + "é")
	require.Len(t, s, ResponseSnippetLimit-1)
	require.Equal(t, "ab", Snippet("a\x00b"), "NUL is not storable in Postgres text")
}

func TestBuildPayload(t *testing.T) {
	body, err := BuildPayload("badges.awarded", "ev1", "t1", t0, json.RawMessage(`{"badge_id":"b1","tenant_id":"t1"}`))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "badges.awarded", got["event"])
	require.Equal(t, "ev1", got["event_id"])
	require.Equal(t, "t1", got["tenant_id"])
	require.Equal(t, "2026-10-06T12:00:00Z", got["occurred_at"])
	require.Equal(t, map[string]any{"badge_id": "b1", "tenant_id": "t1"}, got["data"])

	_, err = BuildPayload("x", "e", "t", t0, json.RawMessage(`{nope`))
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}
