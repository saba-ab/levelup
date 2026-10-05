package domain

import (
	"time"

	"levelup/internal/shared/effect"
	"levelup/internal/shared/id"
)

// PlayerBadge is the per-(player, badge) holding. EarnedCount is the number
// of stacks; a freshly locked-in placeholder (insert-or-lock) has 0 and is
// never committed with 0 because the award that created it always applies.
type PlayerBadge struct {
	ID             string
	TenantID       string
	PlayerID       string
	BadgeID        string
	EarnedCount    int
	FirstAwardedAt time.Time
	LastAwardedAt  time.Time
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewPlayerBadge is the placeholder row used by insert-or-lock.
func NewPlayerBadge(tenantID, playerID, badgeID string, now time.Time) PlayerBadge {
	return PlayerBadge{
		ID:             id.NewID(),
		TenantID:       tenantID,
		PlayerID:       playerID,
		BadgeID:        badgeID,
		FirstAwardedAt: now,
		LastAwardedAt:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// Award adds one stack. Callers check Badge.CanAward first.
func (pb *PlayerBadge) Award(now time.Time) (isFirstEarn bool) {
	isFirstEarn = pb.EarnedCount == 0
	if isFirstEarn {
		pb.FirstAwardedAt = now
	}
	pb.EarnedCount++
	pb.LastAwardedAt = now
	pb.UpdatedAt = now
	return isFirstEarn
}

// AwardStatus is the outcome recorded on the append-only award ledger.
type AwardStatus string

const (
	AwardApplied  AwardStatus = "applied"
	AwardRejected AwardStatus = "rejected"
)

// Award is one row of the append-only badge_awards ledger: every award
// command, applied or rejected, keyed by its idempotency key (unique per
// tenant), so a redelivered command is a no-op.
type Award struct {
	ID             string
	TenantID       string
	PlayerID       string
	BadgeID        string
	PlayerBadgeID  string // applied only; ties the stack to its holding generation
	IdempotencyKey string
	Source         effect.Source
	AwardedBy      string // acting user for manual awards; empty for commands
	EarnedCount    int    // earned_count after this award (applied only)
	OccurredAt     time.Time
	Status         AwardStatus
	Reason         string // effect.Reason* when rejected
	CreatedAt      time.Time
}

// Applied reports whether the award added a stack.
func (a Award) Applied() bool { return a.Status == AwardApplied }

// Revocation records an admin revoke of a whole holding (all stacks).
type Revocation struct {
	ID                 string
	TenantID           string
	PlayerID           string
	BadgeID            string
	PlayerBadgeID      string
	EarnedCountRemoved int
	RevokedBy          string
	RevokedAt          time.Time
}

// Drift is one reconcile finding.
type Drift struct {
	PlayerBadgeID string
	TenantID      string
	PlayerID      string
	BadgeID       string
	EarnedCount   int
	AppliedAwards int
	Stackable     bool
	MaxAwards     *int
}

// Check names the invariant a drift breaks.
const (
	DriftCountMismatch = "earned_count_mismatch"
	DriftOverMax       = "earned_count_over_max"
)

// Checks returns every invariant the row breaks.
func (d Drift) Checks() []string {
	var out []string
	if d.EarnedCount != d.AppliedAwards {
		out = append(out, DriftCountMismatch)
	}
	limit := 1
	if d.Stackable {
		limit = -1
		if d.MaxAwards != nil {
			limit = *d.MaxAwards
		}
	}
	if limit >= 0 && d.EarnedCount > limit {
		out = append(out, DriftOverMax)
	}
	return out
}
