package segments

import "levelup/internal/modules/segments/internal/ports"

// Aliases so the composition root can name segments' ports.
type (
	PlayerReader   = ports.PlayerReader
	PlayerSnapshot = ports.PlayerSnapshot
	ProgressReader = ports.ProgressReader
	WalletReader   = ports.WalletReader
	WalletSnapshot = ports.WalletSnapshot
	BadgeReader    = ports.BadgeReader
	ActivityReader = ports.ActivityReader
)
