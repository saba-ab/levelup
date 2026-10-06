// Package adapters bridges notifications' ports to providers' contracts.
package adapters

import (
	"context"

	"levelup/internal/modules/notifications/internal/ports"
	playercontracts "levelup/internal/modules/player/contracts"
)

// LocalPlayers wraps the in-process player module's Reader.
type LocalPlayers struct {
	players playercontracts.Reader
}

// NewLocalPlayers is the registry's local adapter:
// adapters.NewLocalPlayers(playerMod.Reader()).
func NewLocalPlayers(players playercontracts.Reader) ports.PlayerReader {
	return &LocalPlayers{players: players}
}

func (l *LocalPlayers) ByID(ctx context.Context, tenantID, playerID string) (ports.PlayerSnapshot, bool, error) {
	got, err := l.ByIDs(ctx, tenantID, []string{playerID})
	if err != nil {
		return ports.PlayerSnapshot{}, false, err
	}
	snap, ok := got[playerID]
	return snap, ok, nil
}

func (l *LocalPlayers) ByIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]ports.PlayerSnapshot, error) {
	out := make(map[string]ports.PlayerSnapshot, len(playerIDs))
	if len(playerIDs) == 0 {
		return out, nil
	}
	players, err := l.players.PlayersByIDs(ctx, tenantID, playerIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range players {
		// Defence in depth: a provider bug must not leak another tenant's player.
		if p.TenantID != tenantID {
			continue
		}
		out[p.ID] = ports.PlayerSnapshot{
			ID:          p.ID,
			TenantID:    p.TenantID,
			ExternalID:  p.ExternalID,
			DisplayName: p.DisplayName,
			Email:       p.Email,
			Active:      p.Active,
		}
	}
	return out, nil
}
