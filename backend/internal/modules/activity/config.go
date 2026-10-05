package activity

import "time"

// Config is activity's settings. The composition root embeds it with
// envPrefix "ACTIVITY_".
type Config struct {
	// RequireKnownEventType rejects (422 unknown_event_type) activities
	// whose event_type is not defined and active for the tenant.
	RequireKnownEventType bool `env:"REQUIRE_KNOWN_EVENT_TYPE" envDefault:"true"`
	// AutoCreatePlayers flags activity.received.v1 for unresolved players
	// (auto_create_player) so a player owner can create them. Off: unknown
	// players are rejected by rules.
	AutoCreatePlayers bool `env:"AUTO_CREATE_PLAYERS" envDefault:"false"`
	// InternalTriggers turns level/badge/mission facts into internal
	// activities (doc 06 §11.9). Off: those subscriptions are not registered.
	InternalTriggers bool `env:"INTERNAL_TRIGGERS" envDefault:"false"`

	// MaxAge rejects occurred_at older than now-MaxAge.
	MaxAge time.Duration `env:"MAX_AGE" envDefault:"720h"`
	// MaxFutureSkew rejects occurred_at later than now+MaxFutureSkew.
	MaxFutureSkew time.Duration `env:"MAX_FUTURE_SKEW" envDefault:"5m"`
	// MaxPayloadBytes caps serialized properties and context, each.
	MaxPayloadBytes int `env:"MAX_PAYLOAD_BYTES" envDefault:"32768"`

	// StuckSweepSchedule is the cron spec of activity.stuck_sweep.
	StuckSweepSchedule string `env:"STUCK_SWEEP_SCHEDULE" envDefault:"* * * * *"`
	// StuckAfter is how long an activity may stay pending before the sweep
	// re-publishes it (and the minimum gap between re-publishes).
	StuckAfter time.Duration `env:"STUCK_AFTER" envDefault:"5m"`
	// MaxRepublishes caps re-publishes per activity.
	MaxRepublishes int `env:"MAX_REPUBLISHES" envDefault:"5"`
	// StuckSweepBatch caps rows re-published per run.
	StuckSweepBatch int `env:"STUCK_SWEEP_BATCH" envDefault:"500"`
}

// withDefaults fills zero durations and sizes (a Config built in code
// rather than parsed from the environment). Booleans keep their value.
func (c Config) withDefaults() Config {
	if c.MaxAge <= 0 {
		c.MaxAge = 30 * 24 * time.Hour
	}
	if c.MaxFutureSkew <= 0 {
		c.MaxFutureSkew = 5 * time.Minute
	}
	if c.MaxPayloadBytes <= 0 {
		c.MaxPayloadBytes = 32 << 10
	}
	if c.StuckSweepSchedule == "" {
		c.StuckSweepSchedule = "* * * * *"
	}
	if c.StuckAfter <= 0 {
		c.StuckAfter = 5 * time.Minute
	}
	if c.MaxRepublishes <= 0 {
		c.MaxRepublishes = 5
	}
	if c.StuckSweepBatch <= 0 {
		c.StuckSweepBatch = 500
	}
	return c
}
