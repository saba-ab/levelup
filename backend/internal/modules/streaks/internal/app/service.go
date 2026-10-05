// Package app holds streaks' use cases: transaction boundaries,
// authorization, and every outbox publish (ADR-0013).
package app

import (
	"context"
	"sync"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/streaks/contracts"
	"levelup/internal/modules/streaks/internal/domain"
	"levelup/internal/modules/streaks/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
	sweepBatchSize  = 500
)

// ListFilter narrows a streak listing.
type ListFilter struct {
	Active *bool
	Period string
}

// Page is a keyset page position: rows strictly older than (Before, BeforeID).
type Page struct {
	Before   time.Time
	BeforeID string
	Limit    int
}

// StreakRef names one streak that has live runs.
type StreakRef struct {
	TenantID string
	StreakID string
}

// Repository is implemented by internal/repo. Write methods take the tx.
type Repository interface {
	CreateStreak(ctx context.Context, tx *gorm.DB, s domain.Streak) error
	SaveStreak(ctx context.Context, tx *gorm.DB, s domain.Streak) error
	SoftDeleteStreak(ctx context.Context, tx *gorm.DB, tenantID, id string, at time.Time) error
	StreakByID(ctx context.Context, tenantID, id string) (domain.Streak, error)
	StreakByActivityKey(ctx context.Context, tenantID, key string) (domain.Streak, error)
	StreaksByIDs(ctx context.Context, tenantID string, ids []string) (map[string]domain.Streak, error)
	ListStreaks(ctx context.Context, tenantID string, f ListFilter, p Page) ([]domain.Streak, error)

	EnsurePlayerStreak(ctx context.Context, tx *gorm.DB, ps domain.PlayerStreak) error
	PlayerStreakForUpdate(ctx context.Context, tx *gorm.DB, tenantID, playerID, streakID string) (domain.PlayerStreak, error)
	SavePlayerStreak(ctx context.Context, tx *gorm.DB, ps domain.PlayerStreak) error
	PlayerStreaksByPlayers(ctx context.Context, tenantID string, playerIDs []string) ([]domain.PlayerStreak, error)

	InsertPeriod(ctx context.Context, tx *gorm.DB, b domain.PeriodBucket) (bool, error)
	PeriodStarts(ctx context.Context, tx *gorm.DB, playerStreakID string) ([]time.Time, error)

	AwardedMilestonesBetween(ctx context.Context, tx *gorm.DB, playerStreakID string, from, to time.Time) (map[int]bool, error)
	InsertMilestoneAward(ctx context.Context, tx *gorm.DB, a domain.MilestoneAward) (bool, error)

	RequestExists(ctx context.Context, tenantID, key string) (bool, error)
	InsertRequest(ctx context.Context, tx *gorm.DB, r domain.RecordRequest) (bool, error)
	PruneRequests(ctx context.Context, before time.Time) (int64, error)

	LiveStreakRefs(ctx context.Context) ([]StreakRef, error)
	LapsedForUpdate(ctx context.Context, tx *gorm.DB, tenantID, streakID string, before time.Time, limit int) ([]domain.PlayerStreak, error)
	LastRun(ctx context.Context, job string) (time.Time, error)
	MarkRun(ctx context.Context, job string, at time.Time) error

	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
	DeletePlayer(ctx context.Context, tx *gorm.DB, tenantID, playerID string) error
}

type Service struct {
	repo    Repository
	players ports.PlayerReader
	tenants ports.TenantReader
	outbox  outbox.Store
	authz   authz.Enforcer
	db      *gorm.DB
	clock   clock.Clock

	tzCache sync.Map // tz name → *time.Location

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, players ports.PlayerReader, tenants ports.TenantReader,
	ob outbox.Store, enf authz.Enforcer, db *gorm.DB, c clock.Clock) *Service {
	s := &Service{repo: repo, players: players, tenants: tenants, outbox: ob, authz: enf, db: db, clock: c}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

// guard resolves the tenant principal and checks a role-level permission.
func (s *Service) guard(ctx context.Context, perm authz.Permission, resource any) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, err
	}
	if err := s.authz.Authorize(ctx, p, perm, resource); err != nil {
		return authz.Principal{}, err
	}
	return p, nil
}

func (s *Service) Create(ctx context.Context, in domain.NewStreakInput) (domain.Streak, error) {
	p, err := s.guard(ctx, contracts.PermCreate, nil)
	if err != nil {
		return domain.Streak{}, err
	}
	st, err := domain.NewStreak(p.TenantID, in, s.clock.Now())
	if err != nil {
		return domain.Streak{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.CreateStreak(ctx, tx, st)
	}); err != nil {
		return domain.Streak{}, err
	}
	return st, nil
}

