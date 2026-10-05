package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/platform/authz"
)

// Go returns the typed migrations: version 2 seeds the permission catalogue,
// version 3 the default role grants (IMPLEMENTATION.md §3).
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

// adminPermissions configure boards; everything else is for every member
// role (view_any/view fix L4: the Laravel index had no authorization).
var adminPermissions = []authz.Permission{
	contracts.PermCreate, contracts.PermUpdate, contracts.PermDelete, contracts.PermRebuild,
}

// Grants maps each permission to the roles that receive it by default.
func Grants() map[authz.Permission][]int64 {
	admin := make(map[authz.Permission]bool, len(adminPermissions))
	for _, p := range adminPermissions {
		admin[p] = true
	}
	out := make(map[authz.Permission][]int64, len(contracts.AllPermissions))
	for _, p := range contracts.AllPermissions {
		if admin[p] {
			out[p] = identitycontracts.AdminRoles
		} else {
			out[p] = identitycontracts.MemberRoles
		}
	}
	return out
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
	for perm, roles := range Grants() {
		for _, role := range roles {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO authz_svc.casbin_rule (ptype, v0, v1) VALUES ('p', $1, $2)
				 ON CONFLICT DO NOTHING`, fmt.Sprintf("role:%d", role), perm.Key()); err != nil {
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
