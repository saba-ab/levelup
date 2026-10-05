package httpx

import (
	"net"
	"net/http"
	"strconv"

	redis_rate "github.com/go-redis/redis_rate/v10"
	"go.uber.org/zap"

	"myapp/internal/platform/authz"
)

// RateLimit is GCRA on redis-core, keyed by principal when authenticated and
// client IP otherwise (R38). It FAILS OPEN (PRD §7.8.3): a dead Redis
// returning 429 to every caller converts a cache outage into a full outage.
// Locks fail the other way — that split is deliberate and tested (R43).
func RateLimit(limiter *redis_rate.Limiter, perMinute int, log *zap.Logger) func(http.Handler) http.Handler {
	limit := redis_rate.PerMinute(perMinute)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res, err := limiter.Allow(r.Context(), "rl:api:"+principalOrIP(r), limit)
			if err != nil {
				log.Warn("rate limiter unavailable — failing open", zap.Error(err))
				next.ServeHTTP(w, r) // FAIL OPEN
				return
			}
			if res.Allowed == 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(res.RetryAfter.Seconds())+1))
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"type":"about:blank","title":"rate_limited","status":429,"detail":"rate limit exceeded"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func principalOrIP(r *http.Request) string {
	if p, ok := authz.From(r.Context()); ok {
		return "user:" + p.UserID
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "ip:" + r.RemoteAddr
	}
	return "ip:" + host
}
