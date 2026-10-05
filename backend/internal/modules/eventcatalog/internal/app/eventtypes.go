package app

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"levelup/internal/modules/eventcatalog/contracts"
	"levelup/internal/modules/eventcatalog/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
)

// ListTypesQuery is the list filter as the transport parsed it.
type ListTypesQuery struct {
	// Category is a category id or slug; "" = any.
	Category      string
	Active        *bool
	IncludeGlobal bool
	Search        string
	Cursor        string
	Limit         int
}

type CreateTypeCmd struct {
	Name           string
	Slug           string
	Description    string
	CategoryID     string
	PropertySchema map[string]any
	Active         *bool // nil = active
}

// --- tenant surface (/events) -------------------------------------------

// ListTypes lists the caller's tenant event types and, by default, the
// global catalogue (globals shadowed by an own slug hidden).
func (s *Service) ListTypes(ctx context.Context, q ListTypesQuery) (Page[domain.EventType], error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return Page[domain.EventType]{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, nil); err != nil {
		return Page[domain.EventType]{}, err
	}
	return s.listTypes(ctx, Scope{TenantID: p.TenantID}, q)
}

// GetType returns an own or a global event type (Laravel answered 403 for
// globals); another tenant's is NotFound.
func (s *Service) GetType(ctx context.Context, id string) (domain.EventType, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.EventType{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, nil); err != nil {
		return domain.EventType{}, err
	}
	return s.repo.TypeByID(ctx, Scope{TenantID: p.TenantID}, id)
}

// CreateType creates a tenant-owned event type. There is no way to create a
// global row here: the tenant is stamped from the principal (fixes the
// Laravel "is_predefined → global" leak).
func (s *Service) CreateType(ctx context.Context, cmd CreateTypeCmd) (domain.EventType, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.EventType{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermCreate, nil); err != nil {
		return domain.EventType{}, err
	}
	return s.createType(ctx, p.TenantID, cmd)
}

func (s *Service) UpdateType(ctx context.Context, id string, patch domain.EventTypePatch) (domain.EventType, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.EventType{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermUpdate, nil); err != nil {
		return domain.EventType{}, err
	}
	return s.updateType(ctx, Scope{TenantID: p.TenantID}, id, patch)
}

func (s *Service) DeleteType(ctx context.Context, id string) error {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermDelete, nil); err != nil {
		return err
	}
	return s.deleteType(ctx, Scope{TenantID: p.TenantID}, id)
}

// --- platform surface (/platform/event-types) ---------------------------

// requirePlatform admits only a tenant-less principal holding
// eventcatalog:manage_global (seeded to RolePlatformAdmin alone).
func (s *Service) requirePlatform(ctx context.Context) error {
	p, ok := authz.From(ctx)
	if !ok {
		return errs.New(errs.Unauthenticated, "authentication required")
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermManageGlobal, nil); err != nil {
		return err
	}
	if p.TenantID != "" {
		return domain.ErrPlatformOnly
	}
	return nil
}

func (s *Service) PlatformListTypes(ctx context.Context, q ListTypesQuery) (Page[domain.EventType], error) {
	if err := s.requirePlatform(ctx); err != nil {
		return Page[domain.EventType]{}, err
	}
	return s.listTypes(ctx, Scope{}, q)
}

func (s *Service) PlatformGetType(ctx context.Context, id string) (domain.EventType, error) {
	if err := s.requirePlatform(ctx); err != nil {
		return domain.EventType{}, err
	}
	return s.repo.TypeByID(ctx, Scope{}, id)
}

func (s *Service) PlatformCreateType(ctx context.Context, cmd CreateTypeCmd) (domain.EventType, error) {
	if err := s.requirePlatform(ctx); err != nil {
		return domain.EventType{}, err
	}
	return s.createType(ctx, "", cmd)
}

func (s *Service) PlatformUpdateType(ctx context.Context, id string, patch domain.EventTypePatch) (domain.EventType, error) {
	if err := s.requirePlatform(ctx); err != nil {
		return domain.EventType{}, err
	}
	return s.updateType(ctx, Scope{}, id, patch)
}

func (s *Service) PlatformDeleteType(ctx context.Context, id string) error {
	if err := s.requirePlatform(ctx); err != nil {
		return err
	}
	return s.deleteType(ctx, Scope{}, id)
}

// --- shared implementation ----------------------------------------------

