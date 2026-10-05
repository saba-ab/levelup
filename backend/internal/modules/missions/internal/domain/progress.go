package domain

import "time"

// Progress event outcomes.
const (
	ProgressApplied  = "applied"
	ProgressRejected = "rejected"
)

// ProgressEvent is the idempotency ledger of progress commands: one row per
// (tenant, idempotency key). A redelivered command finds its row and does
// nothing, so progress is never double-counted.
type ProgressEvent struct {
	ID             string
	TenantID       string
	IdempotencyKey string
	MissionID      string
	PlayerID       string
	AttemptID      string // "" when rejected before an attempt was found
	Increment      int64
	Status         string
	Reason         string
	SourceKind     string
	SourceID       string
	CreatedAt      time.Time
}
