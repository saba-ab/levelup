package adapters

import (
	"context"

	progressioncontracts "levelup/internal/modules/progression/contracts"
)

// LocalProgress serves ports.ProgressReader from the in-process progression
// module.
type LocalProgress struct {
	progress progressioncontracts.Reader
}

func NewLocalProgress(progress progressioncontracts.Reader) *LocalProgress {
	return &LocalProgress{progress: progress}
}

func (l *LocalProgress) LevelsByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) (map[string]int, error) {
	got, err := l.progress.ProgressByPlayerIDs(ctx, tenantID, playerIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(got))
	for _, p := range got {
		out[p.PlayerID] = p.LevelNumber
	}
	return out, nil
}
