package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/eventcatalog/contracts"
	"levelup/internal/modules/eventcatalog/internal/app"
	"levelup/internal/modules/eventcatalog/internal/domain"
	"levelup/internal/modules/eventcatalog/migrations"
	authzmigrations "levelup/internal/platform/authz/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

const (
	tenantA = "0199a000-0000-7000-8000-00000000000a"
	tenantB = "0199a000-0000-7000-8000-00000000000b"
)

// setupRepo applies the authz schema and every eventcatalog migration
// (SQL + Go seeds) into a throwaway Postgres. Skipped under -short.
func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t)
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "authz", authzmigrations.FS))
	require.NoError(t, postgres.Apply(ctx, dsn, "eventcatalog", migrations.FS, migrations.Go()...))
	// Re-applying is a clean no-op (seeds are idempotent and versioned).
	require.NoError(t, postgres.Apply(ctx, dsn, "eventcatalog", migrations.FS, migrations.Go()...))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "eventcatalog")

	// Each test starts from the seeded catalogue only.
	require.NoError(t, moduleDB.Exec(`DELETE FROM eventcatalog_svc.event_types WHERE tenant_id IS NOT NULL`).Error)
	require.NoError(t, moduleDB.Exec(`DELETE FROM eventcatalog_svc.event_categories WHERE tenant_id IS NOT NULL`).Error)
	return NewPostgres(moduleDB), moduleDB
}

func newType(t *testing.T, tenantID, slug string, at time.Time) domain.EventType {
	t.Helper()
	et, err := domain.CreateEventType(domain.NewEventType{TenantID: tenantID, Name: slug, Slug: slug, Active: true}, at)
	require.NoError(t, err)
	return et
}

func create(t *testing.T, r *Postgres, db *gorm.DB, et domain.EventType) error {
	t.Helper()
	return postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		return r.CreateType(context.Background(), tx, et)
	})
}

