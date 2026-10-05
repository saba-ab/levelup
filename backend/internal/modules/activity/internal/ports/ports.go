// Package ports declares what activity needs from other modules. Activity
// owns these interfaces and the snapshot types; adapters/ bridges them to
// the providers' contracts (docs/examples.md §10).
package ports

import "context"

// PlayerReader resolves a tenant's players. Unknown players are simply
// absent from the result maps, never an error.
type PlayerReader interface {
	// ByExternalIDs is keyed by external id.
	ByExternalIDs(ctx context.Context, tenantID string, externalIDs []string) (map[string]PlayerSnapshot, error)
	// ByIDs is keyed by player id.
	ByIDs(ctx context.Context, tenantID string, ids []string) (map[string]PlayerSnapshot, error)
}

// PlayerSnapshot is activity's own projection of a player.
type PlayerSnapshot struct {
	ID         string
	ExternalID string
	Active     bool
}

// EventTypeReader resolves the event types visible to a tenant (its own
// plus platform-global ones). Unknown slugs are absent from the result.
type EventTypeReader interface {
	BySlugs(ctx context.Context, tenantID string, slugs []string) (map[string]EventTypeSnapshot, error)
}

// EventTypeSnapshot is activity's own projection of an event type.
type EventTypeSnapshot struct {
	Slug   string
	Active bool
}
