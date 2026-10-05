// Package domain holds eventcatalog's entities and invariants. An EventType
// is a trigger definition (purchase_completed, user_login, ...), not an
// occurrence; the Laravel model was called Event.
package domain

import (
	"maps"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"levelup/internal/shared/id"
)

const (
	// MaxSlugLen matches rules.trigger_event, which references the slug.
	MaxSlugLen        = 100
	MaxNameLen        = 255
	MaxDescriptionLen = 1000
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+([_-][a-z0-9]+)*$`)

// EventType is a trigger definition. TenantID "" marks a platform-global
// row, visible to every tenant and writable only by platform admins.
type EventType struct {
	ID          string
	TenantID    string
	CategoryID  string // "" = uncategorised
	Slug        string
	Name        string
	Description string
	// PropertySchema optionally describes the expected activity properties
	// (JSON Schema subset). nil = free-form. This is the column Laravel's
	// code wrote as "metadata" but never created.
	PropertySchema map[string]any
	Active         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewEventType carries the fields of a new event type. TenantID is stamped by
// the service from the principal, never from a request body.
type NewEventType struct {
	TenantID       string
	CategoryID     string
	Slug           string // "" derives it from Name
	Name           string
	Description    string
	PropertySchema map[string]any
	Active         bool
}

func CreateEventType(in NewEventType, now time.Time) (EventType, error) {
	name := strings.TrimSpace(in.Name)
	if err := checkName(name); err != nil {
		return EventType{}, err
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = Slugify(name)
	}
	if err := CheckSlug(slug); err != nil {
		return EventType{}, err
	}
	desc := strings.TrimSpace(in.Description)
	if err := checkDescription(desc); err != nil {
		return EventType{}, err
	}
	if err := ValidatePropertySchema(in.PropertySchema); err != nil {
		return EventType{}, err
	}
	return EventType{
		ID:             id.NewID(),
		TenantID:       in.TenantID,
		CategoryID:     in.CategoryID,
		Slug:           slug,
		Name:           name,
		Description:    desc,
		PropertySchema: maps.Clone(in.PropertySchema),
		Active:         in.Active,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// IsGlobal reports a platform-global row.
func (e EventType) IsGlobal() bool { return e.TenantID == "" }

// VisibleTo: a tenant sees its own rows and the global catalogue.
func (e EventType) VisibleTo(tenantID string) bool {
	return e.IsGlobal() || e.TenantID == tenantID
}

// EventTypePatch is a partial update: nil fields stay untouched (PATCH,
// fixing Laravel's PUT that nulled omitted fields).
type EventTypePatch struct {
	Name        *string
	Slug        *string
	Description *string // "" clears
	CategoryID  *string // "" clears
	// SetSchema distinguishes "leave as is" from "replace"; with SetSchema
	// and a nil PropertySchema the schema is cleared.
	SetSchema      bool
	PropertySchema map[string]any
	Active         *bool
}

// HasCategoryChange reports whether the patch sets a (non-empty) category,
// which the service must check for visibility.
func (p EventTypePatch) HasCategoryChange() bool {
	return p.CategoryID != nil && *p.CategoryID != ""
}

// Apply validates and applies the patch. The slug is immutable: sending the
// current slug is accepted as a no-op, anything else is ErrSlugImmutable.
func (e *EventType) Apply(p EventTypePatch, now time.Time) error {
	next := *e
	if p.Slug != nil && strings.TrimSpace(*p.Slug) != e.Slug {
		return ErrSlugImmutable
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
	if p.CategoryID != nil {
		next.CategoryID = *p.CategoryID
	}
	if p.SetSchema {
		if err := ValidatePropertySchema(p.PropertySchema); err != nil {
			return err
		}
		next.PropertySchema = maps.Clone(p.PropertySchema)
	}
	if p.Active != nil {
		next.Active = *p.Active
	}
	next.UpdatedAt = now
	*e = next
	return nil
}

// Slugify derives the canonical snake_case slug from a name: the style of the
// seeded catalogue and of rules.trigger_event ("Purchase Completed" →
// "purchase_completed"). Laravel's API generated kebab-case; both styles are
// accepted when given explicitly.
func Slugify(name string) string {
	var b strings.Builder
	pendingSep := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			if pendingSep && b.Len() > 0 {
				b.WriteByte('_')
			}
			pendingSep = false
			b.WriteRune(r)
		default:
			pendingSep = true
		}
	}
	s := b.String()
	if len(s) > MaxSlugLen {
		s = strings.TrimRight(s[:MaxSlugLen], "_")
	}
	return s
}

// CheckSlug enforces the slug format shared with the DB CHECK constraint.
func CheckSlug(slug string) error {
	if slug == "" || len(slug) > MaxSlugLen || !slugPattern.MatchString(slug) {
		return ErrInvalidSlug
	}
	return nil
}

func checkName(name string) error {
	if n := utf8.RuneCountInString(name); n == 0 || n > MaxNameLen {
		return ErrInvalidName
	}
	return nil
}

func checkDescription(desc string) error {
	if utf8.RuneCountInString(desc) > MaxDescriptionLen {
		return ErrInvalidDescription
	}
	return nil
}
