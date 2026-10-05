package app

import (
	"cmp"
	"context"
	"slices"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/program/internal/domain"
	"levelup/internal/modules/program/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// fakeRepo is an in-memory Repository with the same tenant scoping,
// soft-delete, version and ON CONFLICT semantics as the Postgres one.
type fakeRepo struct {
	programs    map[string]domain.Program
	enrollments map[[2]string]domain.Enrollment // (program_id, player_id)
	saveErr     error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{programs: map[string]domain.Program{}, enrollments: map[[2]string]domain.Enrollment{}}
}

func (r *fakeRepo) Create(_ context.Context, _ *gorm.DB, p domain.Program) error {
	for _, o := range r.programs {
		if o.TenantID == p.TenantID && o.Slug == p.Slug && o.DeletedAt == nil {
			return domain.ErrSlugTaken
		}
	}
	r.programs[p.ID] = p
	return nil
}

func (r *fakeRepo) ByID(_ context.Context, tenantID, id string) (domain.Program, error) {
	p, ok := r.programs[id]
	if !ok || p.TenantID != tenantID || p.DeletedAt != nil {
		return domain.Program{}, domain.ErrNotFound
	}
	return p, nil
}

func (r *fakeRepo) ByIDs(ctx context.Context, tenantID string, ids []string) ([]domain.Program, error) {
	var out []domain.Program
	for _, id := range ids {
		if p, err := r.ByID(ctx, tenantID, id); err == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *fakeRepo) ByIDForUpdate(ctx context.Context, _ *gorm.DB, tenantID, id string) (domain.Program, error) {
	return r.ByID(ctx, tenantID, id)
}

func (r *fakeRepo) ByIDForShare(ctx context.Context, _ *gorm.DB, tenantID, id string) (domain.Program, error) {
	return r.ByID(ctx, tenantID, id)
}

func (r *fakeRepo) Save(_ context.Context, _ *gorm.DB, p domain.Program) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	cur, ok := r.programs[p.ID]
	if !ok || cur.TenantID != p.TenantID || cur.Version != p.Version {
		return domain.ErrVersionConflict
	}
	for _, o := range r.programs {
		if o.ID != p.ID && o.TenantID == p.TenantID && o.Slug == p.Slug && o.DeletedAt == nil && p.DeletedAt == nil {
			return domain.ErrSlugTaken
		}
	}
	p.Version++
	r.programs[p.ID] = p
	return nil
}

func (r *fakeRepo) List(_ context.Context, tenantID string, status domain.Status, page Page) ([]domain.Program, error) {
	var out []domain.Program
	for _, p := range r.programs {
		if p.TenantID != tenantID || p.DeletedAt != nil || (status != "" && p.Status != status) {
			continue
		}
		if page.ID != "" && !before(p.CreatedAt, p.ID, page.At, page.ID) {
			continue
		}
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b domain.Program) int {
		return -cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	if len(out) > page.Limit {
		out = out[:page.Limit]
	}
	return out, nil
}

func (r *fakeRepo) DueForAutoEnd(_ context.Context, now time.Time, limit int) ([]domain.Program, error) {
	var out []domain.Program
	for _, p := range r.programs {
		if p.DeletedAt == nil && p.DueForAutoEnd(now) {
			out = append(out, p)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *fakeRepo) Enroll(_ context.Context, _ *gorm.DB, e domain.Enrollment) (bool, error) {
	k := [2]string{e.ProgramID, e.PlayerID}
	if _, ok := r.enrollments[k]; ok {
		return false, nil
	}
	r.enrollments[k] = e
	return true, nil
}

func (r *fakeRepo) Enrollment(_ context.Context, tenantID, programID, playerID string) (domain.Enrollment, bool, error) {
	e, ok := r.enrollments[[2]string{programID, playerID}]
	if !ok || e.TenantID != tenantID {
		return domain.Enrollment{}, false, nil
	}
	return e, true, nil
}

func (r *fakeRepo) Unenroll(_ context.Context, _ *gorm.DB, tenantID, programID, playerID string) (bool, error) {
	k := [2]string{programID, playerID}
	e, ok := r.enrollments[k]
	if !ok || e.TenantID != tenantID {
		return false, nil
	}
	delete(r.enrollments, k)
	return true, nil
}

func (r *fakeRepo) ListEnrollments(_ context.Context, tenantID, programID string, page Page) ([]domain.Enrollment, error) {
	var out []domain.Enrollment
	for _, e := range r.enrollments {
		if e.TenantID != tenantID || e.ProgramID != programID {
			continue
		}
		if page.ID != "" && !before(e.EnrolledAt, e.PlayerID, page.At, page.ID) {
			continue
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b domain.Enrollment) int {
		return -cmp.Or(a.EnrolledAt.Compare(b.EnrolledAt), cmp.Compare(a.PlayerID, b.PlayerID))
	})
	if len(out) > page.Limit {
		out = out[:page.Limit]
	}
	return out, nil
}

func (r *fakeRepo) RemovePlayer(_ context.Context, _ *gorm.DB, tenantID, playerID string) ([]domain.Enrollment, error) {
	var out []domain.Enrollment
	for k, e := range r.enrollments {
		if e.TenantID == tenantID && e.PlayerID == playerID {
			out = append(out, e)
			delete(r.enrollments, k)
		}
	}
	return out, nil
}

func (r *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	for k, e := range r.enrollments {
		if e.TenantID == tenantID {
			delete(r.enrollments, k)
		}
	}
	for id, p := range r.programs {
		if p.TenantID == tenantID {
			delete(r.programs, id)
		}
	}
	return nil
}

func (r *fakeRepo) ActiveProgramIDsForPlayer(_ context.Context, tenantID, playerID string) ([]string, error) {
	var out []string
	for _, e := range r.enrollments {
		if e.TenantID != tenantID || e.PlayerID != playerID {
			continue
		}
		if p, ok := r.programs[e.ProgramID]; ok && p.Status == domain.StatusActive && p.DeletedAt == nil {
			out = append(out, p.ID)
		}
	}
	slices.Sort(out)
	return out, nil
}

func before(at time.Time, id string, cursorAt time.Time, cursorID string) bool {
	if c := at.Compare(cursorAt); c != 0 {
		return c < 0
	}
	return id < cursorID
}

type recordedEvent struct {
	topic   string
	payload any
}

type fakeOutbox struct{ published []recordedEvent }

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.published = append(f.published, recordedEvent{topic, payload})
	return nil
}

func (f *fakeOutbox) topics() []string {
	out := make([]string, len(f.published))
	for i, e := range f.published {
		out[i] = e.topic
	}
	return out
}

// allowKeys enforces only the listed permission keys.
type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

type fakePlayers struct {
	players map[string]ports.PlayerSnapshot
	err     error
	calls   int
}

func (f *fakePlayers) ByID(_ context.Context, tenantID, id string) (ports.PlayerSnapshot, error) {
	f.calls++
	if f.err != nil {
		return ports.PlayerSnapshot{}, f.err
	}
	p, ok := f.players[id]
	if !ok || p.TenantID != tenantID {
		return ports.PlayerSnapshot{}, errs.New(errs.NotFound, "player not found")
	}
	return p, nil
}

func (f *fakePlayers) ByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]ports.PlayerSnapshot{}
	for _, id := range ids {
		if p, ok := f.players[id]; ok && p.TenantID == tenantID {
			out[id] = p
		}
	}
	return out, nil
}
