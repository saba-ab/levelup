package leaderboards

import "levelup/internal/modules/leaderboards/internal/ports"

// Aliases so the composition root can name the ports it satisfies.
type (
	PlayerReader   = ports.PlayerReader
	PlayerSnapshot = ports.PlayerSnapshot
)
