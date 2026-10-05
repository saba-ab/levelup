package missions

import "levelup/internal/modules/missions/internal/ports"

// Aliases so the registry can name missions' ports without reaching into
// internal/.
type (
	PlayerReader   = ports.PlayerReader
	PlayerSnapshot = ports.PlayerSnapshot
)
