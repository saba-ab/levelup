package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/streaks/contracts"
	"levelup/internal/platform/authz"
)

// Go returns the typed migrations: 2 seeds the permission catalogue from the
// same constants the service enforces, 3 grants them to the default roles.
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

// adminOnly are the permissions commented "admin roles" in contracts.
var adminOnly = map[authz.Permission]bool{
	contracts.PermCreate: true,
	contracts.PermUpdate: true,
	contracts.PermDelete: true,
	contracts.PermReset:  true,
}

// DefaultGrants maps each role to the permissions it receives by default.
func DefaultGrants() map[int64][]authz.Permission {
	out := map[int64][]authz.Permission{}
	for _, p := range contracts.AllPermissions {
		roles := identitycontracts.MemberRoles
		if adminOnly[p] {
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

func upSeedGrants(ctx context.Context, tx *sql.Tx) error {
	for role, perms := range DefaultGrants() {
		for _, p := range perms {
			if _, err := tx.ExecContext(ctx,
				// NOT EXISTS, not ON CONFLICT: v2..v5 are NULL and NULLs never
				// collide in the unique index.
				`INSERT INTO authz_svc.casbin_rule (ptype, v0, v1)
				 SELECT 'p', $1::text, $2::text
				 WHERE NOT EXISTS (SELECT 1 FROM authz_svc.casbin_rule
				                   WHERE ptype = 'p' AND v0 = $1::text AND v1 = $2::text)`,
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
