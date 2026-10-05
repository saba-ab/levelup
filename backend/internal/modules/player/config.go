package player

import "time"

// Config is embedded into config.Config with envPrefix "PLAYER_".
type Config struct {
	// CacheTTL bounds how long a Reader snapshot may be served from
	// redis-cache. Writes evict after commit; 0 disables the cache.
	CacheTTL time.Duration `env:"CACHE_TTL" envDefault:"5m"`
	// PurgeBatchSize is how many rows one tenant-purge transaction deletes.
	PurgeBatchSize int `env:"PURGE_BATCH_SIZE" envDefault:"500"`
}
