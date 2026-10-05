package points

// Config is points' settings; the composition root embeds it with
// envPrefix "POINTS_".
type Config struct {
	// ReconcileSchedule is the cron spec (UTC) of points.reconcile (R47).
	ReconcileSchedule string `env:"RECONCILE_SCHEDULE" envDefault:"*/15 * * * *"`
}
