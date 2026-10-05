package redis

import "time"

// WithTTL derives a handle with a module-configured TTL (PRD §7.3: the
// module owns its settings). Same client, same prefix — only the default
// entry lifetime changes. A fresh singleflight group is fine: flights are
// keyed by full key, and two handles for one module do not exist in one
// process wiring.
func (c *ModuleCache) WithTTL(ttl time.Duration) *ModuleCache {
	if ttl <= 0 {
		return c
	}
	return &ModuleCache{rdb: c.rdb, prefix: c.prefix, ttl: ttl, log: c.log}
}
