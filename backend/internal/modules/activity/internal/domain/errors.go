package domain

import "levelup/internal/shared/errs"

// Business errors carry a code clients branch on (ADR-0016).
var (
	ErrNotFound = errs.WithCode(errs.New(errs.NotFound, "activity not found"), "activity_not_found")

	ErrNoTenant             = errs.New(errs.Invalid, "activity needs a tenant")
	ErrEventIDRequired      = errs.WithCode(errs.New(errs.Invalid, "event_id is required"), "invalid_event_id")
	ErrEventIDTooLong       = errs.WithCode(errs.New(errs.Invalid, "event_id must be at most 128 characters"), "invalid_event_id")
	ErrBadEventType         = errs.WithCode(errs.New(errs.Invalid, "event_type must match ^[a-z0-9_.:-]+$ and be at most 100 characters"), "invalid_event_type")
	ErrPlayerRequired       = errs.WithCode(errs.New(errs.Invalid, "player_external_id is required"), "player_external_id_required")
	ErrPlayerIDTooLong      = errs.WithCode(errs.New(errs.Invalid, "player_external_id must be at most 255 characters"), "invalid_player_external_id")
	ErrOccurredInFuture     = errs.WithCode(errs.New(errs.Invalid, "occurred_at is too far in the future"), "occurred_at_in_future")
	ErrOccurredTooOld       = errs.WithCode(errs.New(errs.Invalid, "occurred_at is older than the accepted maximum age"), "occurred_at_too_old")
	ErrPropertiesTooLarge   = errs.WithCode(errs.New(errs.Invalid, "properties exceed the maximum size"), "properties_too_large")
	ErrContextTooLarge      = errs.WithCode(errs.New(errs.Invalid, "context exceeds the maximum size"), "context_too_large")
	ErrPayloadNotJSON       = errs.WithCode(errs.New(errs.Invalid, "properties and context must be JSON objects"), "invalid_payload")
	ErrBadCausationDepth    = errs.New(errs.Invalid, "causation_depth must not be negative")
	ErrUnknownEventType     = errs.WithCode(errs.New(errs.Invalid, "event_type is not defined for this tenant"), "unknown_event_type")
	ErrInactiveEventType    = errs.WithCode(errs.New(errs.Invalid, "event_type is not active"), "inactive_event_type")
	ErrBadStatus            = errs.WithCode(errs.New(errs.Invalid, "status must be pending, decided or rejected"), "invalid_status")
	ErrTooManyItems         = errs.WithCode(errs.New(errs.Invalid, "a batch holds at most 100 items"), "batch_too_large")
	ErrEmptyBatch           = errs.WithCode(errs.New(errs.Invalid, "a batch needs at least one item"), "batch_empty")
	ErrDuplicateVanished    = errs.WithCode(errs.New(errs.Conflict, "the stored activity for this event_id disappeared concurrently, retry"), "retry")
	ErrMalformedDecision    = errs.New(errs.Invalid, "decision payload needs tenant_id, activity_id and decision_id as UUIDs")
	ErrMalformedTrigger     = errs.New(errs.Invalid, "internal trigger payload needs tenant_id and player_id")
	ErrMalformedTenantPurge = errs.New(errs.Invalid, "tenant.deleted.v1 payload needs tenant_id")
)
