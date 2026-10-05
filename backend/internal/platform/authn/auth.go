package authn

import (
	"time"

	"go.uber.org/zap"

	"levelup/internal/platform/clock"
	"levelup/internal/platform/redis"
)

// Auth bundles the issuer and refresh store — what the composition root
// hands to whichever module owns credentials (user), and what the HTTP
// middleware verifies against.
type Auth struct {
	Issuer  *Issuer
	Refresh *RefreshStore
}

func New(secret string, accessTTL, refreshTTL time.Duration, core redis.Core, c clock.Clock, log *zap.Logger) *Auth {
	return &Auth{
		Issuer:  NewIssuer(secret, accessTTL, c),
		Refresh: NewRefreshStore(core, refreshTTL, log),
	}
}
