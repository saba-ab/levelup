// Package contracts is program's public surface. A program is a campaign
// that players enrol in; status: draft → active ⇄ paused → ended.
package contracts

import (
	"context"
	"time"

	"levelup/internal/platform/authz"
)

const (
	TopicProgramCreated   = "program.created.v1"
	TopicProgramUpdated   = "program.updated.v1"
	TopicProgramActivated = "program.activated.v1"
	TopicProgramPaused    = "program.paused.v1"
	TopicProgramEnded     = "program.ended.v1"
	TopicProgramDeleted   = "program.deleted.v1"
	TopicPlayerEnrolled   = "program.player_enrolled.v1"
	TopicPlayerUnenrolled = "program.player_unenrolled.v1"
)

// JobAutoEnd is the reconciling cron that ends running programs whose
// ends_at has passed.
const JobAutoEnd = "program.auto_end"

// Program statuses.
const (
	StatusDraft  = "draft"
	StatusActive = "active"
	StatusPaused = "paused"
	StatusEnded  = "ended"
)

// ProgramChangedV1 is the payload of every program.* lifecycle topic.
type ProgramChangedV1 struct {
	ProgramID string     `json:"program_id"`
	TenantID  string     `json:"tenant_id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	StartsAt  *time.Time `json:"starts_at,omitempty"`
	EndsAt    *time.Time `json:"ends_at,omitempty"`
	At        time.Time  `json:"at"`
	// Additive fields.
	Slug           string   `json:"slug,omitempty"`
	PreviousStatus string   `json:"previous_status,omitempty"` // set on activated/paused/ended
	ChangedFields  []string `json:"changed_fields,omitempty"`  // set on updated
	AutoEnded      bool     `json:"auto_ended,omitempty"`      // ended by the program.auto_end sweep
}

// EnrollmentChangedV1 is the payload of player_enrolled / player_unenrolled.
type EnrollmentChangedV1 struct {
	ProgramID string    `json:"program_id"`
	TenantID  string    `json:"tenant_id"`
	PlayerID  string    `json:"player_id"`
	At        time.Time `json:"at"`
}

const Module = "program"

var (
	PermViewAny  = authz.Permission{Module: Module, Action: "view_any"}
	PermView     = authz.Permission{Module: Module, Action: "view"}
	PermCreate   = authz.Permission{Module: Module, Action: "create"}
	PermUpdate   = authz.Permission{Module: Module, Action: "update"}
	PermDelete   = authz.Permission{Module: Module, Action: "delete"}   // admin roles
	PermActivate = authz.Permission{Module: Module, Action: "activate"} // admin roles
	PermPause    = authz.Permission{Module: Module, Action: "pause"}    // admin roles
	PermEnd      = authz.Permission{Module: Module, Action: "end"}      // admin roles
	PermEnroll   = authz.Permission{Module: Module, Action: "enroll"}
)

var AllPermissions = []authz.Permission{
	PermViewAny, PermView, PermCreate, PermUpdate, PermDelete, PermActivate, PermPause, PermEnd, PermEnroll,
}

type ProgramSnapshot struct {
	ID       string
	TenantID string
	Name     string
	Status   string
	StartsAt *time.Time
	EndsAt   *time.Time
}

type Reader interface {
	ProgramsByIDs(ctx context.Context, tenantID string, ids []string) ([]ProgramSnapshot, error)
	// EnrolledProgramIDs returns the active programs a player is enrolled in.
	EnrolledProgramIDs(ctx context.Context, tenantID, playerID string) ([]string, error)
}
