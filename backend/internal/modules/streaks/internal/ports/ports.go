// Package ports declares what streaks consumes from other modules, in its
// own terms. Adapters under adapters/ bridge these to providers' contracts.
package ports

import "context"

// PlayerReader resolves players inside one tenant. A player of another
// tenant is indistinguishable from an unknown one: absent from the result.
type PlayerReader interface {
	PlayersByIDs(ctx context.Context, tenantID string, ids []string) (map[string]PlayerSnapshot, error)
	// PlayersByExternalIDs resolves the tenant's own player ids; the result
	// is keyed by external id.
	PlayersByExternalIDs(ctx context.Context, tenantID string, externalIDs []string) (map[string]PlayerSnapshot, error)
}

// PlayerSnapshot is streaks' own projection of a player.
type PlayerSnapshot struct {
	ID         string
	TenantID   string
	ExternalID string
	Active     bool
}

// TenantReader resolves tenants. Unknown ids are absent from the result.
type TenantReader interface {
	TenantsByIDs(ctx context.Context, ids []string) (map[string]TenantSnapshot, error)
}

// TenantSnapshot is streaks' own projection of a tenant: periods are
// bucketed in its Timezone (IANA name; empty means UTC).
type TenantSnapshot struct {
	ID       string
	Timezone string
}
