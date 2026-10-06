// Package app holds the rules module's use cases: rule lifecycle (CRUD,
// versions, publish), the decision handler for activity.received.v1, effect
// settlement, simulation and decision queries. It owns every transaction
// boundary and every outbox publish (ADR-0013).
package app

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/modules/rules/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/pagination"
)

// Page is a keyset page over (created_at|evaluated_at, id) DESC.
type Page struct {
	Limit     int
	AfterTime time.Time
	AfterID   string
}

// RuleFilter filters rule lists.
type RuleFilter struct {
	Status       string
	TriggerEvent string
	ProgramID    string
}

// DecisionFilter filters decision lists.
type DecisionFilter struct {
	ActivityID string
	PlayerID   string
}

// LimitCheck asks the counters whether a matched rule may fire.
type LimitCheck struct {
	TenantID string
	RuleID   string
	PlayerID string
	Limits   eval.Limits
	At       time.Time // activity occurred_at: windows and cooldowns use it
}

// HistoryQuery asks for a player's prior activity counts per event type and
// window (grammar v2), relative to the activity time At.
type HistoryQuery struct {
	TenantID   string
	PlayerID   string
	EventTypes []string
	At         time.Time
}

// PlayerEvent is one evaluated activity counted into the history projection.
type PlayerEvent struct {
	TenantID  string
	PlayerID  string
	EventType string
	At        time.Time // activity occurred_at; the UTC day is the bucket
}

// RuleStat aggregates one rule's executions and effects over a period.
type RuleStat struct {
	RuleID          string
	Name            string
	Fired           int64
	NotMatched      int64
	Limited         int64
	OutOfSchedule   int64
	SkippedByStop   int64
	EffectsApplied  int64
	EffectsRejected int64
	PointsAwarded   int64
	XPAwarded       int64
}

// Repository is implemented in internal/repo. Write methods take the tx.
type Repository interface {
	CreateRule(ctx context.Context, tx *gorm.DB, r domain.Rule) error
	RuleByID(ctx context.Context, tenantID, id string) (domain.Rule, error)
	RuleForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Rule, error)
	SaveRule(ctx context.Context, tx *gorm.DB, r domain.Rule) error
	ListRules(ctx context.Context, tenantID string, f RuleFilter, p Page) ([]domain.Rule, error)

	CreateVersion(ctx context.Context, tx *gorm.DB, v domain.Version) error
	SaveDraftVersion(ctx context.Context, tx *gorm.DB, v domain.Version) error
	PublishVersion(ctx context.Context, tx *gorm.DB, v domain.Version) error
	VersionByNumberTx(ctx context.Context, tx *gorm.DB, tenantID, ruleID string, number int) (domain.Version, error)
	LatestVersionTx(ctx context.Context, tx *gorm.DB, tenantID, ruleID string) (domain.Version, error)
	VersionsByIDs(ctx context.Context, tenantID string, ids []string) (map[string]domain.Version, error)
	LatestVersion(ctx context.Context, tenantID, ruleID string) (domain.Version, error)
	ListVersions(ctx context.Context, tenantID, ruleID string, p Page) ([]domain.Version, error)

	BumpGeneration(ctx context.Context, tx *gorm.DB, tenantID string) (int64, error)
	Generation(ctx context.Context, tenantID string) (int64, error)
	LiveRuleSources(ctx context.Context, tenantID, triggerEvent string) ([]eval.RuleSource, error)

	DecisionExists(ctx context.Context, activityID string) (bool, error)
	InsertDecision(ctx context.Context, tx *gorm.DB, d domain.Decision) (bool, error)
	SetDecisionOutcome(ctx context.Context, tx *gorm.DB, d domain.Decision) error
	InsertExecutions(ctx context.Context, tx *gorm.DB, es []domain.Execution) error
	InsertEffects(ctx context.Context, tx *gorm.DB, es []domain.Effect) error
	ApplyLimits(ctx context.Context, tx *gorm.DB, c LimitCheck) (bool, error)
	RecordPlayerEvent(ctx context.Context, tx *gorm.DB, e PlayerEvent) error
	PlayerHistory(ctx context.Context, q HistoryQuery) (map[string]map[string]int64, error)
	RuleStats(ctx context.Context, tenantID string, from, to time.Time) ([]RuleStat, error)
	ListDecisions(ctx context.Context, tenantID string, f DecisionFilter, p Page) ([]domain.Decision, error)
	DecisionByID(ctx context.Context, tenantID, id string) (domain.Decision, error)
	ExecutionsByDecision(ctx context.Context, tenantID, decisionID string) ([]domain.Execution, error)
	EffectsByDecision(ctx context.Context, tenantID, decisionID string) ([]domain.Effect, error)
	SettleEffect(ctx context.Context, tx *gorm.DB, tenantID, key, status, reason string, at time.Time) (bool, error)
	PendingEffects(ctx context.Context, requestedBefore time.Time, maxAttempts, limit int) ([]domain.Effect, error)
	MarkEffectsRetried(ctx context.Context, tx *gorm.DB, ids []string, at time.Time) error

	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
}

