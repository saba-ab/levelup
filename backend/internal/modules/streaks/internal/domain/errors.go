package domain

import "levelup/internal/shared/errs"

// Problem codes (ADR-0016) clients may branch on.
const (
	CodeStreakNotFound        = "streak_not_found"
	CodeStreakInactive        = "streak_inactive"
	CodeSlugTaken             = "streak_slug_taken"
	CodeActivityKeyTaken      = "streak_activity_key_taken"
	CodePlayerNotFound        = "player_not_found"
	CodePlayerInactive        = "player_inactive"
	CodePlayerStreakNotFound  = "player_streak_not_found"
	CodeStreakVersionConflict = "streak_version_conflict"
	CodeInvalidStreak         = "invalid_streak"
)

var (
	ErrStreakNotFound       = errs.WithCode(errs.New(errs.NotFound, "streak not found"), CodeStreakNotFound)
	ErrStreakInactive       = errs.WithCode(errs.New(errs.Conflict, "streak is not active"), CodeStreakInactive)
	ErrSlugTaken            = errs.WithCode(errs.New(errs.AlreadyExists, "a streak with this slug already exists"), CodeSlugTaken)
	ErrActivityKeyTaken     = errs.WithCode(errs.New(errs.AlreadyExists, "a streak with this activity_key already exists"), CodeActivityKeyTaken)
	ErrPlayerNotFound       = errs.WithCode(errs.New(errs.NotFound, "player not found"), CodePlayerNotFound)
	ErrPlayerInactive       = errs.WithCode(errs.New(errs.Conflict, "player is not active"), CodePlayerInactive)
	ErrPlayerStreakNotFound = errs.WithCode(errs.New(errs.NotFound, "player has no record for this streak"), CodePlayerStreakNotFound)
	ErrVersionConflict      = errs.WithCode(errs.New(errs.Conflict, "streak modified concurrently, retry"), CodeStreakVersionConflict)

	ErrNameRequired        = invalid("name is required")
	ErrNameTooLong         = invalid("name must be at most 255 characters")
	ErrBadSlug             = invalid("slug must be 1-100 characters of a-z, 0-9, '-' or '_'")
	ErrBadActivityKey      = invalid("activity_key must be 1-100 characters of a-z, 0-9, '-', '_', '.' or ':'")
	ErrBadPeriod           = invalid("period must be daily, weekly or monthly")
	ErrBadGracePeriods     = invalid("grace_periods must be between 0 and 30")
	ErrNegativePoints      = invalid("points_per_period must be >= 0")
	ErrBadMilestoneCount   = invalid("milestone count must be >= 1")
	ErrNegativeBonus       = invalid("milestone bonus_points must be >= 0")
	ErrDuplicateMilestone  = invalid("milestone counts must be unique")
	ErrTooManyMilestones   = invalid("at most 50 milestones")
	ErrDescriptionTooLong  = invalid("description must be at most 1000 characters")
	ErrNoTenant            = invalid("tenant id is required")
	ErrPlayerStreakNoOwner = invalid("player streak needs a tenant, player and streak")
)

func invalid(msg string) error {
	return errs.WithCode(errs.New(errs.Invalid, msg), CodeInvalidStreak)
}
