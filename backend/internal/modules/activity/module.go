// Package activity is ingestion: it stores each tenant-reported fact once
// per (tenant, event_id), hands it to rules through the outbox, and
// projects rules' decision back onto the activity. Public surface: this
// file, config.go, ports.go, contracts/ and adapters/.
package activity

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus"

	"levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/activity/internal/app"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/modules/activity/internal/repo"
	"levelup/internal/modules/activity/internal/transport"
	"levelup/internal/modules/activity/migrations"
	badgescontracts "levelup/internal/modules/badges/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	missionscontracts "levelup/internal/modules/missions/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	rulescontracts "levelup/internal/modules/rules/contracts"
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

// New wires the module. Constructors do no I/O. players and eventTypes are
// the adapters the registry picked, e.g.
// adapters.NewLocalPlayers(playerMod.Reader()) and
// adapters.NewLocalEventTypes(eventcatalogMod.Reader()).
func New(d modkit.Deps, cfg Config, players PlayerReader, eventTypes EventTypeReader) *Module {
	cfg = cfg.withDefaults()
	svc := app.NewService(repo.NewPostgres(d.DB), players, eventTypes, d.Outbox, d.Authz, d.DB, d.Clock, d.Log,
		app.Settings{
			RequireKnownEventType: cfg.RequireKnownEventType,
			AutoCreatePlayers:     cfg.AutoCreatePlayers,
			Limits: domain.Limits{
				MaxAge:          cfg.MaxAge,
				MaxFutureSkew:   cfg.MaxFutureSkew,
				MaxPayloadBytes: cfg.MaxPayloadBytes,
			},
			StuckAfter:     cfg.StuckAfter,
			MaxRepublishes: cfg.MaxRepublishes,
			SweepBatch:     cfg.StuckSweepBatch,
		},
		republishedCounter(d.Metrics),
	)
	return &Module{svc: svc, deps: d, cfg: cfg}
}

// republishedCounter registers activity_stuck_republished_total once per
// registry; a second module instance on the same registry reuses it.
func republishedCounter(reg *prometheus.Registry) prometheus.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "activity_stuck_republished_total",
		Help: "activity.received.v1 events re-published by activity.stuck_sweep.",
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

// GoMigrations satisfies postgres.GoMigrator: the permission seed and the
// default grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

func (m *Module) Subscriptions() []bus.Subscription {
	subs := []bus.Subscription{
		{Topic: rulescontracts.TopicDecisionMade, Group: contracts.Module, Handler: m.onDecisionMade},
		{Topic: identitycontracts.TopicTenantDeleted, Group: contracts.Module, Handler: m.onTenantDeleted},
	}
	if m.cfg.InternalTriggers {
		subs = append(subs,
			bus.Subscription{Topic: progressioncontracts.TopicLevelReached, Group: contracts.Module, Handler: m.onLevelReached},
			bus.Subscription{Topic: badgescontracts.TopicAwarded, Group: contracts.Module, Handler: m.onBadgeAwarded},
			bus.Subscription{Topic: missionscontracts.TopicCompleted, Group: contracts.Module, Handler: m.onMissionCompleted},
		)
	}
	return subs
}

func (m *Module) onDecisionMade(ctx context.Context, e bus.Envelope) error {
	var ev rulescontracts.DecisionMadeV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.ApplyDecision(ctx, ev)
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.PurgeTenant(ctx, ev.TenantID)
}

func (m *Module) onLevelReached(ctx context.Context, e bus.Envelope) error {
	var ev progressioncontracts.LevelReachedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.RecordInternal(ctx, app.InternalTrigger{
		TenantID:         ev.TenantID,
		PlayerID:         ev.PlayerID,
		EventType:        contracts.EventTypeLevelUp,
		SourceTopic:      e.Topic,
		SourceEventID:    e.EventID,
		ParentActivityID: ev.ActivityID,
		Properties: map[string]any{
			"level_id":     ev.LevelID,
			"level_number": ev.LevelNumber,
			"level_name":   ev.LevelName,
			"total_xp":     ev.TotalXP,
		},
		OccurredAt: ev.At,
	})
}

func (m *Module) onBadgeAwarded(ctx context.Context, e bus.Envelope) error {
	var ev badgescontracts.AwardedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.RecordInternal(ctx, app.InternalTrigger{
		TenantID:         ev.TenantID,
		PlayerID:         ev.PlayerID,
		EventType:        contracts.EventTypeBadgeEarned,
		SourceTopic:      e.Topic,
		SourceEventID:    e.EventID,
		ParentActivityID: ev.Source.ActivityID,
		Properties: map[string]any{
			"award_id":     ev.AwardID,
			"badge_id":     ev.BadgeID,
			"badge_slug":   ev.BadgeSlug,
			"earned_count": ev.EarnedCount,
			"points_value": ev.PointsValue,
			"source_kind":  ev.Source.Kind,
		},
		OccurredAt: ev.At,
	})
}

func (m *Module) onMissionCompleted(ctx context.Context, e bus.Envelope) error {
	var ev missionscontracts.CompletedV1
	if err := decode(e, &ev); err != nil {
		return err
	}
	return m.svc.RecordInternal(ctx, app.InternalTrigger{
		TenantID:         ev.TenantID,
		PlayerID:         ev.PlayerID,
		EventType:        contracts.EventTypeMissionCompleted,
		SourceTopic:      e.Topic,
		SourceEventID:    e.EventID,
		ParentActivityID: ev.ActivityID,
		Properties: map[string]any{
			"attempt_id":    ev.AttemptID,
			"mission_id":    ev.MissionID,
			"mission_slug":  ev.MissionSlug,
			"points_reward": ev.PointsReward,
			"xp_reward":     ev.XPReward,
			"badge_id":      ev.BadgeID,
		},
		OccurredAt: ev.At,
	})
}

// decode turns an undecodable payload into errs.Invalid: the consumer loop
// parks it in the DLQ instead of retrying something that cannot succeed.
func decode(e bus.Envelope, dst any) error {
	if err := json.Unmarshal(e.Payload, dst); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable "+e.Topic, err)
	}
	return nil
}

func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name:     app.StuckSweepJob,
		Schedule: m.cfg.StuckSweepSchedule,
		Run: func(ctx context.Context, _ []byte) error {
			return m.svc.SweepStuck(ctx)
		},
	}}
}

func (m *Module) Health(ctx context.Context) error {
	sqlDB, err := m.deps.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (m *Module) Permissions() []authz.Permission { return contracts.AllPermissions }
