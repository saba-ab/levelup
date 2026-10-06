// Package adapters bridges missions' ports to provider modules' contracts.
package adapters

import (
	"context"

	"levelup/internal/modules/missions/internal/ports"
	playercontracts "levelup/internal/modules/player/contracts"
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
		out[p.ID] = snapshot(p)
	}
	return out, nil
}

func (l *LocalPlayers) PlayersByExternalIDs(ctx context.Context, tenantID string, externalIDs []string) (map[string]ports.PlayerSnapshot, error) {
	out := make(map[string]ports.PlayerSnapshot, len(externalIDs))
	if len(externalIDs) == 0 {
		return out, nil
	}
	got, err := l.players.PlayersByExternalIDs(ctx, tenantID, externalIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range got {
		if p.TenantID != tenantID {
			continue
		}
		out[p.ExternalID] = snapshot(p)
	}
	return out, nil
}

func snapshot(p playercontracts.PlayerSnapshot) ports.PlayerSnapshot {
	return ports.PlayerSnapshot{ID: p.ID, TenantID: p.TenantID, ExternalID: p.ExternalID, Active: p.Active}
}
