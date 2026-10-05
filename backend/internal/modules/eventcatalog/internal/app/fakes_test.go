package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/eventcatalog/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// fakeRepo is an in-memory Repository with the same scoping, uniqueness
// and shadowing semantics as the SQL (proved separately in repo tests).
type fakeRepo struct {
	types      map[string]domain.EventType
	deleted    map[string]time.Time
	categories map[string]domain.Category
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		types:      map[string]domain.EventType{},
		deleted:    map[string]time.Time{},
		categories: map[string]domain.Category{},
	}
}

func inScope(tenantID string, s Scope) bool {
	if s.TenantID == "" {
		return tenantID == ""
	}
	return tenantID == "" || tenantID == s.TenantID
}

func (f *fakeRepo) live(id string) bool {
	_, gone := f.deleted[id]
	return !gone
}

func (f *fakeRepo) slugTaken(et domain.EventType) bool {
	for _, o := range f.types {
		if o.ID != et.ID && f.live(o.ID) && o.TenantID == et.TenantID && o.Slug == et.Slug {
			return true
		}
	}
	return false
}

func (f *fakeRepo) CreateType(_ context.Context, _ *gorm.DB, et domain.EventType) error {
	if f.slugTaken(et) {
		return domain.ErrSlugTaken
	}
	f.types[et.ID] = et
	return nil
}

func (f *fakeRepo) SaveType(_ context.Context, _ *gorm.DB, et domain.EventType) error {
	if _, ok := f.types[et.ID]; !ok || !f.live(et.ID) {
		return domain.ErrNotFound
	}
	f.types[et.ID] = et
	return nil
}

func (f *fakeRepo) SoftDeleteType(_ context.Context, _ *gorm.DB, id string, at time.Time) error {
	if _, ok := f.types[id]; !ok || !f.live(id) {
		return domain.ErrNotFound
	}
	f.deleted[id] = at
	return nil
}

func (f *fakeRepo) TypeByID(_ context.Context, s Scope, id string) (domain.EventType, error) {
	et, ok := f.types[id]
	if !ok || !f.live(id) || !inScope(et.TenantID, s) {
		return domain.EventType{}, domain.ErrNotFound
	}
	return et, nil
}

func (f *fakeRepo) TypeByIDForUpdate(ctx context.Context, _ *gorm.DB, s Scope, id string) (domain.EventType, error) {
	return f.TypeByID(ctx, s, id)
}

func (f *fakeRepo) ListTypes(_ context.Context, fl TypeFilter) ([]domain.EventType, error) {
	var out []domain.EventType
	for _, et := range f.types {
		if !f.live(et.ID) {
			continue
		}
		switch {
		case fl.Scope.TenantID == "":
			if et.TenantID != "" {
				continue
			}
		case fl.IncludeGlobal:
			if et.TenantID != fl.Scope.TenantID && et.TenantID != "" {
				continue
			}
			if et.TenantID == "" && f.shadowed(fl.Scope.TenantID, et.Slug) {
				continue
			}
		default:
			if et.TenantID != fl.Scope.TenantID {
				continue
			}
		}
		if fl.CategoryID != "" && et.CategoryID != fl.CategoryID {
			continue
		}
		if fl.Active != nil && et.Active != *fl.Active {
			continue
		}
		if fl.Search != "" && !strings.Contains(strings.ToLower(et.Name+" "+et.Slug+" "+et.Description), strings.ToLower(fl.Search)) {
			continue
		}
		if !fl.AfterTime.IsZero() && !before(et, fl.AfterTime, fl.AfterID) {
			continue
		}
		out = append(out, et)
	}
	slices.SortFunc(out, func(a, b domain.EventType) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(b.ID, a.ID)
	})
	if len(out) > fl.Limit {
		out = out[:fl.Limit]
	}
	return out, nil
}

func before(et domain.EventType, at time.Time, id string) bool {
	if et.CreatedAt.Before(at) {
		return true
	}
	return et.CreatedAt.Equal(at) && et.ID < id
}

func (f *fakeRepo) shadowed(tenantID, slug string) bool {
	for _, o := range f.types {
		if f.live(o.ID) && o.TenantID == tenantID && o.Slug == slug {
			return true
		}
	}
	return false
}

