// Package ports declares what leaderboards needs from other modules, in
// leaderboards' own types. Adapters under adapters/ implement them.
package ports

import "context"

// PlayerReader hydrates ranking rows with display data and validates the
// player of a rank lookup. Unknown ids are absent from the result.
type PlayerReader interface {
	ByIDs(ctx context.Context, tenantID string, ids []string) (map[string]PlayerSnapshot, error)
}

// PlayerSnapshot is leaderboards' projection of a player.
type PlayerSnapshot struct {
	ID          string
	ExternalID  string
	DisplayName string
	Active      bool
}

// ExternalIDResolver is implemented by player adapters that can resolve a
// tenant's external id. It is optional (type-asserted) so test fakes of
// PlayerReader need not implement it.
type ExternalIDResolver interface {
	IDByExternalID(ctx context.Context, tenantID, externalID string) (string, bool, error)
}
