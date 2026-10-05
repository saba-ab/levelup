package domain

import (
	"levelup/internal/modules/player/contracts"
	"levelup/internal/shared/errs"
)

var (
	ErrNotFound = errs.WithCode(
		errs.New(errs.NotFound, "player not found"), contracts.CodePlayerNotFound)
	ErrExternalIDTaken = errs.WithCode(
		errs.New(errs.AlreadyExists, "a player with this external ID already exists"), contracts.CodeExternalIDTaken)
	ErrVersionConflict = errs.WithCode(
		errs.New(errs.Conflict, "player modified concurrently, retry"), contracts.CodePlayerVersionConflict)

	ErrNoTenant          = errs.New(errs.Invalid, "player needs a tenant")
	ErrNoExternalID      = errs.New(errs.Invalid, "external_id is required")
	ErrExternalIDTooLong = errs.New(errs.Invalid, "external_id must be at most 255 characters")
	ErrDisplayNameLong   = errs.New(errs.Invalid, "display_name must be at most 255 characters")
	ErrEmailInvalid      = errs.New(errs.Invalid, "email must be a valid address of at most 255 characters")
	ErrTooManyAttributes = errs.New(errs.Invalid, "attributes may hold at most 100 keys")
	ErrBadAttributeKey   = errs.New(errs.Invalid, "attribute keys must be 1 to 64 characters")
)
