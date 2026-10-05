package transport

import (
	"bytes"
	"encoding/json"
	"time"

	"levelup/internal/modules/badges/internal/app"
	"levelup/internal/modules/badges/internal/domain"
)

// optional distinguishes an absent field from an explicit null in PATCH
// bodies (JSON merge-patch semantics; fixes Laravel B20).
type optional[T any] struct {
	Set   bool
	Value *T
}

func (o *optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// CreateBadgeReq is POST /badges. points_value defaults to the tier's
// default; slug defaults to a slug of name and is stable on rename.
type CreateBadgeReq struct {
	Name         string         `json:"name"          validate:"required,max=255"`
	Slug         string         `json:"slug"          validate:"omitempty,max=200"`
	Description  string         `json:"description"   validate:"max=1000"`
	IconURL      string         `json:"icon_url"      validate:"omitempty,url,max=500"`
	Tier         string         `json:"tier"          validate:"required,oneof=bronze silver gold platinum diamond"`
	Category     string         `json:"category"      validate:"required,oneof=achievement milestone skill social exploration collection special seasonal"`
	PointsValue  *int64         `json:"points_value"  validate:"omitempty,gte=0"`
	IsStackable  bool           `json:"is_stackable"`
	MaxAwards    *int           `json:"max_awards"    validate:"omitempty,gte=1"`
	Requirements map[string]any `json:"requirements"`
	IsActive     *bool          `json:"is_active"`
	IsSecret     bool           `json:"is_secret"`
	SortOrder    int            `json:"sort_order"`
}

func (r CreateBadgeReq) params() domain.NewBadgeParams {
	active := true
	if r.IsActive != nil {
		active = *r.IsActive
	}
	return domain.NewBadgeParams{
		Name:         r.Name,
		Slug:         r.Slug,
		Description:  r.Description,
		IconURL:      r.IconURL,
		Tier:         r.Tier,
		Category:     r.Category,
		PointsValue:  r.PointsValue,
		Stackable:    r.IsStackable,
		MaxAwards:    r.MaxAwards,
		Requirements: r.Requirements,
		Active:       active,
		Secret:       r.IsSecret,
		SortOrder:    r.SortOrder,
	}
}

// UpdateBadgeReq is PATCH /badges/{id}: omitted fields stay untouched;
// max_awards and requirements accept null to clear.
type UpdateBadgeReq struct {
	Name         *string                  `json:"name"         validate:"omitempty,max=255"`
	Slug         *string                  `json:"slug"         validate:"omitempty,max=200"`
	Description  *string                  `json:"description"  validate:"omitempty,max=1000"`
	IconURL      *string                  `json:"icon_url"     validate:"omitempty,max=500"`
	Tier         *string                  `json:"tier"         validate:"omitempty,oneof=bronze silver gold platinum diamond"`
	Category     *string                  `json:"category"     validate:"omitempty,oneof=achievement milestone skill social exploration collection special seasonal"`
	PointsValue  *int64                   `json:"points_value" validate:"omitempty,gte=0"`
	IsStackable  *bool                    `json:"is_stackable"`
	MaxAwards    optional[int]            `json:"max_awards"   swaggertype:"integer"`
	Requirements optional[map[string]any] `json:"requirements" swaggertype:"object"`
	IsActive     *bool                    `json:"is_active"`
	IsSecret     *bool                    `json:"is_secret"`
	SortOrder    *int                     `json:"sort_order"`
}

func (r UpdateBadgeReq) patch() domain.Patch {
	p := domain.Patch{
		Name:            r.Name,
		Slug:            r.Slug,
		Description:     r.Description,
		IconURL:         r.IconURL,
		Tier:            r.Tier,
		Category:        r.Category,
		PointsValue:     r.PointsValue,
		Stackable:       r.IsStackable,
		MaxAwardsSet:    r.MaxAwards.Set,
		MaxAwards:       r.MaxAwards.Value,
		RequirementsSet: r.Requirements.Set,
		Active:          r.IsActive,
		Secret:          r.IsSecret,
		SortOrder:       r.SortOrder,
	}
	if r.Requirements.Value != nil {
		p.Requirements = *r.Requirements.Value
	}
	return p
}

// AwardReq is POST /badges/{id}/award.
type AwardReq struct {
	PlayerID string `json:"player_id" validate:"required,uuid"`
}

type BadgeResp struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	Slug         string         `json:"slug"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	IconURL      string         `json:"icon_url"`
	Tier         string         `json:"tier"`
	Category     string         `json:"category"`
	PointsValue  int64          `json:"points_value"`
	IsStackable  bool           `json:"is_stackable"`
	MaxAwards    *int           `json:"max_awards"`
	Requirements map[string]any `json:"requirements"`
	IsActive     bool           `json:"is_active"`
	IsSecret     bool           `json:"is_secret"`
	SortOrder    int            `json:"sort_order"`
	Version      int            `json:"version"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    *time.Time     `json:"deleted_at,omitempty"`
}

