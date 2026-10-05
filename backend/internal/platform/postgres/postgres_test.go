package postgres_test

import (
	"context"
	"embed"
	"io/fs"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
)

//go:embed testdata/migrations/*.sql
var testMigrations embed.FS

func migrationsFS(t *testing.T) fs.FS {
	sub, err := fs.Sub(testMigrations, "testdata/migrations")
	require.NoError(t, err)
	return sub
}

func TestApplyCreatesSchemaAndIsIdempotent(t *testing.T) {
	dsn := pgtest.DSN(t)
	ctx := context.Background()

	require.NoError(t, postgres.Apply(ctx, dsn, "demo", migrationsFS(t)))
	// Second run must be a clean no-op.
	require.NoError(t, postgres.Apply(ctx, dsn, "demo", migrationsFS(t)))

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'demo_svc' AND table_name = 'items'`).Scan(&n))
	require.Equal(t, 1, n, "migration table lands in the module schema")

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM demo_svc.goose_db_version`).Scan(&n))
	require.Positive(t, n, "per-module goose version table (R3)")
}

func TestModuleRoleCannotTouchOtherSchemas(t *testing.T) {
	dsn := pgtest.DSN(t)
	ctx := context.Background()

	require.NoError(t, postgres.Apply(ctx, dsn, "alpha", migrationsFS(t)))
	require.NoError(t, postgres.Apply(ctx, dsn, "beta", migrationsFS(t)))
	require.NoError(t, postgres.CreateModuleRole(ctx, dsn, "alpha"))

	roleDSN := postgres.ModuleRoleDSN(dsn, "alpha")
	pool, err := pgxpool.New(ctx, roleDSN)
	require.NoError(t, err)
	defer pool.Close()

	// Own schema: unqualified name resolves via search_path and is readable.
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM items`).Scan(&n))

	// Foreign schema: Postgres itself rejects the read (PRD §4 P4).
	_, err = pool.Exec(ctx, `SELECT count(*) FROM beta_svc.items`)
	require.Error(t, err, "cross-module access must be rejected by the database")
	require.Contains(t, err.Error(), "permission denied")
}

func TestNewPoolsPingAndSplit(t *testing.T) {
	dsn := pgtest.DSN(t)

	db, cleanup, err := postgres.NewFromDSNs(context.Background(), dsn, dsn, 4)
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, db.Writer().Ping(context.Background()))
	require.NoError(t, db.Reader().Ping(context.Background()))
}

func TestGormBaseConnectsThroughSharedPool(t *testing.T) {
	dsn := pgtest.DSN(t)

	db, cleanup, err := postgres.NewFromDSNs(context.Background(), dsn, "", 4)
	require.NoError(t, err)
	defer cleanup()

	gdb, err := db.GormBase(nil)
	require.NoError(t, err)

	var one int
	require.NoError(t, gdb.Raw("SELECT 1").Scan(&one).Error)
	require.Equal(t, 1, one)
}

func TestNewModuleDBPrefixesTables(t *testing.T) {
	dsn := pgtest.DSN(t)
	ctx := context.Background()

	require.NoError(t, postgres.Apply(ctx, dsn, "gamma", migrationsFS(t)))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	defer cleanup()

	base, err := db.GormBase(nil)
	require.NoError(t, err)

	type item struct {
		ID   int64 `gorm:"primaryKey"`
		Name string
	}

	gammaDB := postgres.NewModuleDB(base, "gamma")
	require.NoError(t, gammaDB.Create(&item{Name: "in-gamma"}).Error)

	// The same model through a differently-prefixed session must fail:
	// delta_svc.items does not exist. This is §7.1 Mechanism 1 working.
	deltaDB := postgres.NewModuleDB(base, "delta")
	err = deltaDB.Create(&item{Name: "in-delta"}).Error
	require.Error(t, err)
	require.Contains(t, err.Error(), "delta_svc")
}
