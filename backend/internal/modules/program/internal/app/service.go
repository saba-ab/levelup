// Package app holds program's use cases: transaction boundaries, tenant
// scoping, authorization and every outbox publish (ADR-0013, ADR-0015).
package app

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"levelup/internal/modules/program/contracts"
	"levelup/internal/modules/program/internal/domain"
	"levelup/internal/modules/program/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
)

const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

// Page is a keyset position: rows strictly older than (At, ID).
type Page struct {
	At    time.Time
	ID    string
	Limit int
}

// Repository is consumed by this service and implemented in internal/repo.
// Every read and write is scoped by an explicit tenantID (ADR-0015).
type Repository interface {
	Create(ctx context.Context, tx *gorm.DB, p domain.Program) error
	ByID(ctx context.Context, tenantID, id string) (domain.Program, error)
	ByIDs(ctx context.Context, tenantID string, ids []string) ([]domain.Program, error)
	// ByIDForUpdate takes a row lock: transitions and edits serialize here.
	ByIDForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Program, error)
	// ByIDForShare blocks concurrent transitions while an enrolment commits.
	ByIDForShare(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Program, error)
	// Save writes every mutable column guarded by p.Version and bumps it.
	Save(ctx context.Context, tx *gorm.DB, p domain.Program) error
	List(ctx context.Context, tenantID string, f ListFilter, page Page) ([]domain.Program, error)
	// MemberCounts counts enrolments per program in ONE grouped query;
	// programs without members are absent from the map.
	MemberCounts(ctx context.Context, tenantID string, programIDs []string) (map[string]int64, error)
	// DueForAutoEnd returns running programs (any tenant) whose ends_at <= now.
	DueForAutoEnd(ctx context.Context, now time.Time, limit int) ([]domain.Program, error)

	// Enroll inserts unless (program_id, player_id) exists; reports whether
	// a row was inserted.
	Enroll(ctx context.Context, tx *gorm.DB, e domain.Enrollment) (bool, error)
	Enrollment(ctx context.Context, tenantID, programID, playerID string) (domain.Enrollment, bool, error)
	Unenroll(ctx context.Context, tx *gorm.DB, tenantID, programID, playerID string) (bool, error)
	ListEnrollments(ctx context.Context, tenantID, programID string, page Page) ([]domain.Enrollment, error)
	// RemovePlayer deletes a player's enrolments and returns the deleted rows.
	RemovePlayer(ctx context.Context, tx *gorm.DB, tenantID, playerID string) ([]domain.Enrollment, error)
	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
	// ActiveProgramIDsForPlayer lists active, non-deleted programs the
	// player is enrolled in.
	ActiveProgramIDsForPlayer(ctx context.Context, tenantID, playerID string) ([]string, error)
}

type Service struct {
	repo    Repository
	players ports.PlayerReader
	outbox  outbox.Store
	authz   authz.Enforcer
	db      *gorm.DB
	clock   clock.Clock

	autoEndBatch int

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, players ports.PlayerReader, ob outbox.Store, enf authz.Enforcer,
	db *gorm.DB, c clock.Clock, autoEndBatch int) *Service {
	if autoEndBatch <= 0 {
		autoEndBatch = 500
	}
	s := &Service{repo: repo, players: players, outbox: ob, authz: enf, db: db, clock: c, autoEndBatch: autoEndBatch}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

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

// ---- programs --------------------------------------------------------------

type CreateCmd struct {
	Name        string
	Slug        string
	Description *string
	StartsAt    *time.Time
	EndsAt      *time.Time
	Settings    map[string]any
	Mechanics   map[string]any
}

func (s *Service) Create(ctx context.Context, cmd CreateCmd) (domain.Program, error) {
	p, err := s.authorize(ctx, contracts.PermCreate)
	if err != nil {
		return domain.Program{}, err
	}
	now := s.clock.Now()
	prog, err := domain.NewProgram(domain.NewProgramInput{
		TenantID:    p.TenantID,
		Name:        cmd.Name,
		Slug:        cmd.Slug,
		Description: cmd.Description,
		StartsAt:    cmd.StartsAt,
		EndsAt:      cmd.EndsAt,
		Settings:    cmd.Settings,
		Mechanics:   cmd.Mechanics,
	}, now)
	if err != nil {
		return domain.Program{}, err
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.Create(ctx, tx, prog); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicProgramCreated, changed(prog, now))
	})
	if err != nil {
		return domain.Program{}, err
	}
	return prog, nil
}

func (s *Service) Get(ctx context.Context, id string) (domain.Program, error) {
	p, err := s.authorize(ctx, contracts.PermView)
	if err != nil {
		return domain.Program{}, err
	}
	return s.repo.ByID(ctx, p.TenantID, id)
}

