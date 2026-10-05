// Package repo implements progression's persistence. Models stay
// unexported; table names derive from struct names under the
// progression_svc. prefix (no TableName overrides).
package repo

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/progression/internal/domain"
)

// level → progression_svc.levels
type level struct {
	ID            string `gorm:"primaryKey;type:uuid"`
	TenantID      string `gorm:"type:uuid;not null"`
	LevelNumber   int    `gorm:"not null"`
	Name          string `gorm:"not null"`
	Description   *string
	XPRequired    int64   `gorm:"column:xp_required;not null"`
	PointsReward  int64   `gorm:"not null"`
	BadgeRewardID *string `gorm:"type:uuid"` // bare uuid into badges_svc, NO FK
	Perks         *string `gorm:"type:jsonb"`
	IconURL       *string `gorm:"column:icon_url"`
	IsActive      bool    `gorm:"not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt
}

// playerProgress → progression_svc.player_progresses
type playerProgress struct {
	TenantID    string  `gorm:"primaryKey;type:uuid"`
	PlayerID    string  `gorm:"primaryKey;type:uuid"` // bare uuid, NO FK to player_svc
	TotalXP     int64   `gorm:"column:total_xp;not null"`
	LevelID     *string `gorm:"type:uuid"`
	LevelNumber int     `gorm:"not null"`
	Version     int     `gorm:"not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// xpGrant → progression_svc.xp_grants
type xpGrant struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid;not null"`
	PlayerID       string `gorm:"type:uuid;not null"`
	IdempotencyKey string `gorm:"not null"`
	Amount         int64  `gorm:"not null"`
	Description    *string
	SourceKind     *string
	SourceID       *string
	ActivityID     *string
	OccurredAt     time.Time
	CreatedBy      *string `gorm:"type:uuid"`
	CreatedAt      time.Time
}

// levelReward → progression_svc.level_rewards
type levelReward struct {
	ID        string `gorm:"primaryKey;type:uuid"`
	TenantID  string `gorm:"type:uuid;not null"`
	PlayerID  string `gorm:"type:uuid;not null"`
	LevelID   string `gorm:"type:uuid;not null"`
	GrantedAt time.Time
}

// grantRejection → progression_svc.grant_rejections
type grantRejection struct {
	TenantID       string `gorm:"primaryKey;type:uuid"`
	IdempotencyKey string `gorm:"primaryKey"`
	PlayerID       string `gorm:"type:uuid;not null"`
	Reason         string `gorm:"not null"`
	CreatedAt      time.Time
}

// reconcileMarker → progression_svc.reconcile_markers
type reconcileMarker struct {
	ID      int `gorm:"primaryKey"`
	LastRun time.Time
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func levelFromDomain(l domain.Level) level {
	m := level{
		ID:            l.ID,
		TenantID:      l.TenantID,
		LevelNumber:   l.Number,
		Name:          l.Name,
		Description:   nullable(l.Description),
		XPRequired:    l.XPRequired,
		PointsReward:  l.PointsReward,
		BadgeRewardID: nullable(l.BadgeRewardID),
		IconURL:       nullable(l.IconURL),
		IsActive:      l.Active,
		CreatedAt:     l.CreatedAt,
		UpdatedAt:     l.UpdatedAt,
	}
	if l.Perks != nil {
		if raw, err := json.Marshal(l.Perks); err == nil {
			s := string(raw)
			m.Perks = &s
		}
	}
	return m
}

func (m level) toDomain() domain.Level {
	l := domain.Level{
		ID:            m.ID,
		TenantID:      m.TenantID,
		Number:        m.LevelNumber,
		Name:          m.Name,
		Description:   deref(m.Description),
		XPRequired:    m.XPRequired,
		PointsReward:  m.PointsReward,
		BadgeRewardID: deref(m.BadgeRewardID),
		IconURL:       deref(m.IconURL),
		Active:        m.IsActive,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
	}
	if m.Perks != nil {
		var perks map[string]any
		if json.Unmarshal([]byte(*m.Perks), &perks) == nil {
			l.Perks = perks
		}
	}
	return l
}

func progressFromDomain(p domain.Progress) playerProgress {
	return playerProgress{
		TenantID:    p.TenantID,
		PlayerID:    p.PlayerID,
		TotalXP:     p.TotalXP,
		LevelID:     nullable(p.LevelID),
		LevelNumber: p.LevelNumber,
		Version:     p.Version,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func (m playerProgress) toDomain() domain.Progress {
	return domain.Progress{
		TenantID:    m.TenantID,
		PlayerID:    m.PlayerID,
		TotalXP:     m.TotalXP,
		LevelID:     deref(m.LevelID),
		LevelNumber: m.LevelNumber,
		Version:     m.Version,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func grantFromDomain(g domain.XPGrant) xpGrant {
	return xpGrant{
		ID:             g.ID,
		TenantID:       g.TenantID,
		PlayerID:       g.PlayerID,
		IdempotencyKey: g.IdempotencyKey,
		Amount:         g.Amount,
		Description:    nullable(g.Description),
		SourceKind:     nullable(g.SourceKind),
		SourceID:       nullable(g.SourceID),
		ActivityID:     nullable(g.ActivityID),
		OccurredAt:     g.OccurredAt,
		CreatedBy:      nullable(g.CreatedBy),
		CreatedAt:      g.CreatedAt,
	}
}

func (m xpGrant) toDomain() domain.XPGrant {
	return domain.XPGrant{
		ID:             m.ID,
		TenantID:       m.TenantID,
		PlayerID:       m.PlayerID,
		IdempotencyKey: m.IdempotencyKey,
		Amount:         m.Amount,
		Description:    deref(m.Description),
		SourceKind:     deref(m.SourceKind),
		SourceID:       deref(m.SourceID),
		ActivityID:     deref(m.ActivityID),
		OccurredAt:     m.OccurredAt,
		CreatedBy:      deref(m.CreatedBy),
		CreatedAt:      m.CreatedAt,
	}
}
