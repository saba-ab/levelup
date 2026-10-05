package app

import (
	"context"

	"gorm.io/gorm"

	"levelup/internal/modules/eventcatalog/contracts"
	"levelup/internal/modules/eventcatalog/internal/domain"
	"levelup/internal/platform/authz"
)

type CreateCategoryCmd struct {
	Slug        string
	Name        string
	Description string
	SortOrder   int
}

// ListCategories returns the caller tenant's categories plus the global
// ones, ordered by sort_order then name (a bounded catalogue, not paged).
func (s *Service) ListCategories(ctx context.Context) ([]domain.Category, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, nil); err != nil {
		return nil, err
	}
	return s.repo.ListCategories(ctx, Scope{TenantID: p.TenantID}, MaxCategories)
}

// --- platform surface (/platform/event-categories) ----------------------

func (s *Service) PlatformListCategories(ctx context.Context) ([]domain.Category, error) {
	if err := s.requirePlatform(ctx); err != nil {
		return nil, err
	}
	return s.repo.ListCategories(ctx, Scope{}, MaxCategories)
}

func (s *Service) PlatformCreateCategory(ctx context.Context, cmd CreateCategoryCmd) (domain.Category, error) {
	if err := s.requirePlatform(ctx); err != nil {
		return domain.Category{}, err
	}
	c, err := domain.CreateCategory(domain.NewCategory{
		Slug:        cmd.Slug,
		Name:        cmd.Name,
		Description: cmd.Description,
		SortOrder:   cmd.SortOrder,
	}, s.clock.Now())
	if err != nil {
		return domain.Category{}, err
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.CreateCategory(ctx, tx, c)
	})
	if err != nil {
		return domain.Category{}, err
	}
	return c, nil
}

func (s *Service) PlatformUpdateCategory(ctx context.Context, id string, patch domain.CategoryPatch) (domain.Category, error) {
	if err := s.requirePlatform(ctx); err != nil {
		return domain.Category{}, err
	}
	c, err := s.repo.CategoryByID(ctx, Scope{}, id)
	if err != nil {
		return domain.Category{}, err
	}
	if err := c.Apply(patch, s.clock.Now()); err != nil {
		return domain.Category{}, err
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.SaveCategory(ctx, tx, c)
	})
	if err != nil {
		return domain.Category{}, err
	}
	return c, nil
}

// PlatformDeleteCategory hard-deletes a global category; event types that
// used it become uncategorised (FK ON DELETE SET NULL).
func (s *Service) PlatformDeleteCategory(ctx context.Context, id string) error {
	if err := s.requirePlatform(ctx); err != nil {
		return err
	}
	if _, err := s.repo.CategoryByID(ctx, Scope{}, id); err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.DeleteCategory(ctx, tx, id)
	})
}
