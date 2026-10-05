package streaks

import "time"

// Config is streaks' settings; the composition root embeds it with
// envPrefix "STREAKS_".
type Config struct {
	// BreakSweepSchedule is the cron spec (UTC) of streaks.break_sweep (R47).
	BreakSweepSchedule string `env:"BREAK_SWEEP_SCHEDULE" envDefault:"5 * * * *"`
	// PruneSchedule is the cron spec (UTC) of streaks.prune_requests.
	PruneSchedule string `env:"PRUNE_SCHEDULE" envDefault:"30 3 * * *"`
	// RequestRetention is how long record idempotency rows are kept.
	RequestRetention time.Duration `env:"REQUEST_RETENTION" envDefault:"720h"`
}
