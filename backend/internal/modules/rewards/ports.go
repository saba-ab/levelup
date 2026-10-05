package rewards

import "levelup/internal/modules/rewards/internal/ports"

// Port aliases so the composition root can name rewards' dependencies.
type (
	PlayerReader   = ports.PlayerReader
	PlayerSnapshot = ports.PlayerSnapshot
	ProgressReader = ports.ProgressReader
	PointsReader   = ports.PointsReader
	PaymentOutcome = ports.PaymentOutcome
)
