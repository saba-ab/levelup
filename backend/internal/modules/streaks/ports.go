package streaks

import "levelup/internal/modules/streaks/internal/ports"

// Aliases so the composition root can name streaks' ports.
type (
	PlayerReader   = ports.PlayerReader
	PlayerSnapshot = ports.PlayerSnapshot
	TenantReader   = ports.TenantReader
	TenantSnapshot = ports.TenantSnapshot
)
