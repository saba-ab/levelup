// Package contracts is progression's public surface: XP and levels. A
// player's level is derived from total XP against the tenant's ladder, so
// grants commute and arrive in any order.
package contracts

import (
	"context"
	"time"

	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
)

const (
	TopicXPGained      = "progression.xp_gained.v1"
	TopicLevelReached  = "progression.level_reached.v1"
	TopicGrantRejected = "progression.grant_rejected.v1"
)

const JobGrantXP = "progression.grant_xp"

// JobReconcile is the hourly reconciling sweep (cron, not a command).
const JobReconcile = "progression.reconcile"

func Topic(job string) string { return "job." + job }

type GrantXPCmdV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	Amount         int64         `json:"amount"` // > 0
	Description    string        `json:"description,omitempty"`
	Source         effect.Source `json:"source"`
	OccurredAt     time.Time     `json:"occurred_at"`
}

type XPGainedV1 struct {
	GrantID        string        `json:"grant_id"`
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	Amount         int64         `json:"amount"`
	TotalXP        int64         `json:"total_xp"`
	LevelNumber    int           `json:"level_number"`
	Source         effect.Source `json:"source"`
	OccurredAt     time.Time     `json:"occurred_at"`
	At             time.Time     `json:"at"`
}

// LevelReachedV1 is published once per (player, level), ever.
type LevelReachedV1 struct {
	TenantID    string    `json:"tenant_id"`
	PlayerID    string    `json:"player_id"`
	LevelID     string    `json:"level_id"`
	LevelNumber int       `json:"level_number"`
	LevelName   string    `json:"level_name"`
	TotalXP     int64     `json:"total_xp"`
	At          time.Time `json:"at"`

	// Additive: the level_rewards row id (the Source.ID of the reward
	// commands) and what the level grants.
	LevelRewardID string `json:"level_reward_id,omitempty"`
	PointsReward  int64  `json:"points_reward,omitempty"`
	BadgeRewardID string `json:"badge_reward_id,omitempty"`
	// ActivityID is the activity whose XP grant crossed the level, when
	// there was one; internal triggers use it to cap causation depth.
	ActivityID string `json:"activity_id,omitempty"`
}

type GrantRejectedV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	Amount         int64         `json:"amount"`
	Reason         string        `json:"reason"`
	Source         effect.Source `json:"source"`
	At             time.Time     `json:"at"`
}

const Module = "progression"

var (
	PermViewAny = authz.Permission{Module: Module, Action: "view_any"}
	PermView    = authz.Permission{Module: Module, Action: "view"}
	PermCreate  = authz.Permission{Module: Module, Action: "create"} // admin roles
	PermUpdate  = authz.Permission{Module: Module, Action: "update"} // admin roles
	PermDelete  = authz.Permission{Module: Module, Action: "delete"} // admin roles
	PermGrantXP = authz.Permission{Module: Module, Action: "grant_xp"}
)

var AllPermissions = []authz.Permission{PermViewAny, PermView, PermCreate, PermUpdate, PermDelete, PermGrantXP}

type ProgressSnapshot struct {
	PlayerID    string
	TotalXP     int64
	LevelID     string // "" when the tenant has no ladder yet
	LevelNumber int    // 0 when below the first level
}

type Reader interface {
	ProgressByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) ([]ProgressSnapshot, error)
}
