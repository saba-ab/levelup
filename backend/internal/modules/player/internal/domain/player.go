// Package domain holds the player entity and its invariants. No framework
// tags: persistence and transport map into and out of these types.
package domain

import (
	"maps"
	"net/mail"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxExternalIDLen  = 255
	MaxDisplayNameLen = 255
	MaxEmailLen       = 255
	MaxAttributes     = 100
	MaxAttributeKey   = 64
)

// Changed-field names, as reported in player.updated.v1.
const (
	FieldDisplayName = "display_name"
	FieldEmail       = "email"
	FieldAttributes  = "attributes"
	FieldActive      = "is_active"
)

// Player is a tenant's end user, addressed by the tenant's own ExternalID.
// Empty DisplayName / Email mean "not set" (stored as NULL).
type Player struct {
	ID          string
	TenantID    string
	ExternalID  string
	DisplayName string
	Email       string
	// Attributes is the tenant-defined profile; rule conditions read it as
	// player.*. Always a JSON object, never nil.
	Attributes map[string]any
	Active     bool
	CreatedBy  string // operator user id; empty when unknown
	// Version is the optimistic-concurrency token; Save refuses a write
	// when it moved underneath the transaction.
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// Profile is the client-settable part of a player at creation.
type Profile struct {
	DisplayName string
	Email       string
	Attributes  map[string]any
}

// NewPlayer builds an active player with canonical values: external id
// trimmed, email lower-cased, attributes an object.
func NewPlayer(id, tenantID, externalID string, prof Profile, createdBy string, now time.Time) (Player, error) {
	if strings.TrimSpace(tenantID) == "" {
		return Player{}, ErrNoTenant
	}
	ext, err := canonicalExternalID(externalID)
	if err != nil {
		return Player{}, err
	}
	name, err := canonicalDisplayName(prof.DisplayName)
	if err != nil {
		return Player{}, err
	}
	email, err := canonicalEmail(prof.Email)
	if err != nil {
		return Player{}, err
	}
	attrs := map[string]any{}
	if prof.Attributes != nil {
		attrs = maps.Clone(prof.Attributes)
	}
	if err := checkAttributes(attrs); err != nil {
		return Player{}, err
	}
	return Player{
		ID:          id,
		TenantID:    tenantID,
		ExternalID:  ext,
		DisplayName: name,
		Email:       email,
		Attributes:  attrs,
		Active:      true,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Patch is a true partial update: a nil field leaves the player untouched.
// An empty string clears DisplayName / Email. Attributes is a JSON merge
// patch (RFC 7396, one level): listed keys are set, a nil value removes the
// key, unlisted keys stay.
type Patch struct {
	DisplayName *string
	Email       *string
	Attributes  map[string]any
	Active      *bool
}

// ApplyPatch applies p and reports which fields actually changed. Nothing
// changed means nothing to save and nothing to publish.
func (pl *Player) ApplyPatch(p Patch, now time.Time) ([]string, error) {
	next := *pl
	var changed []string

	if p.DisplayName != nil {
		name, err := canonicalDisplayName(*p.DisplayName)
		if err != nil {
			return nil, err
		}
		if name != next.DisplayName {
			next.DisplayName = name
			changed = append(changed, FieldDisplayName)
		}
	}
	if p.Email != nil {
		email, err := canonicalEmail(*p.Email)
		if err != nil {
			return nil, err
		}
		if email != next.Email {
			next.Email = email
			changed = append(changed, FieldEmail)
		}
	}
	if p.Attributes != nil {
		merged := maps.Clone(next.Attributes)
		if merged == nil {
			merged = map[string]any{}
		}
		for k, v := range p.Attributes {
			if v == nil {
				delete(merged, k)
				continue
			}
			merged[k] = v
		}
		if err := checkAttributes(merged); err != nil {
			return nil, err
		}
		if !attributesEqual(merged, next.Attributes) {
			next.Attributes = merged
			changed = append(changed, FieldAttributes)
		}
	}
	if p.Active != nil && *p.Active != next.Active {
		next.Active = *p.Active
		changed = append(changed, FieldActive)
	}

	if len(changed) == 0 {
		return nil, nil
	}
	next.UpdatedAt = now
	*pl = next
	return changed, nil
}

// Activate reports whether the status changed.
func (pl *Player) Activate(now time.Time) bool {
	if pl.Active {
		return false
	}
	pl.Active = true
	pl.UpdatedAt = now
	return true
}

// Deactivate reports whether the status changed.
func (pl *Player) Deactivate(now time.Time) bool {
	if !pl.Active {
		return false
	}
	pl.Active = false
	pl.UpdatedAt = now
	return true
}

// MarkDeleted soft-deletes the player; its external id becomes reusable.
func (pl *Player) MarkDeleted(now time.Time) {
	t := now
	pl.DeletedAt = &t
	pl.UpdatedAt = now
}

func (pl Player) Deleted() bool { return pl.DeletedAt != nil }

// BelongsTo is the tenant-ownership check; a foreign row is reported as
// not found, never as forbidden.
func (pl Player) BelongsTo(tenantID string) bool {
	return tenantID != "" && pl.TenantID == tenantID
}

func canonicalExternalID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ErrNoExternalID
	}
	if utf8.RuneCountInString(s) > MaxExternalIDLen {
		return "", ErrExternalIDTooLong
	}
	return s, nil
}

func canonicalDisplayName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxDisplayNameLen {
		return "", ErrDisplayNameLong
	}
	return s, nil
}

func canonicalEmail(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", nil
	}
	if len(s) > MaxEmailLen {
		return "", ErrEmailInvalid
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s {
		return "", ErrEmailInvalid
	}
	return s, nil
}

func checkAttributes(a map[string]any) error {
	if len(a) > MaxAttributes {
		return ErrTooManyAttributes
	}
	for k := range a {
		if k == "" || utf8.RuneCountInString(k) > MaxAttributeKey {
			return ErrBadAttributeKey
		}
	}
	return nil
}

// attributesEqual compares two JSON-shaped objects structurally.
func attributesEqual(a, b map[string]any) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
