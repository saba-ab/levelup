package adapters

import (
	"context"

	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/rules/internal/ports"
)

// LocalPoints wraps points' contracts.Reader.
type LocalPoints struct{ r pointscontracts.Reader }

func NewLocalPoints(r pointscontracts.Reader) *LocalPoints { return &LocalPoints{r: r} }

func (l *LocalPoints) ByPlayerIDs(ctx context.Context, tenantID string, ids []string) (map[string]ports.BalanceSnapshot, error) {
	ws, err := l.r.WalletsByPlayerIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ports.BalanceSnapshot, len(ws))
	for _, w := range ws {
		out[w.PlayerID] = ports.BalanceSnapshot{PlayerID: w.PlayerID, Balance: w.Balance}
	}
	return out, nil
}
