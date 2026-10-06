package domain

import (
	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/shared/errs"
)

// Package-level errors. The kind decides the HTTP status (and, in a job
// handler, whether the consumer retries); the code is what clients branch on.
var (
	ErrTemplateInvalid      = errs.WithCode(errs.New(errs.Invalid, "notification template is invalid"), contracts.CodeTemplateInvalid)
	ErrTemplateNotFound     = errs.WithCode(errs.New(errs.NotFound, "notification template not found"), contracts.CodeTemplateNotFound)
	ErrTemplateNameTaken    = errs.WithCode(errs.New(errs.AlreadyExists, "a notification template with this name already exists"), contracts.CodeTemplateNameTaken)
	ErrNotificationNotFound = errs.WithCode(errs.New(errs.NotFound, "notification not found"), contracts.CodeNotificationNotFound)
	ErrPlayerNotFound       = errs.WithCode(errs.New(errs.NotFound, "player not found"), contracts.CodePlayerNotFound)
	ErrVersionConflict      = errs.WithCode(errs.New(errs.Conflict, "notification template modified concurrently, retry"), contracts.CodeVersionConflict)

	// ErrRenderTimeout and ErrRenderTooLarge are execution failures; they
	// surface as render_error on the notification row.
	ErrRenderTimeout  = errs.New(errs.Invalid, "template rendering timed out")
	ErrRenderTooLarge = errs.New(errs.Invalid, "rendered template exceeds the size limit")
)

// invalid wraps field messages into ErrTemplateInvalid (422 with fields).
func invalid(fields map[string]string) error {
	return errs.WithFields(ErrTemplateInvalid, fields)
}