func (s *Service) Get(ctx context.Context, id string) (domain.Streak, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Streak{}, err
	}
	st, err := s.repo.StreakByID(ctx, p.TenantID, id)
	if err != nil {
		return domain.Streak{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, st); err != nil {
		return domain.Streak{}, err
	}
	return st, nil
}

// List pages newest-first by (created_at, id).
func (s *Service) List(ctx context.Context, f ListFilter, cursor string, limit int) ([]domain.Streak, string, error) {
	p, err := s.guard(ctx, contracts.PermViewAny, nil)
	if err != nil {
		return nil, "", err
	}
	if f.Period != "" {
		if _, err := domain.ParsePeriod(f.Period); err != nil {
			return nil, "", err
		}
	}
	page, err := pageOf(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	page.Limit++ // one extra row tells whether another page exists
	rows, err := s.repo.ListStreaks(ctx, p.TenantID, f, page)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) == page.Limit {
		rows = rows[:page.Limit-1]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

func (s *Service) Update(ctx context.Context, id string, patch domain.StreakPatch) (domain.Streak, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Streak{}, err
	}
	st, err := s.repo.StreakByID(ctx, p.TenantID, id)
	if err != nil {
		return domain.Streak{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermUpdate, st); err != nil {
		return domain.Streak{}, err
	}
	if err := st.Apply(patch, s.clock.Now()); err != nil {
		return domain.Streak{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.SaveStreak(ctx, tx, st)
	}); err != nil {
		return domain.Streak{}, err
	}
	st.Version++
	return st, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return err
	}
	st, err := s.repo.StreakByID(ctx, p.TenantID, id)
	if err != nil {
		return err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermDelete, st); err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.SoftDeleteStreak(ctx, tx, p.TenantID, st.ID, s.clock.Now())
	})
}

// PlayerStreakView pairs a player's state with its definition.
type PlayerStreakView struct {
	PlayerStreak domain.PlayerStreak
	Streak       domain.Streak
}

// ListPlayerStreaks lists a player's streak states. Read-only: unlike Laravel
// (S6) a read never creates rows.
func (s *Service) ListPlayerStreaks(ctx context.Context, playerID string) ([]PlayerStreakView, error) {
	p, err := s.guard(ctx, contracts.PermView, nil)
	if err != nil {
		return nil, err
	}
	if _, err := s.player(ctx, p.TenantID, playerID); err != nil {
		return nil, err
	}
	rows, err := s.repo.PlayerStreaksByPlayers(ctx, p.TenantID, []string{playerID})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.StreakID)
	}
	defs, err := s.repo.StreaksByIDs(ctx, p.TenantID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]PlayerStreakView, 0, len(rows))
	for _, r := range rows {
		def, ok := defs[r.StreakID]
		if !ok {
			continue // definition deleted
		}
		out = append(out, PlayerStreakView{PlayerStreak: r, Streak: def})
	}
	return out, nil
}

// player loads one player of the tenant, mapping absence to 404.
func (s *Service) player(ctx context.Context, tenantID, playerID string) (ports.PlayerSnapshot, error) {
	got, err := s.players.PlayersByIDs(ctx, tenantID, []string{playerID})
	if err != nil {
		return ports.PlayerSnapshot{}, err
	}
	snap, ok := got[playerID]
	if !ok || snap.TenantID != tenantID {
		return ports.PlayerSnapshot{}, domain.ErrPlayerNotFound
	}
	return snap, nil
}

// locations returns each tenant's timezone, UTC when unknown or invalid.
func (s *Service) locations(ctx context.Context, tenantIDs []string) (map[string]*time.Location, error) {
	got, err := s.tenants.TenantsByIDs(ctx, tenantIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*time.Location, len(tenantIDs))
	for _, id := range tenantIDs {
		out[id] = s.loadLocation(got[id].Timezone)
	}
	return out, nil
}

func (s *Service) location(ctx context.Context, tenantID string) (*time.Location, error) {
	locs, err := s.locations(ctx, []string{tenantID})
	if err != nil {
		return nil, err
	}
	return locs[tenantID], nil
}

func (s *Service) loadLocation(name string) *time.Location {
	if name == "" || name == "UTC" {
		return time.UTC
	}
	if l, ok := s.tzCache.Load(name); ok {
		return l.(*time.Location)
	}
	l, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	s.tzCache.Store(name, l)
	return l
}

func pageOf(cursor string, limit int) (Page, error) {
	switch {
	case limit <= 0:
		limit = defaultPageSize
	case limit > maxPageSize:
		limit = maxPageSize
	}
	p := Page{Limit: limit}
	if cursor != "" {
		before, beforeID, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
		p.Before, p.BeforeID = before, beforeID
	}
	return p, nil
}

var errDuplicateRequest = errs.New(errs.AlreadyExists, "record request already applied")
