package authn

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	"levelup/internal/platform/authz"
)

// Middleware parses a bearer token, or an API key when keys is non-nil,
// into the request context. It NEVER rejects: public routes must work
// without a token, so rejection is a route-group decision
// (httpx.RequireAuth). Each failure mode logs its own distinct reason (R18).
func Middleware(iss *Issuer, store *RefreshStore, log *zap.Logger, keys KeyVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key, ok := apiKeyFrom(r); ok {
				if keys == nil {
					next.ServeHTTP(w, r)
					return
				}
				p, err := keys.VerifyAPIKey(r.Context(), key)
				if err != nil {
					log.Warn("api key rejected", zap.Error(err), zap.String("path", r.URL.Path))
					next.ServeHTTP(w, r)
					return
				}
				next.ServeHTTP(w, r.WithContext(authz.Into(r.Context(), p)))
				return
			}
			raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || raw == "" {
				next.ServeHTTP(w, r)
				return
			}
			p, jti, err := iss.Verify(raw)
			if err != nil {
				// The distinct sentinel IS the log reason (R18).
				log.Warn("bearer token rejected", zap.Error(err), zap.String("path", r.URL.Path))
				next.ServeHTTP(w, r)
				return
			}
			if store != nil && store.IsDenied(r.Context(), jti) {
				log.Warn("bearer token rejected", zap.Error(ErrDenied), zap.String("path", r.URL.Path))
				next.ServeHTTP(w, r)
				return
			}
			ctx := authz.Into(r.Context(), p)
			ctx = withJTI(ctx, jti)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
