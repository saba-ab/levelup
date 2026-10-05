package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/pressly/goose/v3"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/program/contracts"
	"levelup/internal/platform/authz"
)

// AdminPermissions are granted to identity's AdminRoles only; every other
// program permission goes to MemberRoles (IMPLEMENTATION.md §3).
var AdminPermissions = []authz.Permission{
	contracts.PermDelete, contracts.PermActivate, contracts.PermPause, contracts.PermEnd,
}

// Go returns the typed seeds. Versions 2 and 3 follow 0001_init.sql.
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

// Grants returns the default role → permission grants.
func Grants() map[int64][]authz.Permission {
	out := map[int64][]authz.Permission{}
	for _, p := range contracts.AllPermissions {
		roles := identitycontracts.MemberRoles
		if slices.Contains(AdminPermissions, p) {
			roles = identitycontracts.AdminRoles
		}
		for _, r := range roles {
			out[r] = append(out[r], p)
		}
	}
	return out
}

func upSeedGrants(ctx context.Context, tx *sql.Tx) error {
	for roleID, perms := range Grants() {
		for _, p := range perms {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO authz_svc.casbin_rule (ptype, v0, v1) VALUES ('p', $1, $2)
				 ON CONFLICT DO NOTHING`, fmt.Sprintf("role:%d", roleID), p.Key()); err != nil {
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