func TestSeedsGlobalCatalogueAndGrants(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()

	got, err := r.TypesBySlugs(ctx, app.Scope{}, []string{
		"purchase_completed", "user_signup", "user_login", "referral_completed", "subscription_upgraded",
		"task_completed", "achievement_unlocked", "level_up", "badge_earned", "mission_completed",
	})
	require.NoError(t, err)
	require.Len(t, got, 10)
	for _, et := range got {
		require.Equal(t, migrations.EventTypeID(et.Slug), et.ID, "deterministic id")
		require.True(t, et.IsGlobal())
		require.True(t, et.Active)
		require.NotEmpty(t, et.CategoryID)
	}

	cs, err := r.ListCategories(ctx, app.Scope{}, 100)
	require.NoError(t, err)
	require.Len(t, cs, len(migrations.GlobalCategories))

	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM authz_svc.casbin_rule WHERE v0 = 'role:1' AND v1 LIKE 'eventcatalog:%'`).Scan(&n).Error)
	require.EqualValues(t, 1, n, "platform admin holds manage_global only")
	require.NoError(t, db.Raw(`SELECT count(*) FROM authz_svc.casbin_rule WHERE v1 = ?`, contracts.PermManageGlobal.Key()).Scan(&n).Error)
	require.EqualValues(t, 1, n, "manage_global is granted to no tenant role")
	require.NoError(t, db.Raw(`SELECT count(*) FROM authz_svc.casbin_rule WHERE v1 = ?`, contracts.PermDelete.Key()).Scan(&n).Error)
	require.EqualValues(t, 3, n, "delete goes to the three admin roles")
}

func TestSlugUniquenessPerScopeAndSoftDelete(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	first := newType(t, tenantA, "checkout", now)
	require.NoError(t, create(t, r, db, first))
	require.ErrorIs(t, create(t, r, db, newType(t, tenantA, "checkout", now)), domain.ErrSlugTaken)
	require.NoError(t, create(t, r, db, newType(t, tenantB, "checkout", now)), "per tenant")
	require.NoError(t, create(t, r, db, newType(t, tenantA, "level_up", now)), "may shadow a global slug")
	require.ErrorIs(t, create(t, r, db, newType(t, "", "level_up", now)), domain.ErrSlugTaken, "unique among globals")

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		return r.SoftDeleteType(ctx, tx, first.ID, now)
	}))
	require.NoError(t, create(t, r, db, newType(t, tenantA, "checkout", now)), "a soft-deleted slug is free")
}

func TestScopingShadowingAndKeyset(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Microsecond)

	own := newType(t, tenantA, "user_login", base)
	require.NoError(t, create(t, r, db, own))
	foreign := newType(t, tenantB, "theirs", base.Add(time.Second))
	require.NoError(t, create(t, r, db, foreign))

	_, err := r.TypeByID(ctx, app.Scope{TenantID: tenantA}, foreign.ID)
	require.ErrorIs(t, err, domain.ErrNotFound, "cross-tenant read is NotFound")
	_, err = r.TypeByID(ctx, app.Scope{}, own.ID)
	require.ErrorIs(t, err, domain.ErrNotFound, "platform scope sees globals only")
	g, err := r.TypeByID(ctx, app.Scope{TenantID: tenantA}, migrations.EventTypeID("level_up"))
	require.NoError(t, err, "globals are visible to tenants")
	require.True(t, g.IsGlobal())

	all, err := r.ListTypes(ctx, app.TypeFilter{Scope: app.Scope{TenantID: tenantA}, IncludeGlobal: true, Limit: 100})
	require.NoError(t, err)
	require.Len(t, all, 10, "9 unshadowed globals + the own user_login")
	for _, et := range all {
		require.NotEqual(t, migrations.EventTypeID("user_login"), et.ID, "shadowed global hidden")
		require.NotEqual(t, foreign.ID, et.ID)
	}

	// Keyset: page through with limit 3, no repeats, newest first.
	seen := map[string]bool{}
	f := app.TypeFilter{Scope: app.Scope{TenantID: tenantA}, IncludeGlobal: true, Limit: 3}
	for {
		page, err := r.ListTypes(ctx, f)
		require.NoError(t, err)
		for _, et := range page {
			require.False(t, seen[et.ID])
			seen[et.ID] = true
		}
		if len(page) < f.Limit {
			break
		}
		last := page[len(page)-1]
		f.AfterTime, f.AfterID = last.CreatedAt, last.ID
	}
	require.Len(t, seen, 10)

	bySlug, err := r.TypesBySlugs(ctx, app.Scope{TenantID: tenantA}, []string{"user_login", "theirs"})
	require.NoError(t, err)
	require.Len(t, bySlug, 2, "own user_login and the global one; never the foreign row")
	require.Equal(t, own.ID, domain.ResolveBySlug(tenantA, bySlug)["user_login"].ID)
}

func TestPropertySchemaRoundTripAndPatch(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	et, err := domain.CreateEventType(domain.NewEventType{
		TenantID: tenantA, Name: "Buy", Active: true,
		PropertySchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"amount": map[string]any{"type": "number"}},
		},
	}, now)
	require.NoError(t, err)
	require.NoError(t, create(t, r, db, et))

	got, err := r.TypeByID(ctx, app.Scope{TenantID: tenantA}, et.ID)
	require.NoError(t, err)
	require.Equal(t, et.PropertySchema, got.PropertySchema)

	require.NoError(t, got.Apply(domain.EventTypePatch{SetSchema: true}, now))
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		locked, err := r.TypeByIDForUpdate(ctx, tx, app.Scope{TenantID: tenantA}, et.ID)
		require.NoError(t, err)
		require.Equal(t, et.ID, locked.ID)
		return r.SaveType(ctx, tx, got)
	}))
	got, err = r.TypeByID(ctx, app.Scope{TenantID: tenantA}, et.ID)
	require.NoError(t, err)
	require.Nil(t, got.PropertySchema)
}

func TestCategoryFKAndPurge(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	cat, err := domain.CreateCategory(domain.NewCategory{TenantID: tenantA, Name: "Ours", Slug: "commerce"}, now)
	require.NoError(t, err)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateCategory(ctx, tx, cat) }))

	resolved, err := r.CategoryBySlug(ctx, app.Scope{TenantID: tenantA}, "commerce")
	require.NoError(t, err)
	require.Equal(t, cat.ID, resolved.ID, "own category shadows the global slug")
	cs, err := r.ListCategories(ctx, app.Scope{TenantID: tenantA}, 100)
	require.NoError(t, err)
	require.Len(t, cs, len(migrations.GlobalCategories), "global commerce hidden behind the tenant's")

	et := newType(t, tenantA, "buy", now)
	et.CategoryID = cat.ID
	require.NoError(t, create(t, r, db, et))

	bad := newType(t, tenantA, "orphan", now)
	bad.CategoryID = id.NewID()
	require.ErrorIs(t, create(t, r, db, bad), domain.ErrUnknownCategory, "FK violation maps to a domain error")

	other := newType(t, tenantB, "keep", now)
	require.NoError(t, create(t, r, db, other))

	purge := func() error {
		return postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenantA) })
	}
	require.NoError(t, purge())
	require.NoError(t, purge(), "idempotent")

	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM eventcatalog_svc.event_types WHERE tenant_id = ?`, tenantA).Scan(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Raw(`SELECT count(*) FROM eventcatalog_svc.event_categories WHERE tenant_id = ?`, tenantA).Scan(&n).Error)
	require.Zero(t, n)
	_, err = r.TypeByID(ctx, app.Scope{TenantID: tenantB}, other.ID)
	require.NoError(t, err, "other tenants untouched")
	_, err = r.TypeByID(ctx, app.Scope{}, migrations.EventTypeID("level_up"))
	require.NoError(t, err, "globals untouched")
}
