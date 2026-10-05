package migrations

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/program/contracts"
	"levelup/internal/platform/authz"
)

func TestGrantsFollowAdminAndMemberRoles(t *testing.T) {
	grants := Grants()
	for _, p := range contracts.AllPermissions {
		admin := slices.Contains(AdminPermissions, p)
		for _, role := range identitycontracts.MemberRoles {
			want := !admin || slices.Contains(identitycontracts.AdminRoles, role)
			require.Equal(t, want, slices.Contains(grants[role], p), "role %d perm %s", role, p.Key())
		}
		require.NotContains(t, grants[identitycontracts.RolePlatformAdmin], p,
			"platform admins carry no tenant; tenant permissions are not theirs")
	}
	require.ElementsMatch(t,
		[]string{"program:delete", "program:activate", "program:pause", "program:end"},
		keys(AdminPermissions))
}

func keys(ps []authz.Permission) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Key()
	}
	return out
}
