package httpx

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// CORS lets browser clients on the listed origins call the API (the portal
// runs on its own origin). It runs first in the chain so a preflight never
// reaches auth, rate limiting or idempotency. Credentials are not allowed:
// the API authenticates with a bearer token, never cookies, so there is no
// ambient authority for another origin to ride on.
//
// "*" in allowed allows any origin (development only; the config default
// lists explicit origins).
func CORS(allowed []string) func(http.Handler) http.Handler {
	allowAny := slices.Contains(allowed, "*")
	const (
		allowMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
		allowHeaders = "Authorization, Content-Type, Accept, Idempotency-Key, X-Request-Id"
		exposeHeader = "X-Request-Id, Idempotent-Replay, Retry-After, Link"
		maxAge       = 600
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			h := w.Header()
			h.Add("Vary", "Origin")
			if !allowAny && !slices.Contains(allowed, strings.TrimRight(origin, "/")) {
				// Not ours: answer preflights without CORS headers (the
				// browser blocks the call) and serve simple requests as-is.
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Expose-Headers", exposeHeader)
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				h.Set("Access-Control-Allow-Methods", allowMethods)
				h.Set("Access-Control-Allow-Headers", allowHeaders)
				h.Set("Access-Control-Max-Age", strconv.Itoa(maxAge))
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
