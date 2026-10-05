// Package rules is the rules engine (doc 06 §11, flow F2): rule definitions
// with immutable versions, a pure evaluator, and the decision log. It turns
// activity.received.v1 into one decision and one job command per effect.
// Public surface: this file, config.go, ports.go, contracts/, adapters/.
package rules

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"

	activitycontracts "levelup/internal/modules/activity/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/rules/contracts"
	"levelup/internal/modules/rules/internal/app"
	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/modules/rules/internal/repo"
	"levelup/internal/modules/rules/internal/transport"
	"levelup/internal/modules/rules/migrations"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/modkit"
	"levelup/internal/shared/errs"
)

// Group is the consumer group of every rules subscription.
const Group = "rules"

type Module struct {
	svc  *app.Service
	deps modkit.Deps
	cfg  Config
}

// New wires the module. Constructors do no I/O. players resolves activity
// players (by id or external id), progress and points feed player.level /
// player.xp / player.points conditions (read only when a rule references
// them).
func New(d modkit.Deps, cfg Config, players PlayerReader, progress ProgressReader, points PointsReader) *Module {
	var cache app.RulesetCache
	if d.Cache != nil {
		cache = repo.NewRulesetCache(d.Cache.WithTTL(cfg.CacheTTL))
	}
	svc := app.NewService(app.Deps{
		Repo: repo.NewPostgres(d.DB), Cache: cache, Players: players, Progress: progress, Points: points,
		Outbox: d.Outbox, Authz: d.Authz, DB: d.DB, Clock: d.Clock, Log: d.Log,
	}, app.Config{
		MaxCausationDepth: cfg.MaxCausationDepth,
		Compile:           eval.Options{MaxActions: cfg.MaxActionsPerRule, MaxConditions: cfg.MaxConditionsPerRule},
		PendingAfter:      cfg.EffectsPendingAfter,
		MaxAttempts:       cfg.EffectsMaxAttempts,
	})
	return &Module{svc: svc, deps: d, cfg: cfg}
}

// WithPrograms enables program scoping: a rule with program_id applies only
// to players enrolled in that program. Without it program_id is stored but
// ignored (rules are tenant-wide, Laravel parity). Call once, at wiring.
func (m *Module) WithPrograms(p ProgramReader) *Module {
	m.svc.SetPrograms(p)
	return m
}

func (m *Module) Name() string { return contracts.Module }

// Migrations land in schema rules_svc (R3).
func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations seeds the permission catalogue and default grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

// Subscriptions: the activity to decide, every outcome fact that settles an
// effect, and tenant deletion. All handlers are idempotent (ADR-0012).
func (m *Module) Subscriptions() []bus.Subscription {
	subs := []bus.Subscription{
		{Topic: activitycontracts.TopicReceived, Group: Group, Handler: m.onActivityReceived},
		{Topic: identitycontracts.TopicTenantDeleted, Group: Group, Handler: m.onTenantDeleted},
	}
	for _, topic := range OutcomeTopics() {
		subs = append(subs, bus.Subscription{Topic: topic, Group: Group, Handler: m.onOutcome(topic)})
	}
	return subs
}

// OutcomeTopics lists the outcome facts rules settles effects from, sorted.
func OutcomeTopics() []string {
	out := make([]string, 0, len(app.OutcomeTopics))
	for t := range app.OutcomeTopics {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func (m *Module) onActivityReceived(ctx context.Context, e bus.Envelope) error {
	var ev activitycontracts.ReceivedV1
	dec := json.NewDecoder(bytes.NewReader(e.Payload))
	dec.UseNumber() // keep numbers exact for typed comparisons
	if err := dec.Decode(&ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable activity.received.v1", err)
	}
	return m.svc.Decide(ctx, ev)
}

// onOutcome binds the handler to its topic instead of trusting e.Topic.
func (m *Module) onOutcome(topic string) bus.Handler {
	return func(ctx context.Context, e bus.Envelope) error {
		return m.svc.Settle(ctx, topic, e.Payload)
	}
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	return m.svc.PurgeTenant(ctx, ev.TenantID, ev.At)
}

// Jobs: the reconciling sweep that re-publishes effects whose outcome never
// arrived (R47). It is state-based: every run looks at all still-requested
// effects older than EffectsPendingAfter, so a skipped tick leaves no gap.
func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{{
		Name:     contracts.JobEffectsReconcile,
		Schedule: m.cfg.EffectsReconcileSchedule,
		Run: func(ctx context.Context, _ []byte) error {
			return m.svc.ReconcileEffects(ctx)
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
