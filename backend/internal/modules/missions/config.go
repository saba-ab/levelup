package missions

// Config is missions' settings; the composition root embeds it with
// envPrefix "MISSIONS_".
type Config struct {
	// ExpireSweepSchedule is the cron spec (UTC) of missions.expire_sweep.
	ExpireSweepSchedule string `env:"EXPIRE_SWEEP_SCHEDULE" envDefault:"*/5 * * * *"`
	// SweepBatchSize bounds the rows one sweep transaction expires.
	SweepBatchSize int `env:"SWEEP_BATCH_SIZE" envDefault:"500"`
	// MaxCausationDepth: activities derived deeper than this (internal
	// triggers such as mission_completed) never progress missions.
	MaxCausationDepth int `env:"MAX_CAUSATION_DEPTH" envDefault:"3"`
}
