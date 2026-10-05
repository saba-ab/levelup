// Package ports declares what points consumes from other modules, in its
// own terms. Adapters under adapters/ bridge these to providers' contracts.
package ports

import "context"

// PlayerReader resolves players inside one tenant. A player of another
// tenant is indistinguishable from an unknown one: absent from the result.
type PlayerReader interface {
	PlayersByIDs(ctx context.Context, tenantID string, ids []string) (map[string]PlayerSnapshot, error)
}

// PlayerSnapshot is points' own projection of a player — never player's type.
type PlayerSnapshot struct {
	ID       string
	TenantID string
	Active   bool
}
