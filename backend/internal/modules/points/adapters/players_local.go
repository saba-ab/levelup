// Package adapters bridges points' ports to provider modules' contracts.
package adapters

import (
	"context"

	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/points/internal/ports"
)

// LocalPlayers reads players in-process through player's offered Reader.
type LocalPlayers struct {
	players playercontracts.Reader
}

func NewLocalPlayers(players playercontracts.Reader) *LocalPlayers {
	return &LocalPlayers{players: players}
}

var _ ports.PlayerReader = (*LocalPlayers)(nil)

func (l *LocalPlayers) PlayersByIDs(ctx context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := make(map[string]ports.PlayerSnapshot, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	got, err := l.players.PlayersByIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	for _, p := range got {
		// Belt to the provider's own scoping: never accept another tenant's player.
		if p.TenantID != tenantID {
			continue
		}
		out[p.ID] = ports.PlayerSnapshot{ID: p.ID, TenantID: p.TenantID, Active: p.Active}
	}
	return out, nil
}
