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

// namespace is the fixed UUIDv5 namespace for derived identifiers. Never
// change it: every idempotency key derived from it would change too.
var namespace = uuid.MustParse("6f1d4c52-2b7e-5c3a-9a41-6c65766c7570")

// Derive returns a deterministic UUIDv5 from parts. Use it for idempotency
// keys of effects ("activity|rule_version|action_index") so a redelivered
// event or a retried job produces the same key and inserts nothing new.
func Derive(parts ...string) string {
	name := ""
	for i, p := range parts {
		if i > 0 {
			name += "|"
		}
		name += p
	}
	return uuid.NewSHA1(namespace, []byte(name)).String()
}
