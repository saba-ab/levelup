package leaderboards

import "time"

// Config is embedded by the composition root with envPrefix "LEADERBOARDS_".
type Config struct {
	// RolloverSchedule closes ended periods (snapshot + period_closed.v1).
	RolloverSchedule string `env:"ROLLOVER_SCHEDULE" envDefault:"5 * * * *"`
	// RebuildSchedule reloads every active board's redis read model.
	RebuildSchedule string `env:"REBUILD_SCHEDULE" envDefault:"30 3 * * *"`
	// PruneSchedule drops old applied_events idempotency rows.
	PruneSchedule string `env:"PRUNE_SCHEDULE" envDefault:"45 3 * * *"`
	// AppliedEventsRetention is how long applied_events rows are kept.
	AppliedEventsRetention time.Duration `env:"APPLIED_EVENTS_RETENTION" envDefault:"720h"`
	// CloseGrace delays closing a period so late facts still land in it.
	CloseGrace time.Duration `env:"CLOSE_GRACE" envDefault:"10m"`
	// ClosedPeriodTTL keeps a closed period's redis set this long after it ends.
	ClosedPeriodTTL time.Duration `env:"CLOSED_PERIOD_TTL" envDefault:"48h"`
	// AllTimeTTL is the rolling redis TTL of never-resetting boards; the daily
	// rebuild refreshes it.
	AllTimeTTL time.Duration `env:"ALL_TIME_TTL" envDefault:"72h"`
	// SnapshotLimit caps the ranks stored per closed period.
	SnapshotLimit int `env:"SNAPSHOT_LIMIT" envDefault:"1000"`
}
