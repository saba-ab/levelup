package app

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/segments/contracts"
	"levelup/internal/modules/segments/internal/domain"
	"levelup/internal/modules/segments/internal/ports"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

const (
	sweepBatch   = 500
	refreshBatch = 100
)

// RefreshOutcome reports one refresh run.
type RefreshOutcome struct {
	Ran     bool // false: segment gone or held by another run
	Scanned int
	Added   int
	Removed int
}

// RefreshSegment recomputes one segment's membership: it pages through
// every player of the tenant, evaluates the conditions, upserts matches
// and removes non-matches, then sweeps members the scan no longer lists.
// Each page commits with its membership_changed.v1 events. A lease keeps
// two runs off one segment; a request while a run holds it is coalesced.
// When the definition changed during the run, it runs again (bounded).
func (s *Service) RefreshSegment(ctx context.Context, tenantID, segmentID string) (RefreshOutcome, error) {
	if tenantID == "" || segmentID == "" {
		return RefreshOutcome{}, errs.New(errs.Invalid, "refresh without tenant_id or segment_id")
	}
	var total RefreshOutcome
	for range s.set.MaxRefreshRun {
		out, version, err := s.refreshOnce(ctx, tenantID, segmentID)
		total.Ran = total.Ran || out.Ran
		total.Scanned += out.Scanned
		total.Added += out.Added
		total.Removed += out.Removed
		if err != nil || !out.Ran {
			return total, err
		}
		cur, err := s.repo.SegmentByID(ctx, tenantID, segmentID)
		if errors.Is(err, domain.ErrSegmentNotFound) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
		if cur.Version == version {
			return total, nil
		}
	}
	return total, nil
}

func (s *Service) refreshOnce(ctx context.Context, tenantID, segmentID string) (RefreshOutcome, int, error) {
	runID := id.NewID()
	now := s.clock.Now()
	seg, ok, err := s.repo.AcquireRefresh(ctx, tenantID, segmentID, runID, now, now.Add(s.set.Lease))
	if errors.Is(err, domain.ErrSegmentNotFound) {
		return RefreshOutcome{}, 0, nil
	}
	if err != nil || !ok {
		return RefreshOutcome{}, 0, err
	}
	out := RefreshOutcome{Ran: true}
	after := ""
	for {
		ids, err := s.rd.Players.ListPlayerIDs(ctx, tenantID, after, s.set.PageSize)
		if err != nil {
			return out, seg.Version, err
		}
		if len(ids) == 0 {
			break
		}
		_, matched, err := s.evaluate(ctx, tenantID, seg.Conditions, ids)
		if err != nil {
			return out, seg.Version, err
		}
		held := true
		at := s.clock.Now()
		err = s.tx(ctx, func(tx *gorm.DB) error {
			var leaseErr error
			if held, leaseErr = s.repo.ExtendLease(ctx, tx, seg.ID, runID, at.Add(s.set.Lease)); leaseErr != nil || !held {
				return leaseErr
			}
			res, err := s.repo.ApplyPage(ctx, tx, tenantID, seg.ID, runID, ids, matched, at)
			if err != nil {
				return err
			}
			out.Added += len(res.Added)
			out.Removed += len(res.Removed)
			return s.publishChanges(ctx, tx, seg, runID, res, at)
		})
		if err != nil {
			return out, seg.Version, err
		}
		if !held {
			return out, seg.Version, nil // another run took over after our lease expired
		}
		out.Scanned += len(ids)
		if len(ids) < s.set.PageSize {
			break
		}
		after = ids[len(ids)-1]
	}
	for {
		n := 0
		at := s.clock.Now()
		err := s.tx(ctx, func(tx *gorm.DB) error {
			removed, err := s.repo.SweepStale(ctx, tx, seg.ID, runID, sweepBatch)
			if err != nil {
				return err
			}
			n = len(removed)
			return s.publishChanges(ctx, tx, seg, runID, PageResult{Removed: removed}, at)
		})
		if err != nil {
			return out, seg.Version, err
		}
		out.Removed += n
		if n < sweepBatch {
			break
		}
	}
	return out, seg.Version, s.repo.FinishRefresh(ctx, seg.ID, runID, s.clock.Now())
}

