// Package player owns a tenant's end users (players), addressed by the
// tenant's own external_id. Public surface: this file, config.go and
// contracts/. Everything else is compiler-private (PRD §4 P1).
package player

import (
	"context"
	"encoding/json"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"

	activitycontracts "levelup/internal/modules/activity/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/player/contracts"
	"levelup/internal/modules/player/internal/app"
	"levelup/internal/modules/player/internal/repo"
	"levelup/internal/modules/player/internal/transport"
	"levelup/internal/modules/player/migrations"
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

// New wires the module. Constructors do no I/O. player depends on no other
// module's reader: it is a provider (Reader()) and consumes identity's
// tenant.deleted.v1 and activity's activity.received.v1 events.
func New(d modkit.Deps, cfg Config) *Module {
	pg := repo.NewPostgres(d.DB)
	var r app.Repository = pg
	if d.Cache != nil && cfg.CacheTTL > 0 {
		r = repo.NewCached(pg, d.Cache.WithTTL(cfg.CacheTTL))
	}
	svc := app.NewService(r, d.Outbox, d.Authz, d.DB, d.Clock, cfg.PurgeBatchSize)
	return &Module{svc: svc, deps: d, cfg: cfg}
}

// Reader is player's offered synchronous read surface for every mechanic.
func (m *Module) Reader() contracts.Reader { return m.svc }

// IDLister exposes keyset paging over player ids (segments refresh).
func (m *Module) IDLister() contracts.IDLister { return m.svc }

func (m *Module) Name() string { return contracts.Module }

// Migrations land in schema player_svc with an own goose version table (R3).
func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue and
// default role grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Subscriptions: tenant.deleted.v1 replaces the Laravel FK cascade. The
// purge is idempotent by construction (delete-where-tenant finds nothing
// the second time). activity.received.v1 drives player auto-creation
// (idempotent: derived id plus the live unique index).
func (m *Module) Subscriptions() []bus.Subscription {
	subs := []bus.Subscription{{
		Topic:   identitycontracts.TopicTenantDeleted,
		Group:   contracts.Module,
		Handler: m.onTenantDeleted,
	}}
	if m.cfg.AutoCreateFromActivities {
		subs = append(subs, bus.Subscription{
			Topic:   activitycontracts.TopicReceived,
			Group:   contracts.Module,
			Handler: m.onActivityReceived,
		})
	}
	return subs
}

func (m *Module) onActivityReceived(ctx context.Context, e bus.Envelope) error {
	return handleActivityReceived(ctx, m.svc, e)
}

type autoCreator interface {
	AutoCreateFromActivity(ctx context.Context, ev activitycontracts.ReceivedV1) (bool, error)
}

func handleActivityReceived(ctx context.Context, svc autoCreator, e bus.Envelope) error {
	var ev activitycontracts.ReceivedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable activity.received.v1", err)
	}
	_, err := svc.AutoCreateFromActivity(ctx, ev)
	return err
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	return handleTenantDeleted(ctx, m.svc, e)
}

type tenantPurger interface {
	PurgeTenant(ctx context.Context, tenantID string) (int, error)
}

func handleTenantDeleted(ctx context.Context, svc tenantPurger, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		// errs.Invalid → dead-letter at once; it will not decode in 5s either.
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	_, err := svc.PurgeTenant(ctx, ev.TenantID)
	return err
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
