// Package contracts is rewards' public surface. Claiming a reward that costs
// points is a saga owned by rewards: hold stock, debit points via
// job.points.debit, settle on points.debited.v1 / points.debit_rejected.v1.
package contracts

import (
	"time"

	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
)

const (
	TopicClaimRequested = "rewards.claim_requested.v1"
	TopicClaimed        = "rewards.claimed.v1"
	TopicClaimRejected  = "rewards.claim_rejected.v1"
	TopicRedeemed       = "rewards.redeemed.v1"
	TopicClaimExpired   = "rewards.claim_expired.v1"
	TopicClaimCancelled = "rewards.claim_cancelled.v1"
)

// JobGrant gives a reward for free (rule action grant_reward): no points
// are debited, stock and per-player limits still apply.
const JobGrant = "rewards.grant"

func Topic(job string) string { return "job." + job }

// Claim statuses.
const (
	ClaimPendingPayment = "pending_payment"
	ClaimClaimed        = "claimed"
	ClaimRejected       = "rejected"
	ClaimRedeemed       = "redeemed"
	ClaimExpired        = "expired"
	ClaimCancelled      = "cancelled"
	ClaimRefundPending  = "refund_pending"
	ClaimRefunded       = "refunded"
)

// DebitKey is the points idempotency key of a claim's payment.
func DebitKey(claimID string) string { return "reward_claim:" + claimID }

// RefundKey is the points idempotency key of a claim's refund.
func RefundKey(claimID string) string { return "reward_refund:" + claimID }

type GrantCmdV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	RewardID       string        `json:"reward_id"`
	Source         effect.Source `json:"source"`
	OccurredAt     time.Time     `json:"occurred_at"`
}

type ClaimV1 struct {
	ClaimID    string    `json:"claim_id"`
	TenantID   string    `json:"tenant_id"`
	PlayerID   string    `json:"player_id"`
	RewardID   string    `json:"reward_id"`
	RewardSlug string    `json:"reward_slug"`
	Status     string    `json:"status"`
	PointsCost int64     `json:"points_cost"`
	Reason     string    `json:"reason,omitempty"`
	At         time.Time `json:"at"`

	// Additive fields. IdempotencyKey is the GrantCmdV1 key for claims made
	// by rewards.grant, so the issuer can match its result; empty for HTTP
	// claims. DebitKey is the points key of a paid claim.
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	DebitKey       string         `json:"debit_key,omitempty"`
	RewardType     string         `json:"reward_type,omitempty"`
	Code           string         `json:"code,omitempty"`
	BadgeRewardID  string         `json:"badge_reward_id,omitempty"`
	LevelRewardID  string         `json:"level_reward_id,omitempty"`
	Value          string         `json:"value,omitempty"`
	ValueType      string         `json:"value_type,omitempty"`
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
	Source         *effect.Source `json:"source,omitempty"`
}

// Reward types.
const (
	TypePoints   = "points"
	TypeDiscount = "discount"
	TypeItem     = "item"
	TypeBadge    = "badge"
	TypeLevel    = "level"
	TypeCustom   = "custom"
)

// Reward statuses.
const (
	RewardDraft    = "draft"
	RewardActive   = "active"
	RewardPaused   = "paused"
	RewardExpired  = "expired"
	RewardDepleted = "depleted"
)

// Rejection reasons published in rewards.claim_rejected.v1 besides the
// shared effect.Reason* values (points' debit rejection reasons are passed
// through unchanged).
const (
	ReasonRewardNotFound         = "reward_not_found"
	ReasonRewardNotAvailable     = "reward_not_available"
	ReasonRewardDepleted         = "reward_depleted"
	ReasonPlayerLimitReached     = "player_limit_reached"
	ReasonLevelRequirementNotMet = "level_requirement_not_met"
)

// Cancellation reasons published in rewards.claim_cancelled.v1.
const (
	CancelHoldExpired = "hold_expired"
	CancelByAdmin     = "cancelled_by_admin"
)

const Module = "rewards"

var (
	PermViewAny    = authz.Permission{Module: Module, Action: "view_any"}
	PermView       = authz.Permission{Module: Module, Action: "view"}
	PermCreate     = authz.Permission{Module: Module, Action: "create"} // admin roles
	PermUpdate     = authz.Permission{Module: Module, Action: "update"} // admin roles
	PermDelete     = authz.Permission{Module: Module, Action: "delete"} // admin roles
	PermClaim      = authz.Permission{Module: Module, Action: "claim"}
	PermRedeem     = authz.Permission{Module: Module, Action: "redeem"}
	PermCancel     = authz.Permission{Module: Module, Action: "cancel"} // admin roles
	PermViewClaims = authz.Permission{Module: Module, Action: "view_claims"}
)

var AllPermissions = []authz.Permission{
	PermViewAny, PermView, PermCreate, PermUpdate, PermDelete, PermClaim, PermRedeem, PermCancel, PermViewClaims,
}
