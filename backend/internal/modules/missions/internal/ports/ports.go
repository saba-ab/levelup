// Package ports declares what missions needs from other modules, in its own
// terms. Adapters under missions/adapters implement them.
package ports

import "context"

// PlayerReader resolves players of a tenant. Unknown ids are absent from the
// result, never an error.
type PlayerReader interface {
	PlayersByIDs(ctx context.Context, tenantID string, ids []string) (map[string]PlayerSnapshot, error)
}

// PlayerSnapshot is missions' own projection of a player.
type PlayerSnapshot struct {
	ID       string
	TenantID string
	Active   bool
}
