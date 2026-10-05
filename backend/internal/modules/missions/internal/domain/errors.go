package domain

import "levelup/internal/shared/errs"

// Business errors carry a code clients branch on (ADR-0016).
var (
	ErrMissionNotFound = errs.WithCode(errs.New(errs.NotFound, "mission not found"), "mission_not_found")
	ErrAttemptNotFound = errs.WithCode(errs.New(errs.NotFound, "mission attempt not found"), "mission_attempt_not_found")

	ErrNoTenant          = errs.New(errs.Invalid, "mission needs a tenant")
	ErrNameRequired      = errs.WithCode(errs.New(errs.Invalid, "name is required"), "name_required")
	ErrNameTooLong       = errs.WithCode(errs.New(errs.Invalid, "name must be at most 255 characters"), "name_too_long")
	ErrDescriptionLong   = errs.WithCode(errs.New(errs.Invalid, "description must be at most 1000 characters"), "description_too_long")
	ErrBadSlug           = errs.WithCode(errs.New(errs.Invalid, "slug must be lower-case letters, digits and single dashes, at most 120 characters"), "invalid_slug")
	ErrBadType           = errs.WithCode(errs.New(errs.Invalid, "type must be one of one_time, daily, weekly, repeating"), "invalid_mission_type")
	ErrBadStatus         = errs.WithCode(errs.New(errs.Invalid, "unknown mission status"), "invalid_status")
	ErrBadInitialStatus  = errs.WithCode(errs.New(errs.Invalid, "a mission is created as draft or active"), "invalid_status")
	ErrBadTarget         = errs.WithCode(errs.New(errs.Invalid, "target must be greater than zero"), "invalid_target")
	ErrNegativeReward    = errs.WithCode(errs.New(errs.Invalid, "rewards must not be negative"), "invalid_reward")
	ErrBadMaxCompletions = errs.WithCode(errs.New(errs.Invalid, "max_completions_per_player must be at least 1"), "invalid_max_completions")
	ErrOneTimeOnce       = errs.WithCode(errs.New(errs.Invalid, "a one_time mission completes at most once per player"), "invalid_max_completions")
	ErrBadWindow         = errs.WithCode(errs.New(errs.Invalid, "ends_at must be after starts_at"), "invalid_mission_window")
	ErrSlugTaken         = errs.WithCode(errs.New(errs.AlreadyExists, "a mission with this slug already exists"), "slug_taken")
	ErrVersionConflict   = errs.WithCode(errs.New(errs.Conflict, "mission modified concurrently, retry"), "version_conflict")
	ErrInvalidTransition = errs.WithCode(errs.New(errs.Conflict, "invalid mission status transition"), "invalid_status_transition")
	ErrTypeImmutable     = errs.WithCode(errs.New(errs.Conflict, "type can only change while the mission is a draft"), "mission_type_immutable")

	ErrNotAvailable        = errs.WithCode(errs.New(errs.Conflict, "mission is not active or outside its window"), "mission_not_available")
	ErrLimitReached        = errs.WithCode(errs.New(errs.Conflict, "player reached this mission's completion limit"), "mission_limit_reached")
	ErrAlreadyStarted      = errs.WithCode(errs.New(errs.AlreadyExists, "mission already started for this period"), "mission_already_started")
	ErrNotStarted          = errs.WithCode(errs.New(errs.NotFound, "mission not started"), "mission_not_started")
	ErrNotCompleted        = errs.WithCode(errs.New(errs.Conflict, "mission target not reached"), "mission_not_completed")
	ErrAttemptNotOpen      = errs.WithCode(errs.New(errs.Conflict, "attempt is not in progress"), "attempt_not_in_progress")
	ErrNonPositiveIncrease = errs.WithCode(errs.New(errs.Invalid, "increment must be greater than zero"), "invalid_increment")

	ErrPlayerNotFound = errs.WithCode(errs.New(errs.NotFound, "player not found"), "player_not_found")
	ErrPlayerInactive = errs.WithCode(errs.New(errs.Conflict, "player is not active"), "player_inactive")
)
