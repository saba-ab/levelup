package rules

import "levelup/internal/modules/rules/internal/ports"

// Aliases so the registry can name rules' ports.
type (
	PlayerReader     = ports.PlayerReader
	PlayerSnapshot   = ports.PlayerSnapshot
	ProgressReader   = ports.ProgressReader
	ProgressSnapshot = ports.ProgressSnapshot
	PointsReader     = ports.PointsReader
	BalanceSnapshot  = ports.BalanceSnapshot
	ProgramReader    = ports.ProgramReader
)
