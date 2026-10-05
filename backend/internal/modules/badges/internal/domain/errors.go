package domain

import "levelup/internal/shared/errs"

// Package-level errors. The kind decides the HTTP status (and, in a job
// handler, whether the consumer retries); the code is what clients branch on
// (ADR-0016).
var (
	ErrNameRequired     = errs.WithCode(errs.New(errs.Invalid, "badge name is required"), "badge_name_required")
	ErrInvalidSlug      = errs.WithCode(errs.New(errs.Invalid, "slug must be lowercase letters, digits and dashes"), "badge_invalid_slug")
	ErrInvalidTier      = errs.WithCode(errs.New(errs.Invalid, "unknown badge tier"), "badge_invalid_tier")
	ErrInvalidCategory  = errs.WithCode(errs.New(errs.Invalid, "unknown badge category"), "badge_invalid_category")
	ErrNegativePoints   = errs.WithCode(errs.New(errs.Invalid, "points_value must be >= 0"), "badge_negative_points")
	ErrInvalidMaxAwards = errs.WithCode(errs.New(errs.Invalid, "max_awards must be >= 1"), "badge_invalid_max_awards")

	ErrBadgeNotFound   = errs.WithCode(errs.New(errs.NotFound, "badge not found"), "badge_not_found")
	ErrPlayerNotFound  = errs.WithCode(errs.New(errs.NotFound, "player not found"), "player_not_found")
	ErrSlugTaken       = errs.WithCode(errs.New(errs.AlreadyExists, "a badge with this slug already exists"), "badge_slug_taken")
	ErrVersionConflict = errs.WithCode(errs.New(errs.Conflict, "badge data modified concurrently, retry"), "version_conflict")

	// Award rejections, surfaced over HTTP only. Job consumers turn the same
	// outcomes into badges.award_rejected.v1 and ack.
	ErrBadgeInactive    = errs.WithCode(errs.New(errs.Conflict, "badge is inactive"), "badge_inactive")
	ErrAlreadyEarned    = errs.WithCode(errs.New(errs.Conflict, "player already earned this badge"), "badge_already_earned")
	ErrMaxAwardsReached = errs.WithCode(errs.New(errs.Conflict, "badge max awards reached"), "badge_max_awards_reached")
	ErrPlayerInactive   = errs.WithCode(errs.New(errs.Conflict, "player is inactive"), "player_inactive")
)
