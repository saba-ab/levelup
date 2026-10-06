package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/notifications/contracts"
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
	require.ElementsMatch(t, []string{
		contracts.PermViewAny.Key(), contracts.PermView.Key(),
		contracts.PermFeedView.Key(), contracts.PermFeedMarkRead.Key(),
	}, keys(identitycontracts.RoleDeveloper))
	require.ElementsMatch(t, contracts.AllPermissions, grants[identitycontracts.RoleAdmin])
	require.Empty(t, grants[identitycontracts.RolePlatformAdmin], "no platform-only permissions in notifications")
	require.Len(t, Go(), 2)
}
