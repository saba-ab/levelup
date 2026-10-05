package app

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/progression/internal/domain"
)

// reconcileOverlap re-reads a little before the marker: a transaction that
// stamped updated_at before the last run started but committed after it
// read must not slip through the gap.
const reconcileOverlap = 10 * time.Minute

// XPDrift is a progress row whose total disagrees with its ledger.
type XPDrift struct {
	TenantID string
	PlayerID string
	TotalXP  int64
	LedgerXP int64
}

// Reconciler is the sweep's view of storage (implemented by internal/repo).
type Reconciler interface {
	LastRun(ctx context.Context) (time.Time, error)
	MarkRun(ctx context.Context, at time.Time) error
	// XPDrift lists rows touched since `since` whose total_xp != Σ grants.
	XPDrift(ctx context.Context, since time.Time) ([]XPDrift, error)
	// ProgressCandidates pages, keyset by (tenant_id, player_id), through
	// rows touched since `since` plus every row of a tenant whose ladder
	// changed since then.
	ProgressCandidates(ctx context.Context, since time.Time, afterTenant, afterPlayer string, limit int) ([]domain.Progress, error)
}

// ReconcileReport is what one sweep found.
type ReconcileReport struct {
	XPDrift     int
	LevelDrift  int
	Replaced    int
	SweptSince  time.Time
	CompletedAt time.Time
}

// Reconcile (progression.reconcile, hourly) checks total_xp == Σ grants and
// that the stored level matches the ladder. Drift is logged and counted;
// with ReplaceLevels the stored level is re-placed, never with rewards.
// It sweeps from the last successful run (R47), so a skipped tick leaves
// no gap.
func (s *Service) Reconcile(ctx context.Context, rec Reconciler) (ReconcileReport, error) {
	last, err := rec.LastRun(ctx)
	if err != nil {
		return ReconcileReport{}, err
	}
	now := s.clock.Now()
	since := last
	if !since.IsZero() {
		since = since.Add(-reconcileOverlap)
	}
	report := ReconcileReport{SweptSince: since}

	drifts, err := rec.XPDrift(ctx, since)
	if err != nil {
		return ReconcileReport{}, err
	}
	for _, d := range drifts {
		s.log.Warn("progression: total_xp drifts from the xp_grants ledger",
			zap.String("tenant_id", d.TenantID), zap.String("player_id", d.PlayerID),
			zap.Int64("total_xp", d.TotalXP), zap.Int64("ledger_xp", d.LedgerXP))
	}
	report.XPDrift = len(drifts)
	s.opts.Drift("total_xp", len(drifts))

	ladders := map[string]domain.Ladder{}
	afterTenant, afterPlayer := "", ""
	for {
		batch, err := rec.ProgressCandidates(ctx, since, afterTenant, afterPlayer, s.opts.ReconcileBatchSize)
		if err != nil {
			return ReconcileReport{}, err
		}
		for _, p := range batch {
			ladder, ok := ladders[p.TenantID]
			if !ok {
				if ladder, err = s.repo.Ladder(ctx, nil, p.TenantID, false); err != nil {
					return ReconcileReport{}, err
				}
				ladders[p.TenantID] = ladder
			}
			expected := p
			if !expected.Place(ladder) {
				continue
			}
			report.LevelDrift++
			s.log.Warn("progression: stored level does not match the ladder",
				zap.String("tenant_id", p.TenantID), zap.String("player_id", p.PlayerID),
				zap.Int64("total_xp", p.TotalXP), zap.Int("stored_level", p.LevelNumber),
				zap.Int("expected_level", expected.LevelNumber))
			if s.opts.ReplaceLevels {
				replaced, err := s.replace(ctx, p.TenantID, p.PlayerID)
				if err != nil {
					return ReconcileReport{}, err
				}
				if replaced {
					report.Replaced++
				}
			}
		}
		if len(batch) < s.opts.ReconcileBatchSize {
			break
		}
		last := batch[len(batch)-1]
		afterTenant, afterPlayer = last.TenantID, last.PlayerID
	}
	s.opts.Drift("level", report.LevelDrift)

	if err := rec.MarkRun(ctx, now); err != nil {
		return ReconcileReport{}, err
	}
	report.CompletedAt = now
	return report, nil
}

// replace re-derives the level under the row lock. No level_rewards row,
// no event, no command: thresholds moved, the player did not earn anything.
func (s *Service) replace(ctx context.Context, tenantID, playerID string) (bool, error) {
	var replaced bool
	err := s.tx(ctx, func(tx *gorm.DB) error {
		ladder, err := s.repo.Ladder(ctx, tx, tenantID, false)
		if err != nil {
			return err
		}
		p, err := s.repo.ProgressForUpdate(ctx, tx, tenantID, playerID)
		if err != nil {
			return err
		}
		if !p.Place(ladder) {
			return nil
		}
		p.UpdatedAt = s.clock.Now()
		replaced = true
		return s.repo.SaveProgress(ctx, tx, p)
	})
	if errors.Is(err, domain.ErrVersionConflict) {
		// A concurrent grant re-placed it already; the next sweep re-checks.
		return false, nil
	}
	return replaced, err
}
