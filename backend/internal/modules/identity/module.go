// Package identity owns tenants, users, credentials and role assignments
// (doc 02 §11). Public surface: this file, config.go and contracts/.
package identity

import (
	"context"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/repo"
	"levelup/internal/modules/identity/internal/transport"
	"levelup/internal/modules/identity/migrations"
	"levelup/internal/platform/authn"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/modkit"
)

type Module struct {
	svc  *app.Service
	deps modkit.Deps
}

// New wires the module. auth is passed explicitly because identity owns
// credentials (like the blueprint's reference user module). Constructors do
// no I/O; a nil auth (binaries that never serve HTTP) only disables the
// token-issuing endpoints.
func New(d modkit.Deps, cfg Config, auth *authn.Auth) *Module {
	var (
		issuer  app.AccessIssuer
		refresh app.RefreshTokens
	)
	if auth != nil {
		if auth.Issuer != nil {
			issuer = auth.Issuer
		}
		if auth.Refresh != nil {
			refresh = auth.Refresh
		}
	}
	svc := app.NewService(repo.NewPostgres(d.DB), d.Outbox, d.Authz, issuer, refresh, d.DB, d.Clock, d.Log,
		app.Settings{BcryptCost: cfg.BcryptCost, AllowSelfSignup: cfg.AllowSelfSignup})
	return &Module{svc: svc, deps: d}
}

func (m *Module) Name() string { return contracts.Module }

// Migrations land in schema identity_svc.
func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue and
// default grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Subscriptions: identity is upstream of every module and consumes nothing.
func (m *Module) Subscriptions() []bus.Subscription { return nil }

func (m *Module) Jobs() []jobs.Job { return nil }

func (m *Module) Health(ctx context.Context) error {
	sqlDB, err := m.deps.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (m *Module) Permissions() []authz.Permission { return contracts.AllPermissions }

// TenantReader is offered to other modules; their adapters wrap it into
// their own ports.
func (m *Module) TenantReader() contracts.TenantReader { return m.svc }
