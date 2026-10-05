package badges

// Config holds BADGES_* settings; the composition root embeds it with
// envPrefix "BADGES_".
type Config struct {
	// ReconcileSchedule is the cron spec (UTC, 5 fields) of badges.reconcile:
	// earned_count == applied awards and <= max_awards, log + metric only.
	ReconcileSchedule string `env:"RECONCILE_SCHEDULE" envDefault:"0 * * * *"`
}
