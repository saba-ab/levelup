package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/pagination"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

// GetProgress returns a player's XP and level against the live ladder. A
// player who never gained XP gets the zero state; nothing is written
// (Laravel created a row on read, B17).
func (s *Service) GetProgress(ctx context.Context, playerID string) (domain.View, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.View{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, nil); err != nil {
		return domain.View{}, err
	}
	if err := s.requirePlayer(ctx, p.TenantID, playerID); err != nil {
		return domain.View{}, err
	}
	got, err := s.repo.ProgressByPlayers(ctx, p.TenantID, []string{playerID})
	if err != nil {
		return domain.View{}, err
	}
	ladder, err := s.repo.Ladder(ctx, nil, p.TenantID, false)
	if err != nil {
		return domain.View{}, err
	}
	return domain.ViewOf(playerID, got[playerID].TotalXP, ladder), nil
}

// GrantPage is one page of a player's XP ledger, newest first.
type GrantPage struct {
	Grants     []domain.XPGrant
	NextCursor string
}

func (s *Service) ListGrants(ctx context.Context, playerID, cursor string, limit int) (GrantPage, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return GrantPage{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, nil); err != nil {
		return GrantPage{}, err
	}
	if err := s.requirePlayer(ctx, p.TenantID, playerID); err != nil {
		return GrantPage{}, err
	}
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	var before time.Time
	var beforeID string
	if cursor != "" {
		if before, beforeID, err = pagination.DecodeCursor(cursor); err != nil {
			return GrantPage{}, err
		}
	}
	// One extra row tells whether another page exists.
	rows, err := s.repo.ListGrants(ctx, p.TenantID, playerID, before, beforeID, limit+1)
	if err != nil {
		return GrantPage{}, err
	}
	page := GrantPage{Grants: rows}
	if len(rows) > limit {
		page.Grants = rows[:limit]
		last := page.Grants[limit-1]
		page.NextCursor = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

// ProgressByPlayerIDs implements contracts.Reader. Every requested player
// gets a snapshot; one without a progress row is at 0 XP, placed on the
// live ladder like any other.
func (s *Service) ProgressByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) ([]contracts.ProgressSnapshot, error) {
	if len(playerIDs) == 0 {
		return nil, nil
	}
	rows, err := s.repo.ProgressByPlayers(ctx, tenantID, playerIDs)
	if err != nil {
		return nil, err
	}
	ladder, err := s.repo.Ladder(ctx, nil, tenantID, false)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.ProgressSnapshot, 0, len(playerIDs))
	seen := make(map[string]bool, len(playerIDs))
	for _, pid := range playerIDs {
		if seen[pid] {
			continue
		}
		seen[pid] = true
		total := rows[pid].TotalXP
		snap := contracts.ProgressSnapshot{PlayerID: pid, TotalXP: total}
		if l := ladder.LevelFor(total); l != nil {
			snap.LevelID, snap.LevelNumber = l.ID, l.Number
		}
		out = append(out, snap)
	}
	return out, nil
}

// PurgeTenant handles tenant.deleted.v1: every row of the tenant goes.
// Deleting nothing is success, so redelivery is harmless.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, tenantID)
	})
}
