package adapters

import (
	"context"

	progressioncontracts "levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/rules/internal/ports"
)

// LocalProgress wraps progression's contracts.Reader.
type LocalProgress struct{ r progressioncontracts.Reader }

func NewLocalProgress(r progressioncontracts.Reader) *LocalProgress { return &LocalProgress{r: r} }

func (l *LocalProgress) ByPlayerIDs(ctx context.Context, tenantID string, ids []string) (map[string]ports.ProgressSnapshot, error) {
	ps, err := l.r.ProgressByPlayerIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ports.ProgressSnapshot, len(ps))
	for _, p := range ps {
		out[p.PlayerID] = ports.ProgressSnapshot{PlayerID: p.PlayerID, TotalXP: p.TotalXP, Level: p.LevelNumber}
	}
	return out, nil
}
