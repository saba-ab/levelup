// Package rewards is the reward catalogue and the owner of the claim saga:
// hold stock (tx1) → job.points.debit → settle on points.debited.v1 /
// points.debit_rejected.v1 (tx2), with reconciling sweeps for stuck holds
// and expiries. Public surface: this file, config.go, ports.go, adapters/
// and contracts/.
package rewards

import (
	"context"
	"encoding/json"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"

	identitycontracts "levelup/internal/modules/identity/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/app"
	"levelup/internal/modules/rewards/internal/repo"
	"levelup/internal/modules/rewards/internal/transport"
	"levelup/internal/modules/rewards/migrations"
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

// New wires the module. Constructors do no I/O. players wraps the player
// module's Reader, progress progression's Reader, points points' Reader
// (adapters.NewLocalPlayers / NewLocalProgress / NewLocalPoints).
func New(d modkit.Deps, cfg Config, players PlayerReader, progress ProgressReader, points PointsReader) *Module {
	svc := app.NewService(repo.NewPostgres(d.DB), players, progress, points, d.Outbox, d.Authz, d.DB, d.Clock,
		app.Settings{
			HoldTTL:           cfg.HoldTTL,
			SweepBatchSize:    cfg.SweepBatchSize,
			LateDebitLookback: cfg.LateDebitLookback,
		})
	return &Module{svc: svc, deps: d, cfg: cfg}
}

func (m *Module) Name() string { return contracts.Module }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: the permission catalogue and
// its default grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Subscriptions settle the claim saga and purge deleted tenants. All are
// idempotent: settlement is guarded by the claim's status under its lock.
func (m *Module) Subscriptions() []bus.Subscription {
	return []bus.Subscription{
		{Topic: pointscontracts.TopicDebited, Group: contracts.Module, Handler: m.onDebited},
		{Topic: pointscontracts.TopicDebitRejected, Group: contracts.Module, Handler: m.onDebitRejected},
		{Topic: pointscontracts.TopicRefunded, Group: contracts.Module, Handler: m.onRefunded},
		{Topic: identitycontracts.TopicTenantDeleted, Group: contracts.Module, Handler: m.onTenantDeleted},
	}
}

func (m *Module) onDebited(ctx context.Context, e bus.Envelope) error {
	var ev pointscontracts.LedgerMovedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable points.debited.v1", err)
	}
	return m.svc.OnDebited(ctx, ev)
}

func (m *Module) onDebitRejected(ctx context.Context, e bus.Envelope) error {
	var ev pointscontracts.MoveRejectedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable points.debit_rejected.v1", err)
	}
	return m.svc.OnDebitRejected(ctx, ev)
}

func (m *Module) onRefunded(ctx context.Context, e bus.Envelope) error {
	var ev pointscontracts.LedgerMovedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable points.refunded.v1", err)
	}
	return m.svc.OnRefunded(ctx, ev)
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	return m.svc.PurgeTenant(ctx, ev.TenantID)
}

// Jobs: the rewards.grant command consumer and the two reconciling crons.
func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{
		{
			Name: contracts.JobGrant,
			Run: func(ctx context.Context, body []byte) error {
				var cmd contracts.GrantCmdV1
				if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
					return err
				}
				return m.svc.Grant(ctx, cmd)
			},
		},
		{
			Name:     app.JobClaimsReconcile,
			Schedule: m.cfg.ClaimsReconcileSchedule,
			Run:      func(ctx context.Context, _ []byte) error { return m.svc.ReconcileClaims(ctx) },
		},
		{
			Name:     app.JobClaimsExpire,
			Schedule: m.cfg.ClaimsExpireSchedule,
			Run:      func(ctx context.Context, _ []byte) error { return m.svc.ExpireClaims(ctx) },
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
