package authn

import (
	"context"
	"net/http"
	"strings"

	"levelup/internal/platform/authz"
)

// APIKeyPrefix starts every API key, so the middleware can tell a key from
// a JWT without trying both, and so leaked keys are easy to grep for.
const APIKeyPrefix = "lvl_"

// KeyVerifier authenticates API keys (ADR-0017). Platform defines the
// contract; the module that owns keys (identity) implements it, and the
// composition root finds it by type assertion so platform never imports a
// module. Any failure means "not authenticated", never a server error the
// caller could probe.
type KeyVerifier interface {
	VerifyAPIKey(ctx context.Context, raw string) (authz.Principal, error)
}

// apiKeyFrom returns a presented API key: "Authorization: Bearer lvl_..."
// or "X-API-Key: lvl_...".
func apiKeyFrom(r *http.Request) (string, bool) {
	if raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && strings.HasPrefix(raw, APIKeyPrefix) {
		return raw, true
	}
	if raw := r.Header.Get("X-API-Key"); strings.HasPrefix(raw, APIKeyPrefix) {
		return raw, true
	}
	return "", false
}
