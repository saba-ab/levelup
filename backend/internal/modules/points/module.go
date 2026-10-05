// Package points owns per-player wallets and their append-only ledger.
// Public surface: this file, config.go, ports.go, contracts/ and adapters/.
package points

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus"

	identitycontracts "levelup/internal/modules/identity/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/points/contracts"
	"levelup/internal/modules/points/internal/app"
	"levelup/internal/modules/points/internal/repo"
	"levelup/internal/modules/points/internal/transport"
	"levelup/internal/modules/points/migrations"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/modkit"
	"levelup/internal/shared/errs"
)

// JobReconcile is the cron that verifies balances against the ledger.
const JobReconcile = "points.reconcile"

type Module struct {
	svc  *app.Service
	repo *repo.Postgres
	deps modkit.Deps
	cfg  Config
}

// New wires the module. players is the port the composition root chose:
// adapters.NewLocalPlayers(playerMod.Reader()). Constructors do no I/O.
func New(d modkit.Deps, cfg Config, players PlayerReader) *Module {
	r := repo.NewPostgres(d.DB)
	svc := app.NewService(r, players, d.Outbox, d.Authz, d.DB, d.Clock, d.Log, driftCounter(d.Metrics))
	return &Module{svc: svc, repo: r, deps: d, cfg: cfg}
}

// driftCounter registers points_reconcile_drift_total; a second registry
// build (tests, multiple binaries in one process) reuses the first.
func driftCounter(reg *prometheus.Registry) prometheus.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "points_reconcile_drift_total",
		Help: "Wallets whose balance or lifetime counters disagree with the ledger, per reconcile run.",
	})
	if reg == nil {
		return c
	}
	if err := reg.Register(c); err != nil {
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			if existing, ok := already.ExistingCollector.(prometheus.Counter); ok {
				return existing
			}
		}
	}
	return c
}

func (m *Module) Name() string { return contracts.Module }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue + grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Reader is points' offered synchronous read surface.
func (m *Module) Reader() contracts.Reader { return m.svc }

func (m *Module) Subscriptions() []bus.Subscription {
	return []bus.Subscription{
		{Topic: playercontracts.TopicPlayerCreated, Group: contracts.Module, Handler: m.onPlayerCreated},
		{Topic: identitycontracts.TopicTenantDeleted, Group: contracts.Module, Handler: m.onTenantDeleted},
	}
}

func (m *Module) onPlayerCreated(ctx context.Context, e bus.Envelope) error {
	var ev playercontracts.PlayerCreatedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable player.created.v1", err)
	}
	return m.svc.OpenForPlayer(ctx, ev.TenantID, ev.PlayerID)
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	return m.svc.PurgeTenant(ctx, ev.TenantID)
}

func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{
		{Name: contracts.JobCredit, Run: m.runCredit},
		{Name: contracts.JobDebit, Run: m.runDebit},
		{Name: contracts.JobRefund, Run: m.runRefund},
		{
			Name:     JobReconcile,
			Schedule: m.cfg.ReconcileSchedule,
			Run: func(ctx context.Context, _ []byte) error {
				return m.svc.Reconcile(ctx, m.repo)
			},
		},
	}
}

func (m *Module) runCredit(ctx context.Context, body []byte) error {
	var cmd contracts.CreditCmdV1
	if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
		return err
	}
	return m.svc.HandleCredit(ctx, cmd)
}

func (m *Module) runDebit(ctx context.Context, body []byte) error {
	var cmd contracts.DebitCmdV1
	if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
		return err
	}
	return m.svc.HandleDebit(ctx, cmd)
}

func (m *Module) runRefund(ctx context.Context, body []byte) error {
	var cmd contracts.RefundCmdV1
	if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
		return err
	}
	return m.svc.HandleRefund(ctx, cmd)
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
