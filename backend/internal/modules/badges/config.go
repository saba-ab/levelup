package badges

import "time"

// Config holds BADGES_* settings; the composition root embeds it with
// envPrefix "BADGES_".
type Config struct {
	// ReconcileSchedule is the cron spec (UTC, 5 fields) of badges.reconcile:
	// earned_count == applied awards and <= max_awards, log + metric only.
	ReconcileSchedule string `env:"RECONCILE_SCHEDULE" envDefault:"0 * * * *"`
	// PruneSchedule is the cron spec of badges.prune_applied_events.
	PruneSchedule string `env:"PRUNE_SCHEDULE" envDefault:"30 3 * * *"`
	// AppliedEventsRetention is how long the requirements projection keeps
	// a dedupe key (a redelivery older than this would count twice). Must
	// exceed the outbox retention (7 days) by a safe margin.
	AppliedEventsRetention time.Duration `env:"APPLIED_EVENTS_RETENTION" envDefault:"720h"`
}
