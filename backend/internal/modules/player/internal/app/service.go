// Package app holds player's use cases: tenant scoping, authorization,
// transaction boundaries and every outbox publish (ADR-0013, ADR-0015).
package app

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"levelup/internal/modules/player/contracts"
	"levelup/internal/modules/player/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

const (
	DefaultPageSize = 25
	MaxPageSize     = 100
	// MaxBatch bounds one Reader call; callers chunk larger sets.
	MaxBatch = 500
	// MaxIDPage bounds one ListPlayerIDs page.
	MaxIDPage = 1000
)

// Ref identifies a player for cache eviction: both lookup keys.
type Ref struct {
	ID         string
	ExternalID string
}

// ListFilter is a keyset page request. Sort is one of the contracts.Sort*
// values (never empty here); the keyset is (created_at, id) for the
// created sorts and (SortName, id) for display_name.
type ListFilter struct {
	Active      *bool
	Search      string // case-insensitive prefix of external_id, display_name or email
	Sort        string
	CreatedFrom *time.Time // inclusive
	CreatedTo   *time.Time // exclusive
	After       time.Time
	AfterKey    string // display_name sort only
	AfterID     string
	Limit       int
	HasAfter    bool
}

// Repository is consumed here and implemented in internal/repo. Write
// methods take the transaction explicitly; reads use the repo's own handle.
// Every method filters on tenantID and excludes soft-deleted rows.
type Repository interface {
	// Create inserts unless a live player holds (tenant, external_id); then
	// it returns domain.ErrExternalIDTaken (race-safe, no pre-check).
	Create(ctx context.Context, tx *gorm.DB, p domain.Player) error
	// CreateIfAbsent inserts unless ANY unique key collides (the primary key,
	// even of a soft-deleted row, or the live (tenant, external_id)); it
	// reports whether a row was written and never aborts the transaction.
	CreateIfAbsent(ctx context.Context, tx *gorm.DB, p domain.Player) (bool, error)
	ByID(ctx context.Context, tenantID, id string) (domain.Player, error)
	ByExternalID(ctx context.Context, tenantID, externalID string) (domain.Player, error)
	ByIDForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Player, error)
	// Save writes p guarded by p.Version and bumps it.
	Save(ctx context.Context, tx *gorm.DB, p domain.Player) error
	List(ctx context.Context, tenantID string, f ListFilter) ([]domain.Player, error)
	ByIDs(ctx context.Context, tenantID string, ids []string) ([]domain.Player, error)
	ByExternalIDs(ctx context.Context, tenantID string, externalIDs []string) ([]domain.Player, error)
	IDsAfter(ctx context.Context, tenantID, afterID string, limit int) ([]string, error)
	// PurgeTenantBatch hard-deletes up to limit of the tenant's rows (live
	// or soft-deleted) and returns what it removed.
	PurgeTenantBatch(ctx context.Context, tx *gorm.DB, tenantID string, limit int) ([]Ref, error)
	// Evict drops cached reads. Called AFTER commit only (R44).
	Evict(ctx context.Context, tenantID string, refs ...Ref)
}

type Service struct {
	repo      Repository
	outbox    outbox.Store
	authz     authz.Enforcer
	db        *gorm.DB
	clock     clock.Clock
	purgeSize int

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, ob outbox.Store, enf authz.Enforcer, db *gorm.DB, c clock.Clock, purgeBatch int) *Service {
	if purgeBatch <= 0 {
		purgeBatch = 500
	}
	s := &Service{repo: repo, outbox: ob, authz: enf, db: db, clock: c, purgeSize: purgeBatch}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

// WithTxRunner swaps the transaction runner. Tests outside this package
// pass a pass-through so the service needs no database.
func (s *Service) WithTxRunner(run func(ctx context.Context, fn func(tx *gorm.DB) error) error) *Service {
	s.tx = run
	return s
}

// CreateCmd is the client-settable part of a new player.
type CreateCmd struct {
	ExternalID  string
	DisplayName string
	Email       string
	Attributes  map[string]any
}

// Create registers a player in the principal's tenant and records
// player.created.v1 in the same transaction.
func (s *Service) Create(ctx context.Context, cmd CreateCmd) (domain.Player, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Player{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermCreate, nil); err != nil {
		return domain.Player{}, err
	}
	pl, err := domain.NewPlayer(id.NewID(), p.TenantID, cmd.ExternalID, domain.Profile{
		DisplayName: cmd.DisplayName,
		Email:       cmd.Email,
		Attributes:  cmd.Attributes,
	}, uuidOrEmpty(p.UserID), s.clock.Now())
	if err != nil {
		return domain.Player{}, err
	}

	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.Create(ctx, tx, pl); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicPlayerCreated, contracts.PlayerCreatedV1{
			PlayerID:    pl.ID,
			TenantID:    pl.TenantID,
			ExternalID:  pl.ExternalID,
			DisplayName: pl.DisplayName,
			At:          pl.CreatedAt,
		})
	})
	if err != nil {
		return domain.Player{}, err
	}
	// A reader may have tombstoned this external id while it did not exist.
	s.repo.Evict(ctx, pl.TenantID, Ref{ID: pl.ID, ExternalID: pl.ExternalID})
	return pl, nil
}

