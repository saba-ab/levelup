// Package repo implements badges' persistence. Models are unexported; table
// names derive from struct names under the badges_svc. prefix (NO
// TableName() overrides — the arch test bans them).
package repo

import (
	"encoding/json"
	"time"

	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/shared/effect"
)

// badge → badges_svc.badges.
type badge struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	TenantID     string `gorm:"type:uuid;not null"`
	Slug         string `gorm:"not null"`
	Name         string `gorm:"not null"`
	Description  string `gorm:"not null"`
	IconURL      string `gorm:"column:icon_url;not null"`
	Tier         string `gorm:"not null"`
	Category     string `gorm:"not null"`
	PointsValue  int64  `gorm:"not null"`
	IsStackable  bool   `gorm:"not null"`
	MaxAwards    *int
	Requirements *string `gorm:"type:jsonb"`
	IsActive     bool    `gorm:"not null"`
	IsSecret     bool    `gorm:"not null"`
	SortOrder    int     `gorm:"not null"`
	Version      int     `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

func badgeFromDomain(b domain.Badge) (badge, error) {
	m := badge{
		ID:          b.ID,
		TenantID:    b.TenantID,
		Slug:        b.Slug,
		Name:        b.Name,
		Description: b.Description,
		IconURL:     b.IconURL,
		Tier:        string(b.Tier),
		Category:    string(b.Category),
		PointsValue: b.PointsValue,
		IsStackable: b.Stackable,
		MaxAwards:   b.MaxAwards,
		IsActive:    b.Active,
		IsSecret:    b.Secret,
		SortOrder:   b.SortOrder,
		Version:     b.Version,
		CreatedAt:   b.CreatedAt,
		UpdatedAt:   b.UpdatedAt,
		DeletedAt:   b.DeletedAt,
	}
	if b.Requirements != nil {
		raw, err := json.Marshal(b.Requirements)
		if err != nil {
			return badge{}, err
		}
		s := string(raw)
		m.Requirements = &s
	}
	return m, nil
}

func (m badge) toDomain() domain.Badge {
	b := domain.Badge{
		ID:          m.ID,
		TenantID:    m.TenantID,
		Slug:        m.Slug,
		Name:        m.Name,
		Description: m.Description,
		IconURL:     m.IconURL,
		Tier:        domain.Tier(m.Tier),
		Category:    domain.Category(m.Category),
		PointsValue: m.PointsValue,
		Stackable:   m.IsStackable,
		MaxAwards:   m.MaxAwards,
		Active:      m.IsActive,
		Secret:      m.IsSecret,
		SortOrder:   m.SortOrder,
		Version:     m.Version,
		CreatedAt:   m.CreatedAt.UTC(),
		UpdatedAt:   m.UpdatedAt.UTC(),
	}
	if m.DeletedAt != nil {
		t := m.DeletedAt.UTC()
		b.DeletedAt = &t
	}
	if m.Requirements != nil {
		var req map[string]any
		if json.Unmarshal([]byte(*m.Requirements), &req) == nil {
			b.Requirements = req
		}
	}
	return b
}

// playerBadge → badges_svc.player_badges.
type playerBadge struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid;not null"`
	PlayerID       string `gorm:"type:uuid;not null"` // bare uuid, no FK to player_svc
	BadgeID        string `gorm:"type:uuid;not null"`
	EarnedCount    int    `gorm:"not null"`
	FirstAwardedAt time.Time
	LastAwardedAt  time.Time
	Version        int `gorm:"not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func playerBadgeFromDomain(pb domain.PlayerBadge) playerBadge {
	return playerBadge{
		ID:             pb.ID,
		TenantID:       pb.TenantID,
		PlayerID:       pb.PlayerID,
		BadgeID:        pb.BadgeID,
		EarnedCount:    pb.EarnedCount,
		FirstAwardedAt: pb.FirstAwardedAt,
		LastAwardedAt:  pb.LastAwardedAt,
		Version:        pb.Version,
		CreatedAt:      pb.CreatedAt,
		UpdatedAt:      pb.UpdatedAt,
	}
}

func (m playerBadge) toDomain() domain.PlayerBadge {
	return domain.PlayerBadge{
		ID:             m.ID,
		TenantID:       m.TenantID,
		PlayerID:       m.PlayerID,
		BadgeID:        m.BadgeID,
		EarnedCount:    m.EarnedCount,
		FirstAwardedAt: m.FirstAwardedAt.UTC(),
		LastAwardedAt:  m.LastAwardedAt.UTC(),
		Version:        m.Version,
		CreatedAt:      m.CreatedAt.UTC(),
		UpdatedAt:      m.UpdatedAt.UTC(),
	}
}

// badgeAward → badges_svc.badge_awards (append-only ledger).
type badgeAward struct {
	ID             string  `gorm:"primaryKey;type:uuid"`
	TenantID       string  `gorm:"type:uuid;not null"`
	PlayerID       string  `gorm:"type:uuid;not null"`
	BadgeID        string  `gorm:"type:uuid;not null"`
	PlayerBadgeID  *string `gorm:"type:uuid"`
	IdempotencyKey string  `gorm:"not null"`
	SourceKind     string  `gorm:"not null"`
	SourceID       string  `gorm:"not null"`
	ActivityID     string  `gorm:"not null"`
	AwardedBy      *string `gorm:"type:uuid"`
	EarnedCount    int     `gorm:"not null"`
	OccurredAt     time.Time
	Status         string `gorm:"not null"`
	Reason         string `gorm:"not null"`
	CreatedAt      time.Time
}

func awardFromDomain(a domain.Award) badgeAward {
	return badgeAward{
		ID:             a.ID,
		TenantID:       a.TenantID,
		PlayerID:       a.PlayerID,
		BadgeID:        a.BadgeID,
		PlayerBadgeID:  nullable(a.PlayerBadgeID),
		IdempotencyKey: a.IdempotencyKey,
		SourceKind:     a.Source.Kind,
		SourceID:       a.Source.ID,
		ActivityID:     a.Source.ActivityID,
		AwardedBy:      nullable(a.AwardedBy),
		EarnedCount:    a.EarnedCount,
		OccurredAt:     a.OccurredAt,
		Status:         string(a.Status),
		Reason:         a.Reason,
		CreatedAt:      a.CreatedAt,
	}
}

func (m badgeAward) toDomain() domain.Award {
	return domain.Award{
		ID:             m.ID,
		TenantID:       m.TenantID,
		PlayerID:       m.PlayerID,
		BadgeID:        m.BadgeID,
		PlayerBadgeID:  deref(m.PlayerBadgeID),
		IdempotencyKey: m.IdempotencyKey,
		Source:         effect.Source{Kind: m.SourceKind, ID: m.SourceID, ActivityID: m.ActivityID},
		AwardedBy:      deref(m.AwardedBy),
		EarnedCount:    m.EarnedCount,
		OccurredAt:     m.OccurredAt.UTC(),
		Status:         domain.AwardStatus(m.Status),
		Reason:         m.Reason,
		CreatedAt:      m.CreatedAt.UTC(),
	}
}

// badgeRevocation → badges_svc.badge_revocations.
type badgeRevocation struct {
	ID                 string  `gorm:"primaryKey;type:uuid"`
	TenantID           string  `gorm:"type:uuid;not null"`
	PlayerID           string  `gorm:"type:uuid;not null"`
	BadgeID            string  `gorm:"type:uuid;not null"`
	PlayerBadgeID      string  `gorm:"type:uuid;not null"`
	EarnedCountRemoved int     `gorm:"not null"`
	RevokedBy          *string `gorm:"type:uuid"`
	RevokedAt          time.Time
}

// reconcileMarker → badges_svc.reconcile_markers.
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

// badgePlayerStat → badges_svc.badge_player_stats (requirements projection).
type badgePlayerStat struct {
	TenantID          string `gorm:"primaryKey;type:uuid"`
	PlayerID          string `gorm:"primaryKey;type:uuid"`
	LifetimePoints    int64
	LifetimePointsAt  *time.Time
	MissionsCompleted int64
	MaxStreak         int64
	Level             int64
	BadgesEarned      int64
	UpdatedAt         time.Time
}

// badgePlayerActivityCount → badges_svc.badge_player_activity_counts.
type badgePlayerActivityCount struct {
	TenantID  string `gorm:"primaryKey;type:uuid"`
	PlayerID  string `gorm:"primaryKey;type:uuid"`
	EventType string `gorm:"primaryKey"`
	Count     int64
	UpdatedAt time.Time
}

// appliedEvent → badges_svc.applied_events (projection dedupe keys).
type appliedEvent struct {
	TenantID  string `gorm:"primaryKey;type:uuid"`
	EventKey  string `gorm:"primaryKey"`
	AppliedAt time.Time
}
