package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"levelup/internal/modules/badges/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/platform/authz"
)

// adminPerms configure the catalogue or take badges away; everything else
// (view, award) is granted to every tenant role (parity: BadgePolicy::award
// allowed any same-tenant user).
var adminPerms = map[string]bool{
	contracts.PermCreate.Key(): true,
	contracts.PermUpdate.Key(): true,
	contracts.PermDelete.Key(): true,
	contracts.PermRevoke.Key(): true,
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

// DefaultGrants is the role → permission seed, exported for tests.
func DefaultGrants() map[int64][]authz.Permission {
	out := map[int64][]authz.Permission{}
	for _, p := range contracts.AllPermissions {
		roles := identitycontracts.MemberRoles
		if adminPerms[p.Key()] {
			roles = identitycontracts.AdminRoles
		}
		for _, role := range roles {
			out[role] = append(out[role], p)
		}
	}
	return out
}

func upSeedGrants(ctx context.Context, tx *sql.Tx) error {
	for role, perms := range DefaultGrants() {
		for _, p := range perms {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO authz_svc.casbin_rule (ptype, v0, v1) VALUES ('p', $1, $2)
				 ON CONFLICT DO NOTHING`, fmt.Sprintf("role:%d", role), p.Key()); err != nil {
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