// Get returns a player of the principal's tenant; another tenant's player
// is not found.
func (s *Service) Get(ctx context.Context, playerID string) (domain.Player, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Player{}, err
	}
	if !isUUID(playerID) {
		return domain.Player{}, domain.ErrNotFound
	}
	pl, err := s.repo.ByID(ctx, p.TenantID, playerID)
	if err != nil {
		return domain.Player{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, pl); err != nil {
		return domain.Player{}, err
	}
	return pl, nil
}

func (s *Service) GetByExternalID(ctx context.Context, externalID string) (domain.Player, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Player{}, err
	}
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return domain.Player{}, domain.ErrNotFound
	}
	pl, err := s.repo.ByExternalID(ctx, p.TenantID, externalID)
	if err != nil {
		return domain.Player{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, pl); err != nil {
		return domain.Player{}, err
	}
	return pl, nil
}

// ListQuery is the HTTP-facing page request. Sort "" means
// contracts.SortCreatedDesc. CreatedFrom is inclusive, CreatedTo exclusive.
type ListQuery struct {
	Cursor      string
	Limit       int
	Active      *bool
	Search      string
	Sort        string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
}

// Page is one keyset page; NextCursor is empty on the last page.
type Page struct {
	Players    []domain.Player
	NextCursor string
}

