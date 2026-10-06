// Package repo is ai's Postgres persistence (schema ai_svc).
package repo

import (
	"time"

	"levelup/internal/modules/ai/internal/domain"
)

// aiUsage maps ai_svc.ai_usages (table name derived by GORM).
type aiUsage struct {
	TenantID     string    `gorm:"primaryKey;type:uuid"`
	Day          time.Time `gorm:"primaryKey;type:date"`
	Requests     int64
	InputTokens  int64
	OutputTokens int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (m aiUsage) toDomain() domain.UsageDay {
	return domain.UsageDay{
		Day:          domain.DayOf(m.Day),
		Requests:     m.Requests,
		InputTokens:  m.InputTokens,
		OutputTokens: m.OutputTokens,
	}
}
