package authz

import "context"

// AllowAll is the pre-Phase-4 placeholder enforcer. Referenced only by the
// composition root; the casbin-backed enforcer replaces it there. Any other
// use is a boundary violation.
type AllowAll struct{}

func (AllowAll) Authorize(context.Context, Principal, Permission, any) error { return nil }
