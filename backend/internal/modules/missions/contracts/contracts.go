// Package contracts is missions' public surface. A mission is a goal with a
// target; a player's attempt accumulates progress and completes once.
package contracts

import (
	"context"
	"time"

	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
)

const (
	TopicMissionCreated   = "missions.created.v1"
	TopicStarted          = "missions.started.v1"
	TopicProgressUpdated  = "missions.progress_updated.v1"
	TopicCompleted        = "missions.completed.v1"
	TopicExpired          = "missions.expired.v1"
	TopicProgressRejected = "missions.progress_rejected.v1"
)

const JobProgress = "missions.progress"

func Topic(job string) string { return "job." + job }

// Mission types and statuses.
const (
	TypeOneTime   = "one_time"
	TypeDaily     = "daily"
	TypeWeekly    = "weekly"
	TypeRepeating = "repeating"

	MissionDraft    = "draft"
	MissionActive   = "active"
	MissionPaused   = "paused"
	MissionExpired  = "expired"
	MissionArchived = "archived"

	AttemptInProgress = "in_progress"
	AttemptCompleted  = "completed"
	AttemptExpired    = "expired"
	AttemptAbandoned  = "abandoned"
)

// ProgressCmdV1 adds progress; it starts an attempt when none is open.
type ProgressCmdV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	MissionID      string        `json:"mission_id"`
	Increment      int64         `json:"increment"` // > 0
	Source         effect.Source `json:"source"`
	OccurredAt     time.Time     `json:"occurred_at"`
}

type AttemptV1 struct {
	AttemptID string    `json:"attempt_id"`
	TenantID  string    `json:"tenant_id"`
	PlayerID  string    `json:"player_id"`
	MissionID string    `json:"mission_id"`
	Progress  int64     `json:"progress"`
	Target    int64     `json:"target"`
	At        time.Time `json:"at"`
	// IdempotencyKey is the progress command that produced this update
	// (progress_updated only); the rules module settles its effect by it.
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type CompletedV1 struct {
	AttemptID    string    `json:"attempt_id"`
	TenantID     string    `json:"tenant_id"`
	PlayerID     string    `json:"player_id"`
	MissionID    string    `json:"mission_id"`
	MissionSlug  string    `json:"mission_slug"`
	PointsReward int64     `json:"points_reward"`
	XPReward     int64     `json:"xp_reward"`
	BadgeID      string    `json:"badge_id,omitempty"`
	At           time.Time `json:"at"`
	// ActivityID is the activity whose progress completed the attempt,
	// when there was one; internal triggers use it to cap causation depth.
	ActivityID string `json:"activity_id,omitempty"`
}

type MissionChangedV1 struct {
	MissionID string    `json:"mission_id"`
	TenantID  string    `json:"tenant_id"`
	Slug      string    `json:"slug"`
	Status    string    `json:"status"`
	At        time.Time `json:"at"`
}

type ProgressRejectedV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	MissionID      string        `json:"mission_id"`
	Reason         string        `json:"reason"`
	Source         effect.Source `json:"source"`
	At             time.Time     `json:"at"`
}

const Module = "missions"

var (
	PermViewAny  = authz.Permission{Module: Module, Action: "view_any"}
	PermView     = authz.Permission{Module: Module, Action: "view"}
	PermCreate   = authz.Permission{Module: Module, Action: "create"} // admin roles
	PermUpdate   = authz.Permission{Module: Module, Action: "update"} // admin roles
	PermDelete   = authz.Permission{Module: Module, Action: "delete"} // admin roles
	PermStart    = authz.Permission{Module: Module, Action: "start"}
	PermProgress = authz.Permission{Module: Module, Action: "progress"}
	PermComplete = authz.Permission{Module: Module, Action: "complete"}
)

var AllPermissions = []authz.Permission{
	PermViewAny, PermView, PermCreate, PermUpdate, PermDelete, PermStart, PermProgress, PermComplete,
}

type Reader interface {
	CompletedCounts(ctx context.Context, tenantID string, playerIDs []string) (map[string]int, error)
}

// Additive topics and names (IMPLEMENTATION.md §0: additions only).
const (
	// TopicMissionUpdated and TopicMissionDeleted carry MissionChangedV1.
	TopicMissionUpdated = "missions.updated.v1"
	TopicMissionDeleted = "missions.deleted.v1"
	// TopicAttemptExpired carries AttemptV1 for an open attempt the expire
	// sweep closed (its period ended or its mission expired/archived/deleted).
	TopicAttemptExpired = "missions.attempt_expired.v1"
)

// JobExpireSweep is the reconciling cron that expires missions past ends_at
// and open attempts past their period.
const JobExpireSweep = "missions.expire_sweep"
