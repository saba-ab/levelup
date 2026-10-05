// Package adapters bridges activity's ports to providers' contracts.
package adapters

import (
	"context"

	"levelup/internal/modules/activity/internal/ports"
	playercontracts "levelup/internal/modules/player/contracts"
)

// LocalPlayers serves activity's PlayerReader from the in-process player
// module.
type LocalPlayers struct {
	players playercontracts.Reader
}

func NewLocalPlayers(players playercontracts.Reader) *LocalPlayers {
	return &LocalPlayers{players: players}
}

func (l *LocalPlayers) ByExternalIDs(ctx context.Context, tenantID string, externalIDs []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	if len(externalIDs) == 0 {
		return out, nil
	}
	got, err := l.players.PlayersByExternalIDs(ctx, tenantID, externalIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range got {
		out[p.ExternalID] = toPlayerSnapshot(p)
	}
	return out, nil
}

func (l *LocalPlayers) ByIDs(ctx context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	if len(ids) == 0 {
		return out, nil
	}
	got, err := l.players.PlayersByIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	for _, p := range got {
		out[p.ID] = toPlayerSnapshot(p)
	}
	return out, nil
}

func toPlayerSnapshot(p playercontracts.PlayerSnapshot) ports.PlayerSnapshot {
	return ports.PlayerSnapshot{ID: p.ID, ExternalID: p.ExternalID, Active: p.Active}
}
