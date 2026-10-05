package app

import (
	"context"
	"time"

	"levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/pagination"
)

const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

// Page is one keyset page; NextCursor is "" on the last page.
type Page struct {
	Items      []domain.Activity
	NextCursor string
}

// Get returns one of the caller's tenant's activities, including its
// decision projection. Another tenant's id is a 404.
func (s *Service) Get(ctx context.Context, activityID string) (domain.Activity, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Activity{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, nil); err != nil {
		return domain.Activity{}, err
	}
	if !isUUID(activityID) {
		return domain.Activity{}, domain.ErrNotFound
	}
	return s.repo.ByID(ctx, p.TenantID, activityID)
}

// List pages the tenant's activities newest-first by (created_at, id).
func (s *Service) List(ctx context.Context, f ListFilter, cursor string, limit int) (Page, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return Page{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, nil); err != nil {
		return Page{}, err
	}
	if f.Status != "" && !domain.ValidStatus(f.Status) {
		return Page{}, domain.ErrBadStatus
	}
	switch {
	case limit <= 0:
		limit = DefaultPageSize
	case limit > MaxPageSize:
		limit = MaxPageSize
	}
	var before time.Time
	var beforeID string
	if cursor != "" {
		before, beforeID, err = pagination.DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
	}
	rows, err := s.repo.List(ctx, p.TenantID, f, before, beforeID, limit+1)
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: rows}
	if len(rows) > limit {
		page.Items = rows[:limit]
		last := page.Items[limit-1]
		page.NextCursor = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}
