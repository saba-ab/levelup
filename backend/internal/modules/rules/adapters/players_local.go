// Package adapters bridges rules' ports to provider modules' contracts
// (local, in-process). The registry chooses the adapter.
package adapters

import (
	"context"

	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/rules/internal/ports"
)

// LocalPlayers wraps player's contracts.Reader.
type LocalPlayers struct{ r playercontracts.Reader }

func NewLocalPlayers(r playercontracts.Reader) *LocalPlayers { return &LocalPlayers{r: r} }

func (l *LocalPlayers) ByIDs(ctx context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	ps, err := l.r.PlayersByIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ports.PlayerSnapshot, len(ps))
	for _, p := range ps {
		out[p.ID] = toPlayer(p)
	}
	return out, nil
}

func (l *LocalPlayers) ByExternalIDs(ctx context.Context, tenantID string, ext []string) (map[string]ports.PlayerSnapshot, error) {
	ps, err := l.r.PlayersByExternalIDs(ctx, tenantID, ext)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ports.PlayerSnapshot, len(ps))
	for _, p := range ps {
		out[p.ExternalID] = toPlayer(p)
	}
	return out, nil
}

func toPlayer(p playercontracts.PlayerSnapshot) ports.PlayerSnapshot {
	return ports.PlayerSnapshot{
		ID: p.ID, TenantID: p.TenantID, ExternalID: p.ExternalID, DisplayName: p.DisplayName,
		Email: p.Email, Active: p.Active, Attributes: p.Attributes, CreatedAt: p.CreatedAt,
	}
}
