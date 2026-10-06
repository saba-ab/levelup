// Package domain holds segments' invariants: the segment definition and
// its condition language.
package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"levelup/internal/shared/id"
)

// Segment is a tenant's named player set, defined by Conditions and
// materialized by refresh runs.
type Segment struct {
	ID              string
	TenantID        string
	Name            string
	Description     string
	Conditions      Group
	MemberCount     int
	LastRefreshedAt *time.Time
	// RefreshLeaseUntil is set while a refresh run holds the segment.
	RefreshLeaseUntil *time.Time
	RefreshRunID      string
	CreatedBy         string
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type NewSegmentInput struct {
	Name        string
	Description string
	Conditions  map[string]any
}

// SegmentPatch is a partial update: nil fields stay untouched.
type SegmentPatch struct {
	Name        *string
	Description *string
	Conditions  map[string]any // nil: unchanged
}

func NewSegment(tenantID, createdBy string, in NewSegmentInput, now time.Time) (Segment, error) {
	if tenantID == "" {
		return Segment{}, ErrNoTenant
	}
	s := Segment{
		ID:        id.NewID(),
		TenantID:  tenantID,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.setName(in.Name); err != nil {
		return Segment{}, err
	}
	if err := s.setDescription(in.Description); err != nil {
		return Segment{}, err
	}
	g, err := ParseConditions(in.Conditions)
	if err != nil {
		return Segment{}, err
	}
	s.Conditions = g
	return s, nil
}

// Apply changes the segment; it reports whether the conditions changed (the
// membership must then be recomputed).
func (s *Segment) Apply(p SegmentPatch, now time.Time) (bool, error) {
	if p.Name != nil {
		if err := s.setName(*p.Name); err != nil {
			return false, err
		}
	}
	if p.Description != nil {
		if err := s.setDescription(*p.Description); err != nil {
			return false, err
		}
	}
	changed := false
	if p.Conditions != nil {
		g, err := ParseConditions(p.Conditions)
		if err != nil {
			return false, err
		}
		s.Conditions = g
		changed = true
	}
	s.UpdatedAt = now
	return changed, nil
}

func (s *Segment) setName(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return ErrNameRequired
	case utf8.RuneCountInString(name) > 255:
		return ErrNameTooLong
	}
	s.Name = name
	return nil
}

func (s *Segment) setDescription(d string) error {
	if utf8.RuneCountInString(d) > 1000 {
		return ErrDescriptionTooLong
	}
	s.Description = d
	return nil
}

// Refreshing reports whether a refresh run holds the segment at now.
func (s Segment) Refreshing(now time.Time) bool {
	return s.RefreshLeaseUntil != nil && s.RefreshLeaseUntil.After(now)
}
