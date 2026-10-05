package domain

import (
	"fmt"
	"time"

	"levelup/internal/modules/missions/contracts"
	"levelup/internal/shared/id"
)

// PeriodAll is the period key of one_time and repeating missions: they have
// a single, never-ending period.
const PeriodAll = "all"

// Attempt is one player's run at a mission within one period. Target is a
// snapshot of the mission's target when the attempt started, so editing a
// mission never moves the goalposts of a running attempt.
type Attempt struct {
	ID        string
	TenantID  string
	MissionID string
	PlayerID  string
	Status    string
	Progress  int64
	Target    int64
	// PeriodKey identifies the period the attempt belongs to, in UTC:
	//   daily     → "2026-10-05"  (calendar day, UTC)
	//   weekly    → "2026-W41"    (ISO-8601 week, Monday 00:00 UTC start)
	//   one_time, repeating → "all"
	// At most one in_progress attempt exists per (mission, player, period).
	PeriodKey    string
	PeriodEndsAt *time.Time // nil for "all"; the sweep expires open attempts past it
	StartedAt    time.Time
	CompletedAt  *time.Time
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PeriodFor returns the period an action at now falls into. All periods
// are UTC; per-tenant time zones are a possible later refinement.
func PeriodFor(missionType string, now time.Time) (string, *time.Time) {
	now = now.UTC()
	switch missionType {
	case contracts.TypeDaily:
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 0, 1)
		return start.Format("2006-01-02"), &end
	case contracts.TypeWeekly:
		year, week := now.ISOWeek()
		offset := (int(now.Weekday()) + 6) % 7 // days since Monday
		start := time.Date(now.Year(), now.Month(), now.Day()-offset, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 0, 7)
		return fmt.Sprintf("%04d-W%02d", year, week), &end
	default:
		return PeriodAll, nil
	}
}

// StartAttempt opens a fresh attempt with zero progress. A restart never
// carries old progress over (fixes Laravel B3).
func StartAttempt(m Mission, playerID string, now time.Time) Attempt {
	key, ends := PeriodFor(m.Type, now)
	return Attempt{
		ID:           id.NewID(),
		TenantID:     m.TenantID,
		MissionID:    m.ID,
		PlayerID:     playerID,
		Status:       contracts.AttemptInProgress,
		Target:       m.Target,
		PeriodKey:    key,
		PeriodEndsAt: ends,
		StartedAt:    now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// AddProgress adds increment, capped at the target. It reports true exactly
// once: on the call that moves the attempt from in_progress to completed.
func (a *Attempt) AddProgress(increment int64, now time.Time) (bool, error) {
	if increment <= 0 {
		return false, ErrNonPositiveIncrease
	}
	if a.Status != contracts.AttemptInProgress {
		return false, ErrAttemptNotOpen
	}
	a.Progress = min(a.Progress+increment, a.Target)
	a.UpdatedAt = now
	if a.Progress >= a.Target {
		a.markCompleted(now)
		return true, nil
	}
	return false, nil
}

// Complete is the explicit completion: only an in_progress attempt whose
// progress reached the target completes. A completed attempt cannot
// complete again (fixes Laravel B2, "complete forever").
func (a *Attempt) Complete(now time.Time) error {
	if a.Status != contracts.AttemptInProgress {
		return ErrAttemptNotOpen
	}
	if a.Progress < a.Target {
		return ErrNotCompleted
	}
	a.markCompleted(now)
	return nil
}

func (a *Attempt) markCompleted(now time.Time) {
	a.Status = contracts.AttemptCompleted
	a.CompletedAt = &now
	a.UpdatedAt = now
}

// Open reports whether the attempt still accepts progress.
func (a Attempt) Open() bool { return a.Status == contracts.AttemptInProgress }

// Close ends an open attempt without completion: expired (period or
// mission over) or abandoned (player deleted). Closed attempts never reopen.
func (a *Attempt) Close(status string, now time.Time) error {
	if a.Status != contracts.AttemptInProgress {
		return ErrAttemptNotOpen
	}
	a.Status = status
	a.UpdatedAt = now
	return nil
}