func (s *Service) listTypes(ctx context.Context, scope Scope, q ListTypesQuery) (Page[domain.EventType], error) {
	f := TypeFilter{
		Scope:         scope,
		IncludeGlobal: q.IncludeGlobal,
		Active:        q.Active,
		Search:        strings.TrimSpace(q.Search),
		Limit:         clampLimit(q.Limit),
	}
	if q.Cursor != "" {
		at, afterID, err := pagination.DecodeCursor(q.Cursor)
		if err != nil {
			return Page[domain.EventType]{}, err
		}
		f.AfterTime, f.AfterID = at, afterID
	}
	if q.Category != "" {
		categoryID, found, err := s.resolveCategoryFilter(ctx, scope, q.Category)
		if err != nil {
			return Page[domain.EventType]{}, err
		}
		if !found {
			return Page[domain.EventType]{Items: []domain.EventType{}}, nil
		}
		f.CategoryID = categoryID
	}

	limit := f.Limit
	f.Limit = limit + 1 // one extra row tells whether another page exists
	rows, err := s.repo.ListTypes(ctx, f)
	if err != nil {
		return Page[domain.EventType]{}, err
	}
	page := Page[domain.EventType]{Items: rows}
	if len(rows) > limit {
		page.Items = rows[:limit]
		last := page.Items[limit-1]
		page.NextCursor = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

// resolveCategoryFilter accepts a category id or slug. An unknown slug
// yields an empty result rather than an error, like any other filter.
func (s *Service) resolveCategoryFilter(ctx context.Context, scope Scope, category string) (string, bool, error) {
	if _, err := uuid.Parse(category); err == nil {
		return category, true, nil
	}
	c, err := s.repo.CategoryBySlug(ctx, scope, category)
	if errors.Is(err, domain.ErrCategoryNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return c.ID, true, nil
}

func (s *Service) createType(ctx context.Context, tenantID string, cmd CreateTypeCmd) (domain.EventType, error) {
	active := true
	if cmd.Active != nil {
		active = *cmd.Active
	}
	et, err := domain.CreateEventType(domain.NewEventType{
		TenantID:       tenantID,
		CategoryID:     cmd.CategoryID,
		Slug:           cmd.Slug,
		Name:           cmd.Name,
		Description:    cmd.Description,
		PropertySchema: cmd.PropertySchema,
		Active:         active,
	}, s.clock.Now())
	if err != nil {
		return domain.EventType{}, err
	}
	if et.CategoryID != "" {
		if err := s.checkCategory(ctx, tenantID, et.CategoryID); err != nil {
			return domain.EventType{}, err
		}
	}

	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.CreateType(ctx, tx, et); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicTypeCreated, s.changed(et))
	})
	if err != nil {
		return domain.EventType{}, err
	}
	return et, nil
}

func (s *Service) updateType(ctx context.Context, scope Scope, id string, patch domain.EventTypePatch) (domain.EventType, error) {
	if patch.HasCategoryChange() {
		if err := s.checkCategory(ctx, scope.TenantID, *patch.CategoryID); err != nil {
			return domain.EventType{}, err
		}
	}
	var out domain.EventType
	err := s.tx(ctx, func(tx *gorm.DB) error {
		et, err := s.repo.TypeByIDForUpdate(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		if et.TenantID != scope.TenantID {
			return domain.ErrGlobalReadOnly
		}
		if err := et.Apply(patch, s.clock.Now()); err != nil {
			return err
		}
		if err := s.repo.SaveType(ctx, tx, et); err != nil {
			return err
		}
		out = et
		return s.outbox.Publish(ctx, tx, contracts.TopicTypeUpdated, s.changed(et))
	})
	if err != nil {
		return domain.EventType{}, err
	}
	return out, nil
}

// deleteType soft-deletes. Rules keep their trigger slug; eventcatalog.
// type_deleted.v1 lets the rules module flag them.
func (s *Service) deleteType(ctx context.Context, scope Scope, id string) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		et, err := s.repo.TypeByIDForUpdate(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		if et.TenantID != scope.TenantID {
			return domain.ErrGlobalReadOnly
		}
		if err := s.repo.SoftDeleteType(ctx, tx, et.ID, s.clock.Now()); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicTypeDeleted, s.changed(et))
	})
}

// checkCategory: the category must exist, be visible to the owner of the
// event type and be assignable (globals may only use global categories).
func (s *Service) checkCategory(ctx context.Context, tenantID, categoryID string) error {
	c, err := s.repo.CategoryByID(ctx, Scope{TenantID: tenantID}, categoryID)
	if errors.Is(err, domain.ErrCategoryNotFound) {
		return domain.ErrUnknownCategory
	}
	if err != nil {
		return err
	}
	if !c.AssignableTo(tenantID) {
		return domain.ErrUnknownCategory
	}
	return nil
}

func (s *Service) changed(et domain.EventType) contracts.EventTypeChangedV1 {
	return contracts.EventTypeChangedV1{
		EventTypeID: et.ID,
		TenantID:    et.TenantID,
		Slug:        et.Slug,
		Active:      et.Active,
		At:          s.clock.Now(),
	}
}
