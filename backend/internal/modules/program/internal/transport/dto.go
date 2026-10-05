package transport

import (
	"bytes"
	"encoding/json"
	"time"

	"levelup/internal/modules/program/internal/app"
	"levelup/internal/modules/program/internal/domain"
)

// CreateReq validates request shape only; invariants live in the domain.
type CreateReq struct {
	Name        string         `json:"name" validate:"required,max=255"`
	Slug        string         `json:"slug,omitempty" validate:"omitempty,max=120"`
	Description *string        `json:"description,omitempty" validate:"omitempty,max=1000"`
	StartsAt    *time.Time     `json:"starts_at,omitempty"`
	EndsAt      *time.Time     `json:"ends_at,omitempty"`
	Settings    map[string]any `json:"settings,omitempty"`
	Mechanics   map[string]any `json:"mechanics,omitempty"`
}

// PatchReq is a partial update: omitted fields stay untouched. description,
// starts_at and ends_at may be sent as null to clear them. There is no
// status field: an unknown field is rejected with 422, and status changes go
// through /activate, /pause and /end.
type PatchReq struct {
	Name        *string        `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Slug        *string        `json:"slug,omitempty" validate:"omitempty,min=1,max=120"`
	Description OptionalString `json:"description" swaggertype:"string"`
	StartsAt    OptionalTime   `json:"starts_at" swaggertype:"string" format:"date-time"`
	EndsAt      OptionalTime   `json:"ends_at" swaggertype:"string" format:"date-time"`
	Settings    map[string]any `json:"settings,omitempty"`
	Mechanics   map[string]any `json:"mechanics,omitempty"`
}

func (r PatchReq) toPatch() domain.Patch {
	return domain.Patch{
		Name:        r.Name,
		Slug:        r.Slug,
		Description: domain.Nullable[string](r.Description),
		StartsAt:    domain.Nullable[time.Time](r.StartsAt),
		EndsAt:      domain.Nullable[time.Time](r.EndsAt),
		Settings:    r.Settings,
		Mechanics:   r.Mechanics,
	}
}

// OptionalString distinguishes an absent key (Set=false) from null.
type OptionalString struct {
	Set   bool
	Value *string
}

func (o *OptionalString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

// OptionalTime distinguishes an absent key (Set=false) from null.
type OptionalTime struct {
	Set   bool
	Value *time.Time
}

func (o *OptionalTime) UnmarshalJSON(b []byte) error {
	o.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		o.Value = nil
		return nil
	}
	var v time.Time
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.Value = &v
	return nil
}

type EnrollReq struct {
	PlayerID string `json:"player_id" validate:"required,uuid"`
}

type ProgramResp struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Name        string         `json:"name"`
	Slug        string         `json:"slug"`
	Description *string        `json:"description"`
	Status      string         `json:"status"`
	StartsAt    *time.Time     `json:"starts_at"`
	EndsAt      *time.Time     `json:"ends_at"`
	Settings    map[string]any `json:"settings"`
	Mechanics   map[string]any `json:"mechanics"`
	Version     int            `json:"version"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type ProgramListResp struct {
	Data       []ProgramResp `json:"data"`
	NextCursor string        `json:"next_cursor"`
}

type EnrollmentResp struct {
	ProgramID  string    `json:"program_id"`
	PlayerID   string    `json:"player_id"`
	EnrolledAt time.Time `json:"enrolled_at"`
}

type PlayerResp struct {
	ExternalID  string `json:"external_id"`
	DisplayName string `json:"display_name"`
	Active      bool   `json:"active"`
}

type MemberResp struct {
	PlayerID   string      `json:"player_id"`
	EnrolledAt time.Time   `json:"enrolled_at"`
	Player     *PlayerResp `json:"player"`
}

type MemberListResp struct {
	Data       []MemberResp `json:"data"`
	NextCursor string       `json:"next_cursor"`
}

func toProgramResp(p domain.Program) ProgramResp {
	return ProgramResp{
		ID:          p.ID,
		TenantID:    p.TenantID,
		Name:        p.Name,
		Slug:        p.Slug,
		Description: p.Description,
		Status:      string(p.Status),
		StartsAt:    utc(p.StartsAt),
		EndsAt:      utc(p.EndsAt),
		Settings:    nonNil(p.Settings),
		Mechanics:   nonNil(p.Mechanics),
		Version:     p.Version,
		CreatedAt:   p.CreatedAt.UTC(),
		UpdatedAt:   p.UpdatedAt.UTC(),
	}
}

func toEnrollmentResp(e domain.Enrollment) EnrollmentResp {
	return EnrollmentResp{ProgramID: e.ProgramID, PlayerID: e.PlayerID, EnrolledAt: e.EnrolledAt.UTC()}
}

func toMemberResp(m app.Member) MemberResp {
	out := MemberResp{PlayerID: m.Enrollment.PlayerID, EnrolledAt: m.Enrollment.EnrolledAt.UTC()}
	if m.Player != nil {
		out.Player = &PlayerResp{
			ExternalID:  m.Player.ExternalID,
			DisplayName: m.Player.DisplayName,
			Active:      m.Player.Active,
		}
	}
	return out
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}

func nonNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