func toBadgeResp(b domain.Badge) BadgeResp {
	return BadgeResp{
		ID:           b.ID,
		TenantID:     b.TenantID,
		Slug:         b.Slug,
		Name:         b.Name,
		Description:  b.Description,
		IconURL:      b.IconURL,
		Tier:         string(b.Tier),
		Category:     string(b.Category),
		PointsValue:  b.PointsValue,
		IsStackable:  b.Stackable,
		MaxAwards:    b.MaxAwards,
		Requirements: b.Requirements,
		IsActive:     b.Active,
		IsSecret:     b.Secret,
		SortOrder:    b.SortOrder,
		Version:      b.Version,
		CreatedAt:    b.CreatedAt,
		UpdatedAt:    b.UpdatedAt,
		DeletedAt:    b.DeletedAt,
	}
}

type BadgeListResp struct {
	Data       []BadgeResp `json:"data"`
	NextCursor string      `json:"next_cursor"`
}

type PlayerBadgeResp struct {
	ID             string     `json:"id"`
	PlayerID       string     `json:"player_id"`
	BadgeID        string     `json:"badge_id"`
	EarnedCount    int        `json:"earned_count"`
	FirstAwardedAt time.Time  `json:"first_awarded_at"`
	LastAwardedAt  time.Time  `json:"last_awarded_at"`
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	Badge          *BadgeResp `json:"badge,omitempty"`
}

func toPlayerBadgeResp(pb domain.PlayerBadge, b *domain.Badge) PlayerBadgeResp {
	out := PlayerBadgeResp{
		ID:             pb.ID,
		PlayerID:       pb.PlayerID,
		BadgeID:        pb.BadgeID,
		EarnedCount:    pb.EarnedCount,
		FirstAwardedAt: pb.FirstAwardedAt,
		LastAwardedAt:  pb.LastAwardedAt,
		Version:        pb.Version,
		CreatedAt:      pb.CreatedAt,
	}
	if b != nil {
		br := toBadgeResp(*b)
		out.Badge = &br
	}
	return out
}

type PlayerBadgeListResp struct {
	Data       []PlayerBadgeResp `json:"data"`
	NextCursor string            `json:"next_cursor"`
}

// AwardResp is the applied award (201) or its replay (200).
type AwardResp struct {
	AwardID        string          `json:"award_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Status         string          `json:"status"`
	Replay         bool            `json:"replay"`
	PlayerBadge    PlayerBadgeResp `json:"player_badge"`
}

func toAwardResp(o app.AwardOutcome) AwardResp {
	return AwardResp{
		AwardID:        o.Award.ID,
		IdempotencyKey: o.Award.IdempotencyKey,
		Status:         string(o.Award.Status),
		Replay:         o.Replay,
		PlayerBadge:    toPlayerBadgeResp(o.PlayerBadge, nil),
	}
}
