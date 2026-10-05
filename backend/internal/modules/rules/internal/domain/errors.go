package domain

import "levelup/internal/shared/errs"

// Business errors carry a code clients branch on (ADR-0016).
var (
	ErrRuleNotFound     = errs.WithCode(errs.New(errs.NotFound, "rule not found"), "rule_not_found")
	ErrVersionNotFound  = errs.WithCode(errs.New(errs.NotFound, "rule version not found"), "rule_version_not_found")
	ErrDecisionNotFound = errs.WithCode(errs.New(errs.NotFound, "decision not found"), "decision_not_found")
	ErrPlayerNotFound   = errs.WithCode(errs.New(errs.NotFound, "player not found"), "player_not_found")

	ErrNameRequired      = errs.WithCode(errs.New(errs.Invalid, "name is required"), "name_required")
	ErrNameTooLong       = errs.WithCode(errs.New(errs.Invalid, "name must be at most 255 characters"), "name_too_long")
	ErrDescriptionLong   = errs.WithCode(errs.New(errs.Invalid, "description must be at most 1000 characters"), "description_too_long")
	ErrBadSlug           = errs.WithCode(errs.New(errs.Invalid, "slug must be lower-case letters, digits and single dashes, at most 120 characters"), "invalid_slug")
	ErrBadTriggerEvent   = errs.WithCode(errs.New(errs.Invalid, "trigger_event must match ^[a-z0-9_.:-]+$ and be at most 100 characters"), "invalid_trigger_event")
	ErrBadProgramID      = errs.WithCode(errs.New(errs.Invalid, "program_id must be a UUID"), "invalid_program_id")
	ErrBadStatus         = errs.WithCode(errs.New(errs.Invalid, "status must be active, inactive or archived"), "invalid_status")
	ErrBadPriority       = errs.WithCode(errs.New(errs.Invalid, "priority must be between 0 and 1000000"), "invalid_priority")
	ErrSlugTaken         = errs.WithCode(errs.New(errs.AlreadyExists, "a rule with this slug already exists"), "slug_taken")
	ErrVersionConflict   = errs.WithCode(errs.New(errs.Conflict, "rule version created concurrently, retry"), "version_conflict")
	ErrVersionPublished  = errs.WithCode(errs.New(errs.Conflict, "published versions are immutable: create a new version"), "version_immutable")
	ErrNoDraft           = errs.WithCode(errs.New(errs.Conflict, "the rule has no draft version to edit: POST /rules/{id}/versions first"), "no_draft_version")
	ErrPublishRequired   = errs.WithCode(errs.New(errs.Conflict, "a rule becomes active by publishing a version"), "publish_required")
	ErrRuleArchived      = errs.WithCode(errs.New(errs.Conflict, "archived rules cannot change"), "rule_archived")
	ErrInvalidTransition = errs.WithCode(errs.New(errs.Conflict, "invalid rule status transition"), "invalid_status_transition")
)
