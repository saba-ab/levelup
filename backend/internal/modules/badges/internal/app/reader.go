package app

import (
	"context"

	"levelup/internal/modules/badges/contracts"
)

// Service implements badges' offered synchronous read surface. In-process
// callers are trusted modules: the tenant is an explicit argument, no
// principal is involved.
var _ contracts.Reader = (*Service)(nil)

// BadgesByIDs returns live (not deleted) badges of the tenant; unknown ids
// are absent.
func (s *Service) BadgesByIDs(ctx context.Context, tenantID string, ids []string) ([]contracts.BadgeSnapshot, error) {
	if len(ids) == 0 {
		return []contracts.BadgeSnapshot{}, nil
	}
	rows, err := s.repo.BadgesByIDs(ctx, tenantID, ids, false)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.BadgeSnapshot, len(rows))
	for i, b := range rows {
		out[i] = contracts.BadgeSnapshot{
			ID:          b.ID,
			TenantID:    b.TenantID,
			Slug:        b.Slug,
			Name:        b.Name,
			Tier:        string(b.Tier),
			PointsValue: b.PointsValue,
			Active:      b.Active,
		}
	}
	return out, nil
}

// PlayerBadges returns the holdings of the given players.
func (s *Service) PlayerBadges(ctx context.Context, tenantID string, playerIDs []string) ([]contracts.PlayerBadgeSnapshot, error) {
	if len(playerIDs) == 0 {
		return []contracts.PlayerBadgeSnapshot{}, nil
	}
	rows, err := s.repo.PlayerBadgesByPlayerIDs(ctx, tenantID, playerIDs)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.PlayerBadgeSnapshot, 0, len(rows))
	for _, pb := range rows {
		if pb.EarnedCount < 1 {
			continue
		}
		out = append(out, contracts.PlayerBadgeSnapshot{
			BadgeID:     pb.BadgeID,
			PlayerID:    pb.PlayerID,
			EarnedCount: pb.EarnedCount,
			FirstAt:     pb.FirstAwardedAt,
		})
	}
	return out, nil
}
