package app

import (
	"context"
	"time"

	"levelup/internal/modules/rules/contracts"
	"levelup/internal/modules/rules/internal/domain"
)

// Stats ranges: the default window and the widest one a caller may ask for.
const (
	DefaultStatsRange = 30 * 24 * time.Hour
	MaxStatsRange     = 366 * 24 * time.Hour
)

// RuleStats is per-rule activity over [From, To) plus the totals.
type RuleStats struct {
	From   time.Time
	To     time.Time
	Rules  []RuleStat
	Totals RuleStat
}

// Stats aggregates rule executions and effects over [from, to) in one
// grouped query (portal Analytics "Rules" tab, Overview "Top rules"). A nil
// to is now; a nil from is to minus 30 days. Rules come ordered by fired
// DESC, then rule id.
func (s *Service) Stats(ctx context.Context, from, to *time.Time) (RuleStats, error) {
	p, err := s.authorize(ctx, contracts.PermViewDecisions)
	if err != nil {
		return RuleStats{}, err
	}
	end := s.clock.Now().UTC()
	if to != nil {
		end = to.UTC()
	}
	start := end.Add(-DefaultStatsRange)
	if from != nil {
		start = from.UTC()
	}
	if !start.Before(end) || end.Sub(start) > MaxStatsRange {
		return RuleStats{}, domain.ErrBadStatsRange
	}
	rows, err := s.repo.RuleStats(ctx, p.TenantID, start, end)
	if err != nil {
		return RuleStats{}, err
	}
	out := RuleStats{From: start, To: end, Rules: rows}
	if out.Rules == nil {
		out.Rules = []RuleStat{}
	}
	for _, r := range rows {
		t := &out.Totals
		t.Fired += r.Fired
		t.NotMatched += r.NotMatched
		t.Limited += r.Limited
		t.OutOfSchedule += r.OutOfSchedule
		t.SkippedByStop += r.SkippedByStop
		t.EffectsApplied += r.EffectsApplied
		t.EffectsRejected += r.EffectsRejected
		t.PointsAwarded += r.PointsAwarded
		t.XPAwarded += r.XPAwarded
	}
	return out, nil
}
