package domain

import (
	"maps"
	"strings"
	"time"
	_ "time/tzdata" // IANA validation must not depend on the host's zoneinfo

	"levelup/internal/shared/id"
)

const (
	maxNameLen      = 255
	maxSlugBaseLen  = 48
	DefaultTimezone = "UTC"
)

// Tenant is a customer organisation. OwnerUserID always points at a member
// holding the owner role (enforced by the service and a deferred FK).
type Tenant struct {
	ID          string
	Name        string
	Slug        string
	OwnerUserID string
	Active      bool
	Timezone    string
	Settings    map[string]any
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// NewTenant builds an active tenant. suffix is the random uniqueness part of
// the slug; the caller regenerates it on a slug collision.
func NewTenant(name, timezone, suffix string, now time.Time) (Tenant, error) {
	name, err := cleanName(name)
	if err != nil {
		return Tenant{}, err
	}
	if timezone == "" {
		timezone = DefaultTimezone
	}
	if err := ValidateTimezone(timezone); err != nil {
		return Tenant{}, err
	}
	return Tenant{
		ID:        id.NewID(),
		Name:      name,
		Slug:      SlugFor(name, suffix),
		Active:    true,
		Timezone:  timezone,
		Settings:  map[string]any{},
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// SlugFor is slugify(name) + "-" + suffix (Laravel's Str::slug + random(6)).
func SlugFor(name, suffix string) string {
	base := Slugify(name)
	if base == "" {
		base = "tenant"
	}
	if suffix == "" {
		return base
	}
	return base + "-" + suffix
}

// Slugify lower-cases and keeps [a-z0-9], collapsing every other run into a
// single dash.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= maxSlugBaseLen {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

// ValidateTimezone accepts IANA names only ("Local" is host-dependent).
func ValidateTimezone(tz string) error {
	if tz == "" || tz == "Local" {
		return ErrInvalidTimezone
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return ErrInvalidTimezone
	}
	return nil
}

// IsActive is what login and the TenantReader report.
func (t Tenant) IsActive() bool { return t.Active && t.DeletedAt == nil }

func (t Tenant) OwnedBy(userID string) bool { return t.OwnerUserID != "" && t.OwnerUserID == userID }

// TenantChanges is a partial update; nil fields stay untouched.
type TenantChanges struct {
	Name     *string
	Timezone *string
	Settings map[string]any // nil = untouched; replaces the whole object
}

// Apply mutates t and reports whether anything changed.
func (t *Tenant) Apply(c TenantChanges, now time.Time) (bool, error) {
	changed := false
	if c.Name != nil {
		name, err := cleanName(*c.Name)
		if err != nil {
			return false, err
		}
		if name != t.Name {
			t.Name, changed = name, true
		}
	}
	if c.Timezone != nil {
		if err := ValidateTimezone(*c.Timezone); err != nil {
			return false, err
		}
		if *c.Timezone != t.Timezone {
			t.Timezone, changed = *c.Timezone, true
		}
	}
	if c.Settings != nil && !settingsEqual(t.Settings, c.Settings) {
		t.Settings, changed = maps.Clone(c.Settings), true
	}
	if changed {
		t.UpdatedAt = now
	}
	return changed, nil
}

// SetActive reports whether the flag moved.
func (t *Tenant) SetActive(active bool, now time.Time) bool {
	if t.Active == active {
		return false
	}
	t.Active = active
	t.UpdatedAt = now
	return true
}

// SoftDelete marks the tenant gone; modules purge asynchronously on
// tenant.deleted.v1.
func (t *Tenant) SoftDelete(now time.Time) {
	t.Active = false
	t.DeletedAt = &now
	t.UpdatedAt = now
}

func settingsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !shallowEqual(av, bv) {
			return false
		}
	}
	return true
}

// shallowEqual treats nested objects/arrays as always changed: comparing
// arbitrary JSON deeply is not worth a dependency for a "changed?" flag.
func shallowEqual(a, b any) bool {
	switch a.(type) {
	case map[string]any, []any:
		return false
	}
	switch b.(type) {
	case map[string]any, []any:
		return false
	}
	return a == b
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > maxNameLen {
		return "", ErrInvalidName
	}
	return name, nil
}
