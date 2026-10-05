package badges

import "levelup/internal/modules/badges/internal/ports"

// Aliases so the registry (and mockery) can name badges' consumer ports.
type (
	PlayerReader   = ports.PlayerReader
	PlayerSnapshot = ports.PlayerSnapshot
)
