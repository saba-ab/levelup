// Package missions owns missions, player attempts and completions. Public
// surface: this file, config.go, ports.go, contracts/ and adapters/.
package missions

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"

	activitycontracts "levelup/internal/modules/activity/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/missions/internal/app"
	"levelup/internal/modules/missions/internal/repo"
	"levelup/internal/modules/missions/internal/transport"
	"levelup/internal/modules/missions/migrations"
	playercontracts "levelup/internal/modules/player/contracts"
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
// adapters.NewLocalPlayers(playerMod.Reader()) in-process. No I/O here.
func New(d modkit.Deps, cfg Config, players PlayerReader) *Module {
	svc := app.NewService(repo.NewPostgres(d.DB), players, d.Outbox, d.Authz, d.DB, d.Clock, cfg.SweepBatchSize)
	svc.SetMaxCausationDepth(cfg.MaxCausationDepth)
	return &Module{svc: svc, deps: d, cfg: cfg}
}

func (m *Module) Name() string { return contracts.Module }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue and
// default grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

// Reader is missions' offered synchronous read surface.
func (m *Module) Reader() contracts.Reader { return m.svc }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

func (m *Module) Subscriptions() []bus.Subscription {
	return []bus.Subscription{
		{Topic: identitycontracts.TopicTenantDeleted, Group: contracts.Module, Handler: m.onTenantDeleted},
		{Topic: playercontracts.TopicPlayerDeleted, Group: contracts.Module, Handler: m.onPlayerDeleted},
		{Topic: activitycontracts.TopicReceived, Group: contracts.Module, Handler: m.onActivityReceived},
	}
}

// onActivityReceived progresses every mission whose criteria match the
// activity. Numbers stay json.Number so criteria compare them exactly.
func (m *Module) onActivityReceived(ctx context.Context, e bus.Envelope) error {
	var ev activitycontracts.ReceivedV1
	dec := json.NewDecoder(bytes.NewReader(e.Payload))
	dec.UseNumber()
	if err := dec.Decode(&ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable activity.received.v1", err)
	}
	_, err := m.svc.HandleActivity(ctx, ev)
	return err
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	return m.svc.OnTenantDeleted(ctx, ev.TenantID)
}

func (m *Module) onPlayerDeleted(ctx context.Context, e bus.Envelope) error {
	var ev playercontracts.PlayerDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable player.deleted.v1", err)
	}
	return m.svc.OnPlayerDeleted(ctx, ev.TenantID, ev.PlayerID)
}

func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{
		{
			// Command consumer (job.missions.progress through the outbox).
			Name: contracts.JobProgress,
			Run: func(ctx context.Context, body []byte) error {
				var cmd contracts.ProgressCmdV1
				if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
					return err
				}
				return m.svc.HandleProgressJob(ctx, cmd)
			},
		},
		{
			// Reconciling cron (R47).
			Name:     contracts.JobExpireSweep,
			Schedule: m.cfg.ExpireSweepSchedule,
			Run: func(ctx context.Context, _ []byte) error {
				return m.svc.ExpireSweep(ctx)
			},
		},
	}
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
