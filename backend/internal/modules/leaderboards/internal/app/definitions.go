package app

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/pagination"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

// CreateInput is the shape the transport hands over; the tenant is taken
// from the principal (fixes L2: Laravel never set tenant_id on create).
type CreateInput struct {
	Name           string
	Slug           string
	Description    string
	Type           string
	Metric         string
	ResetFrequency string
	ProgramID      string
	MaxEntries     int
	Active         bool
}

func (s *Service) Create(ctx context.Context, in CreateInput) (domain.Leaderboard, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Leaderboard{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermCreate, nil); err != nil {
		return domain.Leaderboard{}, err
	}
	lb, err := domain.NewLeaderboard(domain.NewLeaderboardInput{
		TenantID:       p.TenantID,
		Name:           in.Name,
		Slug:           in.Slug,
		Description:    in.Description,
		Type:           in.Type,
		Metric:         in.Metric,
		ResetFrequency: in.ResetFrequency,
		ProgramID:      in.ProgramID,
		MaxEntries:     in.MaxEntries,
		Active:         in.Active,
	}, s.clock.Now())
	if err != nil {
		return domain.Leaderboard{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.Create(ctx, tx, lb)
	}); err != nil {
		return domain.Leaderboard{}, err
	}
	return lb, nil
}

// load fetches a board of the principal's tenant and authorizes perm on it.
// Another tenant's board is NotFound.
func (s *Service) load(ctx context.Context, perm authz.Permission, leaderboardID string) (authz.Principal, domain.Leaderboard, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, domain.Leaderboard{}, err
	}
	lb, err := s.repo.ByID(ctx, p.TenantID, leaderboardID)
	if err != nil {
		return authz.Principal{}, domain.Leaderboard{}, err
	}
	if err := s.authz.Authorize(ctx, p, perm, lb); err != nil {
		return authz.Principal{}, domain.Leaderboard{}, err
	}
	return p, lb, nil
}

func (s *Service) Get(ctx context.Context, leaderboardID string) (domain.Leaderboard, error) {
	_, lb, err := s.load(ctx, contracts.PermView, leaderboardID)
	return lb, err
}

// List pages a tenant's boards newest first by (created_at, id). Fixes L4:
// the Laravel index had no authorization at all.
func (s *Service) List(ctx context.Context, f ListFilter, cursor string, limit int) ([]domain.Leaderboard, string, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, nil); err != nil {
		return nil, "", err
	}
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	var beforeAt time.Time
	var beforeID string
	if cursor != "" {
		beforeAt, beforeID, err = pagination.DecodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
	}
	rows, err := s.repo.List(ctx, p.TenantID, f, beforeAt, beforeID, limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

func (s *Service) Update(ctx context.Context, leaderboardID string, patch domain.Patch) (domain.Leaderboard, error) {
	_, lb, err := s.load(ctx, contracts.PermUpdate, leaderboardID)
	if err != nil {
		return domain.Leaderboard{}, err
	}
	if err := lb.Apply(patch, s.clock.Now()); err != nil {
		return domain.Leaderboard{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.Save(ctx, tx, lb)
	}); err != nil {
		return domain.Leaderboard{}, err
	}
	lb.Version++
	return lb, nil
}

// Delete soft-deletes the board and drops its read model after commit.
func (s *Service) Delete(ctx context.Context, leaderboardID string) error {
	_, lb, err := s.load(ctx, contracts.PermDelete, leaderboardID)
	if err != nil {
		return err
	}
	var periods []domain.Period
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.SoftDelete(ctx, tx, lb, s.clock.Now())
	}); err != nil {
		return err
	}
	periods, err = s.repo.Periods(ctx, lb.TenantID, lb.ID, false)
	if err != nil {
		s.log.Warn("list periods for read-model cleanup", zap.String("leaderboard_id", lb.ID), zap.Error(err))
		return nil
	}
	periods = append(periods, domain.PeriodOf(lb, s.clock.Now()))
	if err := s.ranks.Delete(ctx, periods...); err != nil {
		s.log.Warn("drop leaderboard read model", zap.String("leaderboard_id", lb.ID), zap.Error(err))
	}
	return nil
}
