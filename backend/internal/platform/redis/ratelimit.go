package redis

import (
	redis_rate "github.com/go-redis/redis_rate/v10"
)

// NewLimiter builds the GCRA limiter (PRD §7.8.3) on redis-core. GCRA over a
// fixed window because a fixed window lets a client spend its full budget in
// the last 100ms of one window and again in the first 100ms of the next —
// a "120/min" limit that is really 240 in a 200ms burst.
func NewLimiter(core Core) *redis_rate.Limiter {
	return redis_rate.NewLimiter(core.UniversalClient)
}
