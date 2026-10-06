package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
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

// MaxBatchPlayers caps GET /progress?player_ids=.
const MaxBatchPlayers = 100

// BatchProgress is GET /progress?player_ids=: one view per known player of
// the caller's tenant, in request order (duplicates collapsed), each shaped
// like GetProgress. Unknown and foreign players are omitted, never an
// error. Three reads whatever the batch size: players, progress, ladder.
func (s *Service) BatchProgress(ctx context.Context, playerIDs []string) ([]domain.View, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, nil); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(playerIDs))
	seen := make(map[string]bool, len(playerIDs))
	for _, pid := range playerIDs {
		if pid != "" && !seen[pid] {
			seen[pid] = true
			ids = append(ids, pid)
		}
	}
	if len(ids) > MaxBatchPlayers {
		return nil, errs.WithFields(errs.WithCode(errs.New(errs.Invalid, "too many player ids"), "too_many_ids"),
			map[string]string{"player_ids": "at most 100 ids"})
	}
	if len(ids) == 0 {
		return []domain.View{}, nil
	}
	known, err := s.players.ByIDs(ctx, p.TenantID, ids)
	if err != nil {
		return nil, err
	}
	present := ids[:0]
	for _, pid := range ids {
		if pl, ok := known[pid]; ok && pl.TenantID == p.TenantID {
			present = append(present, pid)
		}
	}
	if len(present) == 0 {
		return []domain.View{}, nil
	}
	rows, err := s.repo.ProgressByPlayers(ctx, p.TenantID, present)
	if err != nil {
		return nil, err
	}
	ladder, err := s.repo.Ladder(ctx, nil, p.TenantID, false)
	if err != nil {
		return nil, err
	}
	out := make([]domain.View, len(present))
	for i, pid := range present {
		out[i] = domain.ViewOf(pid, rows[pid].TotalXP, ladder)
	}
	return out, nil
}
