package rewards

import "time"

// Config is embedded by the composition root with envPrefix "REWARDS_".
type Config struct {
	// HoldTTL is how long a paid claim may wait in pending_payment before
	// the reconcile sweep asks points and cancels it.
	HoldTTL time.Duration `env:"HOLD_TTL" envDefault:"10m"`
	// ClaimsReconcileSchedule is the cron spec of rewards.claims_reconcile.
	ClaimsReconcileSchedule string `env:"CLAIMS_RECONCILE_SCHEDULE" envDefault:"* * * * *"`
	// ClaimsExpireSchedule is the cron spec of rewards.claims_expire.
	ClaimsExpireSchedule string `env:"CLAIMS_EXPIRE_SCHEDULE" envDefault:"0 * * * *"`
	// SweepBatchSize bounds the rows one sweep query loads.
	SweepBatchSize int `env:"SWEEP_BATCH_SIZE" envDefault:"500"`
	// LateDebitLookback is how far before its last run the reconcile sweep
	// re-checks cancelled paid claims for a debit whose event was lost.
	LateDebitLookback time.Duration `env:"LATE_DEBIT_LOOKBACK" envDefault:"24h"`
}
