package app

import (
	"context"

	"gorm.io/gorm"

	"levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/badges/internal/domain"
)

// DefaultPageSize and MaxPageSize bound every list (ADR-0016).
const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultPageSize
	case limit > MaxPageSize:
		return MaxPageSize
	default:
		return limit
	}
}

// CreateBadge adds a badge to the caller's tenant. tenant_id is stamped
// from the principal, never from the request.
func (s *Service) CreateBadge(ctx context.Context, params domain.NewBadgeParams) (domain.Badge, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermCreate, nil)
	if err != nil {
		return domain.Badge{}, err
	}
	params.TenantID = p.TenantID
	b, err := domain.NewBadge(params, s.now())
	if err != nil {
		return domain.Badge{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.CreateBadge(ctx, tx, b)
	}); err != nil {
		return domain.Badge{}, err
	}
	return b, nil
}

// GetBadge returns a live badge of the caller's tenant; another tenant's
// badge is 404.
func (s *Service) GetBadge(ctx context.Context, badgeID string) (domain.Badge, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermView, nil)
	if err != nil {
		return domain.Badge{}, err
	}
	return s.repo.BadgeByID(ctx, p.TenantID, badgeID)
}

// ListBadges pages the catalogue newest first. Secret and inactive badges
// are included with their flags (parity, Q9); the caller filters.
func (s *Service) ListBadges(ctx context.Context, f BadgeFilter) ([]domain.Badge, bool, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermViewAny, nil)
	if err != nil {
		return nil, false, err
	}
	limit := clampLimit(f.Limit)
	f.Limit = limit + 1
	rows, err := s.repo.ListBadges(ctx, p.TenantID, f)
	if err != nil {
		return nil, false, err
	}
	if len(rows) > limit {
		return rows[:limit], true, nil
	}
	return rows, false, nil
}

// UpdateBadge applies a partial update under a row lock + version.
func (s *Service) UpdateBadge(ctx context.Context, badgeID string, patch domain.Patch) (domain.Badge, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermUpdate, nil)
	if err != nil {
		return domain.Badge{}, err
	}
	var out domain.Badge
	err = s.tx(ctx, func(tx *gorm.DB) error {
		b, err := s.repo.BadgeByIDForUpdate(ctx, tx, p.TenantID, badgeID)
		if err != nil {
			return err
		}
		if err := b.Apply(patch, s.now()); err != nil {
			return err
		}
		if err := s.repo.SaveBadge(ctx, tx, b); err != nil {
			return err
		}
		b.Version++
		out = b
		return nil
	})
	if err != nil {
		return domain.Badge{}, err
	}
	return out, nil
}

// DeleteBadge soft-deletes. Existing holdings stay (parity); new awards of
// a deleted badge are rejected as target_not_found.
func (s *Service) DeleteBadge(ctx context.Context, badgeID string) error {
	p, err := s.requireTenantPerm(ctx, contracts.PermDelete, nil)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		b, err := s.repo.BadgeByIDForUpdate(ctx, tx, p.TenantID, badgeID)
		if err != nil {
			return err
		}
		now := s.now()
		b.DeletedAt = &now
		b.UpdatedAt = now
		return s.repo.SaveBadge(ctx, tx, b)
	})
}

// PlayerBadgeView is a holding with its badge (nil when the badge row is
// gone, which cannot happen while the FK holds, but readers stay total).
type PlayerBadgeView struct {
	PlayerBadge domain.PlayerBadge
	Badge       *domain.Badge
}

// ListPlayerBadges pages one player's holdings newest first. Soft-deleted
// badges are still shown (the player earned them).
func (s *Service) ListPlayerBadges(ctx context.Context, playerID string, cur PageCursor, limit int) ([]PlayerBadgeView, bool, error) {
	p, err := s.requireTenantPerm(ctx, contracts.PermViewAny, nil)
	if err != nil {
		return nil, false, err
	}
	if _, err := s.requirePlayer(ctx, p.TenantID, playerID); err != nil {
		return nil, false, err
	}
	limit = clampLimit(limit)
	rows, err := s.repo.ListPlayerBadges(ctx, p.TenantID, playerID, cur, limit+1)
	if err != nil {
		return nil, false, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.BadgeID)
	}
	badges, err := s.repo.BadgesByIDs(ctx, p.TenantID, ids, true)
	if err != nil {
		return nil, false, err
	}
	byID := make(map[string]domain.Badge, len(badges))
	for _, b := range badges {
		byID[b.ID] = b
	}
	out := make([]PlayerBadgeView, len(rows))
	for i, r := range rows {
		out[i] = PlayerBadgeView{PlayerBadge: r}
		if b, ok := byID[r.BadgeID]; ok {
			out[i].Badge = &b
		}
	}
	return out, more, nil
}
