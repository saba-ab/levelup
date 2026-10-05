package domain

import (
	"math"
	"slices"

	"levelup/internal/modules/identity/contracts"
)

// RoleScope separates the one platform role from the tenant roles.
type RoleScope string

const (
	ScopeTenant   RoleScope = "tenant"
	ScopePlatform RoleScope = "platform"
)

// Role is a catalogue row. IDs are contract (casbin subjects role:{id}); the
// 0001_init.sql seed mirrors this table exactly.
type Role struct {
	ID    int64
	Key   string
	Label string
	Scope RoleScope
}

var catalogue = map[int64]Role{
	contracts.RolePlatformAdmin:  {ID: contracts.RolePlatformAdmin, Key: "platform_admin", Label: "Platform Admin", Scope: ScopePlatform},
	contracts.RoleOwner:          {ID: contracts.RoleOwner, Key: "owner", Label: "Owner", Scope: ScopeTenant},
	contracts.RoleSuperAdmin:     {ID: contracts.RoleSuperAdmin, Key: "super_admin", Label: "Super Admin", Scope: ScopeTenant},
	contracts.RoleAdmin:          {ID: contracts.RoleAdmin, Key: "admin", Label: "Admin", Scope: ScopeTenant},
	contracts.RoleProgramManager: {ID: contracts.RoleProgramManager, Key: "program_manager", Label: "Program Manager", Scope: ScopeTenant},
	contracts.RoleDeveloper:      {ID: contracts.RoleDeveloper, Key: "developer", Label: "Developer", Scope: ScopeTenant},
}

// RoleByID returns the catalogue row for id.
func RoleByID(id int64) (Role, bool) {
	r, ok := catalogue[id]
	return r, ok
}

// Roles maps ids to catalogue rows, skipping unknown ids.
func Roles(ids []int64) []Role {
	out := make([]Role, 0, len(ids))
	for _, id := range ids {
		if r, ok := catalogue[id]; ok {
			out = append(out, r)
		}
	}
	return out
}

// noRank is the rank of a principal holding no tenant role: it outranks
// nobody and can grant nothing.
const noRank = int64(math.MaxInt64)

// rank is the best (numerically lowest) tenant role id held. The tenant
// role ids are ordered by seniority: owner(2) > super_admin(3) > admin(4) >
// program_manager(5) > developer(6).
func rank(roleIDs []int64) int64 {
	best := noRank
	for _, id := range roleIDs {
		if r, ok := catalogue[id]; ok && r.Scope == ScopeTenant && id < best {
			best = id
		}
	}
	return best
}

// NormalizeRoleIDs sorts and de-duplicates.
func NormalizeRoleIDs(ids []int64) []int64 {
	out := slices.Clone(ids)
	slices.Sort(out)
	return slices.Compact(out)
}

// CheckGrantable decides whether a caller holding callerRoles may hand out
// requested inside a tenant: never the platform role, only catalogue tenant
// roles, and never a role senior to the caller's own.
func CheckGrantable(callerRoles, requested []int64) error {
	callerRank := rank(callerRoles)
	for _, id := range requested {
		if id == contracts.RolePlatformAdmin {
			return ErrPlatformRoleForbidden
		}
		r, ok := catalogue[id]
		if !ok || r.Scope != ScopeTenant {
			return ErrUnknownRole
		}
		if id < callerRank {
			return ErrRoleEscalation
		}
	}
	return nil
}

// CheckCanManage refuses acting on a user senior to the caller.
func CheckCanManage(callerRoles, targetRoles []int64) error {
	if rank(targetRoles) < rank(callerRoles) {
		return ErrInsufficientRank
	}
	return nil
}
