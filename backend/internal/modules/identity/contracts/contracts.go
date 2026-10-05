// Package contracts is identity's public surface: topics, event payloads,
// the role catalogue, permissions and the Reader other modules use to learn
// about tenants. Data only (PRD §4 P2).
package contracts

import (
	"context"
	"time"

	"levelup/internal/platform/authz"
)

// Topics. Identity publishes entity-named topics (tenant.*, user.*), the
// way the blueprint's reference user module did.
const (
	TopicTenantCreated    = "tenant.created.v1"
	TopicTenantUpdated    = "tenant.updated.v1"
	TopicTenantDeleted    = "tenant.deleted.v1"
	TopicUserRegistered   = "user.registered.v1"
	TopicUserCreated      = "user.created.v1"
	TopicUserUpdated      = "user.updated.v1"
	TopicUserDeleted      = "user.deleted.v1"
	TopicUserRolesChanged = "user.roles_changed.v1"
)

type TenantCreatedV1 struct {
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	OwnerUserID string    `json:"owner_user_id"`
	Timezone    string    `json:"timezone"`
	At          time.Time `json:"at"`
}

type TenantUpdatedV1 struct {
	TenantID string    `json:"tenant_id"`
	Name     string    `json:"name"`
	Slug     string    `json:"slug"`
	Active   bool      `json:"active"`
	Timezone string    `json:"timezone"`
	At       time.Time `json:"at"`
}

// TenantDeletedV1 replaces the database cascades of the Laravel schema:
// every module purges its rows for TenantID, idempotently.
type TenantDeletedV1 struct {
	TenantID string    `json:"tenant_id"`
	At       time.Time `json:"at"`
}

type UserRegisteredV1 struct {
	UserID   string    `json:"user_id"`
	TenantID string    `json:"tenant_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	At       time.Time `json:"at"`
}

type UserCreatedV1 struct {
	UserID    string    `json:"user_id"`
	TenantID  string    `json:"tenant_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	RoleIDs   []int64   `json:"role_ids"`
	CreatedBy string    `json:"created_by"`
	At        time.Time `json:"at"`
}

type UserUpdatedV1 struct {
	UserID   string    `json:"user_id"`
	TenantID string    `json:"tenant_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Active   bool      `json:"active"`
	At       time.Time `json:"at"`
}

type UserDeletedV1 struct {
	UserID   string    `json:"user_id"`
	TenantID string    `json:"tenant_id"`
	At       time.Time `json:"at"`
}

type UserRolesChangedV1 struct {
	UserID   string    `json:"user_id"`
	TenantID string    `json:"tenant_id"`
	RoleIDs  []int64   `json:"role_ids"`
	At       time.Time `json:"at"`
}

// Role catalogue. IDs are stable forever: casbin subjects are role:{id}
// and every module's permission seed grants to these ids (ADR-0011).
const (
	RolePlatformAdmin  int64 = 1 // platform staff; carries no tenant
	RoleOwner          int64 = 2
	RoleSuperAdmin     int64 = 3
	RoleAdmin          int64 = 4
	RoleProgramManager int64 = 5
	RoleDeveloper      int64 = 6
)

// AdminRoles may configure a tenant's mechanics (create/update/delete
// definitions). MemberRoles are every tenant role; they may operate them.
var (
	AdminRoles  = []int64{RoleOwner, RoleSuperAdmin, RoleAdmin}
	MemberRoles = []int64{RoleOwner, RoleSuperAdmin, RoleAdmin, RoleProgramManager, RoleDeveloper}
)

const Module = "identity"

// Default grants (identity/migrations.DefaultGrants): unmarked permissions go
// to MemberRoles, "admin roles" to AdminRoles, tenant_delete to RoleOwner.
var (
	PermUsersViewAny     = authz.Permission{Module: Module, Action: "users_view_any"}
	PermUsersCreate      = authz.Permission{Module: Module, Action: "users_create"}       // admin roles
	PermUsersUpdate      = authz.Permission{Module: Module, Action: "users_update"}       // admin roles
	PermUsersDelete      = authz.Permission{Module: Module, Action: "users_delete"}       // admin roles
	PermUsersAssignRoles = authz.Permission{Module: Module, Action: "users_assign_roles"} // admin roles
	PermTenantView       = authz.Permission{Module: Module, Action: "tenant_view"}
	PermTenantUpdate     = authz.Permission{Module: Module, Action: "tenant_update"} // admin roles
	PermTenantDelete     = authz.Permission{Module: Module, Action: "tenant_delete"} // owner role only
	// Platform-level: only RolePlatformAdmin, on /platform routes.
	PermPlatformTenantsManage = authz.Permission{Module: Module, Action: "platform_tenants_manage"}
	// Create, list and revoke API keys (admin roles; humans only, ADR-0017).
	PermAPIKeysManage = authz.Permission{Module: Module, Action: "api_keys_manage"}
)

var AllPermissions = []authz.Permission{
	PermUsersViewAny, PermUsersCreate, PermUsersUpdate, PermUsersDelete, PermUsersAssignRoles,
	PermTenantView, PermTenantUpdate, PermTenantDelete, PermPlatformTenantsManage,
	PermAPIKeysManage,
}

// TenantSnapshot is what other modules may know about a tenant.
type TenantSnapshot struct {
	ID       string
	Name     string
	Slug     string
	Active   bool
	Timezone string // IANA name, e.g. "Asia/Tbilisi"; streak periods use it
}

// TenantReader is offered to other modules (consumer-defined ports wrap it).
type TenantReader interface {
	TenantsByIDs(ctx context.Context, ids []string) ([]TenantSnapshot, error)
}
