package redis

import (
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// ModuleCache is a prefix-bound cache handle (PRD §7.8.1): the same
// containment mechanism as GORM's TablePrefix. A module receives one from
// DepsFor and physically cannot read or evict another module's keys through
// it (R42). Read-path behaviour (singleflight, TTL jitter, negative caching)
// lives in cache.go.
type ModuleCache struct {
	rdb    redis.UniversalClient
	prefix string // "cache:<module>:"
	sf     singleflight.Group
	ttl    time.Duration
	log    *zap.Logger
}

// NewModuleCache binds a handle to one module's key space on redis-cache.
func NewModuleCache(rdb redis.UniversalClient, module string, ttl time.Duration, log *zap.Logger) *ModuleCache {
	return &ModuleCache{
		rdb:    rdb,
		prefix: "cache:" + module + ":",
		ttl:    ttl,
		log:    log,
	}
}

func (c *ModuleCache) key(parts ...string) string {
	return c.prefix + strings.Join(parts, ":")
}
