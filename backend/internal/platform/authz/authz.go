// Package authz defines the authorization contract (PRD §7.6). No module —
// and nothing outside this package's casbin.go — imports casbin; the
// interface is the seam that makes OpenFGA/SpiceDB adoptable later (R33).
package authz

import (
	"context"
	"fmt"

	"levelup/internal/shared/errs"
)

// Permission is module-owned: each module declares its own catalogue in
// contracts/permissions.go and returns it from Module.Permissions().
type Permission struct {
	Module string
	Action string
}

func (p Permission) Key() string { return p.Module + ":" + p.Action }

// Principal is who is acting. RoleIDs are IDs, never role-name strings:
// renaming a role in an admin UI must not change who can withdraw money.
type Principal struct {
	UserID   string
	RoleIDs  []int64
	TenantID string
}

func (p Principal) Subject() string { return "user:" + p.UserID }

func (p Principal) RoleSubjects() []string {
	out := make([]string, len(p.RoleIDs))
	for i, id := range p.RoleIDs {
		out[i] = fmt.Sprintf("role:%d", id)
	}
	return out
}

// Enforcer answers "may this principal perform this action". Resource-scoped
// decisions ("on *this* wallet") live in the service layer, which passes the
// loaded entity as resource (PRD §7.6 constraint 3).
type Enforcer interface {
	Authorize(ctx context.Context, p Principal, perm Permission, resource any) error
}

type ctxKey struct{}

// Into stores the authenticated principal; authn middleware calls this.
func Into(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// From returns the principal and whether one is present.
func From(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// ErrNoTenant is returned on tenant routes for a principal that acts at
// platform level (no tid claim). Tenant data is never read "unscoped".
var ErrNoTenant = errs.New(errs.PermissionDenied, "this action requires a tenant context")

// RequireTenant returns the authenticated principal acting inside a tenant.
// Every tenant-owned service method starts here: the tenant id it returns is
// the only tenant filter repositories accept (ADR-0015).
func RequireTenant(ctx context.Context) (Principal, error) {
	p, ok := From(ctx)
	if !ok {
		return Principal{}, errs.New(errs.Unauthenticated, "authentication required")
	}
	if p.TenantID == "" {
		return Principal{}, ErrNoTenant
	}
	return p, nil
}