func (f *fakeRepo) TypesBySlugs(_ context.Context, s Scope, slugs []string) ([]domain.EventType, error) {
	var out []domain.EventType
	for _, et := range f.types {
		if f.live(et.ID) && inScope(et.TenantID, s) && slices.Contains(slugs, et.Slug) {
			out = append(out, et)
		}
	}
	return out, nil
}

func (f *fakeRepo) CreateCategory(_ context.Context, _ *gorm.DB, c domain.Category) error {
	for _, o := range f.categories {
		if o.TenantID == c.TenantID && o.Slug == c.Slug {
			return domain.ErrCategorySlugTaken
		}
	}
	f.categories[c.ID] = c
	return nil
}

func (f *fakeRepo) SaveCategory(_ context.Context, _ *gorm.DB, c domain.Category) error {
	if _, ok := f.categories[c.ID]; !ok {
		return domain.ErrCategoryNotFound
	}
	for _, o := range f.categories {
		if o.ID != c.ID && o.TenantID == c.TenantID && o.Slug == c.Slug {
			return domain.ErrCategorySlugTaken
		}
	}
	f.categories[c.ID] = c
	return nil
}

func (f *fakeRepo) DeleteCategory(_ context.Context, _ *gorm.DB, id string) error {
	if _, ok := f.categories[id]; !ok {
		return domain.ErrCategoryNotFound
	}
	delete(f.categories, id)
	for tid, et := range f.types {
		if et.CategoryID == id {
			et.CategoryID = ""
			f.types[tid] = et
		}
	}
	return nil
}

func (f *fakeRepo) CategoryByID(_ context.Context, s Scope, id string) (domain.Category, error) {
	c, ok := f.categories[id]
	if !ok || !inScope(c.TenantID, s) {
		return domain.Category{}, domain.ErrCategoryNotFound
	}
	return c, nil
}

func (f *fakeRepo) CategoryBySlug(_ context.Context, s Scope, slug string) (domain.Category, error) {
	var found *domain.Category
	for _, c := range f.categories {
		if c.Slug != slug || !inScope(c.TenantID, s) {
			continue
		}
		if found == nil || (found.TenantID == "" && c.TenantID != "") {
			found = &c
		}
	}
	if found == nil {
		return domain.Category{}, domain.ErrCategoryNotFound
	}
	return *found, nil
}

func (f *fakeRepo) ListCategories(_ context.Context, s Scope, limit int) ([]domain.Category, error) {
	var out []domain.Category
	for _, c := range f.categories {
		if !inScope(c.TenantID, s) {
			continue
		}
		if c.TenantID == "" && s.TenantID != "" {
			shadowed := false
			for _, o := range f.categories {
				if o.TenantID == s.TenantID && o.Slug == c.Slug {
					shadowed = true
				}
			}
			if shadowed {
				continue
			}
		}
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b domain.Category) int {
		if a.SortOrder != b.SortOrder {
			return a.SortOrder - b.SortOrder
		}
		return strings.Compare(a.Name, b.Name)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	for id, et := range f.types {
		if et.TenantID == tenantID {
			delete(f.types, id)
			delete(f.deleted, id)
		}
	}
	for id, c := range f.categories {
		if c.TenantID == tenantID {
			delete(f.categories, id)
		}
	}
	return nil
}

// countTenantRows counts every row (live or soft-deleted) a tenant owns.
func (f *fakeRepo) countTenantRows(tenantID string) int {
	n := 0
	for _, et := range f.types {
		if et.TenantID == tenantID {
			n++
		}
	}
	for _, c := range f.categories {
		if c.TenantID == tenantID {
			n++
		}
	}
	return n
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

// allowKeys grants only the listed permission keys, per role id, the way
// the seeded casbin grants do: a principal is allowed if any of its roles
// holds the key.
type allowKeys map[int64][]string

func (a allowKeys) Authorize(_ context.Context, p authz.Principal, perm authz.Permission, _ any) error {
	for _, role := range p.RoleIDs {
		if slices.Contains(a[role], perm.Key()) {
			return nil
		}
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}