func (s *Service) publishChanges(ctx context.Context, tx *gorm.DB, seg domain.Segment, runID string, res PageResult, at time.Time) error {
	for _, p := range res.Added {
		if err := s.publishChange(ctx, tx, seg.TenantID, seg.ID, p, contracts.ChangeAdded, runID, at); err != nil {
			return err
		}
	}
	for _, p := range res.Removed {
		if err := s.publishChange(ctx, tx, seg.TenantID, seg.ID, p, contracts.ChangeRemoved, runID, at); err != nil {
			return err
		}
	}
	return nil
}

// RefreshAll is the hourly cron: every live segment of every tenant.
// Reconciling (R47): each run recomputes from current state, so a skipped
// tick is healed by the next. A failing segment does not stop the others;
// the first error is returned after the sweep so the job is retried.
func (s *Service) RefreshAll(ctx context.Context) (int, error) {
	var firstErr error
	done, after := 0, ""
	for {
		refs, err := s.repo.LiveSegmentsAfter(ctx, after, refreshBatch)
		if err != nil {
			return done, err
		}
		for _, ref := range refs {
			if _, err := s.RefreshSegment(ctx, ref.TenantID, ref.ID); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			done++
		}
		if len(refs) < refreshBatch {
			break
		}
		after = refs[len(refs)-1].ID
	}
	if firstErr != nil {
		return done, firstErr
	}
	return done, s.repo.MarkRun(ctx, contracts.JobRefresh, s.clock.Now())
}

// evaluate loads what the conditions need for ids and returns the players
// found and the ids matching, in ids order. Ids the player module no
// longer knows never match.
func (s *Service) evaluate(ctx context.Context, tenantID string, g domain.Group, ids []string) (map[string]ports.PlayerSnapshot, []string, error) {
	if len(ids) == 0 {
		return map[string]ports.PlayerSnapshot{}, nil, nil
	}
	players, err := s.rd.Players.PlayersByIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, nil, err
	}
	known := make([]string, 0, len(players))
	for _, pid := range ids {
		if _, ok := players[pid]; ok {
			known = append(known, pid)
		}
	}
	needs := g.Needs()
	var (
		levels   map[string]int
		wallets  map[string]ports.WalletSnapshot
		badges   map[string][]string
		lastSeen map[string]time.Time
	)
	if needs.Level && len(known) > 0 {
		if levels, err = s.rd.Progress.LevelsByPlayerIDs(ctx, tenantID, known); err != nil {
			return nil, nil, err
		}
	}
	if needs.Wallet && len(known) > 0 {
		if wallets, err = s.rd.Wallets.WalletsByPlayerIDs(ctx, tenantID, known); err != nil {
			return nil, nil, err
		}
	}
	if needs.Badges && len(known) > 0 {
		if badges, err = s.rd.Badges.EarnedBadges(ctx, tenantID, known); err != nil {
			return nil, nil, err
		}
	}
	if needs.LastSeen && len(known) > 0 {
		if lastSeen, err = s.rd.Activity.LastSeen(ctx, tenantID, known); err != nil {
			return nil, nil, err
		}
	}
	now := s.clock.Now()
	matched := make([]string, 0, len(known))
	for _, pid := range known {
		p := players[pid]
		f := domain.PlayerFacts{
			Active:         p.Active,
			CreatedAt:      p.CreatedAt,
			Attributes:     p.Attributes,
			Level:          levels[pid],
			Balance:        wallets[pid].Balance,
			LifetimeEarned: wallets[pid].LifetimeEarned,
		}
		if bs := badges[pid]; len(bs) > 0 {
			f.Badges = make(map[string]bool, len(bs))
			for _, b := range bs {
				f.Badges[b] = true
			}
		}
		if t, ok := lastSeen[pid]; ok {
			f.LastSeen = &t
		}
		if g.Matches(f, now) {
			matched = append(matched, pid)
		}
	}
	return players, matched, nil
}
