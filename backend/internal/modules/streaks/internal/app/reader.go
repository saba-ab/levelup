package app

import (
	"context"

	"levelup/internal/modules/streaks/contracts"
)

// Reader is streaks' offered synchronous read surface (contracts.Reader).
type Reader struct {
	repo Repository
}

func NewReader(repo Repository) *Reader { return &Reader{repo: repo} }

var _ contracts.Reader = (*Reader)(nil)

// PlayerStreaks returns the stored state of every streak of the given
// players in one tenant. Unknown players are simply absent.
func (r *Reader) PlayerStreaks(ctx context.Context, tenantID string, playerIDs []string) ([]contracts.PlayerStreakSnapshot, error) {
	if tenantID == "" || len(playerIDs) == 0 {
		return nil, nil
	}
	rows, err := r.repo.PlayerStreaksByPlayers(ctx, tenantID, playerIDs)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.PlayerStreakSnapshot, 0, len(rows))
	for _, ps := range rows {
		snap := contracts.PlayerStreakSnapshot{
			StreakID:     ps.StreakID,
			PlayerID:     ps.PlayerID,
			CurrentCount: ps.CurrentCount,
			LongestCount: ps.LongestCount,
		}
		if ps.LastPeriodStart != nil {
			last := *ps.LastPeriodStart
			snap.LastPeriod = &last
		}
		out = append(out, snap)
	}
	return out, nil
}
