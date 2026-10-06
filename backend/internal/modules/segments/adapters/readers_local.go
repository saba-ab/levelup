package adapters

import (
	"context"
	"time"

	activitycontracts "levelup/internal/modules/activity/contracts"
	badgescontracts "levelup/internal/modules/badges/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/segments/internal/ports"
)

// LocalProgress wraps progression's Reader.
type LocalProgress struct{ r progressioncontracts.Reader }

func NewLocalProgress(r progressioncontracts.Reader) *LocalProgress { return &LocalProgress{r: r} }

var _ ports.ProgressReader = (*LocalProgress)(nil)

func (l *LocalProgress) LevelsByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(playerIDs))
	if len(playerIDs) == 0 {
		return out, nil
	}
	got, err := l.r.ProgressByPlayerIDs(ctx, tenantID, playerIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range got {
		out[p.PlayerID] = p.LevelNumber
	}
	return out, nil
}

// LocalWallets wraps points' Reader.
type LocalWallets struct{ r pointscontracts.Reader }

func NewLocalWallets(r pointscontracts.Reader) *LocalWallets { return &LocalWallets{r: r} }

var _ ports.WalletReader = (*LocalWallets)(nil)

func (l *LocalWallets) WalletsByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]ports.WalletSnapshot, error) {
	out := make(map[string]ports.WalletSnapshot, len(playerIDs))
	if len(playerIDs) == 0 {
		return out, nil
	}
	got, err := l.r.WalletsByPlayerIDs(ctx, tenantID, playerIDs)
	if err != nil {
		return nil, err
	}
	for _, w := range got {
		if w.TenantID != "" && w.TenantID != tenantID {
			continue
		}
		out[w.PlayerID] = ports.WalletSnapshot{Balance: w.Balance, LifetimeEarned: w.LifetimeEarned}
	}
	return out, nil
}

// LocalBadges wraps badges' Reader.
type LocalBadges struct{ r badgescontracts.Reader }

func NewLocalBadges(r badgescontracts.Reader) *LocalBadges { return &LocalBadges{r: r} }

var _ ports.BadgeReader = (*LocalBadges)(nil)

func (l *LocalBadges) EarnedBadges(ctx context.Context, tenantID string, playerIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(playerIDs))
	if len(playerIDs) == 0 {
		return out, nil
	}
	got, err := l.r.PlayerBadges(ctx, tenantID, playerIDs)
	if err != nil {
		return nil, err
	}
	for _, b := range got {
		if b.EarnedCount <= 0 {
			continue
		}
		out[b.PlayerID] = append(out[b.PlayerID], b.BadgeID)
	}
	return out, nil
}

// LocalActivity wraps activity's Reader. The provider caps one request at
// activitycontracts.MaxLastSeenIDs ids, so larger sets are chunked.
type LocalActivity struct{ r activitycontracts.Reader }

func NewLocalActivity(r activitycontracts.Reader) *LocalActivity { return &LocalActivity{r: r} }

var _ ports.ActivityReader = (*LocalActivity)(nil)

func (l *LocalActivity) LastSeen(ctx context.Context, tenantID string, playerIDs []string) (map[string]time.Time, error) {
	out := make(map[string]time.Time, len(playerIDs))
	for start := 0; start < len(playerIDs); start += activitycontracts.MaxLastSeenIDs {
		end := min(start+activitycontracts.MaxLastSeenIDs, len(playerIDs))
		got, err := l.r.LastSeen(ctx, tenantID, playerIDs[start:end])
		if err != nil {
			return nil, err
		}
		for id, t := range got {
			out[id] = t
		}
	}
	return out, nil
}
