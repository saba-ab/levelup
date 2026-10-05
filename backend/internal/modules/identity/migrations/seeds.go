package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/platform/authz"
)

// Go returns the typed seeds. Versions 2 and 3 follow 0001_init.sql.
func Go() []*goose.Migration {
	return []*goose.Migration{
		goose.NewGoMigration(2,
			&goose.GoFunc{RunTx: upSeedPermissions},
			&goose.GoFunc{RunTx: downSeedPermissions},
		),
		goose.NewGoMigration(3,
			&goose.GoFunc{RunTx: upSeedDefaultGrants},
			&goose.GoFunc{RunTx: downSeedDefaultGrants},
		),
	}
}

// Grant is one default (permission → roles) row set.
type Grant struct {
	Perm  authz.Permission
	Roles []int64
}

// DefaultGrants is the identity default role matrix. Reading users and the
// tenant is open to every tenant role; administering users and tenant
// settings is admin-only; deleting the tenant is owner-only (the service
// also checks tenants.owner_user_id); the platform permission goes to the
// platform role alone.
var DefaultGrants = []Grant{
	{contracts.PermUsersViewAny, contracts.MemberRoles},
	{contracts.PermUsersCreate, contracts.AdminRoles},
	{contracts.PermUsersUpdate, contracts.AdminRoles},
	{contracts.PermUsersDelete, contracts.AdminRoles},
	{contracts.PermUsersAssignRoles, contracts.AdminRoles},
	{contracts.PermTenantView, contracts.MemberRoles},
	{contracts.PermTenantUpdate, contracts.AdminRoles},
	{contracts.PermTenantDelete, []int64{contracts.RoleOwner}},
	{contracts.PermPlatformTenantsManage, []int64{contracts.RolePlatformAdmin}},
}

func upSeedPermissions(ctx context.Context, tx *sql.Tx) error {
	for _, p := range contracts.AllPermissions {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO authz_svc.permissions (key, module) VALUES ($1, $2)
			 ON CONFLICT (key) DO NOTHING`, p.Key(), p.Module); err != nil {
			return err
		}
	}
	return nil
}

func downSeedPermissions(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM authz_svc.permissions WHERE module = $1`, contracts.Module)
	return err
}

// upSeedDefaultGrants inserts with NOT EXISTS rather than ON CONFLICT: the
// casbin unique index spans nullable v2..v5, and NULLs never conflict.
func upSeedDefaultGrants(ctx context.Context, tx *sql.Tx) error {
	for _, g := range DefaultGrants {
		for _, role := range g.Roles {
			sub := fmt.Sprintf("role:%d", role)
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO authz_svc.casbin_rule (ptype, v0, v1)
				 SELECT 'p', $1::varchar, $2::varchar
				 WHERE NOT EXISTS (
				     SELECT 1 FROM authz_svc.casbin_rule WHERE ptype = 'p' AND v0 = $1 AND v1 = $2
				 )`, sub, g.Perm.Key()); err != nil {
				return err
			}
		}
	}
	return nil
}

func downSeedDefaultGrants(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM authz_svc.casbin_rule WHERE ptype = 'p' AND v1 LIKE $1`, contracts.Module+":%")
	return err
}
