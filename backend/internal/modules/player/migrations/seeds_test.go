package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/player/contracts"
	"levelup/internal/platform/authz"
)

func TestDefaultGrants(t *testing.T) {
	grants := DefaultGrants()
	has := func(role int64, p authz.Permission) bool {
		for _, g := range grants[role] {
			if g == p {
				return true
			}
		}
		return false
	}
	for _, role := range identitycontracts.MemberRoles {
		for _, p := range []authz.Permission{contracts.PermViewAny, contracts.PermView, contracts.PermCreate, contracts.PermUpdate} {
			require.True(t, has(role, p), "role %d needs %s", role, p.Key())
		}
	}
	for _, role := range identitycontracts.AdminRoles {
		require.True(t, has(role, contracts.PermDelete), "admin role %d may delete", role)
	}
	for _, role := range []int64{identitycontracts.RoleProgramManager, identitycontracts.RoleDeveloper} {
		require.False(t, has(role, contracts.PermDelete), "member role %d must not delete", role)
	}
	require.Empty(t, grants[identitycontracts.RolePlatformAdmin], "platform staff carry no tenant")
}

func TestGoMigrationVersionsDoNotCollideWithSQL(t *testing.T) {
	for _, m := range Go() {
		require.Greater(t, m.Version, int64(1))
	}
}