func (s *Service) List(ctx context.Context, q ListQuery) (Page, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return Page{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, nil); err != nil {
		return Page{}, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	sortBy := q.Sort
	if sortBy == "" {
		sortBy = contracts.SortCreatedDesc
	}
	if !validSort(sortBy) {
		return Page{}, domain.ErrInvalidSort
	}
	if q.CreatedFrom != nil && q.CreatedTo != nil && !q.CreatedFrom.Before(*q.CreatedTo) {
		return Page{}, domain.ErrInvalidCreatedRange
	}
	f := ListFilter{
		Active:      q.Active,
		Search:      strings.TrimSpace(q.Search),
		Sort:        sortBy,
		CreatedFrom: utc(q.CreatedFrom),
		CreatedTo:   utc(q.CreatedTo),
		Limit:       limit + 1,
	}
	if q.Cursor != "" {
		c, err := decodeListCursor(sortBy, q.Cursor)
		if err != nil {
			return Page{}, err
		}
		f.After, f.AfterKey, f.AfterID, f.HasAfter = c.at, c.key, c.id, true
	}
	rows, err := s.repo.List(ctx, p.TenantID, f)
	if err != nil {
		return Page{}, err
	}
	page := Page{Players: rows}
	if len(rows) > limit {
		page.Players = rows[:limit]
		page.NextCursor = encodeListCursor(sortBy, page.Players[limit-1])
	}
	return page, nil
}

// Update applies a true partial update. Nothing changed: no write, no
// event. A flipped is_active also publishes activated/deactivated.
func (s *Service) Update(ctx context.Context, playerID string, patch domain.Patch) (domain.Player, error) {
	return s.mutate(ctx, playerID, contracts.PermUpdate, func(pl *domain.Player, now time.Time) ([]string, error) {
		return pl.ApplyPatch(patch, now)
	})
}

func (s *Service) Activate(ctx context.Context, playerID string) (domain.Player, error) {
	return s.mutate(ctx, playerID, contracts.PermUpdate, func(pl *domain.Player, now time.Time) ([]string, error) {
		if pl.Activate(now) {
			return []string{domain.FieldActive}, nil
		}
		return nil, nil
	})
}

func (s *Service) Deactivate(ctx context.Context, playerID string) (domain.Player, error) {
	return s.mutate(ctx, playerID, contracts.PermUpdate, func(pl *domain.Player, now time.Time) ([]string, error) {
		if pl.Deactivate(now) {
			return []string{domain.FieldActive}, nil
		}
		return nil, nil
	})
}

// mutate is the shared locked read-modify-write: role check, tenant
// ownership (foreign → 404), row lock, versioned save, publishes in-tx,
// eviction after commit.
func (s *Service) mutate(ctx context.Context, playerID string, perm authz.Permission,
	change func(pl *domain.Player, now time.Time) ([]string, error)) (domain.Player, error) {

	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Player{}, err
	}
	if !isUUID(playerID) {
		return domain.Player{}, domain.ErrNotFound
	}
	if err := s.authz.Authorize(ctx, p, perm, nil); err != nil {
		return domain.Player{}, err
	}

	var out domain.Player
	var changed []string
	err = s.tx(ctx, func(tx *gorm.DB) error {
		pl, err := s.repo.ByIDForUpdate(ctx, tx, p.TenantID, playerID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		changed, err = change(&pl, now)
		if err != nil {
			return err
		}
		if len(changed) == 0 {
			out = pl
			return nil
		}
		if err := s.repo.Save(ctx, tx, pl); err != nil {
			return err
		}
		pl.Version++
		out = pl
		return s.publishChanges(ctx, tx, pl, changed, now)
	})
	if err != nil {
		return domain.Player{}, err
	}
	if len(changed) > 0 {
		s.repo.Evict(ctx, out.TenantID, Ref{ID: out.ID, ExternalID: out.ExternalID})
	}
	return out, nil
}

func (s *Service) publishChanges(ctx context.Context, tx *gorm.DB, pl domain.Player, changed []string, now time.Time) error {
	statusOnly := len(changed) == 1 && changed[0] == domain.FieldActive
	if !statusOnly {
		if err := s.outbox.Publish(ctx, tx, contracts.TopicPlayerUpdated, contracts.PlayerUpdatedV1{
			PlayerID:      pl.ID,
			TenantID:      pl.TenantID,
			ExternalID:    pl.ExternalID,
			DisplayName:   pl.DisplayName,
			At:            now,
			ChangedFields: changed,
			Active:        pl.Active,
		}); err != nil {
			return err
		}
	}
	for _, f := range changed {
		if f != domain.FieldActive {
			continue
		}
		topic := contracts.TopicPlayerDeactivated
		if pl.Active {
			topic = contracts.TopicPlayerActivated
		}
		return s.outbox.Publish(ctx, tx, topic, contracts.PlayerStatusV1{
			PlayerID: pl.ID,
			TenantID: pl.TenantID,
			Active:   pl.Active,
			At:       now,
		})
	}
	return nil
}

// Delete soft-deletes a player (admin permission) and records
// player.deleted.v1 in the same transaction. The external id becomes
// reusable at once.
func (s *Service) Delete(ctx context.Context, playerID string) error {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return err
	}
	if !isUUID(playerID) {
		return domain.ErrNotFound
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermDelete, nil); err != nil {
		return err
	}
	var gone domain.Player
	err = s.tx(ctx, func(tx *gorm.DB) error {
		pl, err := s.repo.ByIDForUpdate(ctx, tx, p.TenantID, playerID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		pl.MarkDeleted(now)
		if err := s.repo.Save(ctx, tx, pl); err != nil {
			return err
		}
		gone = pl
		return s.outbox.Publish(ctx, tx, contracts.TopicPlayerDeleted, contracts.PlayerDeletedV1{
			PlayerID:   pl.ID,
			TenantID:   pl.TenantID,
			ExternalID: pl.ExternalID,
			At:         now,
		})
	})
	if err != nil {
		return err
	}
	s.repo.Evict(ctx, gone.TenantID, Ref{ID: gone.ID, ExternalID: gone.ExternalID})
	return nil
}

// PurgeTenant reacts to tenant.deleted.v1: hard-deletes every player row of
// the tenant in bounded batches. Idempotent: a redelivery finds nothing and
// does nothing. No per-player events: every peer purges its own rows on the
// same tenant.deleted.v1.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) (int, error) {
	if !isUUID(tenantID) {
		return 0, errs.New(errs.Invalid, "tenant.deleted.v1 carries no valid tenant_id")
	}
	total := 0
	for {
		var removed []Ref
		err := s.tx(ctx, func(tx *gorm.DB) error {
			var err error
			removed, err = s.repo.PurgeTenantBatch(ctx, tx, tenantID, s.purgeSize)
			return err
		})
		if err != nil {
			return total, err
		}
		total += len(removed)
		if len(removed) > 0 {
			s.repo.Evict(ctx, tenantID, removed...)
		}
		if len(removed) < s.purgeSize {
			return total, nil
		}
	}
}

