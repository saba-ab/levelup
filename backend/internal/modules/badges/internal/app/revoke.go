package app

import (
	"context"

	"gorm.io/gorm"

	"levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/shared/id"
)

// Revoke removes a player's whole holding of a badge (all stacks), records
// the revocation and publishes badges.revoked.v1.
//
// Product decision (parity with Laravel, doc 04 Q5): points credited by the
// revoked awards are NOT taken back; revoking a badge the player does not
// hold is a no-op that still answers 204. The applied award rows stay on the
// append-only ledger; they reference the deleted holding's id, so the
// reconcile check (which counts per holding id) stays exact after a later
// re-award.
func (s *Service) Revoke(ctx context.Context, badgeID, playerID string) error {
	p, err := s.requireTenantPerm(ctx, contracts.PermRevoke, nil)
	if err != nil {
		return err
	}
	// Soft-deleted badges may still be revoked from holders.
	badges, err := s.repo.BadgesByIDs(ctx, p.TenantID, []string{badgeID}, true)
	if err != nil {
		return err
	}
	if len(badges) == 0 {
		return domain.ErrBadgeNotFound
	}
	if _, err := s.requirePlayer(ctx, p.TenantID, playerID); err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		pb, found, err := s.repo.LockPlayerBadge(ctx, tx, p.TenantID, playerID, badgeID)
		if err != nil || !found {
			return err
		}
		if err := s.repo.DeletePlayerBadge(ctx, tx, pb); err != nil {
			return err
		}
		now := s.now()
		rev := domain.Revocation{
			ID:                 id.NewID(),
			TenantID:           p.TenantID,
			PlayerID:           playerID,
			BadgeID:            badgeID,
			PlayerBadgeID:      pb.ID,
			EarnedCountRemoved: pb.EarnedCount,
			RevokedBy:          p.UserID,
			RevokedAt:          now,
		}
		if err := s.repo.InsertRevocation(ctx, tx, rev); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicRevoked, contracts.RevokedV1{
			TenantID:           p.TenantID,
			PlayerID:           playerID,
			BadgeID:            badgeID,
			RevokedBy:          p.UserID,
			At:                 now,
			RevocationID:       rev.ID,
			PlayerBadgeID:      pb.ID,
			EarnedCountRemoved: pb.EarnedCount,
		})
	})
}
