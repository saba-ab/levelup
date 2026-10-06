// Package webhooks lets tenants register HTTPS endpoints that receive
// signed POSTs for a catalogue of facts (badges awarded, points credited,
// levels reached, ...). Delivery is at-least-once: a subscription fans each
// fact out into delivery rows plus job.webhooks.deliver commands through
// the outbox; the job retries on the broker's ladder and a reconciling
// sweep recovers lost work. Public surface: this file, config.go and
// contracts/.
package webhooks

import (
	"context"
	"encoding/json"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/webhooks/contracts"
	"levelup/internal/modules/webhooks/internal/app"
	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/modules/webhooks/internal/repo"
	"levelup/internal/modules/webhooks/internal/transport"
	"levelup/internal/modules/webhooks/migrations"
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

// New wires the module. Constructors do no I/O. webhooks reads no other
// module synchronously: it only consumes their facts.
func New(d modkit.Deps, cfg Config) *Module {
	cfg = cfg.withDefaults()
	policy := domain.URLPolicy{AllowInsecure: cfg.AllowInsecure}
	sender := app.NewHTTPSender(policy, cfg.Timeout, cfg.UserAgent)
	svc := app.NewService(repo.NewPostgres(d.DB), sender, d.Outbox, d.Authz, d.DB, d.Clock, app.Options{
		Policy:               policy,
		Retry:                domain.RetryPolicy{LadderAttempts: cfg.LadderAttempts, MaxAttempts: cfg.MaxAttempts},
		StaleAfter:           cfg.StaleAfter,
		DisableAfterFailures: cfg.DisableAfterFailures,
		Lease:                cfg.Timeout + cfg.Timeout/2,
	})
	return &Module{svc: svc, deps: d, cfg: cfg}
}

func (m *Module) Name() string { return contracts.Module }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue + grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Subscriptions: tenant purge plus one fan-out subscription per catalogue
// fact.
func (m *Module) Subscriptions() []bus.Subscription {
	subs := []bus.Subscription{
		{Topic: identitycontracts.TopicTenantDeleted, Group: contracts.Module, Handler: m.onTenantDeleted},
	}
	for _, et := range domain.Catalogue {
		subs = append(subs, bus.Subscription{Topic: et.Topic, Group: contracts.Module, Handler: m.onFact(et.Topic)})
	}
	return subs
}

// onFact binds the subscribed topic rather than trusting the envelope's.
func (m *Module) onFact(topic string) bus.Handler {
	return func(ctx context.Context, e bus.Envelope) error {
		return m.svc.HandleFact(ctx, app.Fact{
			Topic:      topic,
			EventID:    e.EventID,
			OccurredAt: e.OccurredAt,
			Payload:    e.Payload,
		})
	}
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
		{Name: contracts.JobDeliver, Run: m.runDeliver},
		{
			Name:     contracts.JobRetrySweep,
			Schedule: m.cfg.RetrySweepSchedule,
			Run: func(ctx context.Context, _ []byte) error {
				res, err := m.svc.RetrySweep(ctx)
				m.log().Info("webhook retry sweep",
					zap.Int("requeued", res.Requeued), zap.Int("failed", res.Failed),
					zap.Int("disabled", res.Disabled), zap.Error(err))
				return err
			},
		},
	}
}

func (m *Module) runDeliver(ctx context.Context, body []byte) error {
	var cmd contracts.DeliverCmdV1
	if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
		return err
	}
	return m.svc.HandleDeliver(ctx, cmd)
}

func (m *Module) log() *zap.Logger {
	if m.deps.Log == nil {
		return zap.NewNop()
	}
	return m.deps.Log
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
