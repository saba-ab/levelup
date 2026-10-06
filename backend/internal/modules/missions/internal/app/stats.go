package app

import (
	"context"

	"levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/missions/internal/domain"
	"levelup/internal/platform/authz"
)

// MissionStats are one mission's completion analytics. CompletionRate is
// Completed / Started (0 when nothing started); AvgHoursToComplete averages
// completed_at - started_at over completed attempts (nil when none).
type MissionStats struct {
	MissionID          string
	Name               string
	Slug               string
	Status             string
	Started            int64
	InProgress         int64
	Completed          int64
	CompletionRate     float64
	AvgHoursToComplete *float64
}

// GetMissionStats is GET /missions/{id}/stats.
func (s *Service) GetMissionStats(ctx context.Context, missionID string) (MissionStats, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return MissionStats{}, err
	}
	m, err := s.repo.MissionByID(ctx, p.TenantID, missionID)
	if err != nil {
		return MissionStats{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, m); err != nil {
		return MissionStats{}, err
	}
	agg, err := s.repo.AttemptStats(ctx, p.TenantID, []string{m.ID})
	if err != nil {
		return MissionStats{}, err
	}
	return statsOf(m, agg[m.ID]), nil
}

// ListMissionStats is GET /missions/stats: one page of missions (newest
// first, same filters as the mission list) with their stats, aggregated by
// one grouped query per page.
func (s *Service) ListMissionStats(ctx context.Context, f MissionFilter, cursor string, limit int) (Page[MissionStats], error) {
	page, err := s.ListMissions(ctx, f, cursor, limit)
	if err != nil {
		return Page[MissionStats]{}, err
	}
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return Page[MissionStats]{}, err
	}
	ids := make([]string, len(page.Items))
	for i, m := range page.Items {
		ids[i] = m.ID
	}
	agg, err := s.repo.AttemptStats(ctx, p.TenantID, ids)
	if err != nil {
		return Page[MissionStats]{}, err
	}
	out := Page[MissionStats]{NextCursor: page.NextCursor, Items: make([]MissionStats, len(page.Items))}
	for i, m := range page.Items {
		out.Items[i] = statsOf(m, agg[m.ID])
	}
	return out, nil
}

func statsOf(m domain.Mission, a AttemptStats) MissionStats {
	out := MissionStats{
		MissionID:          m.ID,
		Name:               m.Name,
		Slug:               m.Slug,
		Status:             m.Status,
		Started:            a.Started,
		InProgress:         a.InProgress,
		Completed:          a.Completed,
		AvgHoursToComplete: a.AvgHoursToComplete,
	}
	if a.Started > 0 {
		out.CompletionRate = float64(a.Completed) / float64(a.Started)
	}
	return out
}
