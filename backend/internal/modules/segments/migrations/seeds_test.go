package migrations

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/segments/contracts"
)

func TestManageIsAdminOnlyAndViewIsMember(t *testing.T) {
	g := DefaultGrants()
	for _, role := range identitycontracts.MemberRoles {
		require.Contains(t, g[role], contracts.PermView)
	}
	admin := map[int64]bool{}
	for _, role := range identitycontracts.AdminRoles {
		admin[role] = true
		require.Contains(t, g[role], contracts.PermManage)
	}
	for _, role := range identitycontracts.MemberRoles {
		if !admin[role] {
			require.NotContains(t, g[role], contracts.PermManage)
		}
	}
	require.NotContains(t, g, identitycontracts.RolePlatformAdmin)
}

func TestGoMigrationVersionsDoNotCollideWithSQL(t *testing.T) {
	sqlFiles, err := fs.Glob(FS, "*.sql")
	require.NoError(t, err)
	require.Equal(t, []string{"0001_init.sql"}, sqlFiles)
	for _, m := range Go() {
		require.Greater(t, m.Version, int64(1))
	}
}
