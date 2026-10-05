package httpx

import (
	"net/http"

	"myapp/internal/platform/authz"
	"myapp/internal/shared/errs"
)

// RequireAuth guards a route group: 401 problem+json when no authenticated
// principal is in context. Token PARSING happens in authn's middleware;
// this only enforces presence, so modules mount public and protected
// subtrees themselves.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authz.From(r.Context()); !ok {
			Error(w, r, errs.New(errs.Unauthenticated, "authentication required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
