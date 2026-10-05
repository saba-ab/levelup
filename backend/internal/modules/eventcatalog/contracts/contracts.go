// Package contracts is eventcatalog's public surface. An event type is a
// trigger definition (purchase_completed, login, ...) that rules react to.
// Rows with no tenant are platform-global and visible to every tenant.
package contracts

import (
	"context"
	"time"

	"levelup/internal/platform/authz"
)

const (
	TopicTypeCreated = "eventcatalog.type_created.v1"
	TopicTypeUpdated = "eventcatalog.type_updated.v1"
	TopicTypeDeleted = "eventcatalog.type_deleted.v1"
)

// EventTypeChangedV1 is the payload of all three topics. TenantID is empty
// for platform-global types.
type EventTypeChangedV1 struct {
	EventTypeID string    `json:"event_type_id"`
	TenantID    string    `json:"tenant_id,omitempty"`
	Slug        string    `json:"slug"`
	Active      bool      `json:"active"`
	At          time.Time `json:"at"`
}

const Module = "eventcatalog"

var (
	PermViewAny = authz.Permission{Module: Module, Action: "view_any"}
	PermView    = authz.Permission{Module: Module, Action: "view"}
	PermCreate  = authz.Permission{Module: Module, Action: "create"}
	PermUpdate  = authz.Permission{Module: Module, Action: "update"}
	PermDelete  = authz.Permission{Module: Module, Action: "delete"} // admin roles
	// Platform-level: global (tenant-less) types and categories.
	PermManageGlobal = authz.Permission{Module: Module, Action: "manage_global"}
)

var AllPermissions = []authz.Permission{PermViewAny, PermView, PermCreate, PermUpdate, PermDelete, PermManageGlobal}

type EventTypeSnapshot struct {
	ID       string
	TenantID string // "" = global
	Slug     string
	Name     string
	Active   bool
	// PropertySchema optionally describes expected activity properties
	// (JSON Schema subset); nil when the type is free-form.
	PropertySchema map[string]any
}

// Reader resolves the event types visible to a tenant (its own + global).
// A tenant type shadows a global one with the same slug.
type Reader interface {
	EventTypesBySlugs(ctx context.Context, tenantID string, slugs []string) ([]EventTypeSnapshot, error)
}
