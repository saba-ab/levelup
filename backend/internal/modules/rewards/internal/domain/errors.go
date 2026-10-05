package domain

import "levelup/internal/shared/errs"

// Business errors carry a code clients branch on (ADR-0016).
var (
	ErrRewardNotFound = errs.WithCode(errs.New(errs.NotFound, "reward not found"), "reward_not_found")
	ErrClaimNotFound  = errs.WithCode(errs.New(errs.NotFound, "reward claim not found"), "reward_claim_not_found")
	ErrPlayerNotFound = errs.WithCode(errs.New(errs.NotFound, "player not found"), "player_not_found")
	ErrPlayerInactive = errs.WithCode(errs.New(errs.Invalid, "player is not active"), "player_inactive")

	ErrNoTenant        = errs.New(errs.Invalid, "reward needs a tenant")
	ErrNameRequired    = errs.WithCode(errs.New(errs.Invalid, "name is required"), "name_required")
	ErrNameTooLong     = errs.WithCode(errs.New(errs.Invalid, "name must be at most 255 characters"), "name_too_long")
	ErrDescriptionLong = errs.WithCode(errs.New(errs.Invalid, "description must be at most 1000 characters"), "description_too_long")
	ErrBadSlug         = errs.WithCode(errs.New(errs.Invalid, "slug must be lower-case letters, digits and single dashes, at most 120 characters"), "invalid_slug")
	ErrBadType         = errs.WithCode(errs.New(errs.Invalid, "unknown reward type"), "invalid_reward_type")
	ErrBadStatus       = errs.WithCode(errs.New(errs.Invalid, "unknown reward status"), "invalid_status")
	ErrBadPointsCost   = errs.WithCode(errs.New(errs.Invalid, "points_cost must be zero or positive"), "invalid_points_cost")
	ErrBadValue        = errs.WithCode(errs.New(errs.Invalid, "value must be a decimal number with at most 2 decimals and 8 integer digits"), "invalid_value")
	ErrBadValueType    = errs.WithCode(errs.New(errs.Invalid, "value_type must be percentage or fixed"), "invalid_value_type")
	ErrBadLimit        = errs.WithCode(errs.New(errs.Invalid, "limits, claim_ttl_days and level_requirement must be at least 1"), "invalid_limit")
	ErrBadWindow       = errs.WithCode(errs.New(errs.Invalid, "end_at must not be before start_at"), "invalid_reward_window")
	ErrStockBelowUsed  = errs.WithCode(errs.New(errs.Conflict, "max_redemptions cannot be lower than the stock already used"), "max_redemptions_below_used")
	ErrSlugTaken       = errs.WithCode(errs.New(errs.AlreadyExists, "a reward with this slug already exists"), "slug_taken")
	ErrVersionConflict = errs.WithCode(errs.New(errs.Conflict, "modified concurrently, retry"), "version_conflict")

	ErrRewardNotAvailable = errs.WithCode(errs.New(errs.Conflict, "reward is not available"), "reward_not_available")
	ErrRewardDepleted     = errs.WithCode(errs.New(errs.Conflict, "reward stock is depleted"), "reward_depleted")
	ErrPlayerLimitReached = errs.WithCode(errs.New(errs.Conflict, "player has reached the maximum claims for this reward"), "player_limit_reached")
	ErrLevelTooLow        = errs.WithCode(errs.New(errs.Conflict, "player level is below the reward's level requirement"), "level_requirement_not_met")
	ErrInvalidTransition  = errs.WithCode(errs.New(errs.Conflict, "invalid claim status transition"), "invalid_status_transition")
	ErrClaimExpired       = errs.WithCode(errs.New(errs.Conflict, "claim has expired and cannot be redeemed"), "claim_expired")
	ErrDuplicateRequest   = errs.New(errs.AlreadyExists, "claim already recorded for this idempotency key")
	ErrCodeCollision      = errs.New(errs.Unavailable, "voucher code collision, retry")
	ErrIdempotencyKeyLong = errs.WithCode(errs.New(errs.Invalid, "Idempotency-Key must be at most 255 characters"), "invalid_idempotency_key")
)
