// Package ports declares what notifications needs from other modules, in
// notifications' own terms. Adapters (../../adapters) bridge these to
// providers' contracts.
package ports

import "context"

// PlayerReader resolves players of one tenant. An unknown id is absent from
// the ByIDs result; ByID reports it with found=false. Errors are transient
// (the provider is down), never "not found".
type PlayerReader interface {
	ByID(ctx context.Context, tenantID, playerID string) (PlayerSnapshot, bool, error)
	ByIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]PlayerSnapshot, error)
}

// PlayerSnapshot is notifications' own projection of a player. Email is
// read at send time and never stored by this module.
type PlayerSnapshot struct {
	ID          string
	TenantID    string
	ExternalID  string
	DisplayName string
	Email       string
	Active      bool
}
