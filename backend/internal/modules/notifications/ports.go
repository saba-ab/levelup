package notifications

import "levelup/internal/modules/notifications/internal/ports"

// Aliases so the registry (and mockery) can name notifications' consumer ports.
type (
	PlayerReader   = ports.PlayerReader
	PlayerSnapshot = ports.PlayerSnapshot
)
