// Package redis owns both Redis instances (PRD §7.8): redis-core for
// correctness state (sessions, locks, rate limits, dedupe — noeviction,
// persisted) and redis-cache for losable cache (allkeys-lru). Distinct Go
// types keep the two from ever being swapped in wiring.
package redis

import (
	"runtime"
	"time"

	"github.com/redis/go-redis/v9"
)

// Core is the client for redis-core. Anything stored through it must carry
// an explicit TTL: the instance runs noeviction, so a leaked key is a memory
// leak that ends in OOM on every write (PRD §7.8.4).
type Core struct{ redis.UniversalClient }

// Cache is the client for redis-cache. Everything reachable through it must
// be safe to lose at any moment.
type Cache struct{ redis.UniversalClient }

func NewCore(addr string) Core {
	return Core{newClient(addr)}
}

func NewCache(addr string) Cache {
	return Cache{newClient(addr)}
}

func newClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:         addr,
		PoolSize:     10 * runtime.GOMAXPROCS(0),
		MinIdleConns: 10,
		// Short timeouts are the point (PRD §7.8.4): a slow Redis behind a
		// 3s default is worse than no Redis — every request waits,
		// goroutines pile up, the process OOMs. 200ms and fall through.
		ReadTimeout:     200 * time.Millisecond,
		WriteTimeout:    200 * time.Millisecond,
		DialTimeout:     500 * time.Millisecond,
		MaxRetries:      2,
		ConnMaxIdleTime: 5 * time.Minute,
	})
}
