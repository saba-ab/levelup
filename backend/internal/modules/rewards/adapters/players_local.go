// Package adapters bridges rewards' ports to other modules' contracts.
package adapters

import (
	"context"

	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/rewards/internal/ports"
)

// LocalPlayers serves ports.PlayerReader from the in-process player module.
type LocalPlayers struct {
	players playercontracts.Reader
}

func NewLocalPlayers(players playercontracts.Reader) *LocalPlayers {
	return &LocalPlayers{players: players}
}

func (l *LocalPlayers) PlayersByIDs(ctx context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	got, err := l.players.PlayersByIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ports.PlayerSnapshot, len(got))
	for _, p := range got {
		if p.TenantID != "" && p.TenantID != tenantID {
			continue
		}
		out[p.ID] = ports.PlayerSnapshot{ID: p.ID, TenantID: tenantID, Active: p.Active}
	}
	return out, nil
}