// ListFilter narrows List. Status "" means all; Search "" means no search,
// otherwise a case-insensitive substring of name or slug (LIKE wildcards in
// it match literally).
type ListFilter struct {
	Status domain.Status
	Search string
}

// MaxSearchLength caps the search query parameter.
const MaxSearchLength = 100

// List pages a tenant's programs newest-first; status "" means all, search
// "" means no search.
func (s *Service) List(ctx context.Context, status, search, cursor string, limit int) ([]domain.Program, string, error) {
	p, err := s.authorize(ctx, contracts.PermViewAny)
	if err != nil {
		return nil, "", err
	}
	var st domain.Status
	if status != "" {
		if st, err = domain.ParseStatus(status); err != nil {
			return nil, "", err
		}
	}
	search = strings.TrimSpace(search)
	if utf8.RuneCountInString(search) > MaxSearchLength {
		return nil, "", errs.WithFields(errs.WithCode(errs.New(errs.Invalid, "search is too long"), "invalid_search"),
			map[string]string{"search": "at most 100 characters"})
	}
	page, err := pageFrom(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.repo.List(ctx, p.TenantID, ListFilter{Status: st, Search: search},
		Page{At: page.At, ID: page.ID, Limit: page.Limit + 1})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > page.Limit {
		rows = rows[:page.Limit]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

// MemberCounts decorates programs the caller already loaded through an
// authorized read with their enrolment counts: one grouped query for the
// whole page, never one per program. Every id gets an entry (0 when empty);
// ids of another tenant count 0.
func (s *Service) MemberCounts(ctx context.Context, programIDs []string) (map[string]int64, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(programIDs))
	if len(programIDs) == 0 {
		return out, nil
	}
	got, err := s.repo.MemberCounts(ctx, p.TenantID, programIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range programIDs {
		out[id] = got[id]
	}
	return out, nil
}

// Update applies a partial patch. Status is not part of the patch: it moves
// only through Activate/Pause/End (fixes the Laravel PUT bypass).
func (s *Service) Update(ctx context.Context, id string, patch domain.Patch) (domain.Program, error) {
	p, err := s.authorize(ctx, contracts.PermUpdate)
	if err != nil {
		return domain.Program{}, err
	}
	var out domain.Program
	err = s.tx(ctx, func(tx *gorm.DB) error {
		prog, err := s.repo.ByIDForUpdate(ctx, tx, p.TenantID, id)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		fields, err := prog.Edit(patch, now)
		if err != nil {
			return err
		}
		if len(fields) == 0 {
			out = prog
			return nil
		}
		if err := s.repo.Save(ctx, tx, prog); err != nil {
			return err
		}
		prog.Version++
		out = prog
		ev := changed(prog, now)
		ev.ChangedFields = fields
		return s.outbox.Publish(ctx, tx, contracts.TopicProgramUpdated, ev)
	})
	if err != nil {
		return domain.Program{}, err
	}
	return out, nil
}

// Delete soft-deletes the program; its slug becomes reusable.
func (s *Service) Delete(ctx context.Context, id string) error {
	p, err := s.authorize(ctx, contracts.PermDelete)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		prog, err := s.repo.ByIDForUpdate(ctx, tx, p.TenantID, id)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		prog.DeletedAt = &now
		prog.UpdatedAt = now
		if err := s.repo.Save(ctx, tx, prog); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicProgramDeleted, changed(prog, now))
	})
}

func (s *Service) Activate(ctx context.Context, id string) (domain.Program, error) {
	return s.transition(ctx, id, contracts.PermActivate, contracts.TopicProgramActivated, (*domain.Program).Activate)
}

func (s *Service) Pause(ctx context.Context, id string) (domain.Program, error) {
	return s.transition(ctx, id, contracts.PermPause, contracts.TopicProgramPaused, (*domain.Program).Pause)
}

func (s *Service) End(ctx context.Context, id string) (domain.Program, error) {
	return s.transition(ctx, id, contracts.PermEnd, contracts.TopicProgramEnded, (*domain.Program).End)
}

func (s *Service) transition(ctx context.Context, id string, perm authz.Permission, topic string,
	move func(*domain.Program, time.Time) error) (domain.Program, error) {
	p, err := s.authorize(ctx, perm)
	if err != nil {
		return domain.Program{}, err
	}
	var out domain.Program
	err = s.tx(ctx, func(tx *gorm.DB) error {
		prog, err := s.repo.ByIDForUpdate(ctx, tx, p.TenantID, id)
		if err != nil {
			return err
		}
		from := prog.Status
		now := s.clock.Now()
		if err := move(&prog, now); err != nil {
			return err
		}
		if err := s.repo.Save(ctx, tx, prog); err != nil {
			return err
		}
		prog.Version++
		out = prog
		ev := changed(prog, now)
		ev.PreviousStatus = string(from)
		return s.outbox.Publish(ctx, tx, topic, ev)
	})
	if err != nil {
		return domain.Program{}, err
	}
	return out, nil
}

