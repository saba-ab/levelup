package app

import (
	"context"
	"time"

	"levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
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

// ListLastSeen is the HTTP read: each requested player's most recent
// activity in the caller's tenant. Ids are deduplicated; players with no
// resolved activity are absent. Results follow the request order.
func (s *Service) ListLastSeen(ctx context.Context, playerIDs []string) ([]domain.LastSeen, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, nil); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(playerIDs))
	seen := make(map[string]bool, len(playerIDs))
	for _, id := range playerIDs {
		if id == "" || seen[id] {
			continue
		}
		if !isUUID(id) {
			return nil, domain.ErrMalformedPlayerID
		}
		seen[id] = true
		ids = append(ids, id)
	}
	switch {
	case len(ids) == 0:
		return nil, domain.ErrPlayerIDsRequired
	case len(ids) > contracts.MaxLastSeenIDs:
		return nil, domain.ErrTooManyPlayerIDs
	}
	rows, err := s.repo.LastSeen(ctx, p.TenantID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.LastSeen, len(rows))
	for _, r := range rows {
		byID[r.PlayerID] = r
	}
	out := make([]domain.LastSeen, 0, len(rows))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// LastSeen implements contracts.Reader: in-process trust, tenant passed
// explicitly. Malformed ids are ignored; unknown players are absent.
func (s *Service) LastSeen(ctx context.Context, tenantID string, playerIDs []string) (map[string]time.Time, error) {
	if !isUUID(tenantID) {
		return nil, errs.New(errs.Invalid, "tenant id required")
	}
	ids := make([]string, 0, len(playerIDs))
	seen := make(map[string]bool, len(playerIDs))
	for _, id := range playerIDs {
		if seen[id] || !isUUID(id) {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	out := make(map[string]time.Time, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	if len(ids) > contracts.MaxLastSeenIDs {
		return nil, domain.ErrTooManyPlayerIDs
	}
	rows, err := s.repo.LastSeen(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.PlayerID] = r.At
	}
	return out, nil
}

var _ contracts.Reader = (*Service)(nil)
