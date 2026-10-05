package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/badges/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
)

func TestDefaultGrants(t *testing.T) {
	grants := DefaultGrants()
	keys := func(role int64) []string {
		var out []string
		for _, p := range grants[role] {
			out = append(out, p.Key())
		}
		return out
	}
	member := keys(identitycontracts.RoleDeveloper)
	require.ElementsMatch(t, []string{contracts.PermViewAny.Key(), contracts.PermView.Key(), contracts.PermAward.Key()}, member)
	require.ElementsMatch(t, contracts.AllPermissions, grants[identitycontracts.RoleAdmin])
	require.Empty(t, grants[identitycontracts.RolePlatformAdmin], "no platform-only permissions in badges")
	require.Len(t, Go(), 2)
}
