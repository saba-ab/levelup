// Package repo implements streaks' persistence. Models stay unexported; no
// TableName() overrides: tables derive from struct names under the module
// prefix (streaks_svc.<plural>).
package repo

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"levelup/internal/modules/streaks/internal/domain"
)

// streak → streaks_svc.streaks
type streak struct {
	ID              string `gorm:"primaryKey;type:uuid"`
	TenantID        string `gorm:"type:uuid"`
	Slug            string
	Name            string
	Description     string
	ActivityKey     string
	Period          string
	GracePeriods    int
	PointsPerPeriod int64
	Milestones      milestoneList `gorm:"type:jsonb"`
	IsActive        bool
	AutoRecord      bool
	Version         int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}

type milestoneJSON struct {
	Count       int   `json:"count"`
	BonusPoints int64 `json:"bonus_points"`
}

// milestoneList is the JSONB milestones column.
type milestoneList []milestoneJSON

func (m milestoneList) Value() (driver.Value, error) {
	if m == nil {
		m = milestoneList{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (m *milestoneList) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case nil:
		*m = milestoneList{}
		return nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("milestones: unsupported type %T", src)
	}
	return json.Unmarshal(raw, m)
}

func (m streak) toDomain() domain.Streak {
	ms := make([]domain.Milestone, len(m.Milestones))
	for i, x := range m.Milestones {
		ms[i] = domain.Milestone{Count: x.Count, BonusPoints: x.BonusPoints}
	}
	return domain.Streak{
		ID:              m.ID,
		TenantID:        m.TenantID,
		Slug:            m.Slug,
		Name:            m.Name,
		Description:     m.Description,
		ActivityKey:     m.ActivityKey,
		Period:          domain.Period(m.Period),
		GracePeriods:    m.GracePeriods,
		PointsPerPeriod: m.PointsPerPeriod,
		Milestones:      ms,
		Active:          m.IsActive,
		AutoRecord:      m.AutoRecord,
		Version:         m.Version,
		CreatedAt:       m.CreatedAt.UTC(),
		UpdatedAt:       m.UpdatedAt.UTC(),
	}
}

func streakFromDomain(s domain.Streak) streak {
	ms := make(milestoneList, len(s.Milestones))
	for i, x := range s.Milestones {
		ms[i] = milestoneJSON{Count: x.Count, BonusPoints: x.BonusPoints}
	}
	return streak{
		ID:              s.ID,
		TenantID:        s.TenantID,
		Slug:            s.Slug,
		Name:            s.Name,
		Description:     s.Description,
		ActivityKey:     s.ActivityKey,
		Period:          string(s.Period),
		GracePeriods:    s.GracePeriods,
		PointsPerPeriod: s.PointsPerPeriod,
		Milestones:      ms,
		IsActive:        s.Active,
		AutoRecord:      s.AutoRecord,
		Version:         s.Version,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
}

// playerStreak → streaks_svc.player_streaks
type playerStreak struct {
	ID                 string `gorm:"primaryKey;type:uuid"`
	TenantID           string `gorm:"type:uuid"`
	PlayerID           string `gorm:"type:uuid"`
	StreakID           string `gorm:"type:uuid"`
	CurrentCount       int
	LongestCount       int
	LastPeriodStart    *time.Time
	RunStartedAt       *time.Time
	RunFloor           *time.Time
	BrokenAt           *time.Time
	BrokenRunStartedAt *time.Time
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (m playerStreak) toDomain() domain.PlayerStreak {
	return domain.PlayerStreak{
		ID:                 m.ID,
		TenantID:           m.TenantID,
		PlayerID:           m.PlayerID,
		StreakID:           m.StreakID,
		CurrentCount:       m.CurrentCount,
		LongestCount:       m.LongestCount,
		LastPeriodStart:    utcPtr(m.LastPeriodStart),
		RunStartedAt:       utcPtr(m.RunStartedAt),
		RunFloor:           utcPtr(m.RunFloor),
		BrokenAt:           utcPtr(m.BrokenAt),
		BrokenRunStartedAt: utcPtr(m.BrokenRunStartedAt),
		Version:            m.Version,
		CreatedAt:          m.CreatedAt.UTC(),
		UpdatedAt:          m.UpdatedAt.UTC(),
	}
}

func playerStreakFromDomain(ps domain.PlayerStreak) playerStreak {
	return playerStreak{
		ID:                 ps.ID,
		TenantID:           ps.TenantID,
		PlayerID:           ps.PlayerID,
		StreakID:           ps.StreakID,
		CurrentCount:       ps.CurrentCount,
		LongestCount:       ps.LongestCount,
		LastPeriodStart:    ps.LastPeriodStart,
		RunStartedAt:       ps.RunStartedAt,
		RunFloor:           ps.RunFloor,
		BrokenAt:           ps.BrokenAt,
		BrokenRunStartedAt: ps.BrokenRunStartedAt,
		Version:            ps.Version,
		CreatedAt:          ps.CreatedAt,
		UpdatedAt:          ps.UpdatedAt,
	}
}

// streakPeriod → streaks_svc.streak_periods
type streakPeriod struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid"`
	PlayerStreakID string `gorm:"type:uuid"`
	PeriodStart    time.Time
	IdempotencyKey string
	RecordedAt     time.Time
}

// streakMilestoneAward → streaks_svc.streak_milestone_awards
type streakMilestoneAward struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid"`
	PlayerStreakID string `gorm:"type:uuid"`
	Milestone      int
	BonusPoints    int64
	RunStartedAt   time.Time
	AwardedAt      time.Time
}

// recordRequest → streaks_svc.record_requests
type recordRequest struct {
	TenantID       string `gorm:"primaryKey;type:uuid"`
	IdempotencyKey string `gorm:"primaryKey"`
	PlayerID       string
	StreakID       string
	ActivityKey    string
	Status         string
	Reason         string
	CreatedAt      time.Time
}

// reconcileMarker → streaks_svc.reconcile_markers
type reconcileMarker struct {
	Job     string `gorm:"primaryKey"`
	LastRun time.Time
}
