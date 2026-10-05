// Package ports declares what program consumes from other modules, in
// program's own vocabulary. Adapters under ../../adapters implement them.
package ports

import "context"

// PlayerReader is program's view of the player module. Batch first: an
// unknown or foreign id is simply absent from ByIDs' result.
type PlayerReader interface {
	// ByID returns errs.NotFound when the player does not exist in tenantID.
	ByID(ctx context.Context, tenantID, playerID string) (PlayerSnapshot, error)
	ByIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]PlayerSnapshot, error)
}

// PlayerSnapshot is program's own projection of a player — never the
// player module's type.
type PlayerSnapshot struct {
	ID          string
	TenantID    string
	ExternalID  string
	DisplayName string
	Active      bool
}
