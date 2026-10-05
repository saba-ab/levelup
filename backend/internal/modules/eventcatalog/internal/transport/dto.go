package transport

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"levelup/internal/modules/eventcatalog/internal/domain"
	"levelup/internal/shared/errs"
)

// CreateEventTypeReq creates an event type. There is deliberately no
// tenant_id / is_predefined field: the tenant comes from the token, and
// global rows are created only on /platform/event-types.
type CreateEventTypeReq struct {
	Name string `json:"name" validate:"required,max=255"`
	// Slug defaults to the snake_case form of name.
	Slug        string `json:"slug" validate:"omitempty,max=100"`
	Description string `json:"description" validate:"omitempty,max=1000"`
	CategoryID  string `json:"category_id" validate:"omitempty,uuid"`
	// PropertySchema is an optional JSON Schema subset (type/properties/required).
	PropertySchema map[string]any `json:"property_schema"`
	// IsActive defaults to true.
	IsActive *bool `json:"is_active"`
}

// UpdateEventTypeReq is a PATCH: omitted fields stay untouched.
type UpdateEventTypeReq struct {
	Name *string `json:"name" validate:"omitnil,min=1,max=255"`
	// Slug is immutable; sending the current value is accepted.
	Slug *string `json:"slug" validate:"omitnil,max=100"`
	// Description "" clears it.
	Description *string `json:"description" validate:"omitnil,max=1000"`
	// CategoryID: a uuid sets it, null clears it.
	CategoryID json.RawMessage `json:"category_id" swaggertype:"string"`
	// PropertySchema: an object replaces it, null clears it.
	PropertySchema json.RawMessage `json:"property_schema" swaggertype:"object"`
	IsActive       *bool           `json:"is_active"`
}

// toPatch converts the nullable raw fields; shape errors are errs.Invalid
// with the offending field, like validate tag failures.
func (req UpdateEventTypeReq) toPatch() (domain.EventTypePatch, error) {
	p := domain.EventTypePatch{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Active:      req.IsActive,
	}
	if req.CategoryID != nil {
		var v *string
		if err := json.Unmarshal(req.CategoryID, &v); err != nil {
			return p, fieldErr("category_id", "must be a uuid or null")
		}
		if v == nil {
			cleared := ""
			p.CategoryID = &cleared
		} else {
			if _, err := uuid.Parse(*v); err != nil {
				return p, fieldErr("category_id", "must be a valid UUID")
			}
			p.CategoryID = v
		}
	}
	if req.PropertySchema != nil {
		var v map[string]any
		if err := json.Unmarshal(req.PropertySchema, &v); err != nil {
			return p, fieldErr("property_schema", "must be an object or null")
		}
		p.SetSchema = true
		p.PropertySchema = v
	}
	return p, nil
}

func fieldErr(field, msg string) error {
	return errs.WithFields(errs.New(errs.Invalid, "validation failed"), map[string]string{field: msg})
}

type EventTypeResp struct {
	ID string `json:"id"`
	// TenantID is null for platform-global types.
	TenantID *string `json:"tenant_id"`
	IsGlobal bool    `json:"is_global"`
	// IsPredefined mirrors is_global (Laravel field name), read-only.
	IsPredefined   bool           `json:"is_predefined"`
	CategoryID     *string        `json:"category_id"`
	Slug           string         `json:"slug"`
	Name           string         `json:"name"`
	Description    *string        `json:"description"`
	PropertySchema map[string]any `json:"property_schema"`
	IsActive       bool           `json:"is_active"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type EventTypeListResp struct {
	Data       []EventTypeResp `json:"data"`
	NextCursor string          `json:"next_cursor"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toEventTypeResp(et domain.EventType) EventTypeResp {
	return EventTypeResp{
		ID:             et.ID,
		TenantID:       optional(et.TenantID),
		IsGlobal:       et.IsGlobal(),
		IsPredefined:   et.IsGlobal(),
		CategoryID:     optional(et.CategoryID),
		Slug:           et.Slug,
		Name:           et.Name,
		Description:    optional(et.Description),
		PropertySchema: et.PropertySchema,
		IsActive:       et.Active,
		CreatedAt:      et.CreatedAt.UTC(),
		UpdatedAt:      et.UpdatedAt.UTC(),
	}
}

type CreateCategoryReq struct {
	Name        string `json:"name" validate:"required,max=255"`
	Slug        string `json:"slug" validate:"omitempty,max=100"`
	Description string `json:"description" validate:"omitempty,max=1000"`
	SortOrder   int    `json:"sort_order"`
}

// UpdateCategoryReq is a PATCH: omitted fields stay untouched.
type UpdateCategoryReq struct {
	Name        *string `json:"name" validate:"omitnil,min=1,max=255"`
	Slug        *string `json:"slug" validate:"omitnil,min=1,max=100"`
	Description *string `json:"description" validate:"omitnil,max=1000"`
	SortOrder   *int    `json:"sort_order"`
}

type CategoryResp struct {
	ID          string    `json:"id"`
	TenantID    *string   `json:"tenant_id"`
	IsGlobal    bool      `json:"is_global"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CategoryListResp keeps the list envelope; categories are a bounded,
// sort_order-ordered catalogue, so next_cursor is always "".
type CategoryListResp struct {
	Data       []CategoryResp `json:"data"`
	NextCursor string         `json:"next_cursor"`
}

func toCategoryResp(c domain.Category) CategoryResp {
	return CategoryResp{
		ID:          c.ID,
		TenantID:    optional(c.TenantID),
		IsGlobal:    c.IsGlobal(),
		Slug:        c.Slug,
		Name:        c.Name,
		Description: optional(c.Description),
		SortOrder:   c.SortOrder,
		CreatedAt:   c.CreatedAt.UTC(),
		UpdatedAt:   c.UpdatedAt.UTC(),
	}
}
