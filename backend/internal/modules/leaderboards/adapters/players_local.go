// Package adapters bridges leaderboards' ports to providers' contracts.
package adapters

import (
	"context"

	"levelup/internal/modules/leaderboards/internal/ports"
	playercontracts "levelup/internal/modules/player/contracts"
)

// LocalPlayers serves ports.PlayerReader from the in-process player module.
type LocalPlayers struct {
	players playercontracts.Reader
}

func NewLocalPlayers(players playercontracts.Reader) ports.PlayerReader {
	return &LocalPlayers{players: players}
}

func (l *LocalPlayers) ByIDs(ctx context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := make(map[string]ports.PlayerSnapshot, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	got, err := l.players.PlayersByIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	for _, p := range got {
		if p.TenantID != "" && p.TenantID != tenantID {
			continue
		}
		out[p.ID] = ports.PlayerSnapshot{
			ID:          p.ID,
			ExternalID:  p.ExternalID,
			DisplayName: p.DisplayName,
			Active:      p.Active,
		}
	}
	return out, nil
}
