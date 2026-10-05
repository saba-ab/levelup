package app

import (
	"context"

	"gorm.io/gorm"

	"levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/platform/authz"
)

// ListLevels returns the tenant's ladder ordered by level_number.
func (s *Service) ListLevels(ctx context.Context, activeOnly bool) ([]domain.Level, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, nil); err != nil {
		return nil, err
	}
	return s.repo.Ladder(ctx, nil, p.TenantID, !activeOnly)
}

func (s *Service) GetLevel(ctx context.Context, levelID string) (domain.Level, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Level{}, err
	}
	l, err := s.repo.LevelByID(ctx, p.TenantID, levelID)
	if err != nil {
		return domain.Level{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, l); err != nil {
		return domain.Level{}, err
	}
	return l, nil
}

// CreateLevel adds a rung; level_number must be unused and xp_required must
// keep the ladder strictly increasing.
func (s *Service) CreateLevel(ctx context.Context, spec domain.LevelSpec) (domain.Level, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Level{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermCreate, nil); err != nil {
		return domain.Level{}, err
	}
	l, err := domain.NewLevel(p.TenantID, spec, s.clock.Now())
	if err != nil {
		return domain.Level{}, err
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.LockLadder(ctx, tx, p.TenantID); err != nil {
			return err
		}
		ladder, err := s.repo.Ladder(ctx, tx, p.TenantID, true)
		if err != nil {
			return err
		}
		if err := ladder.CheckPlacement(l); err != nil {
			return err
		}
		return s.repo.CreateLevel(ctx, tx, l)
	})
	if err != nil {
		return domain.Level{}, err
	}
	return l, nil
}

// UpdateLevel applies a partial update. Existing players are not re-placed
// here: the next grant or the reconcile sweep does that, without rewards.
func (s *Service) UpdateLevel(ctx context.Context, levelID string, patch domain.LevelPatch) (domain.Level, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Level{}, err
	}
	var out domain.Level
	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.LockLadder(ctx, tx, p.TenantID); err != nil {
			return err
		}
		ladder, err := s.repo.Ladder(ctx, tx, p.TenantID, true)
		if err != nil {
			return err
		}
		l, ok := findLevel(ladder, levelID)
		if !ok {
			return domain.ErrLevelNotFound
		}
		if err := s.authz.Authorize(ctx, p, contracts.PermUpdate, l); err != nil {
			return err
		}
		if err := l.Apply(patch, s.clock.Now()); err != nil {
			return err
		}
		if err := ladder.CheckPlacement(l); err != nil {
			return err
		}
		if err := s.repo.UpdateLevel(ctx, tx, l); err != nil {
			return err
		}
		out = l
		return nil
	})
	if err != nil {
		return domain.Level{}, err
	}
	return out, nil
}

// DeleteLevel soft-deletes a rung. Its number becomes reusable (partial
// unique index, fixes B19).
func (s *Service) DeleteLevel(ctx context.Context, levelID string) error {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return err
	}
	l, err := s.repo.LevelByID(ctx, p.TenantID, levelID)
	if err != nil {
		return err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermDelete, l); err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.LockLadder(ctx, tx, p.TenantID); err != nil {
			return err
		}
		return s.repo.SoftDeleteLevel(ctx, tx, p.TenantID, levelID, s.clock.Now())
	})
}

func findLevel(ladder domain.Ladder, levelID string) (domain.Level, bool) {
	for _, l := range ladder {
		if l.ID == levelID {
			return l, true
		}
	}
	return domain.Level{}, false
}
