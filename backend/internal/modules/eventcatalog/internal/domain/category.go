package domain

import (
	"strings"
	"time"

	"levelup/internal/shared/id"
)

// Category groups event types for catalogue UIs. TenantID "" = global.
// Only platform admins manage categories today (it replaces Laravel's
// Filament resource); the schema already allows tenant-owned rows.
type Category struct {
	ID          string
	TenantID    string
	Slug        string
	Name        string
	Description string
	SortOrder   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type NewCategory struct {
	TenantID    string
	Slug        string // "" derives it from Name
	Name        string
	Description string
	SortOrder   int
}

func CreateCategory(in NewCategory, now time.Time) (Category, error) {
	name := strings.TrimSpace(in.Name)
	if err := checkName(name); err != nil {
		return Category{}, err
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = Slugify(name)
	}
	if err := CheckSlug(slug); err != nil {
		return Category{}, err
	}
	desc := strings.TrimSpace(in.Description)
	if err := checkDescription(desc); err != nil {
		return Category{}, err
	}
	return Category{
		ID:          id.NewID(),
		TenantID:    in.TenantID,
		Slug:        slug,
		Name:        name,
		Description: desc,
		SortOrder:   in.SortOrder,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (c Category) IsGlobal() bool { return c.TenantID == "" }

// VisibleTo: own categories plus the global ones.
func (c Category) VisibleTo(tenantID string) bool {
	return c.IsGlobal() || c.TenantID == tenantID
}

// AssignableTo reports whether an event type owned by tenantID ("" = a
// global type) may reference this category. Global types may only use
// global categories, or a tenant's purge would silently uncategorise them.
func (c Category) AssignableTo(tenantID string) bool {
	if tenantID == "" {
		return c.IsGlobal()
	}
	return c.VisibleTo(tenantID)
}

// CategoryPatch is a partial update; nil fields stay untouched. Category
// slugs are not referenced outside this module, so they may change.
type CategoryPatch struct {
	Slug        *string
	Name        *string
	Description *string // "" clears
	SortOrder   *int
}

func (c *Category) Apply(p CategoryPatch, now time.Time) error {
	next := *c
	if p.Slug != nil {
		slug := strings.TrimSpace(*p.Slug)
		if err := CheckSlug(slug); err != nil {
			return err
		}
		next.Slug = slug
	}
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if err := checkName(name); err != nil {
			return err
		}
		next.Name = name
	}
	if p.Description != nil {
		desc := strings.TrimSpace(*p.Description)
		if err := checkDescription(desc); err != nil {
			return err
		}
		next.Description = desc
	}
	if p.SortOrder != nil {
		next.SortOrder = *p.SortOrder
	}
	next.UpdatedAt = now
	*c = next
	return nil
}
