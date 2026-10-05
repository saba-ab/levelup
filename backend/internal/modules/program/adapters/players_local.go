// Package adapters bridges program's ports to provider modules' contracts.
package adapters

import (
	"context"

	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/program/internal/ports"
	"levelup/internal/shared/errs"
)

// LocalPlayers wraps the in-process player module's Reader.
type LocalPlayers struct {
	players playercontracts.Reader
}

func NewLocalPlayers(players playercontracts.Reader) *LocalPlayers {
	return &LocalPlayers{players: players}
}

var _ ports.PlayerReader = (*LocalPlayers)(nil)

func (l *LocalPlayers) ByID(ctx context.Context, tenantID, playerID string) (ports.PlayerSnapshot, error) {
	got, err := l.ByIDs(ctx, tenantID, []string{playerID})
	if err != nil {
		return ports.PlayerSnapshot{}, err
	}
	snap, ok := got[playerID]
	if !ok {
		return ports.PlayerSnapshot{}, errs.New(errs.NotFound, "player not found")
	}
	return snap, nil
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
		// Defence in depth: never let a foreign tenant's row through.
		if p.TenantID != "" && p.TenantID != tenantID {
			continue
		}
		out[p.ID] = toSnapshot(p)
	}
	return out, nil
}

func toSnapshot(p playercontracts.PlayerSnapshot) ports.PlayerSnapshot {
	return ports.PlayerSnapshot{
		ID:          p.ID,
		TenantID:    p.TenantID,
		ExternalID:  p.ExternalID,
		DisplayName: p.DisplayName,
		Active:      p.Active,
	}
}
