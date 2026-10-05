package program

import "levelup/internal/modules/program/internal/ports"

// Aliases so the composition root can name program's ports.
type (
	PlayerReader   = ports.PlayerReader
	PlayerSnapshot = ports.PlayerSnapshot
)