// PlayersByIDs implements contracts.Reader: in-process trust, tenant passed
// explicitly, unknown / foreign / deleted ids are simply absent.
func (s *Service) PlayersByIDs(ctx context.Context, tenantID string, ids []string) ([]contracts.PlayerSnapshot, error) {
	if tenantID == "" {
		return nil, errs.New(errs.Invalid, "tenant id required")
	}
	clean := dedupe(ids, isUUID)
	if len(clean) == 0 {
		return []contracts.PlayerSnapshot{}, nil
	}
	if len(clean) > MaxBatch {
		return nil, errs.New(errs.Invalid, "too many ids in one batch")
	}
	rows, err := s.repo.ByIDs(ctx, tenantID, clean)
	if err != nil {
		return nil, err
	}
	return toSnapshots(rows, tenantID), nil
}

func (s *Service) PlayersByExternalIDs(ctx context.Context, tenantID string, externalIDs []string) ([]contracts.PlayerSnapshot, error) {
	if tenantID == "" {
		return nil, errs.New(errs.Invalid, "tenant id required")
	}
	trimmed := make([]string, len(externalIDs))
	for i, e := range externalIDs {
		trimmed[i] = strings.TrimSpace(e)
	}
	clean := dedupe(trimmed, func(s string) bool { return s != "" })
	if len(clean) == 0 {
		return []contracts.PlayerSnapshot{}, nil
	}
	if len(clean) > MaxBatch {
		return nil, errs.New(errs.Invalid, "too many external ids in one batch")
	}
	rows, err := s.repo.ByExternalIDs(ctx, tenantID, clean)
	if err != nil {
		return nil, err
	}
	return toSnapshots(rows, tenantID), nil
}

// ListPlayerIDs pages the tenant's live player ids in ascending id order,
// strictly after afterID ("" starts at the beginning). A page shorter than
// limit is the last one. In-process trust, like the contracts.Reader methods.
func (s *Service) ListPlayerIDs(ctx context.Context, tenantID, afterID string, limit int) ([]string, error) {
	if tenantID == "" {
		return nil, errs.New(errs.Invalid, "tenant id required")
	}
	if afterID != "" && !isUUID(afterID) {
		return nil, errs.New(errs.Invalid, "after id must be a uuid")
	}
	if limit <= 0 || limit > MaxIDPage {
		limit = MaxIDPage
	}
	return s.repo.IDsAfter(ctx, tenantID, afterID, limit)
}

func toSnapshots(rows []domain.Player, tenantID string) []contracts.PlayerSnapshot {
	out := make([]contracts.PlayerSnapshot, 0, len(rows))
	for _, r := range rows {
		// Belt to the repository's braces: never hand out a foreign or
		// deleted row, even from a stale cache entry.
		if !r.BelongsTo(tenantID) || r.Deleted() {
			continue
		}
		out = append(out, contracts.PlayerSnapshot{
			ID:          r.ID,
			TenantID:    r.TenantID,
			ExternalID:  r.ExternalID,
			DisplayName: r.DisplayName,
			Email:       r.Email,
			Active:      r.Active,
			Attributes:  r.Attributes,
			CreatedAt:   r.CreatedAt,
		})
	}
	return out
}

func dedupe(in []string, keep func(string) bool) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !keep(v) || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil && len(s) == 36
}

func uuidOrEmpty(s string) string {
	if isUUID(s) {
		return s
	}
	return ""
}
