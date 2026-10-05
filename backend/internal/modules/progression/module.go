// Package progression owns XP and levels: the tenant's level ladder, each
// player's total XP and derived level, and the XP grant ledger. Level-up
// rewards are issued as job.points.credit / job.badges.award commands in the
// same transaction that records the level (00 §5 F3).
//
// Public surface: this file + contracts/ + config.go + ports.go + adapters/.
package progression

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/progression/internal/app"
	"levelup/internal/modules/progression/internal/repo"
	"levelup/internal/modules/progression/internal/transport"
	"levelup/internal/modules/progression/migrations"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/modkit"
	"levelup/internal/shared/errs"
)

type Module struct {
	svc  *app.Service
	repo *repo.Postgres
	deps modkit.Deps
	cfg  Config
}

// New wires the module. players is the port the composition root chose:
// adapters.NewLocalPlayers(playerMod.Reader()) in-process. Constructors do
// no I/O.
func New(d modkit.Deps, cfg Config, players PlayerReader) *Module {
	r := repo.NewPostgres(d.DB)
	svc := app.NewService(r, players, d.Outbox, d.Authz, d.DB, d.Clock, d.Log, app.Options{
		ReplaceLevels:      cfg.ReconcileReplaceLevels,
		ReconcileBatchSize: cfg.ReconcileBatchSize,
		Drift:              driftRecorder(d.Metrics, d.Log),
	})
	return &Module{svc: svc, repo: r, deps: d, cfg: cfg}
}

// Reader is progression's offered synchronous read surface.
func (m *Module) Reader() contracts.Reader { return m.svc }

func (m *Module) Name() string { return contracts.Module }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue + grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

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
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	if ev.TenantID == "" {
		return errs.New(errs.Invalid, "tenant.deleted.v1 without tenant_id")
	}
	return m.svc.PurgeTenant(ctx, ev.TenantID)
}

func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{
		{
			Name: contracts.JobGrantXP,
			Run: func(ctx context.Context, body []byte) error {
				var cmd contracts.GrantXPCmdV1
				if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
					return err
				}
				_, err := m.svc.HandleGrantXP(ctx, cmd)
				return err
			},
		},
		{
			Name:     contracts.JobReconcile,
			Schedule: m.cfg.ReconcileSchedule,
			Run: func(ctx context.Context, _ []byte) error {
				_, err := m.svc.Reconcile(ctx, m.repo)
				return err
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

// driftRecorder exposes reconcile findings as
// progression_reconcile_drift_total{kind="total_xp"|"level"}.
func driftRecorder(reg *prometheus.Registry, log *zap.Logger) app.DriftRecorder {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "progression_reconcile_drift_total",
		Help: "Progress rows found drifting by progression.reconcile, by kind.",
	}, []string{"kind"})
	if reg != nil {
		if err := reg.Register(counter); err != nil {
			var are prometheus.AlreadyRegisteredError
			if errors.As(err, &are) {
				if existing, ok := are.ExistingCollector.(*prometheus.CounterVec); ok {
					counter = existing
				}
			} else if log != nil {
				log.Warn("progression: drift metric not registered", zap.Error(err))
			}
		}
	}
	return func(kind string, n int) {
		if n > 0 {
			counter.WithLabelValues(kind).Add(float64(n))
		}
	}
}