// ---- enrolment -------------------------------------------------------------

// Enroll adds a player to an active program inside its window. Enrolling an
// already-enrolled player returns the existing enrolment with created=false
// and publishes nothing.
func (s *Service) Enroll(ctx context.Context, programID, playerID string) (domain.Enrollment, bool, error) {
	p, err := s.authorize(ctx, contracts.PermEnroll)
	if err != nil {
		return domain.Enrollment{}, false, err
	}
	prog, err := s.repo.ByID(ctx, p.TenantID, programID)
	if err != nil {
		return domain.Enrollment{}, false, err
	}
	if existing, ok, err := s.repo.Enrollment(ctx, p.TenantID, prog.ID, playerID); err != nil {
		return domain.Enrollment{}, false, err
	} else if ok {
		return existing, false, nil
	}

	now := s.clock.Now()
	if !prog.AcceptsEnrollment(now) {
		return domain.Enrollment{}, false, domain.ErrNotAccepting
	}
	// The cross-module read happens OUTSIDE the transaction.
	if err := s.checkPlayer(ctx, p.TenantID, playerID); err != nil {
		return domain.Enrollment{}, false, err
	}

	e := domain.Enrollment{ProgramID: prog.ID, TenantID: p.TenantID, PlayerID: playerID, EnrolledAt: now}
	var created bool
	err = s.tx(ctx, func(tx *gorm.DB) error {
		locked, err := s.repo.ByIDForShare(ctx, tx, p.TenantID, prog.ID)
		if err != nil {
			return err
		}
		if !locked.AcceptsEnrollment(now) {
			return domain.ErrNotAccepting
		}
		if created, err = s.repo.Enroll(ctx, tx, e); err != nil || !created {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicPlayerEnrolled, contracts.EnrollmentChangedV1{
			ProgramID: e.ProgramID, TenantID: e.TenantID, PlayerID: e.PlayerID, At: now,
		})
	})
	if err != nil {
		return domain.Enrollment{}, false, err
	}
	if created {
		return e, true, nil
	}
	// Lost a race with a concurrent enrolment: return the winner's row.
	existing, ok, err := s.repo.Enrollment(ctx, p.TenantID, prog.ID, playerID)
	if err != nil {
		return domain.Enrollment{}, false, err
	}
	if !ok {
		return domain.Enrollment{}, false, errs.New(errs.Conflict, "enrolment changed concurrently, retry")
	}
	return existing, false, nil
}

func (s *Service) checkPlayer(ctx context.Context, tenantID, playerID string) error {
	pl, err := s.players.ByID(ctx, tenantID, playerID)
	if err != nil {
		if errs.KindOf(err) == errs.NotFound {
			return domain.ErrPlayerNotFound
		}
		return err
	}
	if pl.TenantID != "" && pl.TenantID != tenantID {
		return domain.ErrPlayerNotFound
	}
	if !pl.Active {
		return domain.ErrPlayerInactive
	}
	return nil
}

// Unenroll removes a player from a program. Idempotent: removing a player
// who is not enrolled succeeds and publishes nothing.
func (s *Service) Unenroll(ctx context.Context, programID, playerID string) error {
	p, err := s.authorize(ctx, contracts.PermEnroll)
	if err != nil {
		return err
	}
	prog, err := s.repo.ByID(ctx, p.TenantID, programID)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		removed, err := s.repo.Unenroll(ctx, tx, p.TenantID, prog.ID, playerID)
		if err != nil || !removed {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicPlayerUnenrolled, contracts.EnrollmentChangedV1{
			ProgramID: prog.ID, TenantID: p.TenantID, PlayerID: playerID, At: s.clock.Now(),
		})
	})
}

// Member is an enrolment hydrated with program's snapshot of the player.
// Player is nil when the player module no longer knows the id.
type Member struct {
	Enrollment domain.Enrollment
	Player     *ports.PlayerSnapshot
}

// ListMembers pages a program's enrolments newest-first by
// (enrolled_at, player_id) and hydrates them with ONE batch player read.
func (s *Service) ListMembers(ctx context.Context, programID, cursor string, limit int) ([]Member, string, error) {
	p, err := s.authorize(ctx, contracts.PermView)
	if err != nil {
		return nil, "", err
	}
	prog, err := s.repo.ByID(ctx, p.TenantID, programID)
	if err != nil {
		return nil, "", err
	}
	page, err := pageFrom(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.repo.ListEnrollments(ctx, p.TenantID, prog.ID, Page{At: page.At, ID: page.ID, Limit: page.Limit + 1})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > page.Limit {
		rows = rows[:page.Limit]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.EnrolledAt, last.PlayerID)
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.PlayerID
	}
	snaps, err := s.players.ByIDs(ctx, p.TenantID, ids)
	if err != nil {
		return nil, "", err
	}
	out := make([]Member, len(rows))
	for i, r := range rows {
		out[i] = Member{Enrollment: r}
		if snap, ok := snaps[r.PlayerID]; ok {
			out[i].Player = &snap
		}
	}
	return out, next, nil
}

