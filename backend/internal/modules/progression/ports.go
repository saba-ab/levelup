package progression

import "levelup/internal/modules/progression/internal/ports"

// Aliases so the composition root can name progression's ports.
type (
	PlayerReader   = ports.PlayerReader
	PlayerSnapshot = ports.PlayerSnapshot
)
