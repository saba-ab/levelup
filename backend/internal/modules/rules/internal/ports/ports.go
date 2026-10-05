// Package ports declares what rules needs from other modules, in rules' own
// terms (consumer-defined ports, docs/examples.md §10). Adapters under
// rules/adapters map providers' contracts into these snapshots.
package ports

import (
	"context"
	"time"
)

// PlayerSnapshot is rules' own projection of a player.
type PlayerSnapshot struct {
	ID          string
	TenantID    string
	ExternalID  string
	DisplayName string
	Email       string
	Active      bool
	Attributes  map[string]any
	CreatedAt   time.Time
}

// PlayerReader resolves players. Unknown ids are absent from the result,
// never an error.
type PlayerReader interface {
	// ByIDs is keyed by player id.
	ByIDs(ctx context.Context, tenantID string, ids []string) (map[string]PlayerSnapshot, error)
	// ByExternalIDs is keyed by the tenant's external id.
	ByExternalIDs(ctx context.Context, tenantID string, externalIDs []string) (map[string]PlayerSnapshot, error)
}

// ProgressSnapshot is a player's XP and level.
type ProgressSnapshot struct {
	PlayerID string
	TotalXP  int64
	Level    int
}

// ProgressReader reads XP/levels, keyed by player id.
type ProgressReader interface {
	ByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]ProgressSnapshot, error)
}

// BalanceSnapshot is a player's points balance.
type BalanceSnapshot struct {
	PlayerID string
	Balance  int64
}

// PointsReader reads balances, keyed by player id.
type PointsReader interface {
	ByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]BalanceSnapshot, error)
}

// ProgramReader answers program enrolment for program-scoped rules. It is
// optional: without it, rules ignore program_id (tenant-wide, Laravel parity).
type ProgramReader interface {
	EnrolledProgramIDs(ctx context.Context, tenantID, playerID string) ([]string, error)
}
