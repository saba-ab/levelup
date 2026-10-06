package transport

import (
	"time"

	"levelup/internal/modules/rewards/internal/app"
	"levelup/internal/modules/rewards/internal/domain"
)

// CreateRewardReq validates shape only; invariants live in domain.NewReward.
type CreateRewardReq struct {
	Name             string         `json:"name"              validate:"required,max=255"`
	Slug             string         `json:"slug"              validate:"omitempty,max=120"`
	Description      string         `json:"description"       validate:"max=1000"`
	Type             string         `json:"type"              validate:"omitempty,oneof=points discount item badge level custom"`
	Status           string         `json:"status"            validate:"omitempty,oneof=draft active paused expired depleted"`
	PointsCost       int64          `json:"points_cost"       validate:"gte=0"`
	Value            *string        `json:"value"             validate:"omitempty,max=12"`
	ValueType        *string        `json:"value_type"        validate:"omitempty,oneof=percentage fixed"`
	BadgeRewardID    *string        `json:"badge_reward_id"   validate:"omitempty,uuid"`
	LevelRewardID    *string        `json:"level_reward_id"   validate:"omitempty,uuid"`
	MaxRedemptions   *int           `json:"max_redemptions"   validate:"omitempty,gte=1"`
	MaxPerPlayer     *int           `json:"max_per_player"    validate:"omitempty,gte=1"`
	ClaimTTLDays     *int           `json:"claim_ttl_days"    validate:"omitempty,gte=1"`
	StartAt          *time.Time     `json:"start_at"`
	EndAt            *time.Time     `json:"end_at"`
	LevelRequirement *int           `json:"level_requirement" validate:"omitempty,gte=1"`
	IsActive         *bool          `json:"is_active"`
	Metadata         map[string]any `json:"metadata"`
}

func (r CreateRewardReq) toPatch() domain.RewardPatch {
	p := domain.RewardPatch{
		Name:             &r.Name,
		Description:      &r.Description,
		PointsCost:       &r.PointsCost,
		Value:            r.Value,
		ValueType:        r.ValueType,
		BadgeRewardID:    r.BadgeRewardID,
		LevelRewardID:    r.LevelRewardID,
		MaxRedemptions:   r.MaxRedemptions,
		MaxPerPlayer:     r.MaxPerPlayer,
		ClaimTTLDays:     r.ClaimTTLDays,
		StartAt:          r.StartAt,
		EndAt:            r.EndAt,
		LevelRequirement: r.LevelRequirement,
		IsActive:         r.IsActive,
		Metadata:         r.Metadata,
	}
	if r.Slug != "" {
		p.Slug = &r.Slug
	}
	if r.Type != "" {
		p.Type = &r.Type
	}
	if r.Status != "" {
		p.Status = &r.Status
	}
	return p
}

// UpdateRewardReq is a partial update: omitted fields stay untouched. For
// nullable fields, "" (strings) or 0 (numbers) clears the value.
type UpdateRewardReq struct {
	Name             *string        `json:"name"              validate:"omitempty,max=255"`
	Slug             *string        `json:"slug"              validate:"omitempty,max=120"`
	Description      *string        `json:"description"       validate:"omitempty,max=1000"`
	Type             *string        `json:"type"              validate:"omitempty,oneof=points discount item badge level custom"`
	Status           *string        `json:"status"            validate:"omitempty,oneof=draft active paused expired depleted"`
	PointsCost       *int64         `json:"points_cost"       validate:"omitempty,gte=0"`
	Value            *string        `json:"value"             validate:"omitempty,max=12"`
	ValueType        *string        `json:"value_type"        validate:"omitempty,max=20"`
	BadgeRewardID    *string        `json:"badge_reward_id"   validate:"omitempty,max=36"`
	LevelRewardID    *string        `json:"level_reward_id"   validate:"omitempty,max=36"`
	MaxRedemptions   *int           `json:"max_redemptions"   validate:"omitempty,gte=0"`
	MaxPerPlayer     *int           `json:"max_per_player"    validate:"omitempty,gte=0"`
	ClaimTTLDays     *int           `json:"claim_ttl_days"    validate:"omitempty,gte=0"`
	StartAt          *time.Time     `json:"start_at"`
	EndAt            *time.Time     `json:"end_at"`
	LevelRequirement *int           `json:"level_requirement" validate:"omitempty,gte=0"`
	IsActive         *bool          `json:"is_active"`
	Metadata         map[string]any `json:"metadata"`
}

func (r UpdateRewardReq) toPatch() domain.RewardPatch {
	return domain.RewardPatch{
		Name: r.Name, Slug: r.Slug, Description: r.Description, Type: r.Type, Status: r.Status,
		PointsCost: r.PointsCost, Value: r.Value, ValueType: r.ValueType,
		BadgeRewardID: r.BadgeRewardID, LevelRewardID: r.LevelRewardID,
		MaxRedemptions: r.MaxRedemptions, MaxPerPlayer: r.MaxPerPlayer, ClaimTTLDays: r.ClaimTTLDays,
		StartAt: r.StartAt, EndAt: r.EndAt, LevelRequirement: r.LevelRequirement,
		IsActive: r.IsActive, Metadata: r.Metadata,
	}
}

type ClaimReq struct {
	PlayerID string `json:"player_id" validate:"required,uuid"`
}

