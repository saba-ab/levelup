package migrations

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/leaderboards/contracts"
)

func TestGrantsSplitAdminAndMember(t *testing.T) {
	g := Grants()
	require.Len(t, g, len(contracts.AllPermissions))
	require.Equal(t, identitycontracts.MemberRoles, g[contracts.PermViewAny])
	require.Equal(t, identitycontracts.MemberRoles, g[contracts.PermView])
	require.Equal(t, identitycontracts.AdminRoles, g[contracts.PermCreate])
	require.Equal(t, identitycontracts.AdminRoles, g[contracts.PermUpdate])
	require.Equal(t, identitycontracts.AdminRoles, g[contracts.PermDelete])
	require.Equal(t, identitycontracts.AdminRoles, g[contracts.PermRebuild])
	require.NotContains(t, g[contracts.PermView], identitycontracts.RolePlatformAdmin)
}

func TestGoMigrationVersionsDoNotCollideWithSQL(t *testing.T) {
	sqlFiles, err := fs.Glob(FS, "*.sql")
	require.NoError(t, err)
	require.Equal(t, []string{"0001_init.sql", "0004_activity_type.sql"}, sqlFiles)
	sqlVersions := map[int64]bool{1: true, 4: true}
	for _, m := range Go() {
		require.Greater(t, m.Version, int64(1))
		require.False(t, sqlVersions[m.Version], "Go migration %d collides with a SQL file", m.Version)
	}
}
