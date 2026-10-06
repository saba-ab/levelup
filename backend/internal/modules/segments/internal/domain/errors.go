package domain

import "levelup/internal/shared/errs"

// Problem codes (ADR-0016) clients may branch on.
const (
	CodeSegmentNotFound        = "segment_not_found"
	CodeNameTaken              = "segment_name_taken"
	CodeSegmentVersionConflict = "segment_version_conflict"
	CodeInvalidSegment         = "invalid_segment"
	CodeInvalidConditions      = "invalid_segment_conditions"
)

var (
	ErrSegmentNotFound = errs.WithCode(errs.New(errs.NotFound, "segment not found"), CodeSegmentNotFound)
	ErrNameTaken       = errs.WithCode(errs.New(errs.AlreadyExists, "a segment with this name already exists"), CodeNameTaken)
	ErrVersionConflict = errs.WithCode(errs.New(errs.Conflict, "segment modified concurrently, retry"), CodeSegmentVersionConflict)

	ErrNameRequired       = invalid("name is required")
	ErrNameTooLong        = invalid("name must be at most 255 characters")
	ErrDescriptionTooLong = invalid("description must be at most 1000 characters")
	ErrNoTenant           = invalid("tenant id is required")
)

func invalid(msg string) error {
	return errs.WithCode(errs.New(errs.Invalid, msg), CodeInvalidSegment)
}

// conditionsError is a conditions validation failure naming the offending
// path (e.g. "all[1].any[0]").
func conditionsError(path, msg string) error {
	if path != "" {
		msg = path + ": " + msg
	}
	return errs.WithCode(errs.New(errs.Invalid, msg), CodeInvalidConditions)
}
