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
	// AutoCreated marks a player created by the system from an
	// activity.received.v1 flagged auto_create_player (created_by is null);
	// SourceActivityID names that activity. Additive.
	AutoCreated      bool   `json:"auto_created,omitempty"`
	SourceActivityID string `json:"source_activity_id,omitempty"`
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
	CodeInvalidSort           = "invalid_sort"
	CodeInvalidCreatedRange   = "invalid_created_range"
)

// List sort orders accepted by GET /players?sort=. Each is keyset-paged on
// a unique total order (the id breaks ties).
const (
	SortCreatedDesc = "-created_at" // default
	SortCreatedAsc  = "created_at"
	// SortDisplayName orders by lower(display_name), falling back to the
	// external id when no display name is set, ascending.
	SortDisplayName = "display_name"
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

// IDLister pages a tenant's live player ids in ascending id order, strictly
// after afterID ("" starts at the beginning). A short page is the last one.
// It is separate from Reader so existing Reader fakes stay valid.
type IDLister interface {
	ListPlayerIDs(ctx context.Context, tenantID, afterID string, limit int) ([]string, error)
}
