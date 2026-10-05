// Package errs is the error taxonomy shared by every layer. It knows nothing
// about HTTP, SQL, or any transport — httpx owns the Kind→status mapping.
package errs

import (
	"errors"
	"fmt"
)

type Kind uint8

const (
	Unknown Kind = iota
	Invalid
	NotFound
	AlreadyExists
	Conflict
	PermissionDenied
	Unauthenticated
	Unavailable
	Internal
)

var kindNames = map[Kind]string{
	Unknown:          "unknown",
	Invalid:          "invalid",
	NotFound:         "not_found",
	AlreadyExists:    "already_exists",
	Conflict:         "conflict",
	PermissionDenied: "permission_denied",
	Unauthenticated:  "unauthenticated",
	Unavailable:      "unavailable",
	Internal:         "internal",
}

func (k Kind) String() string {
	if n, ok := kindNames[k]; ok {
		return n
	}
	return "unknown"
}

// Error is the concrete carrier. Constructed only through New/Wrap/WithFields
// so a zero-value Error can never enter a chain.
type Error struct {
	kind   Kind
	msg    string
	err    error
	fields map[string]string
}

func New(k Kind, msg string) error {
	return &Error{kind: k, msg: msg}
}

func Wrap(k Kind, msg string, err error) error {
	return &Error{kind: k, msg: msg, err: err}
}

func (e *Error) Error() string {
	if e.err == nil {
		return e.msg
	}
	if e.msg == "" {
		return e.err.Error()
	}
	return fmt.Sprintf("%s: %s", e.msg, e.err)
}

func (e *Error) Unwrap() error { return e.err }

// KindOf returns the Kind of the outermost *Error in the chain, or Unknown.
func KindOf(err error) Kind {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.kind
	}
	return Unknown
}

// WithFields attaches per-field detail (validation errors) to the chain.
// A non-errs error is wrapped so the fields survive further %w wrapping.
func WithFields(err error, fields map[string]string) error {
	if err == nil {
		return nil
	}
	if e, ok := errors.AsType[*Error](err); ok {
		clone := *e
		clone.fields = fields
		// Preserve anything the original wrapped, but replace the errs layer
		// so the outermost carrier holds the fields.
		if clone.err == nil && err != e {
			clone.err = err
		}
		return &clone
	}
	return &Error{kind: Unknown, msg: err.Error(), err: err, fields: fields}
}

// FieldsOf returns the field map of the outermost *Error carrying one, or nil.
func FieldsOf(err error) map[string]string {
	for err != nil {
		e, ok := errors.AsType[*Error](err)
		if !ok {
			return nil
		}
		if e.fields != nil {
			return e.fields
		}
		err = e.err
	}
	return nil
}
