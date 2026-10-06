package app

import (
	"context"

	"levelup/internal/modules/segments/contracts"
)

// Reader implements contracts.Reader over the service.
type Reader struct{ svc *Service }

func NewReader(svc *Service) *Reader { return &Reader{svc: svc} }

var _ contracts.Reader = (*Reader)(nil)

func (r *Reader) SegmentsOfPlayers(ctx context.Context, tenantID string, playerIDs []string) (map[string][]string, error) {
	return r.svc.SegmentsOfPlayers(ctx, tenantID, playerIDs)
}
