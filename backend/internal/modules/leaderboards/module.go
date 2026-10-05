// Package leaderboards ranks players per board and period. Scores are
// projections of other modules' facts (points, badges, missions,
// progression); Postgres holds the truth and redis-core sorted sets are the
// rebuildable read model (00-target-architecture D11, flow F7).
package leaderboards

import (
	"context"
	"encoding/json"
	"io/fs"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"

	badgescontracts "levelup/internal/modules/badges/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/modules/leaderboards/internal/app"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/modules/leaderboards/internal/repo"
	"levelup/internal/modules/leaderboards/internal/transport"
	"levelup/internal/modules/leaderboards/migrations"
	missionscontracts "levelup/internal/modules/missions/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	programcontracts "levelup/internal/modules/program/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/modkit"
	"levelup/internal/shared/errs"
)

const group = contracts.Module

type Module struct {
	svc  *app.Service
	deps modkit.Deps
	cfg  Config
}

// New wires the module. players is the port the composition root chose:
// adapters.NewLocalPlayers(playerMod.Reader()). Constructors do no I/O.
func New(d modkit.Deps, cfg Config, players PlayerReader) *Module {
	var ranks app.RankStore = repo.Disabled{}
	if d.Redis != nil {
		ranks = repo.NewRedis(d.Redis)
	}
	svc := app.NewService(repo.NewPostgres(d.DB), ranks, players, d.Outbox, d.Authz, d.DB, d.Clock, d.Log, app.Settings{
		ClosedRetention:  cfg.ClosedPeriodTTL,
		AllTimeTTL:       cfg.AllTimeTTL,
		CloseGrace:       cfg.CloseGrace,
		SnapshotLimit:    cfg.SnapshotLimit,
		AppliedRetention: cfg.AppliedEventsRetention,
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

func (m *Module) Subscriptions() []bus.Subscription {
	sub := func(topic string, h bus.Handler) bus.Subscription {
		return bus.Subscription{Topic: topic, Group: group, Handler: h}
	}
	return []bus.Subscription{
		sub(pointscontracts.TopicCredited, m.onLedger(domain.FactPointsCredited)),
		sub(pointscontracts.TopicDebited, m.onLedger(domain.FactPointsDebited)),
		sub(pointscontracts.TopicRefunded, m.onLedger(domain.FactPointsRefunded)),
		sub(badgescontracts.TopicAwarded, m.onBadgeAwarded),
		sub(missionscontracts.TopicCompleted, m.onMissionCompleted),
		sub(progressioncontracts.TopicXPGained, m.onXPGained),
		sub(playercontracts.TopicPlayerActivated, m.onPlayerStatus),
		sub(playercontracts.TopicPlayerDeactivated, m.onPlayerStatus),
		sub(playercontracts.TopicPlayerDeleted, m.onPlayerDeleted),
		sub(programcontracts.TopicPlayerEnrolled, m.onEnrollment(true)),
		sub(programcontracts.TopicPlayerUnenrolled, m.onEnrollment(false)),
		sub(identitycontracts.TopicTenantDeleted, m.onTenantDeleted),
	}
}

func decode(e bus.Envelope, dst any) error {
	if err := json.Unmarshal(e.Payload, dst); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable "+e.Topic, err)
	}
	return nil
}

// firstTime picks the fact's own time, falling back to the envelope's.
func firstTime(e bus.Envelope, ts ...time.Time) time.Time {
	for _, t := range ts {
		if !t.IsZero() {
			return t.UTC()
		}
	}
	return e.OccurredAt.UTC()
}

func (m *Module) onLedger(kind domain.FactKind) bus.Handler {
	return func(ctx context.Context, e bus.Envelope) error {
		var ev pointscontracts.LedgerMovedV1
		if err := decode(e, &ev); err != nil {
			return err
		}
		return m.svc.ApplyFact(ctx, domain.Fact{
			EventID: e.EventID, TenantID: ev.TenantID, PlayerID: ev.PlayerID, Kind: kind,
			Amount: ev.Amount, Absolute: ev.BalanceAfter, At: firstTime(e, ev.OccurredAt, ev.At),
		})
	}
}

func (m *Module) onBadgeAwarded(ctx context.Context, e bus.Envelope) error {
	var ev badgescontracts.AwardedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.ApplyFact(ctx, domain.Fact{
		EventID: e.EventID, TenantID: ev.TenantID, PlayerID: ev.PlayerID, Kind: domain.FactBadgeAwarded,
		Amount: 1, At: firstTime(e, ev.OccurredAt, ev.At),
	})
}

func (m *Module) onMissionCompleted(ctx context.Context, e bus.Envelope) error {
	var ev missionscontracts.CompletedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.ApplyFact(ctx, domain.Fact{
		EventID: e.EventID, TenantID: ev.TenantID, PlayerID: ev.PlayerID, Kind: domain.FactMissionCompleted,
		Amount: 1, At: firstTime(e, ev.At),
	})
}

func (m *Module) onXPGained(ctx context.Context, e bus.Envelope) error {
	var ev progressioncontracts.XPGainedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.ApplyFact(ctx, domain.Fact{
		EventID: e.EventID, TenantID: ev.TenantID, PlayerID: ev.PlayerID, Kind: domain.FactXPGained,
		Amount: ev.Amount, Absolute: ev.TotalXP, At: firstTime(e, ev.OccurredAt, ev.At),
	})
}

func (m *Module) onPlayerStatus(ctx context.Context, e bus.Envelope) error {
	var ev playercontracts.PlayerStatusV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	// The topic is authoritative for the direction; the payload flag agrees.
	active := e.Topic == playercontracts.TopicPlayerActivated
	if e.Topic == "" {
		active = ev.Active
	}
	return m.svc.SetPlayerActive(ctx, ev.TenantID, ev.PlayerID, active, firstTime(e, ev.At))
}

func (m *Module) onPlayerDeleted(ctx context.Context, e bus.Envelope) error {
	var ev playercontracts.PlayerDeletedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.PlayerDeleted(ctx, ev.TenantID, ev.PlayerID, firstTime(e, ev.At))
}

func (m *Module) onEnrollment(enrolled bool) bus.Handler {
	return func(ctx context.Context, e bus.Envelope) error {
		var ev programcontracts.EnrollmentChangedV1
		if err := decode(e, &ev); err != nil {
			return err
		}
		return m.svc.SetMembership(ctx, ev.TenantID, ev.ProgramID, ev.PlayerID, enrolled, firstTime(e, ev.At))
	}
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.PurgeTenant(ctx, ev.TenantID)
}

func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{
		{Name: app.JobRollover, Schedule: m.cfg.RolloverSchedule, Run: func(ctx context.Context, _ []byte) error {
			return m.svc.Rollover(ctx)
		}},
		{Name: app.JobRebuild, Schedule: m.cfg.RebuildSchedule, Run: func(ctx context.Context, _ []byte) error {
			return m.svc.RebuildAll(ctx)
		}},
		{Name: app.JobPruneEvents, Schedule: m.cfg.PruneSchedule, Run: func(ctx context.Context, _ []byte) error {
			return m.svc.PruneAppliedEvents(ctx)
		}},
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
