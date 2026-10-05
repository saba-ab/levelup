package program

// Config is program's settings; the composition root embeds it with
// envPrefix "PROGRAM_".
type Config struct {
	// AutoEndSchedule is the cron spec (UTC) of the reconciling sweep that
	// ends running programs whose ends_at has passed. Empty disables it.
	AutoEndSchedule string `env:"AUTO_END_SCHEDULE" envDefault:"*/5 * * * *"`
	// AutoEndBatchSize caps how many programs one sweep ends; the next tick
	// picks up the rest.
	AutoEndBatchSize int `env:"AUTO_END_BATCH_SIZE" envDefault:"500"`
}
