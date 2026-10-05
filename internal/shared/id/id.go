// Package id issues UUIDv7 identifiers: time-ordered, so b-tree friendly as
// primary keys, and safe to expose in URLs and event envelopes.
package id

import "github.com/google/uuid"

func NewID() string {
	v, err := uuid.NewV7()
	if err != nil {
		// NewV7 only fails if the entropy source does; at that point the
		// process has no business issuing identifiers.
		panic(err)
	}
	return v.String()
}
