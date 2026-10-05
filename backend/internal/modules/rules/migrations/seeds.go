package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/rules/contracts"
	"levelup/internal/platform/authz"
)

// Go returns the typed seeds: version 2 the permission catalogue, version 3
// the default grants (IMPLEMENTATION.md §3). Versions do not collide with
// 0001_init.sql.
func Go() []*goose.Migration {
	return []*goose.Migration{
		goose.NewGoMigration(2,
			&goose.GoFunc{RunTx: upSeedPermissions},
			&goose.GoFunc{RunTx: downSeedPermissions},
		),
		goose.NewGoMigration(3,
			&goose.GoFunc{RunTx: upSeedGrants},
			&goose.GoFunc{RunTx: downSeedGrants},
		),
	}
}

// AdminPermissions are granted to identity AdminRoles only (configuring
// rules); everything else goes to every MemberRole. Doc 06 §11.12 matrix.
var AdminPermissions = []authz.Permission{
	contracts.PermCreate, contracts.PermUpdate, contracts.PermDelete, contracts.PermPublish,
}

func isAdmin(p authz.Permission) bool {
	for _, a := range AdminPermissions {
		if a == p {
			return true
		}
	}
	return false
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

func upSeedGrants(ctx context.Context, tx *sql.Tx) error {
	for _, p := range contracts.AllPermissions {
		roles := identitycontracts.MemberRoles
		if isAdmin(p) {
			roles = identitycontracts.AdminRoles
		}
		for _, role := range roles {
			// NOT EXISTS instead of ON CONFLICT: the unique index spans the
			// nullable v2..v5 columns, and NULLs never conflict.
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO authz_svc.casbin_rule (ptype, v0, v1)
				 SELECT 'p', $1::varchar, $2::varchar
				 WHERE NOT EXISTS (SELECT 1 FROM authz_svc.casbin_rule WHERE ptype = 'p' AND v0 = $1 AND v1 = $2)`,
				fmt.Sprintf("role:%d", role), p.Key()); err != nil {
				return err
			}
		}
	}
	return nil
}

func downSeedGrants(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM authz_svc.casbin_rule WHERE ptype = 'p' AND v1 LIKE $1`, contracts.Module+":%")
	return err
}
