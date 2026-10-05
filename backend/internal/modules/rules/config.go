package rules

import "time"

// Config is rules' settings; the composition root embeds it with envPrefix
// "RULES_".
type Config struct {
	// CacheTTL bounds a cached ruleset; keys carry the ruleset generation,
	// so a publish never serves stale rules — the TTL only ages out garbage.
	CacheTTL time.Duration `env:"CACHE_TTL" envDefault:"10m"`
	// MaxCausationDepth stops internal-trigger loops (doc 06 §11.9): an
	// activity deeper than this is decided with no effects.
	MaxCausationDepth int `env:"MAX_CAUSATION_DEPTH" envDefault:"3"`
	// MaxActionsPerRule / MaxConditionsPerRule bound a definition at write time.
	MaxActionsPerRule    int `env:"MAX_ACTIONS_PER_RULE" envDefault:"20"`
	MaxConditionsPerRule int `env:"MAX_CONDITIONS_PER_RULE" envDefault:"100"`
	// EffectsReconcileSchedule re-publishes effects with no outcome (R47).
	EffectsReconcileSchedule string `env:"EFFECTS_RECONCILE_SCHEDULE" envDefault:"*/5 * * * *"`
	// EffectsPendingAfter is how long an effect may wait for its outcome
	// before the sweep re-publishes its (idempotent) command.
	EffectsPendingAfter time.Duration `env:"EFFECTS_PENDING_AFTER" envDefault:"5m"`
	// EffectsMaxAttempts caps re-publishes; beyond it the row is left for humans.
	EffectsMaxAttempts int `env:"EFFECTS_MAX_ATTEMPTS" envDefault:"10"`
}
