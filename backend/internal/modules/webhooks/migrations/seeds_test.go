package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/webhooks/contracts"
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
	require.Equal(t, []string{"webhooks:view"}, keys(identitycontracts.RoleDeveloper))
	require.Equal(t, []string{"webhooks:view"}, keys(identitycontracts.RoleProgramManager))
	require.ElementsMatch(t, []string{"webhooks:view", "webhooks:manage"}, keys(identitycontracts.RoleAdmin))
	require.ElementsMatch(t, contracts.AllPermissions, grants[identitycontracts.RoleOwner])
	require.Empty(t, grants[identitycontracts.RolePlatformAdmin])
	require.Len(t, Go(), 2)
}
