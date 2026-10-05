package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/platform/httpx"
)

func seenRemote(t *testing.T, trusted []string, remote, xff string) string {
	t.Helper()
	prefixes, err := httpx.ParseTrustedProxies(trusted)
	require.NoError(t, err)
	var got string
	h := httpx.ClientIP(prefixes)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.RemoteAddr }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestClientIPFromTrustedProxy(t *testing.T) {
	trusted := []string{"172.29.0.0/24", "127.0.0.1"}
	require.Equal(t, "203.0.113.7:4000", seenRemote(t, trusted, "172.29.0.1:4000", "203.0.113.7"))
	// A client-supplied hop to the left of the real one is ignored.
	require.Equal(t, "203.0.113.7:4000", seenRemote(t, trusted, "172.29.0.1:4000", "1.2.3.4, 203.0.113.7"))
	// Chained trusted proxies are skipped.
	require.Equal(t, "203.0.113.7:4000", seenRemote(t, trusted, "127.0.0.1:4000", "203.0.113.7, 172.29.0.5"))
}

func TestClientIPIgnoresUntrustedPeers(t *testing.T) {
	trusted := []string{"172.29.0.0/24"}
	require.Equal(t, "198.51.100.9:4000", seenRemote(t, trusted, "198.51.100.9:4000", "1.2.3.4"), "spoofed header from the internet")
	require.Equal(t, "172.29.0.1:4000", seenRemote(t, trusted, "172.29.0.1:4000", ""), "no header: keep the peer")
	require.Equal(t, "172.29.0.1:4000", seenRemote(t, nil, "172.29.0.1:4000", "203.0.113.7"), "no trusted proxies configured")
}

func TestParseTrustedProxies(t *testing.T) {
	p, err := httpx.ParseTrustedProxies([]string{"10.0.0.1/8", " 127.0.0.1 ", "", "::1"})
	require.NoError(t, err)
	require.Equal(t, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("::1/128")}, p)
	_, err = httpx.ParseTrustedProxies([]string{"nope"})
	require.Error(t, err)
}
