package webhooks

import "time"

// Config is webhooks' settings; the composition root embeds it with
// envPrefix "WEBHOOKS_".
type Config struct {
	// AllowInsecure (dev only) permits http:// and loopback destinations
	// (localhost, 127.0.0.0/8, ::1). Private and link-local ranges stay
	// blocked regardless.
	AllowInsecure bool `env:"ALLOW_INSECURE" envDefault:"false"`
	// UserAgent is sent on every delivery.
	UserAgent string `env:"USER_AGENT" envDefault:"LevelUp-Webhooks/1.0"`
	// Timeout bounds one delivery POST (connect + response).
	Timeout time.Duration `env:"TIMEOUT" envDefault:"10s"`
	// RetrySweepSchedule is the cron spec (UTC) of webhooks.retry_sweep (R47).
	RetrySweepSchedule string `env:"RETRY_SWEEP_SCHEDULE" envDefault:"*/5 * * * *"`
	// StaleAfter is how long a pending delivery sits untouched before the
	// sweep re-enqueues it.
	StaleAfter time.Duration `env:"STALE_AFTER" envDefault:"10m"`
	// MaxAttempts caps the attempts of one delivery cycle (fan-out or manual
	// redeliver), across sweeps.
	MaxAttempts int `env:"MAX_ATTEMPTS" envDefault:"6"`
	// LadderAttempts is how many times one queued job runs before the
	// broker's retry ladder ends: the first try plus the retry tiers
	// (5s, 30s, 2m). After it the delivery is marked failed instead of
	// being dead-lettered. Keep it equal to 1 + len(rabbit.RetryTiers).
	LadderAttempts int `env:"LADDER_ATTEMPTS" envDefault:"4"`
	// DisableAfterFailures disables an endpoint after this many consecutive
	// failed deliveries.
	DisableAfterFailures int `env:"DISABLE_AFTER_FAILURES" envDefault:"50"`
}

// withDefaults fills zero values (a Config{} literal in tests or wiring).
func (c Config) withDefaults() Config {
	if c.UserAgent == "" {
		c.UserAgent = "LevelUp-Webhooks/1.0"
	}
	if c.Timeout <= 0 {
		c.Timeout = 10 * time.Second
	}
	if c.RetrySweepSchedule == "" {
		c.RetrySweepSchedule = "*/5 * * * *"
	}
	if c.StaleAfter <= 0 {
		c.StaleAfter = 10 * time.Minute
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 6
	}
	if c.LadderAttempts <= 0 {
		c.LadderAttempts = 4
	}
	if c.DisableAfterFailures <= 0 {
		c.DisableAfterFailures = 50
	}
	return c
}
