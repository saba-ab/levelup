package points

import "levelup/internal/modules/points/internal/ports"

// Aliases so the composition root can name points' ports.
type (
	PlayerReader   = ports.PlayerReader
	PlayerSnapshot = ports.PlayerSnapshot
)