// RulesetCache caches a tenant's live rule sources per (generation, trigger).
// The generation is part of the key, so a publish never serves stale rules;
// Evict is hygiene for the superseded generation and runs AFTER commit (R44).
type RulesetCache interface {
	Load(ctx context.Context, tenantID string, generation int64, trigger string,
		load func(ctx context.Context) ([]eval.RuleSource, error)) ([]eval.RuleSource, error)
	Evict(ctx context.Context, tenantID string, generation int64, triggers ...string)
}

// Config is the app-level slice of the module config.
type Config struct {
	MaxCausationDepth int
	Compile           eval.Options
	PendingAfter      time.Duration
	MaxAttempts       int
}

// Deps bundles the service's collaborators.
type Deps struct {
	Repo     Repository
	Cache    RulesetCache
	Players  ports.PlayerReader
	Progress ports.ProgressReader
	Points   ports.PointsReader
	Outbox   outbox.Store
	Authz    authz.Enforcer
	DB       *gorm.DB
	Clock    clock.Clock
	Log      *zap.Logger
}

type Service struct {
	repo     Repository
	cache    RulesetCache
	players  ports.PlayerReader
	progress ports.ProgressReader
	points   ports.PointsReader
	programs ports.ProgramReader
	outbox   outbox.Store
	authz    authz.Enforcer
	db       *gorm.DB
	clock    clock.Clock
	log      *zap.Logger
	cfg      Config

	compiled *programCache

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(d Deps, cfg Config) *Service {
	if cfg.MaxCausationDepth <= 0 {
		cfg.MaxCausationDepth = 3
	}
	if cfg.PendingAfter <= 0 {
		cfg.PendingAfter = 5 * time.Minute
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 10
	}
	log := d.Log
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{
		repo: d.Repo, cache: d.Cache, players: d.Players, progress: d.Progress, points: d.Points,
		outbox: d.Outbox, authz: d.Authz, db: d.DB, clock: d.Clock, log: log, cfg: cfg,
		compiled: newProgramCache(1024),
	}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

// SetPrograms enables program scoping (optional port, see ports.ProgramReader).
func (s *Service) SetPrograms(p ports.ProgramReader) { s.programs = p }

// authorize is the role gate every HTTP-facing method starts with.
func (s *Service) authorize(ctx context.Context, perm authz.Permission) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, err
	}
	if err := s.authz.Authorize(ctx, p, perm, nil); err != nil {
		return authz.Principal{}, err
	}
	return p, nil
}

const (
	defaultPageSize = 25
	maxPageSize     = 100
)

// pageFrom decodes ?limit=&cursor=. The repo is asked for one extra row to
// know whether a next page exists.
func pageFrom(cursor string, limit int) (Page, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	p := Page{Limit: limit + 1}
	if cursor != "" {
		t, id, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
		p.AfterTime, p.AfterID = t, id
	}
	return p, nil
}

// trimPage cuts the probe row and returns the next cursor.
func trimPage[T any](rows []T, p Page, key func(T) (time.Time, string)) ([]T, string) {
	if len(rows) < p.Limit {
		return rows, ""
	}
	rows = rows[:p.Limit-1]
	t, id := key(rows[len(rows)-1])
	return rows, pagination.EncodeCursor(t, id)
}

// programCache keeps compiled Programs per (tenant, generation, trigger).
// Programs are immutable, so sharing them is safe.
type programCache struct {
	mu  sync.Mutex
	max int
	m   map[string]*eval.Program
}

func newProgramCache(maxEntries int) *programCache {
	return &programCache{max: maxEntries, m: map[string]*eval.Program{}}
}

func (c *programCache) get(key string) (*eval.Program, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.m[key]
	return p, ok
}

func (c *programCache) put(key string, p *eval.Program) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= c.max {
		// Keys embed the generation, so dropping everything only costs a
		// recompile; no stale program can survive a publish either way.
		c.m = make(map[string]*eval.Program, c.max)
	}
	c.m[key] = p
}
