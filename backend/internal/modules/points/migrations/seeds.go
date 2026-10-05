package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/points/contracts"
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

// adminOnly are granted to identity's AdminRoles; everything else goes to
// MemberRoles (IMPLEMENTATION.md §3).
var adminOnly = map[string]bool{
	contracts.PermManageWallet.Key(): true,
}

// Grants is the default role → permission matrix the seed writes.
func Grants() map[int64][]authz.Permission {
	out := map[int64][]authz.Permission{}
	for _, p := range contracts.AllPermissions {
		roles := identitycontracts.MemberRoles
		if adminOnly[p.Key()] {
			roles = identitycontracts.AdminRoles
		}
		for _, r := range roles {
			out[r] = append(out[r], p)
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

func upSeedDefaultGrants(ctx context.Context, tx *sql.Tx) error {
	for roleID, perms := range Grants() {
		for _, p := range perms {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO authz_svc.casbin_rule (ptype, v0, v1) VALUES ('p', $1, $2)
				 ON CONFLICT DO NOTHING`,
				fmt.Sprintf("role:%d", roleID), p.Key()); err != nil {
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
