package migrations

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/analytics/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
)

func TestViewIsGrantedToEveryMemberRole(t *testing.T) {
	g := DefaultGrants()
	for _, role := range identitycontracts.MemberRoles {
		require.Contains(t, g[role], contracts.PermView)
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
