package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"

	"levelup/internal/shared/errs"
)

// GoMigrator is the optional module hook for typed Go migrations (PRD §7.4)
// — the flagship use is seeding the permission catalogue from the same
// constants the code enforces. Kept here so modkit never learns about goose.
type GoMigrator interface {
	GoMigrations() []*goose.Migration
}

// Apply runs a module's embedded migrations into schema <name>_svc with a
// per-module version table (PRD §7.4, R3). dsn must reach Postgres directly,
// not through PgBouncer: search_path is a session setting and transaction
// pooling shares sessions between clients.
func Apply(ctx context.Context, dsn, name string, fsys fs.FS, goMigrations ...*goose.Migration) error {
	schemaName := name + "_svc"
	if err := validSchemaName(schemaName); err != nil {
		return err
	}

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return errs.Wrap(errs.Internal, "parse migrate dsn", err)
	}
	// Unqualified DDL in migration files lands in the module schema. Stubs
	// stay copy-paste simple; the schema stays per-module.
	cfg.RuntimeParams["search_path"] = schemaName + ",public"

	db := stdlib.OpenDB(*cfg)
	defer db.Close()

	if _, err := db.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS `+quoteIdent(schemaName)); err != nil {
		return errs.Wrap(errs.Internal, "create schema "+schemaName, err)
	}

	store, err := database.NewStore(goose.DialectPostgres, schemaName+".goose_db_version")
	if err != nil {
		return errs.Wrap(errs.Internal, "goose store", err)
	}
	opts := []goose.ProviderOption{goose.WithStore(store)}
	if len(goMigrations) > 0 {
		opts = append(opts, goose.WithGoMigrations(goMigrations...))
	}
	provider, err := goose.NewProvider("", db, fsys, opts...)
	if err != nil {
		return errs.Wrap(errs.Internal, "goose provider for "+name, err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return errs.Wrap(errs.Internal, "migrate "+name, err)
	}
	return nil
}

// CreateModuleRole creates role <name>_svc that can only see its own schema
// (R3). Postgres itself then rejects cross-module reads — integration tests
// connect as the role to prove it (PRD §4 P4). Local/test convenience: the
// password equals the role name; production overrides credentials outside
// the blueprint.
func CreateModuleRole(ctx context.Context, dsn, name string) error {
	schemaName := name + "_svc"
	if err := validSchemaName(schemaName); err != nil {
		return err
	}
	role := schemaName // role and schema share the name, per PRD §4 P4

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return errs.Wrap(errs.Internal, "parse dsn", err)
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return errs.Wrap(errs.Unavailable, "connect for role setup", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	stmts := []string{
		fmt.Sprintf(`DO $$ BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '%[1]s') THEN
				CREATE ROLE %[2]s LOGIN PASSWORD '%[1]s';
			END IF;
		END $$`, role, quoteIdent(role)),
		fmt.Sprintf(`REVOKE ALL ON SCHEMA public FROM %s`, quoteIdent(role)),
		fmt.Sprintf(`GRANT USAGE ON SCHEMA %s TO %s`, quoteIdent(schemaName), quoteIdent(role)),
		fmt.Sprintf(`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %s TO %s`,
			quoteIdent(schemaName), quoteIdent(role)),
		fmt.Sprintf(`GRANT USAGE ON ALL SEQUENCES IN SCHEMA %s TO %s`,
			quoteIdent(schemaName), quoteIdent(role)),
		fmt.Sprintf(`ALTER DEFAULT PRIVILEGES IN SCHEMA %s GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %s`,
			quoteIdent(schemaName), quoteIdent(role)),
		fmt.Sprintf(`ALTER ROLE %s SET search_path = %s`, quoteIdent(role), quoteIdent(schemaName)),
	}
	for _, s := range stmts {
		if _, err := conn.Exec(ctx, s); err != nil {
			return errs.Wrap(errs.Internal, "role setup: "+s, err)
		}
	}
	return nil
}

// GrantOutboxWrite lets a module role record events in its own transactions
// (PRD §4 P3). The outbox table is the one deliberate platform exception to
// per-module schema privacy — reads/updates stay with the dispatcher.
func GrantOutboxWrite(ctx context.Context, dsn, name string) error {
	role := name + "_svc"
	if err := validSchemaName(role); err != nil {
		return err
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return errs.Wrap(errs.Internal, "parse dsn", err)
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return errs.Wrap(errs.Unavailable, "connect for outbox grant", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	for _, s := range []string{
		fmt.Sprintf(`GRANT USAGE ON SCHEMA outbox_svc TO %s`, quoteIdent(role)),
		fmt.Sprintf(`GRANT INSERT, SELECT ON outbox_svc.outbox_events TO %s`, quoteIdent(role)),
	} {
		if _, err := conn.Exec(ctx, s); err != nil {
			return errs.Wrap(errs.Internal, "outbox grant: "+s, err)
		}
	}
	return nil
}

// ModuleRoleDSN rewrites a DSN to authenticate as the module role.
func ModuleRoleDSN(dsn, name string) string {
	role := name + "_svc"
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	u.User = url.UserPassword(role, role)
	return u.String()
}

func validSchemaName(s string) error {
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return errs.New(errs.Invalid, "illegal schema name "+s)
	}
	return nil
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
