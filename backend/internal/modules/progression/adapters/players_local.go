// Package adapters bridges progression's ports to provider modules.
package adapters

import (
	"context"

	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/progression/internal/ports"
	"levelup/internal/shared/errs"
)

// LocalPlayers serves progression's PlayerReader from the in-process
// player module.
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
	if len(playerIDs) == 0 {
		return map[string]ports.PlayerSnapshot{}, nil
	}
	players, err := l.players.PlayersByIDs(ctx, tenantID, playerIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ports.PlayerSnapshot, len(players))
	for _, p := range players {
		// Defence in depth: a provider bug must not leak another tenant's player.
		if p.TenantID != "" && p.TenantID != tenantID {
			continue
		}
		out[p.ID] = ports.PlayerSnapshot{ID: p.ID, TenantID: tenantID, Active: p.Active}
	}
	return out, nil
}
