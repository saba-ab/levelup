package progression

// Config is progression's settings; the composition root embeds it with
// envPrefix "PROGRESSION_".
type Config struct {
	// ReconcileSchedule is the cron spec (UTC, 5 fields) of progression.reconcile.
	ReconcileSchedule string `env:"RECONCILE_SCHEDULE" envDefault:"0 * * * *"`
	// ReconcileReplaceLevels lets the sweep re-place players whose stored
	// level no longer matches the ladder (after threshold edits). It never
	// issues rewards. Off by default: drift is only reported.
	ReconcileReplaceLevels bool `env:"RECONCILE_REPLACE_LEVELS" envDefault:"false"`
	// ReconcileBatchSize bounds how many progress rows one sweep step loads.
	ReconcileBatchSize int `env:"RECONCILE_BATCH_SIZE" envDefault:"500"`
}
