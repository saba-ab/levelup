package authz_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"myapp/internal/platform/authz"
	authzmigrations "myapp/internal/platform/authz/migrations"
	"myapp/internal/platform/postgres"
	"myapp/internal/platform/postgres/pgtest"
	"myapp/internal/platform/redis"
	"myapp/internal/platform/redis/redistest"
	"myapp/internal/shared/errs"
)

func authzDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := pgtest.DSN(t)
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "authz", authzmigrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 8)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	return postgres.NewModuleDB(base, "authz")
}

func newEnforcer(t *testing.T, db *gorm.DB) *authz.Casbin {
	t.Helper()
	enf, cleanup, err := authz.NewCasbin(db, redis.NewCore(redistest.Addr(t)).UniversalClient, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(cleanup)
	return enf
}

var permView = authz.Permission{Module: "user", Action: "view_any"}

func TestRoleGrantAuthorizes(t *testing.T) {
	db := authzDB(t)
	enf := newEnforcer(t, db)
	ctx := context.Background()

	require.NoError(t, enf.Grant(1, permView))
	require.NoError(t, enf.AssignRole("u-allowed", 1))

	err := enf.Authorize(ctx, authz.Principal{UserID: "u-allowed"}, permView, nil)
	require.NoError(t, err, "role membership must authorize via the g-graph")

	err = enf.Authorize(ctx, authz.Principal{UserID: "u-other"}, permView, nil)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestRoleIDsInPrincipalAuthorize(t *testing.T) {
	db := authzDB(t)
	enf := newEnforcer(t, db)

	require.NoError(t, enf.Grant(7, permView))
	// Token carries role IDs; no g-row needed for this path.
	err := enf.Authorize(context.Background(), authz.Principal{UserID: "u-x", RoleIDs: []int64{7}}, permView, nil)
	require.NoError(t, err)
}

func TestVerifyCatalogueCatchesOrphans(t *testing.T) {
	db := authzDB(t)
	enf := newEnforcer(t, db)

	require.NoError(t, enf.Grant(2, authz.Permission{Module: "billing", Action: "charge"}))

	err := authz.VerifyCatalogue(enf, []authz.Permission{permView})
	require.Error(t, err, "a grant no enabled module declares must fail the boot (R19)")
	require.Contains(t, err.Error(), "billing:charge")

	require.NoError(t, enf.Revoke(2, authz.Permission{Module: "billing", Action: "charge"}))
	require.NoError(t, authz.VerifyCatalogue(enf, []authz.Permission{permView}))
}

// R19: a policy change through one replica reaches the other within 5s.
func TestWatcherPropagatesAcrossReplicas(t *testing.T) {
	db := authzDB(t)
	writer := newEnforcer(t, db)
	reader := newEnforcer(t, db) // second replica, same storage + channel
	ctx := context.Background()

	perm := authz.Permission{Module: "user", Action: "view_any"}
	principal := authz.Principal{UserID: "u-prop"}

	err := reader.Authorize(ctx, principal, perm, nil)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err), "starts denied")

	require.NoError(t, writer.Grant(9, perm))
	require.NoError(t, writer.AssignRole("u-prop", 9))

	require.Eventually(t, func() bool {
		return reader.Authorize(ctx, principal, perm, nil) == nil
	}, 5*time.Second, 100*time.Millisecond,
		"policy change must reach every replica within 5s via redis-watcher (R19)")
}
