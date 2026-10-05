// Package contracts is streaks' public surface. A streak counts consecutive
// periods (day/week/month in the tenant's timezone) with recorded activity.
package contracts

import (
	"context"
	"time"

	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
)

const (
	TopicActivityRecorded = "streaks.activity_recorded.v1"
	TopicMilestoneReached = "streaks.milestone_reached.v1"
	TopicBroken           = "streaks.broken.v1"
	TopicRecordRejected   = "streaks.record_rejected.v1"
)

const JobRecord = "streaks.record"

// Cron jobs (Schedule set in Jobs(); enqueued by the scheduler).
const (
	JobBreakSweep    = "streaks.break_sweep"
	JobPruneRequests = "streaks.prune_requests"
)

// Rejection reasons specific to streaks (effect.Reason* cover the rest).
const (
	ReasonStreakNotFound = "streak_not_found"
	ReasonStreakInactive = "streak_inactive"
)

// BrokenV1.Reason values.
const (
	BrokenReasonLapsed = "lapsed"
	BrokenReasonReset  = "reset"
)

func Topic(job string) string { return "job." + job }

// Period types.
const (
	PeriodDaily   = "daily"
	PeriodWeekly  = "weekly"
	PeriodMonthly = "monthly"
)

// RecordCmdV1 records activity for one streak. Either StreakID or
// ActivityKey (the streak definition's trigger slug) identifies it.
type RecordCmdV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	StreakID       string        `json:"streak_id,omitempty"`
	ActivityKey    string        `json:"activity_key,omitempty"`
	Source         effect.Source `json:"source"`
	OccurredAt     time.Time     `json:"occurred_at"` // decides the period bucket
}

type ActivityRecordedV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	StreakID       string        `json:"streak_id"`
	PeriodStart    time.Time     `json:"period_start"`
	NewPeriod      bool          `json:"new_period"` // false when this period was already recorded
	CurrentCount   int           `json:"current_count"`
	LongestCount   int           `json:"longest_count"`
	Source         effect.Source `json:"source"`
	At             time.Time     `json:"at"`
	// Additive.
	PlayerStreakID string `json:"player_streak_id,omitempty"`
	PointsAwarded  int64  `json:"points_awarded"`
}

type MilestoneReachedV1 struct {
	TenantID    string    `json:"tenant_id"`
	PlayerID    string    `json:"player_id"`
	StreakID    string    `json:"streak_id"`
	Milestone   int       `json:"milestone"`
	BonusPoints int64     `json:"bonus_points"`
	At          time.Time `json:"at"`
	// Additive.
	PlayerStreakID string    `json:"player_streak_id,omitempty"`
	AwardID        string    `json:"award_id,omitempty"`
	RunStartedAt   time.Time `json:"run_started_at"`
}

type BrokenV1 struct {
	TenantID   string    `json:"tenant_id"`
	PlayerID   string    `json:"player_id"`
	StreakID   string    `json:"streak_id"`
	FinalCount int       `json:"final_count"`
	LastPeriod time.Time `json:"last_period"`
	At         time.Time `json:"at"`
	// Additive.
	PlayerStreakID string `json:"player_streak_id,omitempty"`
	Reason         string `json:"reason,omitempty"` // BrokenReason*
}

type RecordRejectedV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	StreakID       string        `json:"streak_id,omitempty"`
	ActivityKey    string        `json:"activity_key,omitempty"`
	Reason         string        `json:"reason"`
	Source         effect.Source `json:"source"`
	At             time.Time     `json:"at"`
}

const Module = "streaks"

var (
	PermViewAny = authz.Permission{Module: Module, Action: "view_any"}
	PermView    = authz.Permission{Module: Module, Action: "view"}
	PermCreate  = authz.Permission{Module: Module, Action: "create"} // admin roles
	PermUpdate  = authz.Permission{Module: Module, Action: "update"} // admin roles
	PermDelete  = authz.Permission{Module: Module, Action: "delete"} // admin roles
	PermRecord  = authz.Permission{Module: Module, Action: "record"}
	PermReset   = authz.Permission{Module: Module, Action: "reset"} // admin roles
)

var AllPermissions = []authz.Permission{PermViewAny, PermView, PermCreate, PermUpdate, PermDelete, PermRecord, PermReset}

type PlayerStreakSnapshot struct {
	StreakID     string
	PlayerID     string
	CurrentCount int
	LongestCount int
	LastPeriod   *time.Time
}

type Reader interface {
	PlayerStreaks(ctx context.Context, tenantID string, playerIDs []string) ([]PlayerStreakSnapshot, error)
}
