package domain

import "levelup/internal/shared/errs"

// Business errors carry a code clients branch on (ADR-0016).
var (
	ErrNotFound = errs.WithCode(errs.New(errs.NotFound, "program not found"), "program_not_found")

	ErrNoTenant          = errs.New(errs.Invalid, "program needs a tenant")
	ErrNameRequired      = errs.WithCode(errs.New(errs.Invalid, "name is required"), "name_required")
	ErrNameTooLong       = errs.WithCode(errs.New(errs.Invalid, "name must be at most 255 characters"), "name_too_long")
	ErrDescriptionLong   = errs.WithCode(errs.New(errs.Invalid, "description must be at most 1000 characters"), "description_too_long")
	ErrBadSlug           = errs.WithCode(errs.New(errs.Invalid, "slug must be lower-case letters, digits and single dashes, at most 120 characters"), "invalid_slug")
	ErrBadWindow         = errs.WithCode(errs.New(errs.Invalid, "ends_at must not be before starts_at"), "invalid_program_window")
	ErrSlugTaken         = errs.WithCode(errs.New(errs.AlreadyExists, "a program with this slug already exists"), "slug_taken")
	ErrVersionConflict   = errs.WithCode(errs.New(errs.Conflict, "program modified concurrently, retry"), "version_conflict")
	ErrInvalidTransition = errs.WithCode(errs.New(errs.Conflict, "invalid program status transition"), "invalid_status_transition")
	ErrWindowElapsed     = errs.WithCode(errs.New(errs.Conflict, "program ends_at has already passed"), "program_window_elapsed")
	ErrNotAccepting      = errs.WithCode(errs.New(errs.Conflict, "program cannot accept new players at this time"), "program_not_accepting_players")
	ErrBadStatus         = errs.WithCode(errs.New(errs.Invalid, "unknown program status"), "invalid_status")

	ErrPlayerNotFound = errs.WithCode(errs.New(errs.NotFound, "player not found"), "player_not_found")
	ErrPlayerInactive = errs.WithCode(errs.New(errs.Invalid, "player is not active"), "player_inactive")
)
