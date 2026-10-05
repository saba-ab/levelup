// Package contracts is badges' public surface.
package contracts

import (
	"context"
	"time"

	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
)

const (
	TopicAwarded       = "badges.awarded.v1"
	TopicAwardRejected = "badges.award_rejected.v1"
	TopicRevoked       = "badges.revoked.v1"
)

const JobAward = "badges.award"

func Topic(job string) string { return "job." + job }

// Tiers and categories (Laravel BadgeTier / BadgeCategory values).
const (
	TierBronze   = "bronze"
	TierSilver   = "silver"
	TierGold     = "gold"
	TierPlatinum = "platinum"
	TierDiamond  = "diamond"
)

const (
	CategoryAchievement = "achievement"
	CategoryMilestone   = "milestone"
	CategorySkill       = "skill"
	CategorySocial      = "social"
	CategoryExploration = "exploration"
	CategoryCollection  = "collection"
	CategorySpecial     = "special"
	CategorySeasonal    = "seasonal"
)

// JobReconcile is the hourly cron checking earned_count == applied awards
// and earned_count <= max_awards (log + metric only).
const JobReconcile = "badges.reconcile"

type AwardCmdV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	BadgeID        string        `json:"badge_id"`
	Source         effect.Source `json:"source"`
	OccurredAt     time.Time     `json:"occurred_at"`
}

type AwardedV1 struct {
	AwardID        string        `json:"award_id"`
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	BadgeID        string        `json:"badge_id"`
	BadgeSlug      string        `json:"badge_slug"`
	EarnedCount    int           `json:"earned_count"`
	PointsValue    int64         `json:"points_value"`
	Source         effect.Source `json:"source"`
	OccurredAt     time.Time     `json:"occurred_at"`
	At             time.Time     `json:"at"`
	// Additive fields.
	PlayerBadgeID      string `json:"player_badge_id,omitempty"`
	Tier               string `json:"tier,omitempty"`
	Category           string `json:"category,omitempty"`
	IsFirstEarn        bool   `json:"is_first_earn"`
	AwardedBy          string `json:"awarded_by,omitempty"`
	PlayerBadgeVersion int    `json:"player_badge_version"`
}

type AwardRejectedV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	BadgeID        string        `json:"badge_id"`
	Reason         string        `json:"reason"` // effect.Reason*
	Source         effect.Source `json:"source"`
	At             time.Time     `json:"at"`
}

type RevokedV1 struct {
	TenantID  string    `json:"tenant_id"`
	PlayerID  string    `json:"player_id"`
	BadgeID   string    `json:"badge_id"`
	RevokedBy string    `json:"revoked_by"`
	At        time.Time `json:"at"`
	// Additive fields.
	RevocationID       string `json:"revocation_id,omitempty"`
	PlayerBadgeID      string `json:"player_badge_id,omitempty"`
	EarnedCountRemoved int    `json:"earned_count_removed"`
}

const Module = "badges"

var (
	PermViewAny = authz.Permission{Module: Module, Action: "view_any"}
	PermView    = authz.Permission{Module: Module, Action: "view"}
	PermCreate  = authz.Permission{Module: Module, Action: "create"} // admin roles
	PermUpdate  = authz.Permission{Module: Module, Action: "update"} // admin roles
	PermDelete  = authz.Permission{Module: Module, Action: "delete"} // admin roles
	PermAward   = authz.Permission{Module: Module, Action: "award"}
	PermRevoke  = authz.Permission{Module: Module, Action: "revoke"} // admin roles
)

var AllPermissions = []authz.Permission{PermViewAny, PermView, PermCreate, PermUpdate, PermDelete, PermAward, PermRevoke}

type BadgeSnapshot struct {
	ID          string
	TenantID    string
	Slug        string
	Name        string
	Tier        string
	PointsValue int64
	Active      bool
}

type PlayerBadgeSnapshot struct {
	BadgeID     string
	PlayerID    string
	EarnedCount int
	FirstAt     time.Time
}

type Reader interface {
	BadgesByIDs(ctx context.Context, tenantID string, ids []string) ([]BadgeSnapshot, error)
	PlayerBadges(ctx context.Context, tenantID string, playerIDs []string) ([]PlayerBadgeSnapshot, error)
}
