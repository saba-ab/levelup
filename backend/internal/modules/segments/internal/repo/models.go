// Package repo implements segments' persistence. Models stay unexported; no
// TableName() overrides: tables derive from struct names under the module
// prefix (segments_svc.<plural>).
package repo

import (
	"encoding/json"
	"time"

	"levelup/internal/modules/segments/internal/domain"
	"levelup/internal/shared/errs"
)

// segment → segments_svc.segments
type segment struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	TenantID          string `gorm:"type:uuid"`
	Name              string
	Description       string
	Conditions        string `gorm:"type:jsonb"`
	MemberCount       int
	LastRefreshedAt   *time.Time
	RefreshRunID      *string `gorm:"type:uuid"`
	RefreshLeaseUntil *time.Time
	CreatedBy         string
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (m segment) toDomain() (domain.Segment, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(m.Conditions), &raw); err != nil {
		return domain.Segment{}, errs.Wrap(errs.Internal, "decode segment conditions", err)
	}
	g, err := domain.ParseConditions(raw)
	if err != nil {
		return domain.Segment{}, errs.Wrap(errs.Internal, "stored segment conditions no longer parse", err)
	}
	run := ""
	if m.RefreshRunID != nil {
		run = *m.RefreshRunID
	}
	return domain.Segment{
		ID:                m.ID,
		TenantID:          m.TenantID,
		Name:              m.Name,
		Description:       m.Description,
		Conditions:        g,
		MemberCount:       m.MemberCount,
		LastRefreshedAt:   utcPtr(m.LastRefreshedAt),
		RefreshLeaseUntil: utcPtr(m.RefreshLeaseUntil),
		RefreshRunID:      run,
		CreatedBy:         m.CreatedBy,
		Version:           m.Version,
		CreatedAt:         m.CreatedAt.UTC(),
		UpdatedAt:         m.UpdatedAt.UTC(),
	}, nil
}

func segmentFromDomain(s domain.Segment) (segment, error) {
	cond, err := json.Marshal(s.Conditions.ToMap())
	if err != nil {
		return segment{}, errs.Wrap(errs.Internal, "encode segment conditions", err)
	}
	return segment{
		ID:          s.ID,
		TenantID:    s.TenantID,
		Name:        s.Name,
		Description: s.Description,
		Conditions:  string(cond),
		MemberCount: s.MemberCount,
		CreatedBy:   s.CreatedBy,
		Version:     s.Version,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}, nil
}
