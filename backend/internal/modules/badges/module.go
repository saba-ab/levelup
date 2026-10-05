// Package badges is the badge catalogue, player holdings and the award
// ledger. Public surface: this file + config.go + ports.go + contracts/.
package badges

import (
	"context"
	"errors"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus"

	"levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/badges/internal/app"
	"levelup/internal/modules/badges/internal/repo"
	"levelup/internal/modules/badges/internal/transport"
	"levelup/internal/modules/badges/migrations"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/modkit"
	"levelup/internal/platform/postgres"
)

var (
	_ modkit.Module       = (*Module)(nil)
	_ postgres.GoMigrator = (*Module)(nil)
)

type Module struct {
	svc  *app.Service
	repo *repo.Postgres
	deps modkit.Deps
	cfg  Config
}

// New wires the module. players is the port implementation the registry
// chose, today adapters.NewLocalPlayers(playerMod.Reader()). Constructors do
// no I/O.
func New(d modkit.Deps, cfg Config, players PlayerReader) *Module {
	r := repo.NewPostgres(d.DB)
	svc := app.NewService(r, players, d.Outbox, d.Authz, d.DB, d.Clock, d.Log, driftCounter(d.Metrics))
	return &Module{svc: svc, repo: r, deps: d, cfg: cfg}
}

// driftCounter registers badges_reconcile_drift_total once per registry.
func driftCounter(reg *prometheus.Registry) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "badges_reconcile_drift_total",
		Help: "Player badge holdings breaking an award invariant, by check.",
	}, []string{"check"})
	if reg == nil {
		return c
	}
	if err := reg.Register(c); err != nil {
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			if existing, ok := already.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing
			}
		}
	}
	return c
}

func (m *Module) Name() string { return contracts.Module }

// Migrations land in schema badges_svc with an own goose version table (R3).
func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue and
// default role grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Subscriptions: purge the tenant's rows on tenant.deleted.v1 (idempotent).
func (m *Module) Subscriptions() []bus.Subscription {
	return []bus.Subscription{{
		Topic:   identitycontracts.TopicTenantDeleted,
		Group:   contracts.Module,
		Handler: m.svc.OnTenantDeleted,
	}}
}

// Jobs: the badges.award command consumer and the hourly reconcile sweep.
func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{
		{
			Name: contracts.JobAward,
			Run:  m.svc.HandleAwardJob,
		},
		{
			Name:     contracts.JobReconcile,
			Schedule: m.cfg.ReconcileSchedule,
			Run: func(ctx context.Context, _ []byte) error {
				return m.svc.Reconcile(ctx, m.repo)
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

// Reader is badges' offered synchronous read surface (missions, rewards,
// progression validate badge ids; leaderboards read holdings).
func (m *Module) Reader() contracts.Reader { return m.svc }
