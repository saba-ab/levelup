// Package streaks counts consecutive periods (day/week/month in the
// tenant's timezone) with recorded activity, pays points per period and
// milestone bonuses through job.points.credit, and breaks lapsed runs.
// Public surface: this file, config.go, ports.go, contracts/ and adapters/.
package streaks

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"

	activitycontracts "levelup/internal/modules/activity/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/streaks/contracts"
	"levelup/internal/modules/streaks/internal/app"
	"levelup/internal/modules/streaks/internal/repo"
	"levelup/internal/modules/streaks/internal/transport"
	"levelup/internal/modules/streaks/migrations"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/modkit"
	"levelup/internal/shared/errs"
)

type Module struct {
	svc    *app.Service
	reader *app.Reader
	deps   modkit.Deps
	cfg    Config
}

// New wires the module. Constructors do no I/O. players wraps player's
// Reader, tenants wraps identity's TenantReader (timezone source).
func New(d modkit.Deps, cfg Config, players PlayerReader, tenants TenantReader) *Module {
	r := repo.NewPostgres(d.DB)
	svc := app.NewService(r, players, tenants, d.Outbox, d.Authz, d.DB, d.Clock)
	return &Module{svc: svc, reader: app.NewReader(r), deps: d, cfg: cfg}
}

// Reader is streaks' offered synchronous read surface.
func (m *Module) Reader() contracts.Reader { return m.reader }

func (m *Module) Name() string { return contracts.Module }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue + grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

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

// onActivityReceived records the period of an activity for the streak whose
// activity_key equals its event type (when that streak auto-records).
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
	return m.svc.PurgeTenant(ctx, ev.TenantID)
}

func (m *Module) onPlayerDeleted(ctx context.Context, e bus.Envelope) error {
	var ev playercontracts.PlayerDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable player.deleted.v1", err)
	}
	return m.svc.DeletePlayer(ctx, ev.TenantID, ev.PlayerID)
}

func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{
		{Name: contracts.JobRecord, Run: m.runRecord},
		{
			Name:     contracts.JobBreakSweep,
			Schedule: m.cfg.BreakSweepSchedule,
			Run: func(ctx context.Context, _ []byte) error {
				n, err := m.svc.BreakSweep(ctx)
				m.log().Info("streak break sweep", zap.Int("broken", n), zap.Error(err))
				return err
			},
		},
		{
			Name:     contracts.JobPruneRequests,
			Schedule: m.cfg.PruneSchedule,
			Run: func(ctx context.Context, _ []byte) error {
				_, err := m.svc.PruneRequests(ctx, m.cfg.RequestRetention)
				return err
			},
		},
	}
}

func (m *Module) runRecord(ctx context.Context, body []byte) error {
	var cmd contracts.RecordCmdV1
	if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
		return err
	}
	_, err := m.svc.HandleRecord(ctx, cmd)
	return err
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
