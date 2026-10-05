// Package effect holds the vocabulary shared by every job command that
// changes a player's state (credit points, grant XP, award a badge, ...):
// who caused it and under which idempotency key. Leaf package: stdlib only.
package effect

// Source kinds: what decided the effect. The pair (Kind, ID) is stored on
// the target's ledger row so every balance change is explainable.
const (
	SourceRule      = "rule"      // ID = rule_execution effect id
	SourceMission   = "mission"   // ID = mission attempt id
	SourceStreak    = "streak"    // ID = streak milestone award id
	SourceLevel     = "level"     // ID = level reward id
	SourceBadge     = "badge"     // ID = badge award id
	SourceReward    = "reward"    // ID = reward claim id
	SourceManual    = "manual"    // ID = acting user id (admin API)
	SourceTransfer  = "transfer"  // ID = transfer id
	SourceMigration = "migration" // ID = legacy row id
)

// Source explains why an effect happened. ActivityID is set when the effect
// descends from an ingested activity, so the whole chain is traceable.
type Source struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	ActivityID string `json:"activity_id,omitempty"`
}

// Rejection reasons published in *_rejected.v1 facts. A rejection is a
// business result, never an error: the job is acked, nothing is retried.
const (
	ReasonPlayerInactive      = "player_inactive"
	ReasonPlayerNotFound      = "player_not_found"
	ReasonInsufficientBalance = "insufficient_balance"
	ReasonWalletInactive      = "wallet_inactive"
	ReasonTargetInactive      = "target_inactive"
	ReasonTargetNotFound      = "target_not_found"
	ReasonAlreadyEarned       = "already_earned"
	ReasonLimitReached        = "limit_reached"
)
