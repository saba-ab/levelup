package domain

import (
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// URLPolicy is the SSRF guard. A webhook URL is attacker-controlled input
// that the worker will POST to from inside the network, so it is checked
// twice:
//
//  1. At registration (Validate): the scheme must be https, the URL must not
//     carry credentials or a fragment, and the host must not be a literal
//     IP in a blocked range nor a loopback name ("localhost", "*.localhost").
//  2. At connect time (AllowDial, wired into the HTTP client's dialer
//     Control hook): every address the hostname resolves to is checked
//     again right before the socket connects. This is the check that
//     matters: it defeats public names that resolve to private addresses
//     and DNS rebinding between validation and delivery. The client also
//     ignores proxy environment variables and never follows redirects, so
//     no hop escapes the check.
//
// AllowInsecure (dev only) additionally permits plain http and loopback
// destinations (localhost, 127.0.0.0/8, ::1). It never permits private,
// link-local or other reserved ranges.
type URLPolicy struct {
	AllowInsecure bool
}

const maxURLLength = 2048

// Validate checks a webhook URL at registration and returns it normalised
// (trimmed).
func (p URLPolicy) Validate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrURLRequired
	}
	if len(raw) > maxURLLength {
		return "", ErrURLTooLong
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Hostname() == "" || u.Opaque != "" {
		return "", ErrURLMalformed
	}
	if u.User != nil {
		return "", ErrURLCredentials
	}
	if u.Fragment != "" || strings.Contains(raw, "#") {
		return "", ErrURLFragment
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	loopbackName := host == "localhost" || strings.HasSuffix(host, ".localhost")

	switch strings.ToLower(u.Scheme) {
	case "https":
	case "http":
		if !p.AllowInsecure || !p.isLoopbackHost(host, loopbackName) {
			return "", ErrInsecureURL
		}
	default:
		return "", ErrInsecureURL
	}

	if loopbackName {
		if !p.AllowInsecure {
			return "", ErrForbiddenHost
		}
		return raw, nil
	}
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if !p.allowAddr(addr) {
			return "", ErrForbiddenHost
		}
	} else if looksNumeric(host) {
		// Shorthand IPv4 forms ("127.1", "0x7f.1", "2130706433") are parsed
		// by some resolvers as addresses; refuse rather than guess.
		return "", ErrForbiddenHost
	}
	return raw, nil
}

func (p URLPolicy) isLoopbackHost(host string, loopbackName bool) bool {
	if loopbackName {
		return true
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	return err == nil && addr.Unmap().IsLoopback()
}

// AllowDial reports whether the client may connect to address ("ip:port",
// as handed to a net.Dialer Control hook).
func (p URLPolicy) AllowDial(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return p.allowAddr(addr)
}

func (p URLPolicy) allowAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if p.AllowInsecure && addr.IsLoopback() {
		return true
	}
	return !IsBlockedAddr(addr)
}

// blockedPrefixes are special-purpose ranges netip's predicates miss.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this" network
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved + broadcast
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64 (embeds IPv4)
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local-use NAT64
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("2002::/16"),       // 6to4 (embeds IPv4)
	netip.MustParsePrefix("100::/64"),        // discard-only
}

// IsBlockedAddr reports whether a destination address is private, loopback,
// link-local, multicast, unspecified or otherwise reserved.
func IsBlockedAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// looksNumeric is true for hosts made only of digits, dots and hex markers.
func looksNumeric(host string) bool {
	if host == "" {
		return false
	}
	labels := strings.Split(host, ".")
	last := labels[len(labels)-1]
	if last == "" {
		return false
	}
	for _, c := range strings.ToLower(last) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && c != 'x' {
			return false
		}
	}
	// A purely alphabetic hex-looking TLD ("cafe", "dead") is a real name.
	return strings.ContainsAny(last, "0123456789")
}
