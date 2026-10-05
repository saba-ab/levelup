package activity

import "levelup/internal/modules/activity/internal/ports"

// Aliases so the composition root can name activity's ports and pass the
// adapters it chooses.
type (
	PlayerReader      = ports.PlayerReader
	PlayerSnapshot    = ports.PlayerSnapshot
	EventTypeReader   = ports.EventTypeReader
	EventTypeSnapshot = ports.EventTypeSnapshot
)
