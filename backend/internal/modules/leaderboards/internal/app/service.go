// Package app holds leaderboards' use cases: definition CRUD, score
// projection from upstream facts, ranking reads and the reconciling jobs.
// Postgres is the source of truth; the RankStore (redis-core sorted sets) is
// a derived read model touched only after commit.
package app

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/modules/leaderboards/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
)

// ListFilter narrows GET /leaderboards.
type ListFilter struct {
	Type   string
	Active *bool
}

// ScoreChange is one fact applied to one board period.
type ScoreChange struct {
	EventID  string
	Period   domain.Period
	PlayerID string
	Op       domain.Op
	At       time.Time // the fact's time: last-writer-wins for OpSet
	Now      time.Time
}

// ScoreResult reports what ApplyScore did. Applied=false means the event was
// already applied to this board (redelivery); Changed=false means an OpSet
// lost to a newer fact.
type ScoreResult struct {
	Applied bool
	Changed bool
	Score   int64
}

// PlayerScore is one of a player's score rows with its current visibility.
type PlayerScore struct {
	Period  domain.Period
	Score   int64
	Visible bool
}

// Repository is implemented by internal/repo against Postgres.
type Repository interface {
	Create(ctx context.Context, tx *gorm.DB, lb domain.Leaderboard) error
	ByID(ctx context.Context, tenantID, id string) (domain.Leaderboard, error)
	List(ctx context.Context, tenantID string, f ListFilter, beforeAt time.Time, beforeID string, limit int) ([]domain.Leaderboard, error)
	Save(ctx context.Context, tx *gorm.DB, lb domain.Leaderboard) error
	SoftDelete(ctx context.Context, tx *gorm.DB, lb domain.Leaderboard, at time.Time) error
	ActiveByType(ctx context.Context, tenantID, typ string) ([]domain.Leaderboard, error)
	// ActiveBoards pages every live active board across tenants by id (jobs).
	ActiveBoards(ctx context.Context, afterID string, limit int) ([]domain.Leaderboard, error)

	IsHidden(ctx context.Context, tx *gorm.DB, tenantID, playerID string) (bool, error)
	IsMember(ctx context.Context, tx *gorm.DB, programID, playerID string) (bool, error)
	ApplyScore(ctx context.Context, tx *gorm.DB, c ScoreChange) (ScoreResult, error)

	SetPlayerStatus(ctx context.Context, tx *gorm.DB, tenantID, playerID string, active bool, at time.Time) error
	MarkPlayerDeleted(ctx context.Context, tx *gorm.DB, tenantID, playerID string, at time.Time) error
	SetMembership(ctx context.Context, tx *gorm.DB, tenantID, programID, playerID string, enrolled bool, at time.Time) error
	// PlayerScores lists the player's rows in periods that are open or ended
	// after since (older read-model keys have expired anyway).
	PlayerScores(ctx context.Context, tenantID, playerID string, since time.Time) ([]PlayerScore, error)

	// RangeByPosition returns visible standings at list positions
	// [offset, offset+limit), ranked.
	RangeByPosition(ctx context.Context, lb domain.Leaderboard, periodStart time.Time, offset, limit int64) ([]domain.Standing, error)
	PositionOf(ctx context.Context, lb domain.Leaderboard, periodStart time.Time, playerID string) (domain.Standing, bool, error)
	// VisibleScores returns every visible row of the period, ordered.
	VisibleScores(ctx context.Context, lb domain.Leaderboard, periodStart time.Time) ([]domain.Standing, error)

	// Periods lists a board's known periods (openOnly: not yet closed).
	Periods(ctx context.Context, tenantID, leaderboardID string, openOnly bool) ([]domain.Period, error)
	// DuePeriods lists unclosed periods of live boards that ended before t.
	DuePeriods(ctx context.Context, before time.Time, limit int) ([]domain.Period, error)
	// ClosePeriod marks the period closed and writes its snapshot exactly
	// once, returning its top topN; closed=false when another run already
	// closed it.
	ClosePeriod(ctx context.Context, tx *gorm.DB, lb domain.Leaderboard, p domain.Period, at time.Time, snapshotLimit, topN int) (bool, []domain.Standing, error)

	// PurgeTenant hard-deletes every row of the tenant and returns its
	// boards and known periods, whose read-model keys are dropped after commit.
	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) ([]domain.Leaderboard, []domain.Period, error)
	PruneAppliedEvents(ctx context.Context, before time.Time) (int64, error)
	LastRun(ctx context.Context, job string) (time.Time, error)
	MarkRun(ctx context.Context, job string, at time.Time) error
}

// RankStore is the redis-core read model: one sorted set per board period.
// Increment and Set touch only periods that are fully loaded ("ready"), so a
// partial set can never be mistaken for the whole ranking.
type RankStore interface {
	Ready(ctx context.Context, p domain.Period) (bool, error)
	Increment(ctx context.Context, p domain.Period, playerID string, delta int64) error
	Set(ctx context.Context, p domain.Period, playerID string, score int64) error
	Remove(ctx context.Context, p domain.Period, playerIDs ...string) error
	// Range returns positions [start, stop] (inclusive) with scores; ranks
	// are assigned by the caller.
	Range(ctx context.Context, p domain.Period, start, stop int64) ([]domain.Standing, error)
	Position(ctx context.Context, p domain.Period, playerID string) (pos, score int64, found bool, err error)
	// CountAbove counts members with a strictly higher score.
	CountAbove(ctx context.Context, p domain.Period, score int64) (int64, error)
	// Replace atomically swaps the period's set for rows and marks it ready.
	Replace(ctx context.Context, p domain.Period, rows []domain.Standing, expireAt time.Time) error
	Delete(ctx context.Context, periods ...domain.Period) error
	TryLock(ctx context.Context, p domain.Period, ttl time.Duration) (bool, error)
}

// Settings are the module Config values the service needs.
type Settings struct {
	ClosedRetention  time.Duration // read-model lifetime after a period ends
	AllTimeTTL       time.Duration // rolling read-model lifetime of never boards
	CloseGrace       time.Duration // wait after period end before snapshotting
	SnapshotLimit    int           // ranks kept per closed period
	AppliedRetention time.Duration // applied_events kept for redelivery dedupe
}

type Service struct {
	repo     Repository
	ranks    RankStore
	players  ports.PlayerReader
	outbox   outbox.Store
	authz    authz.Enforcer
	db       *gorm.DB
	clock    clock.Clock
	log      *zap.Logger
	settings Settings

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
	// async runs read-model warm-ups off the request path; tests make it
	// synchronous.
	async func(fn func())
}

func NewService(repo Repository, ranks RankStore, players ports.PlayerReader, ob outbox.Store,
	enf authz.Enforcer, db *gorm.DB, c clock.Clock, log *zap.Logger, st Settings) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{
		repo: repo, ranks: ranks, players: players, outbox: ob, authz: enf,
		db: db, clock: c, log: log, settings: st,
	}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	s.async = func(fn func()) { go fn() }
	return s
}

// SetTxRunner replaces the transaction runner. Test seam for packages
// outside app (transport tests) that drive the service without a database.
func (s *Service) SetTxRunner(fn func(ctx context.Context, fn func(tx *gorm.DB) error) error) {
	s.tx = fn
}

func (s *Service) expiry(p domain.Period) time.Time {
	return domain.ReadModelExpiry(p, s.clock.Now(), s.settings.ClosedRetention, s.settings.AllTimeTTL)
}
