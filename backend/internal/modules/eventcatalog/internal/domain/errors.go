package domain

import "levelup/internal/shared/errs"

// Package-level errors. Every error a client may branch on carries a code
// (ADR-0016).
var (
	ErrNotFound         = errs.WithCode(errs.New(errs.NotFound, "event type not found"), "event_type_not_found")
	ErrCategoryNotFound = errs.WithCode(errs.New(errs.NotFound, "event category not found"), "event_category_not_found")

	// ErrUnknownCategory: a write referenced a category the caller cannot
	// see (another tenant's, deleted, or a tenant category on a global type).
	ErrUnknownCategory = errs.WithCode(errs.New(errs.Invalid, "category does not exist or is not visible"), "unknown_event_category")

	// ErrSlugTaken replaces Laravel's 500 on the global UNIQUE(slug).
	ErrSlugTaken         = errs.WithCode(errs.New(errs.AlreadyExists, "an event type with this slug already exists"), "event_type_slug_taken")
	ErrCategorySlugTaken = errs.WithCode(errs.New(errs.AlreadyExists, "an event category with this slug already exists"), "event_category_slug_taken")

	// ErrGlobalReadOnly: tenants see platform-global rows but never change
	// them (fixes Laravel's tenant-created "predefined" global events).
	ErrGlobalReadOnly = errs.WithCode(errs.New(errs.PermissionDenied, "platform-global event types are read-only for tenants"), "global_event_type_read_only")

	// ErrPlatformOnly: the /platform surface acts on global rows only and
	// refuses principals that carry a tenant.
	ErrPlatformOnly = errs.WithCode(errs.New(errs.PermissionDenied, "this action requires a platform principal without a tenant"), "platform_principal_required")

	// ErrSlugImmutable: rules reference event types by slug; renaming would
	// silently orphan them (Laravel bug 03 §10.2 #17).
	ErrSlugImmutable = errs.WithCode(errs.New(errs.Invalid, "the slug of an event type cannot be changed"), "event_type_slug_immutable")

	ErrInvalidName        = errs.WithCode(errs.New(errs.Invalid, "name must be 1 to 255 characters"), "invalid_name")
	ErrInvalidSlug        = errs.WithCode(errs.New(errs.Invalid, "slug must be 1 to 100 lowercase letters or digits separated by single '_' or '-'"), "invalid_slug")
	ErrInvalidDescription = errs.WithCode(errs.New(errs.Invalid, "description must be at most 1000 characters"), "invalid_description")
)

// invalidSchema builds the property-schema error with a specific reason.
func invalidSchema(reason string) error {
	return errs.WithCode(errs.New(errs.Invalid, "invalid property_schema: "+reason), "invalid_property_schema")
}
