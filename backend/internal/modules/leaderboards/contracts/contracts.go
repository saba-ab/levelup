// Package contracts is leaderboards' public surface. Scores are projections
// of other modules' facts; Postgres is the truth, Redis ZSETs a read model.
package contracts

import (
	"time"

	"levelup/internal/platform/authz"
)

const TopicPeriodClosed = "leaderboards.period_closed.v1"

// Types, metrics and reset frequencies.
const (
	TypePoints   = "points"
	TypeBadges   = "badges"
	TypeMissions = "missions"
	TypeXP       = "xp"

	MetricEarned  = "earned"  // sum of credits in the period
	MetricNet     = "net"     // credits minus debits in the period
	MetricBalance = "balance" // latest balance (all-time boards only)
	MetricCount   = "count"   // number of awards/completions

	ResetNever   = "never"
	ResetDaily   = "daily"
	ResetWeekly  = "weekly"
	ResetMonthly = "monthly"
)

type TopEntryV1 struct {
	PlayerID string `json:"player_id"`
	Rank     int    `json:"rank"`
	Score    int64  `json:"score"`
}

type PeriodClosedV1 struct {
	LeaderboardID string       `json:"leaderboard_id"`
	TenantID      string       `json:"tenant_id"`
	PeriodStart   time.Time    `json:"period_start"`
	PeriodEnd     time.Time    `json:"period_end"`
	Top           []TopEntryV1 `json:"top"`
	At            time.Time    `json:"at"`
}

const Module = "leaderboards"

var (
	PermViewAny = authz.Permission{Module: Module, Action: "view_any"}
	PermView    = authz.Permission{Module: Module, Action: "view"}
	PermCreate  = authz.Permission{Module: Module, Action: "create"}  // admin roles
	PermUpdate  = authz.Permission{Module: Module, Action: "update"}  // admin roles
	PermDelete  = authz.Permission{Module: Module, Action: "delete"}  // admin roles
	PermRebuild = authz.Permission{Module: Module, Action: "rebuild"} // admin roles
)

var AllPermissions = []authz.Permission{PermViewAny, PermView, PermCreate, PermUpdate, PermDelete, PermRebuild}