// ---- async handlers --------------------------------------------------------

// PurgeTenant handles tenant.deleted.v1: hard-deletes every program and
// enrolment of the tenant. Redelivery deletes nothing more.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return errs.New(errs.Invalid, "tenant.deleted.v1 without tenant_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, tenantID)
	})
}

// RemovePlayer handles player.deleted.v1: drops the player's enrolments and
// publishes one program.player_unenrolled.v1 per removed row in the same
// transaction. A redelivery finds no rows and publishes nothing.
func (s *Service) RemovePlayer(ctx context.Context, tenantID, playerID string) error {
	if tenantID == "" || playerID == "" {
		return errs.New(errs.Invalid, "player.deleted.v1 without tenant_id or player_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		removed, err := s.repo.RemovePlayer(ctx, tx, tenantID, playerID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, e := range removed {
			if err := s.outbox.Publish(ctx, tx, contracts.TopicPlayerUnenrolled, contracts.EnrollmentChangedV1{
				ProgramID: e.ProgramID, TenantID: e.TenantID, PlayerID: e.PlayerID, At: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// AutoEnd is the reconciling sweep behind program.auto_end: every running
// program whose ends_at has passed is ended, whatever ticks were missed.
// Each program commits in its own transaction so one failure cannot block
// the rest; the predicate re-checked under the row lock keeps it idempotent.
func (s *Service) AutoEnd(ctx context.Context) (int, error) {
	now := s.clock.Now()
	due, err := s.repo.DueForAutoEnd(ctx, now, s.autoEndBatch)
	if err != nil {
		return 0, err
	}
	ended := 0
	var failures []error
	for _, d := range due {
		var did bool
		err := s.tx(ctx, func(tx *gorm.DB) error {
			prog, err := s.repo.ByIDForUpdate(ctx, tx, d.TenantID, d.ID)
			if err != nil {
				if errs.KindOf(err) == errs.NotFound {
					return nil // deleted meanwhile
				}
				return err
			}
			if !prog.DueForAutoEnd(now) {
				return nil // already ended or its window moved
			}
			from := prog.Status
			if err := prog.End(now); err != nil {
				return err
			}
			if err := s.repo.Save(ctx, tx, prog); err != nil {
				return err
			}
			did = true
			ev := changed(prog, now)
			ev.PreviousStatus = string(from)
			ev.AutoEnded = true
			return s.outbox.Publish(ctx, tx, contracts.TopicProgramEnded, ev)
		})
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if did {
			ended++
		}
	}
	if len(failures) > 0 {
		return ended, errs.Wrap(errs.Unavailable, "program auto-end sweep incomplete", errors.Join(failures...))
	}
	return ended, nil
}

// ---- contracts.Reader -------------------------------------------------------

var _ contracts.Reader = (*Service)(nil)

func (s *Service) ProgramsByIDs(ctx context.Context, tenantID string, ids []string) ([]contracts.ProgramSnapshot, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.repo.ByIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.ProgramSnapshot, len(rows))
	for i, p := range rows {
		out[i] = contracts.ProgramSnapshot{
			ID: p.ID, TenantID: p.TenantID, Name: p.Name, Status: string(p.Status),
			StartsAt: p.StartsAt, EndsAt: p.EndsAt,
		}
	}
	return out, nil
}

func (s *Service) EnrolledProgramIDs(ctx context.Context, tenantID, playerID string) ([]string, error) {
	return s.repo.ActiveProgramIDsForPlayer(ctx, tenantID, playerID)
}

// ---- helpers ----------------------------------------------------------------

func changed(p domain.Program, at time.Time) contracts.ProgramChangedV1 {
	return contracts.ProgramChangedV1{
		ProgramID: p.ID,
		TenantID:  p.TenantID,
		Name:      p.Name,
		Slug:      p.Slug,
		Status:    string(p.Status),
		StartsAt:  p.StartsAt,
		EndsAt:    p.EndsAt,
		At:        at,
	}
}

func pageFrom(cursor string, limit int) (Page, error) {
	switch {
	case limit <= 0:
		limit = DefaultPageSize
	case limit > MaxPageSize:
		limit = MaxPageSize
	}
	pg := Page{Limit: limit}
	if cursor != "" {
		at, id, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
		pg.At, pg.ID = at, id
	}
	return pg, nil
}
