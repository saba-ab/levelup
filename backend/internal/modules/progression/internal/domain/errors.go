package domain

import "levelup/internal/shared/errs"

var (
	ErrLevelNotFound         = errs.WithCode(errs.New(errs.NotFound, "level not found"), "level_not_found")
	ErrLevelNumberTaken      = errs.WithCode(errs.New(errs.AlreadyExists, "a level with this number already exists"), "level_number_taken")
	ErrInvalidLevelNumber    = errs.WithCode(errs.New(errs.Invalid, "level_number must be at least 1"), "invalid_level_number")
	ErrNegativeXPRequired    = errs.WithCode(errs.New(errs.Invalid, "xp_required must not be negative"), "invalid_xp_required")
	ErrNegativePointsReward  = errs.WithCode(errs.New(errs.Invalid, "points_reward must not be negative"), "invalid_points_reward")
	ErrXPNotIncreasing       = errs.WithCode(errs.New(errs.Invalid, "xp_required must strictly increase with level_number"), "xp_required_not_increasing")
	ErrNonPositiveAmount     = errs.WithCode(errs.New(errs.Invalid, "amount must be positive"), "invalid_amount")
	ErrXPOverflow            = errs.WithCode(errs.New(errs.Invalid, "total xp would overflow"), "xp_overflow")
	ErrMissingIdempotencyKey = errs.New(errs.Invalid, "idempotency key is required")
	ErrMissingTenant         = errs.New(errs.Invalid, "tenant id is required")
	ErrMissingPlayer         = errs.New(errs.Invalid, "player id is required")
	ErrVersionConflict       = errs.New(errs.Conflict, "player progress modified concurrently, retry")
	ErrPlayerNotFound        = errs.WithCode(errs.New(errs.NotFound, "player not found"), "player_not_found")
	ErrPlayerInactive        = errs.WithCode(errs.New(errs.Conflict, "player is not active"), "player_inactive")
)
