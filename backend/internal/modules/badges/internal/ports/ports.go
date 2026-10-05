// Package ports declares what badges needs from other modules, in badges'
// own terms. Adapters (../../adapters) bridge these to providers' contracts.
package ports

import "context"

// PlayerReader resolves players of one tenant. An unknown id is absent from
// the ByIDs result; ByID reports it with found=false. Errors are transient
// (the provider is down), never "not found".
type PlayerReader interface {
	ByID(ctx context.Context, tenantID, playerID string) (PlayerSnapshot, bool, error)
	ByIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]PlayerSnapshot, error)
}

// PlayerSnapshot is badges' own projection of a player — never player's type.
type PlayerSnapshot struct {
	ID       string
	TenantID string
	Active   bool
}
