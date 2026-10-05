package httpx

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP rewrites r.RemoteAddr to the real client address when the
// immediate peer is a trusted reverse proxy (Caddy in production). Without
// it every request behind the proxy shares the proxy's address, so the
// per-IP rate limit for anonymous calls (login, register) becomes one
// global bucket. X-Forwarded-For is read right to left and the first
// address that is not itself a trusted proxy wins, so a client cannot
// spoof its IP by sending its own header. Requests from untrusted peers
// are left untouched.
func ClientIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	isTrusted := func(a netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(a.Unmap()) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		if len(trusted) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host, port, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			peer, err := netip.ParseAddr(host)
			if err != nil || !isTrusted(peer) {
				next.ServeHTTP(w, r)
				return
			}
			hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
			for i := len(hops) - 1; i >= 0; i-- {
				a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
				if err != nil {
					break
				}
				if !isTrusted(a) {
					r.RemoteAddr = net.JoinHostPort(a.Unmap().String(), port)
					break
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ParseTrustedProxies turns "10.0.0.0/8,127.0.0.1" into prefixes; a bare
// address is a /32 (or /128).
func ParseTrustedProxies(entries []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(entries))
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.Contains(e, "/") {
			a, err := netip.ParseAddr(e)
			if err != nil {
				return nil, err
			}
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(e)
		if err != nil {
			return nil, err
		}
		out = append(out, p.Masked())
	}
	return out, nil
}
