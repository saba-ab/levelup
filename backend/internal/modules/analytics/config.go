package analytics

import "time"

// Config is analytics' settings; the composition root embeds it with
// envPrefix "ANALYTICS_".
type Config struct {
	// PruneSchedule is the cron spec (UTC) of analytics.prune.
	PruneSchedule string `env:"PRUNE_SCHEDULE" envDefault:"20 4 * * *"`
	// Retention is how long daily counters and activity days are kept.
	Retention time.Duration `env:"RETENTION" envDefault:"17520h"`
	// AppliedEventsRetention is how long applied_events idempotency rows are
	// kept (must exceed the outbox replay window).
	AppliedEventsRetention time.Duration `env:"APPLIED_EVENTS_RETENTION" envDefault:"720h"`
}
