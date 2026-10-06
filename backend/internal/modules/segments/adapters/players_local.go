// Package adapters bridges segments' ports to provider modules' contracts.
package adapters

import (
	"context"

	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/segments/internal/ports"
)

// PlayerIDLister enumerates a tenant's player ids in ascending id order.
// playercontracts.Reader has no list method yet: the composition root must
// supply one (e.g. a new player Reader method with this exact signature).
type PlayerIDLister interface {
	ListPlayerIDs(ctx context.Context, tenantID, afterID string, limit int) ([]string, error)
}

// LocalPlayers reads players in-process through player's offered Reader
// plus the id lister.
type LocalPlayers struct {
	players playercontracts.Reader
	lister  PlayerIDLister
}

func NewLocalPlayers(players playercontracts.Reader, lister PlayerIDLister) *LocalPlayers {
	return &LocalPlayers{players: players, lister: lister}
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
		out[p.ID] = ports.PlayerSnapshot{
			ID: p.ID, TenantID: p.TenantID, ExternalID: p.ExternalID, DisplayName: p.DisplayName,
			Active: p.Active, Attributes: p.Attributes, CreatedAt: p.CreatedAt,
		}
	}
	return out, nil
}

func (l *LocalPlayers) ListPlayerIDs(ctx context.Context, tenantID, afterID string, limit int) ([]string, error) {
	return l.lister.ListPlayerIDs(ctx, tenantID, afterID, limit)
}
