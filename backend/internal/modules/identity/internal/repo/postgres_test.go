package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/modules/identity/migrations"
	authzmigrations "levelup/internal/platform/authz/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "authz", authzmigrations.FS))
	require.NoError(t, postgres.Apply(ctx, dsn, "identity", migrations.FS, migrations.Go()...))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "identity")
	return NewPostgres(moduleDB), moduleDB
}

func newTenantWithOwner(t *testing.T, email string) (domain.Tenant, domain.User) {
	t.Helper()
	now := time.Now().UTC()
	tn, err := domain.NewTenant("Acme", "Asia/Tbilisi", id.NewID()[24:30], now)
	require.NoError(t, err)
	u, err := domain.NewUser(tn.ID, "Owner", email, "$2a$04$hash", []int64{contracts.RoleOwner}, now)
	require.NoError(t, err)
	tn.OwnerUserID = u.ID
	return tn, u
}

// The circular tenant↔owner FK is deferred: tenant first, then user, one tx.
func TestRegisterInsertsWithDeferredOwnerFK(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tn, u := newTenantWithOwner(t, "deferred@example.com")
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		if err := r.CreateTenant(ctx, tx, tn); err != nil {
			return err
		}
		return r.CreateUser(ctx, tx, u)
	}))

	got, err := r.UserByEmail(ctx, "DEFERRED@example.com")
	require.NoError(t, err)
	require.Equal(t, []int64{contracts.RoleOwner}, got.RoleIDs)
	loaded, err := r.TenantByID(ctx, tn.ID)
	require.NoError(t, err)
	require.Equal(t, u.ID, loaded.OwnerUserID)
	require.Equal(t, "Asia/Tbilisi", loaded.Timezone)

	// An owner that never gets inserted fails at commit.
	orphan, _ := newTenantWithOwner(t, "never@example.com")
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateTenant(ctx, tx, orphan) })
	require.Error(t, err)
}

func TestEmailUniqueIsCaseInsensitiveAndGlobal(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	a, ua := newTenantWithOwner(t, "same@example.com")
	b, ub := newTenantWithOwner(t, "other@example.com")
	for _, pair := range []struct {
		tn domain.Tenant
		u  domain.User
	}{{a, ua}, {b, ub}} {
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			if err := r.CreateTenant(ctx, tx, pair.tn); err != nil {
				return err
			}
			return r.CreateUser(ctx, tx, pair.u)
		}))
	}
	dup, err := domain.NewUser(b.ID, "Dup", "same@example.com", "x", nil, time.Now().UTC())
	require.NoError(t, err)
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateUser(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrEmailTaken, "email is unique across tenants")

	// Update to a taken address is a 409, not a 500.
	ub.Email = "same@example.com"
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveUser(ctx, tx, ub) })
	require.ErrorIs(t, err, domain.ErrEmailTaken)

	// Raw mixed-case insert hits the lower(email) index too.
	err = db.Exec(`INSERT INTO identity_svc.users (id, name, email, password_hash, created_at, updated_at)
		VALUES (?, 'x', 'SAME@example.com', 'x', now(), now())`, id.NewID()).Error
	require.Error(t, err, "check constraint rejects non-lower-cased email")
}

func TestSlugCollisionIsTyped(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	a, ua := newTenantWithOwner(t, "slug-a@example.com")
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		if err := r.CreateTenant(ctx, tx, a); err != nil {
			return err
		}
		return r.CreateUser(ctx, tx, ua)
	}))
	b, ub := newTenantWithOwner(t, "slug-b@example.com")
	b.Slug = a.Slug
	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		if err := r.CreateTenant(ctx, tx, b); err != nil {
			return err
		}
		return r.CreateUser(ctx, tx, ub)
	})
	require.ErrorIs(t, err, domain.ErrSlugTaken)
}

func TestTenantScopingAndPaging(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	a, ua := newTenantWithOwner(t, "scope-a@example.com")
	b, ub := newTenantWithOwner(t, "scope-b@example.com")
	for _, pair := range []struct {
		tn domain.Tenant
		u  domain.User
	}{{a, ua}, {b, ub}} {
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			if err := r.CreateTenant(ctx, tx, pair.tn); err != nil {
				return err
			}
			return r.CreateUser(ctx, tx, pair.u)
		}))
	}
	_, err := r.UserInTenant(ctx, a.ID, ub.ID)
	require.ErrorIs(t, err, domain.ErrUserNotFound)
	_, err = r.UserInTenant(ctx, a.ID, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrUserNotFound)

	rows, err := r.ListUsers(ctx, a.ID, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, ua.ID, rows[0].ID)

	// Version guard and role replacement.
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		locked, err := r.UserInTenantForUpdate(ctx, tx, a.ID, ua.ID)
		if err != nil {
			return err
		}
		if err := r.SetUserRoles(ctx, tx, locked.ID, []int64{contracts.RoleOwner, contracts.RoleAdmin}, time.Now()); err != nil {
			return err
		}
		return r.SaveUser(ctx, tx, locked)
	}))
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveUser(ctx, tx, ua) })
	require.ErrorIs(t, err, domain.ErrVersionConflict)
	got, err := r.UserByID(ctx, ua.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{contracts.RoleOwner, contracts.RoleAdmin}, got.RoleIDs)

	// Soft delete hides the tenant from reads and the reader.
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		tn, err := r.TenantByIDForUpdate(ctx, tx, b.ID)
		if err != nil {
			return err
		}
		tn.Settings = map[string]any{"k": "v"}
		tn.SoftDelete(time.Now().UTC())
		return r.SaveTenant(ctx, tx, tn)
	}))
	_, err = r.TenantByID(ctx, b.ID)
	require.ErrorIs(t, err, domain.ErrTenantNotFound)
	list, err := r.TenantsByIDs(ctx, []string{a.ID, b.ID})
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestSeedsGrantPermissions(t *testing.T) {
	_, db := setupRepo(t)
	var keys []string
	require.NoError(t, db.Raw(`SELECT key FROM authz_svc.permissions WHERE module = 'identity' ORDER BY key`).Scan(&keys).Error)
	require.Len(t, keys, len(contracts.AllPermissions))

	var platformSubjects []string
	require.NoError(t, db.Raw(`SELECT v0 FROM authz_svc.casbin_rule WHERE ptype = 'p' AND v1 = ?`,
		contracts.PermPlatformTenantsManage.Key()).Scan(&platformSubjects).Error)
	require.Equal(t, []string{"role:1"}, platformSubjects)

	var roles int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM identity_svc.roles`).Scan(&roles).Error)
	require.EqualValues(t, 6, roles)
}
