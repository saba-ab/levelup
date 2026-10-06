// Package segments defines dynamic player segments (conditions over player
// attributes, status, level, wallet, badges and last activity) and
// materializes their membership with refresh runs, publishing every change
// as segments.membership_changed.v1.
// Public surface: this file, config.go, ports.go, contracts/ and adapters/.
package segments

import (
	"context"
	"encoding/json"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"

	identitycontracts "levelup/internal/modules/identity/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/modules/segments/contracts"
	"levelup/internal/modules/segments/internal/app"
	"levelup/internal/modules/segments/internal/repo"
	"levelup/internal/modules/segments/internal/transport"
	"levelup/internal/modules/segments/migrations"
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

// New wires the module. Constructors do no I/O. The ports wrap other
// modules' Readers (see adapters/): players (player Reader + an id
// lister), progress (progression), wallets (points), badges (badges) and
// activity (activity LastSeen).
func New(d modkit.Deps, cfg Config, players PlayerReader, progress ProgressReader,
	wallets WalletReader, badges BadgeReader, activity ActivityReader) *Module {
	svc := app.NewService(repo.NewPostgres(d.DB), app.Readers{
		Players: players, Progress: progress, Wallets: wallets, Badges: badges, Activity: activity,
	}, d.Outbox, d.Authz, d.DB, d.Clock, app.Settings{
		PageSize:     cfg.PageSize,
		Lease:        cfg.RefreshLease,
		PreviewLimit: cfg.PreviewLimit,
	})
	return &Module{svc: svc, reader: app.NewReader(svc), deps: d, cfg: cfg}
}

// Reader is segments' offered synchronous read surface.
func (m *Module) Reader() contracts.Reader { return m.reader }

func (m *Module) Name() string { return contracts.Module }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue + grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate, m.deps.Clock.Now).Mount(r)
}

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
	return m.svc.PlayerDeleted(ctx, ev.TenantID, ev.PlayerID)
}

func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{
		{Name: contracts.JobRefreshSegment, Run: m.runRefreshSegment},
		{
			Name:     contracts.JobRefresh,
			Schedule: m.cfg.RefreshSchedule,
			Run: func(ctx context.Context, _ []byte) error {
				n, err := m.svc.RefreshAll(ctx)
				m.log().Info("segments refresh", zap.Int("segments", n), zap.Error(err))
				return err
			},
		},
	}
}

func (m *Module) runRefreshSegment(ctx context.Context, body []byte) error {
	var cmd contracts.RefreshSegmentCmdV1
	if _, err := jobs.DecodeCommand(body, &cmd); err != nil {
		return err
	}
	_, err := m.svc.RefreshSegment(ctx, cmd.TenantID, cmd.SegmentID)
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
