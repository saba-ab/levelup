// Package ports declares what rewards needs from other modules, in its own
// terms. Adapters under rewards/adapters bridge these to the providers'
// contracts; rewards never sees the providers' types.
package ports

import "context"

// PlayerSnapshot is rewards' own projection of a player.
type PlayerSnapshot struct {
	ID       string
	TenantID string
	Active   bool
}

// PlayerReader reads players of one tenant. Unknown ids are absent from the
// result, never an error.
type PlayerReader interface {
	PlayersByIDs(ctx context.Context, tenantID string, ids []string) (map[string]PlayerSnapshot, error)
}

// ProgressReader reads players' current level numbers. A player without
// progression is absent (treated as level 0).
type ProgressReader interface {
	LevelsByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]int, error)
}

// Payment outcome statuses.
const (
	PaymentApplied  = "applied"
	PaymentRejected = "rejected"
)

// PaymentOutcome is the settled result of a points command, by key.
type PaymentOutcome struct {
	Status string // PaymentApplied | PaymentRejected
	Reason string
}

// PointsReader asks points how a debit or refund settled. found=false
// means points has no outcome for the key (yet).
type PointsReader interface {
	OutcomeByKey(ctx context.Context, tenantID, idempotencyKey string) (out PaymentOutcome, found bool, err error)
}
