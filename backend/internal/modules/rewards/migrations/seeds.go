package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/platform/authz"
)

// adminPermissions are the ones commented "admin roles" in contracts; every
// other permission goes to all member roles (IMPLEMENTATION.md §3).
var adminPermissions = []authz.Permission{
	contracts.PermCreate, contracts.PermUpdate, contracts.PermDelete, contracts.PermCancel,
}

// Go returns the typed seeds. Versions 2 and 3 follow 0001_init.sql.
func Go() []*goose.Migration {
	return []*goose.Migration{
		goose.NewGoMigration(2,
			&goose.GoFunc{RunTx: upPermissions},
			&goose.GoFunc{RunTx: downPermissions},
		),
		goose.NewGoMigration(3,
			&goose.GoFunc{RunTx: upGrants},
			&goose.GoFunc{RunTx: downGrants},
		),
	}
}

func upPermissions(ctx context.Context, tx *sql.Tx) error {
	for _, p := range contracts.AllPermissions {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO authz_svc.permissions (key, module) VALUES ($1, $2)
			 ON CONFLICT (key) DO NOTHING`, p.Key(), p.Module); err != nil {
			return err
		}
	}
	return nil
}

func downPermissions(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM authz_svc.permissions WHERE module = $1`, contracts.Module)
	return err
}

// Grants returns the default (role, permission) pairs.
func Grants() map[int64][]authz.Permission {
	isAdmin := make(map[string]bool, len(adminPermissions))
	for _, p := range adminPermissions {
		isAdmin[p.Key()] = true
	}
	out := map[int64][]authz.Permission{}
	for _, p := range contracts.AllPermissions {
		roles := identitycontracts.MemberRoles
		if isAdmin[p.Key()] {
			roles = identitycontracts.AdminRoles
		}
		for _, role := range roles {
			out[role] = append(out[role], p)
		}
	}
	return out
}

func upGrants(ctx context.Context, tx *sql.Tx) error {
	for role, perms := range Grants() {
		for _, p := range perms {
			// casbin_rule's unique index includes nullable v2..v5, so ON
			// CONFLICT never fires; guard with NOT EXISTS instead.
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO authz_svc.casbin_rule (ptype, v0, v1)
				 SELECT 'p', $1::text, $2::text
				 WHERE NOT EXISTS (
				     SELECT 1 FROM authz_svc.casbin_rule WHERE ptype = 'p' AND v0 = $1::text AND v1 = $2::text)`,
				fmt.Sprintf("role:%d", role), p.Key()); err != nil {
				return err
			}
		}
	}
	return nil
}

func downGrants(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM authz_svc.casbin_rule WHERE ptype = 'p' AND v1 LIKE $1`, contracts.Module+":%")
	return err
}
