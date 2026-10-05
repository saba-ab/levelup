package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/missions/internal/domain"
)

// MissionFilter narrows a mission list. Empty fields do not filter.
type MissionFilter struct {
	Status string
	Type   string
}

// AttemptFilter narrows an attempt list. Empty fields do not filter.
type AttemptFilter struct {
	MissionID string
	PlayerID  string
	Status    string
}

// Cursor is a keyset position over (created_at, id) DESC. The zero value
// is the first page.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// Repository is consumed by the service and implemented in internal/repo.
// Every tenant-owned read takes tenantID explicitly (ADR-0015); write-path
// methods take the transaction right after ctx (ADR-0013). Methods that
// must see the transaction's own writes or take locks also take tx.
type Repository interface {
	// Missions.
	CreateMission(ctx context.Context, tx *gorm.DB, m domain.Mission) error
	MissionByID(ctx context.Context, tenantID, id string) (domain.Mission, error)
	// MissionInTx loads a non-deleted mission inside tx; lock takes FOR UPDATE.
	MissionInTx(ctx context.Context, tx *gorm.DB, tenantID, id string, lock bool) (domain.Mission, error)
	// SaveMission writes every mutable column, guarded by m.Version.
	SaveMission(ctx context.Context, tx *gorm.DB, m domain.Mission) error
	ListMissions(ctx context.Context, tenantID string, f MissionFilter, after Cursor, limit int) ([]domain.Mission, error)
	// MissionsByIDs includes soft-deleted missions (attempt history keeps
	// its mission summary).
	MissionsByIDs(ctx context.Context, tenantID string, ids []string) (map[string]domain.Mission, error)

	// Attempts.
	// OpenAttemptForUpdate locks the in_progress attempt of a period.
	OpenAttemptForUpdate(ctx context.Context, tx *gorm.DB, tenantID, missionID, playerID, periodKey string) (domain.Attempt, bool, error)
	// LatestAttempt returns the newest attempt of a period, any status.
	LatestAttempt(ctx context.Context, tx *gorm.DB, tenantID, missionID, playerID, periodKey string) (domain.Attempt, bool, error)
	// AttemptCounts returns the player's completed attempts across all
	// periods and whether one of them is in periodKey.
	AttemptCounts(ctx context.Context, tx *gorm.DB, tenantID, missionID, playerID, periodKey string) (int, bool, error)
	// InsertAttempt reports false when an open attempt for the period
	// already exists (ON CONFLICT DO NOTHING on the partial unique index).
	InsertAttempt(ctx context.Context, tx *gorm.DB, a domain.Attempt) (bool, error)
	// SaveAttempt writes progress/status/completed_at, guarded by a.Version.
	SaveAttempt(ctx context.Context, tx *gorm.DB, a domain.Attempt) error
	AttemptByID(ctx context.Context, tenantID, id string) (domain.Attempt, error)
	ListAttempts(ctx context.Context, tenantID string, f AttemptFilter, after Cursor, limit int) ([]domain.Attempt, error)
	CompletedCounts(ctx context.Context, tenantID string, playerIDs []string) (map[string]int, error)

	// Progress idempotency ledger.
	// InsertProgressEvent reports false when the key was already recorded.
	InsertProgressEvent(ctx context.Context, tx *gorm.DB, e domain.ProgressEvent) (bool, error)
	// FinishProgressEvent records the outcome (attempt, status, reason).
	FinishProgressEvent(ctx context.Context, tx *gorm.DB, e domain.ProgressEvent) error
	ProgressEventByKey(ctx context.Context, tenantID, key string) (domain.ProgressEvent, bool, error)

	// Sweep (system scope: rows of every tenant, each carrying its tenant).
	// DueMissions locks active/paused missions past ends_at (SKIP LOCKED).
	DueMissions(ctx context.Context, tx *gorm.DB, now time.Time, limit int) ([]domain.Mission, error)
	// DueAttempts locks open attempts past their period, or whose mission is
	// expired, archived or deleted (SKIP LOCKED).
	DueAttempts(ctx context.Context, tx *gorm.DB, now time.Time, limit int) ([]domain.Attempt, error)
	LastRun(ctx context.Context, job string) (time.Time, error)
	MarkRun(ctx context.Context, job string, at time.Time) error

	// Subscriptions.
	AbandonOpenAttempts(ctx context.Context, tx *gorm.DB, tenantID, playerID string, now time.Time) (int64, error)
	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
}
