// Package notifications sends player-facing notifications (in-app feed and
// email) rendered from tenant templates when a gamification fact happens.
// Public surface: this file + config.go + ports.go + contracts/.
package notifications

import (
	"context"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/app"
	"levelup/internal/modules/notifications/internal/repo"
	"levelup/internal/modules/notifications/internal/transport"
	"levelup/internal/modules/notifications/migrations"
	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/mail"
	"levelup/internal/platform/modkit"
	"levelup/internal/platform/postgres"
)

var (
	_ modkit.Module       = (*Module)(nil)
	_ postgres.GoMigrator = (*Module)(nil)
)

type Module struct {
	svc  *app.Service
	deps modkit.Deps
	cfg  Config
}

// New wires the module. players is the port implementation the registry
// chose, today adapters.NewLocalPlayers(playerMod.Reader()); mailer is the
// platform mail.Mailer. Constructors do no I/O.
func New(d modkit.Deps, cfg Config, players PlayerReader, mailer mail.Mailer) *Module {
	r := repo.NewPostgres(d.DB)
	svc := app.NewService(r, players, mailer, d.Outbox, d.Authz, d.DB, d.Clock, d.Log, app.Options{
		RenderTimeout:    cfg.RenderTimeout,
		SendTimeout:      cfg.SendTimeout,
		EmailMaxAttempts: cfg.EmailMaxAttempts,
		EmailStaleAfter:  cfg.EmailStaleAfter,
	})
	return &Module{svc: svc, deps: d, cfg: cfg}
}

func (m *Module) Name() string { return contracts.Module }

// Migrations land in schema notifications_svc with an own goose version table.
func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue and
// default role grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Subscriptions: every trigger fact fans out notifications; tenant and
// player deletion purge. All idempotent.
func (m *Module) Subscriptions() []bus.Subscription {
	subs := []bus.Subscription{
		{Topic: identitycontracts.TopicTenantDeleted, Group: contracts.Module, Handler: m.svc.OnTenantDeleted},
		{Topic: playercontracts.TopicPlayerDeleted, Group: contracts.Module, Handler: m.svc.OnPlayerDeleted},
	}
	for _, tt := range app.TriggerTopics() {
		subs = append(subs, bus.Subscription{Topic: tt.Topic, Group: contracts.Module, Handler: m.svc.Handler(tt)})
	}
	return subs
}

// Jobs: the send_email consumer and the stale-email sweep.
func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{
		{Name: contracts.JobSendEmail, Run: m.svc.HandleSendEmail},
		{
			Name:     contracts.JobSweepStaleEmail,
			Schedule: m.cfg.SweepSchedule,
			Run: func(ctx context.Context, _ []byte) error {
				return m.svc.SweepStaleEmail(ctx)
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