type RewardResp struct {
	ID               string         `json:"id"`
	TenantID         string         `json:"tenant_id"`
	Name             string         `json:"name"`
	Slug             string         `json:"slug"`
	Description      string         `json:"description"`
	Type             string         `json:"type"`
	Status           string         `json:"status"`
	PointsCost       int64          `json:"points_cost"`
	Value            *string        `json:"value"`
	ValueType        *string        `json:"value_type"`
	BadgeRewardID    *string        `json:"badge_reward_id"`
	LevelRewardID    *string        `json:"level_reward_id"`
	MaxRedemptions   *int           `json:"max_redemptions"`
	MaxPerPlayer     *int           `json:"max_per_player"`
	StockUsed        int            `json:"stock_used"`
	ClaimTTLDays     *int           `json:"claim_ttl_days"`
	StartAt          *time.Time     `json:"start_at"`
	EndAt            *time.Time     `json:"end_at"`
	LevelRequirement *int           `json:"level_requirement"`
	IsActive         bool           `json:"is_active"`
	Metadata         map[string]any `json:"metadata"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type RewardListResp struct {
	Data       []RewardResp `json:"data"`
	NextCursor string       `json:"next_cursor"`
}

type ClaimResp struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	PlayerID      string     `json:"player_id"`
	RewardID      string     `json:"reward_id"`
	RewardSlug    string     `json:"reward_slug"`
	RewardType    string     `json:"reward_type"`
	Status        string     `json:"status"`
	PointsCost    int64      `json:"points_cost"`
	RejectReason  string     `json:"reject_reason,omitempty"`
	Code          *string    `json:"code"`
	HoldExpiresAt *time.Time `json:"hold_expires_at"`
	ClaimedAt     *time.Time `json:"claimed_at"`
	RedeemedAt    *time.Time `json:"redeemed_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	CancelledAt   *time.Time `json:"cancelled_at"`
	FulfilledAt   *time.Time `json:"fulfilled_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type ClaimListResp struct {
	Data       []ClaimResp `json:"data"`
	NextCursor string      `json:"next_cursor"`
}

func toRewardResp(r domain.Reward) RewardResp {
	return RewardResp{
		ID: r.ID, TenantID: r.TenantID, Name: r.Name, Slug: r.Slug, Description: r.Description,
		Type: r.Type, Status: r.Status, PointsCost: r.PointsCost, Value: r.Value, ValueType: r.ValueType,
		BadgeRewardID: r.BadgeRewardID, LevelRewardID: r.LevelRewardID,
		MaxRedemptions: r.MaxRedemptions, MaxPerPlayer: r.MaxPerPlayer, StockUsed: r.StockUsed,
		ClaimTTLDays: r.ClaimTTLDays, StartAt: utc(r.StartAt), EndAt: utc(r.EndAt),
		LevelRequirement: r.LevelRequirement, IsActive: r.IsActive, Metadata: r.Metadata,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func toClaimResp(c domain.Claim) ClaimResp {
	return ClaimResp{
		ID: c.ID, TenantID: c.TenantID, PlayerID: c.PlayerID, RewardID: c.RewardID,
		RewardSlug: c.RewardSlug, RewardType: c.RewardType, Status: c.Status, PointsCost: c.PointsCost,
		RejectReason: c.RejectReason, Code: c.Code,
		HoldExpiresAt: utc(c.HoldExpiresAt), ClaimedAt: utc(c.ClaimedAt), RedeemedAt: utc(c.RedeemedAt),
		ExpiresAt: utc(c.ExpiresAt), CancelledAt: utc(c.CancelledAt), FulfilledAt: utc(c.FulfilledAt),
		CreatedAt: c.CreatedAt.UTC(), UpdatedAt: c.UpdatedAt.UTC(),
	}
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// RewardStatsResp is one reward's claim counters. claimed counts every claim
// that settled (whatever happened next); cancelled includes refunded claims;
// points_spent is the points kept (claims now claimed, redeemed or expired).
type RewardStatsResp struct {
	RewardID    string `json:"reward_id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Deleted     bool   `json:"deleted"`
	Claimed     int64  `json:"claimed"`
	Redeemed    int64  `json:"redeemed"`
	Expired     int64  `json:"expired"`
	Cancelled   int64  `json:"cancelled"`
	PointsSpent int64  `json:"points_spent"`
}

// StatsTotalsResp sums RewardStatsResp over the tenant.
type StatsTotalsResp struct {
	Claimed     int64 `json:"claimed"`
	Redeemed    int64 `json:"redeemed"`
	Expired     int64 `json:"expired"`
	Cancelled   int64 `json:"cancelled"`
	PointsSpent int64 `json:"points_spent"`
}

type StatsResp struct {
	Rewards []RewardStatsResp `json:"rewards"`
	Totals  StatsTotalsResp   `json:"totals"`
}

func toStatsResp(r app.StatsReport) StatsResp {
	out := StatsResp{Rewards: make([]RewardStatsResp, len(r.Rewards)), Totals: StatsTotalsResp{
		Claimed: r.Totals.Claimed, Redeemed: r.Totals.Redeemed, Expired: r.Totals.Expired,
		Cancelled: r.Totals.Cancelled, PointsSpent: r.Totals.PointsSpent,
	}}
	for i, s := range r.Rewards {
		out.Rewards[i] = RewardStatsResp{
			RewardID: s.RewardID, Slug: s.Slug, Name: s.Name, Type: s.Type, Deleted: s.Deleted,
			Claimed: s.Claimed, Redeemed: s.Redeemed, Expired: s.Expired, Cancelled: s.Cancelled,
			PointsSpent: s.PointsSpent,
		}
	}
	return out
}
