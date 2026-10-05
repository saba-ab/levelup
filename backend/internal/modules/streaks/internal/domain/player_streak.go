package domain

import (
	"slices"
	"strconv"
	"time"

	"levelup/internal/shared/id"
)

// PlayerStreak is a player's state on one streak. Its counters are DERIVED
// from the recorded period buckets (Recompute), never incremented, so late or
// out-of-order records converge to the same state (ADR-0012, doc 06 §11.8).
type PlayerStreak struct {
	ID              string
	TenantID        string
	PlayerID        string
	StreakID        string
	CurrentCount    int
	LongestCount    int
	LastPeriodStart *time.Time // newest bucket, any run
	RunStartedAt    *time.Time // first bucket of the latest run (nil when none)
	// RunFloor is set by an admin reset: buckets at or before it never count
	// towards a run again.
	RunFloor *time.Time
	BrokenAt *time.Time // when the latest run was broken; nil while it is alive
	// BrokenRunStartedAt remembers which run streaks.broken.v1 was published
	// for, so one run is announced broken at most once.
	BrokenRunStartedAt *time.Time
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func NewPlayerStreak(tenantID, playerID, streakID string, now time.Time) (PlayerStreak, error) {
	if tenantID == "" || playerID == "" || streakID == "" {
		return PlayerStreak{}, ErrPlayerStreakNoOwner
	}
	return PlayerStreak{
		ID:        id.NewID(),
		TenantID:  tenantID,
		PlayerID:  playerID,
		StreakID:  streakID,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Run is a maximal sequence of buckets whose neighbours are at most
// 1+grace periods apart. Length counts recorded buckets, not elapsed periods.
type Run struct {
	Start  time.Time
	End    time.Time
	Length int
}

// Runs splits sorted-or-not bucket starts into runs, oldest first.
func Runs(p Period, grace int, buckets []time.Time) []Run {
	if len(buckets) == 0 {
		return nil
	}
	sorted := slices.Clone(buckets)
	slices.SortFunc(sorted, func(a, b time.Time) int { return a.Compare(b) })
	sorted = slices.CompactFunc(sorted, func(a, b time.Time) bool { return a.Equal(b) })

	maxGap := int64(1 + grace)
	runs := []Run{{Start: sorted[0], End: sorted[0], Length: 1}}
	for _, b := range sorted[1:] {
		last := &runs[len(runs)-1]
		if p.Index(b)-p.Index(last.End) <= maxGap {
			last.End = b
			last.Length++
			continue
		}
		runs = append(runs, Run{Start: b, End: b, Length: 1})
	}
	return runs
}

// RunContaining returns the run that holds bucket b.
func RunContaining(runs []Run, b time.Time) (Run, bool) {
	for _, r := range runs {
		if !b.Before(r.Start) && !b.After(r.End) {
			return r, true
		}
	}
	return Run{}, false
}

// Alive reports whether a run ending at end can still be continued in the
// period nowPeriod: at most 1+grace periods may separate them.
func Alive(p Period, grace int, end, nowPeriod time.Time) bool {
	return p.Index(nowPeriod)-p.Index(end) <= int64(1+grace)
}

// Recomputed describes what a recompute changed.
type Recomputed struct {
	Runs []Run // runs above the reset floor, oldest first
	// Lapsed is true when the live run ended here (current went >0 → 0)
	// and nobody has announced that run as broken yet.
	Lapsed      bool
	BrokenCount int // the count that was lost when Lapsed
}

// Recompute derives counters from every bucket of this player streak.
// nowPeriod is the bucket containing "now", in the same timezone.
func (ps *PlayerStreak) Recompute(s Streak, buckets []time.Time, nowPeriod, now time.Time) Recomputed {
	var counted []time.Time
	for _, b := range buckets {
		if ps.LastPeriodStart == nil || b.After(*ps.LastPeriodStart) {
			ps.LastPeriodStart = &b
		}
		if ps.RunFloor != nil && !b.After(*ps.RunFloor) {
			continue
		}
		counted = append(counted, b)
	}
	runs := Runs(s.Period, s.GracePeriods, counted)
	out := Recomputed{Runs: runs}
	for _, r := range runs {
		ps.LongestCount = max(ps.LongestCount, r.Length)
	}

	prev := ps.CurrentCount
	ps.UpdatedAt = now
	if len(runs) == 0 {
		ps.CurrentCount = 0
		ps.RunStartedAt = nil
		return out
	}
	last := runs[len(runs)-1]
	start := last.Start
	ps.RunStartedAt = &start
	if Alive(s.Period, s.GracePeriods, last.End, nowPeriod) {
		ps.CurrentCount = last.Length
		ps.BrokenAt = nil
		return out
	}
	ps.CurrentCount = 0
	if prev > 0 && !sameDay(ps.BrokenRunStartedAt, &start) {
		out.Lapsed = true
		out.BrokenCount = prev
		ps.markBroken(now)
	}
	return out
}

// Break ends the live run because it lapsed (break sweep). It returns the
// lost count and whether streaks.broken.v1 should be published.
func (ps *PlayerStreak) Break(now time.Time) (int, bool) {
	prev := ps.CurrentCount
	if prev == 0 {
		return 0, false
	}
	announce := !sameDay(ps.BrokenRunStartedAt, ps.RunStartedAt)
	ps.CurrentCount = 0
	ps.UpdatedAt = now
	ps.markBroken(now)
	return prev, announce
}

// Reset is the admin action: the running count goes to zero and every
// bucket recorded so far is excluded from future runs. Returns the lost
// count (0 means nothing was running).
func (ps *PlayerStreak) Reset(now time.Time) int {
	prev := ps.CurrentCount
	if ps.LastPeriodStart != nil {
		floor := *ps.LastPeriodStart
		ps.RunFloor = &floor
	}
	ps.CurrentCount = 0
	ps.UpdatedAt = now
	if prev > 0 {
		ps.markBroken(now)
	}
	ps.RunStartedAt = nil
	return prev
}

func (ps *PlayerStreak) markBroken(now time.Time) {
	at := now
	ps.BrokenAt = &at
	if ps.RunStartedAt != nil {
		run := *ps.RunStartedAt
		ps.BrokenRunStartedAt = &run
	}
}

func sameDay(a, b *time.Time) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

// PeriodBucket is one recorded period. UNIQUE(player_streak_id, period_start)
// makes a second record in the same period a no-op (fixes S1).
type PeriodBucket struct {
	ID             string
	TenantID       string
	PlayerStreakID string
	PeriodStart    time.Time
	IdempotencyKey string
	RecordedAt     time.Time
}

// MilestoneAward is a paid milestone. UNIQUE(player_streak_id, milestone,
// run_started_at) pays each milestone once per run (fixes S5).
type MilestoneAward struct {
	ID             string
	TenantID       string
	PlayerStreakID string
	Milestone      int
	BonusPoints    int64
	RunStartedAt   time.Time
	AwardedAt      time.Time
}

// MilestoneAwardID is deterministic, so the points credit key derived from it
// is identical however often the award is attempted.
func MilestoneAwardID(playerStreakID string, milestone int, runStartedAt time.Time) string {
	return id.Derive("streak_milestone_award", playerStreakID, strconv.Itoa(milestone), DateKey(runStartedAt))
}

// Record request statuses.
const (
	RequestApplied  = "applied"
	RequestRejected = "rejected"
)

// RecordRequest makes record commands idempotent: UNIQUE(tenant_id,
// idempotency_key).
type RecordRequest struct {
	TenantID       string
	IdempotencyKey string
	PlayerID       string
	StreakID       string
	ActivityKey    string
	Status         string
	Reason         string
	CreatedAt      time.Time
}
