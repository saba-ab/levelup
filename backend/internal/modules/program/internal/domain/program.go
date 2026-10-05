// Package domain holds program's entities and invariants: the status state
// machine, the enrolment window, and the patch rules. No framework tags.
package domain

import (
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"levelup/internal/shared/id"
)

type Status string

const (
	StatusDraft  Status = "draft"
	StatusActive Status = "active"
	StatusPaused Status = "paused"
	StatusEnded  Status = "ended"
)

// ParseStatus accepts only the four known statuses.
func ParseStatus(s string) (Status, error) {
	switch st := Status(s); st {
	case StatusDraft, StatusActive, StatusPaused, StatusEnded:
		return st, nil
	}
	return "", ErrBadStatus
}

// transitions is the whole state machine. ended is terminal; anything not
// listed is refused with ErrInvalidTransition.
var transitions = map[Status][]Status{
	StatusDraft:  {StatusActive},
	StatusActive: {StatusPaused, StatusEnded},
	StatusPaused: {StatusActive, StatusEnded},
}

// CanTransition reports whether from → to is an allowed edge.
func CanTransition(from, to Status) bool {
	return slices.Contains(transitions[from], to)
}

const (
	maxNameLen        = 255
	maxDescriptionLen = 1000
	maxSlugLen        = 120
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Program struct {
	ID          string
	TenantID    string
	Name        string
	Slug        string
	Description *string
	Status      Status
	StartsAt    *time.Time
	EndsAt      *time.Time
	Settings    map[string]any
	Mechanics   map[string]any
	// Version is the optimistic-concurrency token; the repository refuses a
	// write when it moved underneath the transaction.
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type NewProgramInput struct {
	TenantID    string
	Name        string
	Slug        string // empty → derived from Name
	Description *string
	StartsAt    *time.Time
	EndsAt      *time.Time
	Settings    map[string]any
	Mechanics   map[string]any
}

// NewProgram creates a draft program. Status is never taken from input.
func NewProgram(in NewProgramInput, now time.Time) (Program, error) {
	if in.TenantID == "" {
		return Program{}, ErrNoTenant
	}
	p := Program{
		ID:          id.NewID(),
		TenantID:    in.TenantID,
		Name:        strings.TrimSpace(in.Name),
		Slug:        strings.TrimSpace(in.Slug),
		Description: normaliseDescription(in.Description),
		Status:      StatusDraft,
		StartsAt:    utcPtr(in.StartsAt),
		EndsAt:      utcPtr(in.EndsAt),
		Settings:    orEmpty(in.Settings),
		Mechanics:   orEmpty(in.Mechanics),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if p.Slug == "" {
		p.Slug = Slugify(p.Name)
	}
	if err := p.validate(); err != nil {
		return Program{}, err
	}
	return p, nil
}

func (p Program) validate() error {
	if p.Name == "" {
		return ErrNameRequired
	}
	if utf8.RuneCountInString(p.Name) > maxNameLen {
		return ErrNameTooLong
	}
	if p.Description != nil && utf8.RuneCountInString(*p.Description) > maxDescriptionLen {
		return ErrDescriptionLong
	}
	if len(p.Slug) > maxSlugLen || !slugPattern.MatchString(p.Slug) {
		return ErrBadSlug
	}
	if p.StartsAt != nil && p.EndsAt != nil && p.EndsAt.Before(*p.StartsAt) {
		return ErrBadWindow
	}
	return nil
}

// Slugify lower-cases ASCII letters and digits and joins everything else
// with single dashes. A name with no ASCII alphanumerics yields "" and the
// caller must supply a slug explicitly.
func Slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	s := strings.TrimRight(b.String(), "-")
	if len(s) > maxSlugLen {
		s = strings.TrimRight(s[:maxSlugLen], "-")
	}
	return s
}

// transition moves the program along one edge of the state machine.
func (p *Program) transition(to Status, now time.Time) error {
	if !CanTransition(p.Status, to) {
		return ErrInvalidTransition
	}
	p.Status = to
	p.UpdatedAt = now
	return nil
}

// Activate: draft|paused → active. A program whose ends_at has passed can
// never become active again.
func (p *Program) Activate(now time.Time) error {
	if !CanTransition(p.Status, StatusActive) {
		return ErrInvalidTransition
	}
	if p.EndsAt != nil && !p.EndsAt.After(now) {
		return ErrWindowElapsed
	}
	return p.transition(StatusActive, now)
}

// Pause: active → paused.
func (p *Program) Pause(now time.Time) error { return p.transition(StatusPaused, now) }

// End: active|paused → ended. ended is terminal.
func (p *Program) End(now time.Time) error { return p.transition(StatusEnded, now) }

// WithinWindow is true when now ∈ [StartsAt, EndsAt]; nil bounds are open.
func (p Program) WithinWindow(now time.Time) bool {
	if p.StartsAt != nil && now.Before(*p.StartsAt) {
		return false
	}
	if p.EndsAt != nil && now.After(*p.EndsAt) {
		return false
	}
	return true
}

// AcceptsEnrollment: active and inside the window.
func (p Program) AcceptsEnrollment(now time.Time) bool {
	return p.Status == StatusActive && p.WithinWindow(now)
}

// DueForAutoEnd: a running program whose ends_at has passed.
func (p Program) DueForAutoEnd(now time.Time) bool {
	return (p.Status == StatusActive || p.Status == StatusPaused) && p.EndsAt != nil && !p.EndsAt.After(now)
}

// Nullable is a patch field that distinguishes "absent" (Set=false) from
// "explicitly cleared" (Set=true, Value=nil).
type Nullable[T any] struct {
	Set   bool
	Value *T
}

// Patch is a partial update. Absent fields are left untouched. There is
// deliberately no Status field: status moves only through the transitions.
type Patch struct {
	Name        *string
	Slug        *string
	Description Nullable[string]
	StartsAt    Nullable[time.Time]
	EndsAt      Nullable[time.Time]
	Settings    map[string]any // nil → untouched; replaced wholesale otherwise
	Mechanics   map[string]any
}

// Edit applies the patch and returns the names of the fields that actually
// changed. On error the program is left unchanged.
func (p *Program) Edit(patch Patch, now time.Time) ([]string, error) {
	next := *p
	var changed []string

	if patch.Name != nil {
		if n := strings.TrimSpace(*patch.Name); n != next.Name {
			next.Name = n
			changed = append(changed, "name")
		}
	}
	if patch.Slug != nil {
		if s := strings.TrimSpace(*patch.Slug); s != next.Slug {
			next.Slug = s
			changed = append(changed, "slug")
		}
	}
	if patch.Description.Set {
		d := normaliseDescription(patch.Description.Value)
		if !equalPtr(d, next.Description) {
			next.Description = d
			changed = append(changed, "description")
		}
	}
	if patch.StartsAt.Set {
		v := utcPtr(patch.StartsAt.Value)
		if !equalTimePtr(v, next.StartsAt) {
			next.StartsAt = v
			changed = append(changed, "starts_at")
		}
	}
	if patch.EndsAt.Set {
		v := utcPtr(patch.EndsAt.Value)
		if !equalTimePtr(v, next.EndsAt) {
			next.EndsAt = v
			changed = append(changed, "ends_at")
		}
	}
	if patch.Settings != nil && !mapsEqualJSON(patch.Settings, next.Settings) {
		next.Settings = maps.Clone(patch.Settings)
		changed = append(changed, "settings")
	}
	if patch.Mechanics != nil && !mapsEqualJSON(patch.Mechanics, next.Mechanics) {
		next.Mechanics = maps.Clone(patch.Mechanics)
		changed = append(changed, "mechanics")
	}

	if len(changed) == 0 {
		return nil, nil
	}
	if err := next.validate(); err != nil {
		return nil, err
	}
	next.UpdatedAt = now
	*p = next
	return changed, nil
}

// Enrollment is a player's membership of a program. PlayerID is a bare
// uuid owned by the player module.
type Enrollment struct {
	ProgramID  string
	TenantID   string
	PlayerID   string
	EnrolledAt time.Time
}

func normaliseDescription(d *string) *string {
	if d == nil {
		return nil
	}
	v := strings.TrimSpace(*d)
	if v == "" {
		return nil
	}
	return &v
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func equalTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func mapsEqualJSON(a, b map[string]any) bool {
	return reflect.DeepEqual(orEmpty(a), orEmpty(b))
}
