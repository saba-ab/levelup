// Package eventcatalog owns the event-type catalogue: trigger definitions
// (purchase_completed, user_login, ...) that rules react to, grouped by
// categories. Rows without a tenant are the platform-global catalogue,
// visible to every tenant and writable only by platform admins.
//
// Public surface: this file + config.go + contracts/. Everything else is
// compiler-private (PRD §4 P1).
package eventcatalog

import (
	"context"
	"encoding/json"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"

	"levelup/internal/modules/eventcatalog/contracts"
	"levelup/internal/modules/eventcatalog/internal/app"
	"levelup/internal/modules/eventcatalog/internal/repo"
	"levelup/internal/modules/eventcatalog/internal/transport"
	"levelup/internal/modules/eventcatalog/migrations"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/modkit"
	"levelup/internal/shared/errs"
)

type Module struct {
	svc  *app.Service
	deps modkit.Deps
	cfg  Config
}

// New wires the module. Constructors do no I/O. eventcatalog consumes no
// other module synchronously, so it takes no ports.
func New(d modkit.Deps, cfg Config) *Module {
	svc := app.NewService(repo.NewPostgres(d.DB), d.Outbox, d.Authz, d.DB, d.Clock)
	return &Module{svc: svc, deps: d, cfg: cfg}
}

// Reader is the synchronous read surface offered to other modules (rules,
// activity) through their own ports and adapters.
func (m *Module) Reader() contracts.Reader { return m.svc }

func (m *Module) Name() string { return contracts.Module }

// Migrations land in schema eventcatalog_svc.
func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: the permission catalogue,
// default grants and the global event-type catalogue.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Subscriptions: identity's tenant.deleted.v1 purges the tenant's rows.
// Idempotent by construction: a redelivery deletes nothing.
func (m *Module) Subscriptions() []bus.Subscription {
	return []bus.Subscription{{
		Topic:   identitycontracts.TopicTenantDeleted,
		Group:   contracts.Module,
		Handler: m.onTenantDeleted,
	}}
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		// errs.Invalid → the consumer loop dead-letters instead of retrying.
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	return m.svc.PurgeTenant(ctx, ev.TenantID)
}

func (m *Module) Jobs() []jobs.Job { return nil }

func (m *Module) Health(ctx context.Context) error {
	sqlDB, err := m.deps.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (m *Module) Permissions() []authz.Permission { return contracts.AllPermissions }
