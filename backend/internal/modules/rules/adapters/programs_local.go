package adapters

import (
	"context"

	programcontracts "levelup/internal/modules/program/contracts"
)

// LocalPrograms wraps program's contracts.Reader into rules' ProgramReader.
type LocalPrograms struct{ r programcontracts.Reader }

func NewLocalPrograms(r programcontracts.Reader) *LocalPrograms { return &LocalPrograms{r: r} }

func (l *LocalPrograms) EnrolledProgramIDs(ctx context.Context, tenantID, playerID string) ([]string, error) {
	return l.r.EnrolledProgramIDs(ctx, tenantID, playerID)
}
