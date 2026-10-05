package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/streaks/contracts"
	"levelup/internal/modules/streaks/internal/domain"
	"levelup/internal/shared/errs"
)

// BreakSweep breaks every live run whose newest bucket is more than
// 1+grace periods behind the current period in the tenant's timezone, and
// publishes streaks.broken.v1 once per run (fixes S4: Laravel only noticed a
// break on the player's next activity).
//
// Reconciling (R47): it selects by STATE (current_count > 0 and
// last_period_start before the threshold), so a skipped or failed tick is
// healed by the next one; the marker records the last successful run.
func (s *Service) BreakSweep(ctx context.Context) (int, error) {
	if _, err := s.repo.LastRun(ctx, contracts.JobBreakSweep); err != nil {
		return 0, err
	}
	now := s.clock.Now()

	refs, err := s.repo.LiveStreakRefs(ctx)
	if err != nil {
		return 0, err
	}
	byTenant := map[string][]string{}
	for _, r := range refs {
		byTenant[r.TenantID] = append(byTenant[r.TenantID], r.StreakID)
	}
	tenantIDs := make([]string, 0, len(byTenant))
	for t := range byTenant {
		tenantIDs = append(tenantIDs, t)
	}
	locs, err := s.locations(ctx, tenantIDs)
	if err != nil {
		return 0, err
	}

	broken := 0
	for tenantID, streakIDs := range byTenant {
		defs, err := s.repo.StreaksByIDs(ctx, tenantID, streakIDs)
		if err != nil {
			return broken, err
		}
		for _, st := range defs {
			n, err := s.sweepStreak(ctx, st, locs[tenantID], now)
			broken += n
			if err != nil {
				return broken, err
			}
		}
	}
	if err := s.repo.MarkRun(ctx, contracts.JobBreakSweep, now); err != nil {
		return broken, err
	}
	return broken, nil
}

// LapseThreshold is the oldest bucket start that still keeps a run alive at
// now: a run whose newest bucket is before it has lapsed.
func LapseThreshold(st domain.Streak, loc *time.Location, now time.Time) time.Time {
	return st.Period.Shift(st.Period.Start(now, loc), -(1 + st.GracePeriods))
}

func (s *Service) sweepStreak(ctx context.Context, st domain.Streak, loc *time.Location, now time.Time) (int, error) {
	threshold := LapseThreshold(st, loc, now)
	total := 0
	for {
		n := 0
		err := s.tx(ctx, func(tx *gorm.DB) error {
			rows, err := s.repo.LapsedForUpdate(ctx, tx, st.TenantID, st.ID, threshold, sweepBatchSize)
			if err != nil {
				return err
			}
			n = len(rows)
			for _, ps := range rows {
				prev, announce := ps.Break(now)
				if err := s.repo.SavePlayerStreak(ctx, tx, ps); err != nil {
					return err
				}
				if !announce {
					continue
				}
				if err := s.publishBroken(ctx, tx, ps, st, prev, contracts.BrokenReasonLapsed, now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < sweepBatchSize {
			return total, nil
		}
	}
}

// PruneRequests deletes idempotency rows older than retention. Reconciling:
// it deletes by age, so a missed day is caught up by the next run.
func (s *Service) PruneRequests(ctx context.Context, retention time.Duration) (int64, error) {
	if retention <= 0 {
		return 0, errs.New(errs.Invalid, "request retention must be positive")
	}
	now := s.clock.Now()
	n, err := s.repo.PruneRequests(ctx, now.Add(-retention))
	if err != nil {
		return 0, err
	}
	return n, s.repo.MarkRun(ctx, contracts.JobPruneRequests, now)
}

// PurgeTenant handles tenant.deleted.v1: every row of the tenant goes.
// Idempotent: deleting nothing is success.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return errs.New(errs.Invalid, "tenant.deleted.v1 without tenant_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, tenantID)
	})
}

// DeletePlayer handles player.deleted.v1: the player's streak state goes.
func (s *Service) DeletePlayer(ctx context.Context, tenantID, playerID string) error {
	if tenantID == "" || playerID == "" {
		return errs.New(errs.Invalid, "player.deleted.v1 without tenant_id or player_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.DeletePlayer(ctx, tx, tenantID, playerID)
	})
}
