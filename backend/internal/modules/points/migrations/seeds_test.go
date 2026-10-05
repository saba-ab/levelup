package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/points/contracts"
)

func TestManageWalletIsAdminOnly(t *testing.T) {
	grants := Grants()
	has := func(role int64, key string) bool {
		for _, p := range grants[role] {
			if p.Key() == key {
				return true
			}
		}
		return false
	}
	for _, r := range identitycontracts.AdminRoles {
		require.True(t, has(r, contracts.PermManageWallet.Key()))
	}
	require.False(t, has(identitycontracts.RoleProgramManager, contracts.PermManageWallet.Key()))
	require.False(t, has(identitycontracts.RoleDeveloper, contracts.PermManageWallet.Key()))
	require.False(t, has(identitycontracts.RolePlatformAdmin, contracts.PermCredit.Key()))
	for _, r := range identitycontracts.MemberRoles {
		require.True(t, has(r, contracts.PermCredit.Key()))
		require.True(t, has(r, contracts.PermViewWallet.Key()))
	}
	require.Len(t, Go(), 2)
}
