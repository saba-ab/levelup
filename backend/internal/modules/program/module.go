// Package program owns programs (campaigns with a draft → active ⇄ paused →
// ended lifecycle) and player enrolments. Public surface: this file,
// config.go, ports.go, contracts/ and adapters/.
package program

import (
	"context"
	"encoding/json"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"

	identitycontracts "levelup/internal/modules/identity/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/program/contracts"
	"levelup/internal/modules/program/internal/app"
	"levelup/internal/modules/program/internal/repo"
	"levelup/internal/modules/program/internal/transport"
	"levelup/internal/modules/program/migrations"
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

// New wires the module. players is the port the composition root chose:
// adapters.NewLocalPlayers(playerMod.Reader()) in-process. Constructors do
// no I/O.
func New(d modkit.Deps, cfg Config, players PlayerReader) *Module {
	svc := app.NewService(repo.NewPostgres(d.DB), players, d.Outbox, d.Authz, d.DB, d.Clock, cfg.AutoEndBatchSize)
	return &Module{svc: svc, deps: d, cfg: cfg}
}

func (m *Module) Name() string { return contracts.Module }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue + grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Reader is program's offered synchronous read surface.
func (m *Module) Reader() contracts.Reader { return m.svc }

func (m *Module) Subscriptions() []bus.Subscription {
	return []bus.Subscription{
		{Topic: identitycontracts.TopicTenantDeleted, Group: contracts.Module, Handler: m.onTenantDeleted},
		{Topic: playercontracts.TopicPlayerDeleted, Group: contracts.Module, Handler: m.onPlayerDeleted},
	}
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	return m.svc.PurgeTenant(ctx, ev.TenantID)
}

func (m *Module) onPlayerDeleted(ctx context.Context, e bus.Envelope) error {
	var ev playercontracts.PlayerDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable player.deleted.v1", err)
	}
	return m.svc.RemovePlayer(ctx, ev.TenantID, ev.PlayerID)
}

// Jobs: the reconciling auto-end sweep. An empty schedule disables it.
func (m *Module) Jobs() []jobs.Job {
	if m.cfg.AutoEndSchedule == "" {
		return nil
	}
	return []jobs.Job{{
		Name:     contracts.JobAutoEnd,
		Schedule: m.cfg.AutoEndSchedule,
		Run: func(ctx context.Context, _ []byte) error {
			_, err := m.svc.AutoEnd(ctx)
			return err
		},
	}}
}

func (m *Module) Health(ctx context.Context) error {
	sqlDB, err := m.deps.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (m *Module) Permissions() []authz.Permission { return contracts.AllPermissions }

var _ modkit.Module = (*Module)(nil)
