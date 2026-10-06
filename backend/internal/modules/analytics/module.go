// Package analytics projects other modules' facts into per-tenant daily
// counters and player activity days (UTC) and serves the portal's Analytics
// page: overview, engagement, retention cohorts and funnels.
// Public surface: this file, config.go and contracts/.
package analytics

import (
	"context"
	"encoding/json"
	"io/fs"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/analytics/contracts"
	"levelup/internal/modules/analytics/internal/app"
	"levelup/internal/modules/analytics/internal/domain"
	"levelup/internal/modules/analytics/internal/repo"
	"levelup/internal/modules/analytics/internal/transport"
	"levelup/internal/modules/analytics/migrations"
	badgescontracts "levelup/internal/modules/badges/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	missionscontracts "levelup/internal/modules/missions/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	rewardscontracts "levelup/internal/modules/rewards/contracts"
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

// New wires the module. Analytics reads no other module synchronously: it
// needs no ports. Constructors do no I/O.
func New(d modkit.Deps, cfg Config) *Module {
	svc := app.NewService(repo.NewPostgres(d.DB), d.Authz, d.DB, d.Clock, app.Settings{
		Retention:        cfg.Retention,
		AppliedRetention: cfg.AppliedEventsRetention,
	})
	return &Module{svc: svc, deps: d, cfg: cfg}
}

func (m *Module) Name() string { return contracts.Module }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue + grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) { transport.NewHandler(m.svc).Mount(r) }

func (m *Module) Subscriptions() []bus.Subscription {
	sub := func(topic string, h bus.Handler) bus.Subscription {
		return bus.Subscription{Topic: topic, Group: contracts.Module, Handler: h}
	}
	return []bus.Subscription{
		sub(activitycontracts.TopicReceived, m.onActivity),
		sub(pointscontracts.TopicCredited, m.onLedger(contracts.MetricPointsCredited)),
		sub(pointscontracts.TopicDebited, m.onLedger(contracts.MetricPointsDebited)),
		sub(badgescontracts.TopicAwarded, m.onBadgeAwarded),
		sub(missionscontracts.TopicStarted, m.onMissionStarted),
		sub(missionscontracts.TopicCompleted, m.onMissionCompleted),
		sub(progressioncontracts.TopicLevelReached, m.onLevelReached),
		sub(rewardscontracts.TopicClaimed, m.onRewardClaimed),
		sub(playercontracts.TopicPlayerCreated, m.onPlayerCreated),
		sub(identitycontracts.TopicTenantDeleted, m.onTenantDeleted),
	}
}

func decode(e bus.Envelope, dst any) error {
	if err := json.Unmarshal(e.Payload, dst); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable "+e.Topic, err)
	}
	return nil
}

// count records one counter increment for a fact at the first non-zero time.
func (m *Module) count(ctx context.Context, e bus.Envelope, tenantID, metric, dim string, value int64, ts ...time.Time) error {
	return m.svc.Record(ctx, domain.Fact{
		EventID:  e.EventID,
		TenantID: tenantID,
		Day:      domain.FirstTime(append(ts, e.OccurredAt)...),
		Counters: []domain.Counter{{Metric: metric, Dimension: dim, Value: value}},
	})
}

// onActivity counts the activity by event type and, when the player was
// resolved at ingest, marks them active that day. Unresolved activities
// (no player_id) are counted but attributed to no player.
func (m *Module) onActivity(ctx context.Context, e bus.Envelope) error {
	var ev activitycontracts.ReceivedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.Record(ctx, domain.Fact{
		EventID:   e.EventID,
		TenantID:  ev.TenantID,
		Day:       domain.FirstTime(ev.OccurredAt, ev.ReceivedAt, e.OccurredAt),
		Counters:  []domain.Counter{{Metric: contracts.MetricActivities, Dimension: ev.EventType, Value: 1}},
		PlayerID:  ev.PlayerID,
		EventType: ev.EventType,
	})
}

func (m *Module) onLedger(metric string) bus.Handler {
	return func(ctx context.Context, e bus.Envelope) error {
		var ev pointscontracts.LedgerMovedV1
		if err := decode(e, &ev); err != nil {
			return err
		}
		return m.count(ctx, e, ev.TenantID, metric, ev.Kind, ev.Amount, ev.OccurredAt, ev.At)
	}
}

func (m *Module) onBadgeAwarded(ctx context.Context, e bus.Envelope) error {
	var ev badgescontracts.AwardedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.count(ctx, e, ev.TenantID, contracts.MetricBadgesAwarded, ev.BadgeID, 1, ev.OccurredAt, ev.At)
}

func (m *Module) onMissionStarted(ctx context.Context, e bus.Envelope) error {
	var ev missionscontracts.AttemptV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.count(ctx, e, ev.TenantID, contracts.MetricMissionsStarted, ev.MissionID, 1, ev.At)
}

func (m *Module) onMissionCompleted(ctx context.Context, e bus.Envelope) error {
	var ev missionscontracts.CompletedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.count(ctx, e, ev.TenantID, contracts.MetricMissionsCompleted, ev.MissionID, 1, ev.At)
}

func (m *Module) onLevelReached(ctx context.Context, e bus.Envelope) error {
	var ev progressioncontracts.LevelReachedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.count(ctx, e, ev.TenantID, contracts.MetricLevelsReached, strconv.Itoa(ev.LevelNumber), 1, ev.At)
}

func (m *Module) onRewardClaimed(ctx context.Context, e bus.Envelope) error {
	var ev rewardscontracts.ClaimV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.count(ctx, e, ev.TenantID, contracts.MetricRewardsClaimed, ev.RewardID, 1, ev.At)
}

func (m *Module) onPlayerCreated(ctx context.Context, e bus.Envelope) error {
	var ev playercontracts.PlayerCreatedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.count(ctx, e, ev.TenantID, contracts.MetricPlayersCreated, "", 1, ev.At)
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.PurgeTenant(ctx, ev.TenantID)
}

func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name:     contracts.JobPrune,
		Schedule: m.cfg.PruneSchedule,
		Run: func(ctx context.Context, _ []byte) error {
			n, err := m.svc.Prune(ctx)
			m.log().Info("analytics prune", zap.Int64("deleted", n), zap.Error(err))
			return err
		},
	}}
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
