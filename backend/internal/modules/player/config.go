package player

import "time"

// Config is embedded into config.Config with envPrefix "PLAYER_".
type Config struct {
	// CacheTTL bounds how long a Reader snapshot may be served from
	// redis-cache. Writes evict after commit; 0 disables the cache.
	CacheTTL time.Duration `env:"CACHE_TTL" envDefault:"5m"`
	// PurgeBatchSize is how many rows one tenant-purge transaction deletes.
	PurgeBatchSize int `env:"PURGE_BATCH_SIZE" envDefault:"500"`
	// AutoCreateFromActivities subscribes to activity.received.v1 and
	// creates players for activities flagged auto_create_player (activity's
	// ACTIVITY_AUTO_CREATE_PLAYERS sets the flag). Off: no subscription, and
	// flagged activities wait for an operator-created player.
	AutoCreateFromActivities bool `env:"AUTO_CREATE_FROM_ACTIVITIES" envDefault:"true"`
}
