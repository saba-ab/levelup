package adapters

import (
	"context"

	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/rewards/internal/ports"
)

// LocalPoints serves ports.PointsReader from the in-process points module.
type LocalPoints struct {
	points pointscontracts.Reader
}

func NewLocalPoints(points pointscontracts.Reader) *LocalPoints {
	return &LocalPoints{points: points}
}

func (l *LocalPoints) OutcomeByKey(ctx context.Context, tenantID, key string) (ports.PaymentOutcome, bool, error) {
	out, found, err := l.points.OutcomeByKey(ctx, tenantID, key)
	if err != nil || !found {
		return ports.PaymentOutcome{}, false, err
	}
	switch out.Status {
	case "applied":
		return ports.PaymentOutcome{Status: ports.PaymentApplied}, true, nil
	case "rejected":
		return ports.PaymentOutcome{Status: ports.PaymentRejected, Reason: out.Reason}, true, nil
	default:
		// Anything else is not a settled outcome yet.
		return ports.PaymentOutcome{}, false, nil
	}
}
