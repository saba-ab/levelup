// Package contracts is player's public surface. Players are a tenant's end
// users, addressed by the tenant's own external_id.
package contracts

import (
	"context"
	"time"

	"levelup/internal/platform/authz"
)

const (
	TopicPlayerCreated     = "player.created.v1"
	TopicPlayerUpdated     = "player.updated.v1"
	TopicPlayerActivated   = "player.activated.v1"
	TopicPlayerDeactivated = "player.deactivated.v1"
	TopicPlayerDeleted     = "player.deleted.v1"
)

type PlayerCreatedV1 struct {
	PlayerID    string    `json:"player_id"`
	TenantID    string    `json:"tenant_id"`
	ExternalID  string    `json:"external_id"`
	DisplayName string    `json:"display_name"`
	At          time.Time `json:"at"`
}

type PlayerUpdatedV1 struct {
	PlayerID    string    `json:"player_id"`
	TenantID    string    `json:"tenant_id"`
	ExternalID  string    `json:"external_id"`
	DisplayName string    `json:"display_name"`
	At          time.Time `json:"at"`
	// ChangedFields names the profile fields this update touched
	// ("display_name", "email", "attributes", "is_active"). Additive.
	ChangedFields []string `json:"changed_fields,omitempty"`
	Active        bool     `json:"is_active"`
}

// PlayerStatusV1 is the payload of player.activated.v1 and player.deactivated.v1.
type PlayerStatusV1 struct {
	PlayerID string    `json:"player_id"`
	TenantID string    `json:"tenant_id"`
	Active   bool      `json:"active"`
	At       time.Time `json:"at"`
}

type PlayerDeletedV1 struct {
	PlayerID string    `json:"player_id"`
	TenantID string    `json:"tenant_id"`
	At       time.Time `json:"at"`
	// ExternalID lets consumers that key by the tenant's id drop their
	// projections without a lookup (the player is gone by then). Additive.
	ExternalID string `json:"external_id,omitempty"`
}

const Module = "player"

// Problem codes (ADR-0016) clients and peer modules may branch on.
const (
	CodePlayerNotFound        = "player_not_found"
	CodeExternalIDTaken       = "player_external_id_taken"
	CodePlayerVersionConflict = "player_version_conflict"
)

var (
	PermViewAny = authz.Permission{Module: Module, Action: "view_any"}
	PermView    = authz.Permission{Module: Module, Action: "view"}
	PermCreate  = authz.Permission{Module: Module, Action: "create"}
	PermUpdate  = authz.Permission{Module: Module, Action: "update"}
	PermDelete  = authz.Permission{Module: Module, Action: "delete"} // admin roles
)

var AllPermissions = []authz.Permission{PermViewAny, PermView, PermCreate, PermUpdate, PermDelete}

type PlayerSnapshot struct {
	ID          string
	TenantID    string
	ExternalID  string
	DisplayName string
	Email       string
	Active      bool
	Attributes  map[string]any // tenant-defined profile fields; rule conditions read them as player.*
	CreatedAt   time.Time
}

// Reader is offered to every mechanic. Batch methods from day one; an
// unknown id is simply absent from the result, never an error.
type Reader interface {
	PlayersByIDs(ctx context.Context, tenantID string, ids []string) ([]PlayerSnapshot, error)
	PlayersByExternalIDs(ctx context.Context, tenantID string, externalIDs []string) ([]PlayerSnapshot, error)
}
