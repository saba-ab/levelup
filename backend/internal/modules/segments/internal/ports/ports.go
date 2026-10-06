// Package ports declares what segments consumes from other modules, in its
// own terms. Adapters under adapters/ bridge these to providers' contracts.
package ports

import (
	"context"
	"time"
)

// PlayerReader resolves and enumerates a tenant's players. A player of
// another tenant is indistinguishable from an unknown one.
type PlayerReader interface {
	PlayersByIDs(ctx context.Context, tenantID string, ids []string) (map[string]PlayerSnapshot, error)
	// ListPlayerIDs pages the tenant's (non-deleted) player ids in ascending
	// id order, strictly after afterID ("" starts at the beginning), at most
	// limit ids. A short page means the end.
	ListPlayerIDs(ctx context.Context, tenantID, afterID string, limit int) ([]string, error)
}

// PlayerSnapshot is segments' own projection of a player.
type PlayerSnapshot struct {
	ID          string
	TenantID    string
	ExternalID  string
	DisplayName string
	Active      bool
	Attributes  map[string]any
	CreatedAt   time.Time
}

// ProgressReader returns each player's current level number; players with
// no progress are absent (level 0).
type ProgressReader interface {
	LevelsByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]int, error)
}

// WalletReader returns each player's wallet; players without one are
// absent (zero balance).
type WalletReader interface {
	WalletsByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]WalletSnapshot, error)
}

type WalletSnapshot struct {
	Balance        int64
	LifetimeEarned int64
}

// BadgeReader returns the ids of the badges each player currently holds.
type BadgeReader interface {
	EarnedBadges(ctx context.Context, tenantID string, playerIDs []string) (map[string][]string, error)
}

// ActivityReader returns each player's last activity time; players never
// seen are absent.
type ActivityReader interface {
	LastSeen(ctx context.Context, tenantID string, playerIDs []string) (map[string]time.Time, error)
}
