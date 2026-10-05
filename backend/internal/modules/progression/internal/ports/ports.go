// Package ports declares what progression consumes from other modules, in
// progression's own types. Adapters under adapters/ map providers onto them.
package ports

import "context"

// PlayerReader resolves players inside one tenant. ByID returns an
// errs.NotFound error for an unknown player; ByIDs omits unknown ids.
type PlayerReader interface {
	ByID(ctx context.Context, tenantID, playerID string) (PlayerSnapshot, error)
	ByIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]PlayerSnapshot, error)
}

// PlayerSnapshot is progression's projection of a player.
type PlayerSnapshot struct {
	ID       string
	TenantID string
	Active   bool
}
