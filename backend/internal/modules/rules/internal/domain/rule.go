// Package domain holds the rules module's entities and invariants: a rule,
// its immutable versions, and the decision log (doc 06 §11).
package domain

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/shared/id"
)

// Rule statuses. A rule is live iff status = active AND not deleted AND it
// has a current version. Only publishing makes a rule active.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusInactive = "inactive"
	StatusArchived = "archived"
)

const (
	MaxNameLen        = 255
	MaxDescriptionLen = 1000
	MaxSlugLen        = 120
	MaxTriggerLen     = 100
	MaxPriority       = 1_000_000
)

var (
	slugRe    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	triggerRe = regexp.MustCompile(`^[a-z0-9_.:-]+$`)
	nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)
)

// Rule is the mutable head of a version chain.
type Rule struct {
	ID               string
	TenantID         string
	Slug             string
	Name             string
	Description      string
	TriggerEvent     string
	ProgramID        string // "" = tenant-wide
	Priority         int
	Status           string
	CurrentVersionID string // the ONE live version; "" until first publish
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

// NewRuleParams are the inputs of NewRule.
type NewRuleParams struct {
	TenantID     string
	Slug         string // derived from Name when empty
	Name         string
	Description  string
	TriggerEvent string
	ProgramID    string
	Priority     int
}

// NewRule validates and builds a draft rule.
func NewRule(p NewRuleParams, now time.Time) (Rule, error) {
	r := Rule{
		ID:           id.NewID(),
		TenantID:     p.TenantID,
		Slug:         p.Slug,
		Name:         strings.TrimSpace(p.Name),
		Description:  p.Description,
		TriggerEvent: p.TriggerEvent,
		ProgramID:    strings.ToLower(p.ProgramID),
		Priority:     p.Priority,
		Status:       StatusDraft,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if r.Slug == "" {
		r.Slug = Slugify(r.Name)
	}
	if err := r.Validate(); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// Validate checks every field invariant.
func (r Rule) Validate() error {
	switch {
	case r.Name == "":
		return ErrNameRequired
	case utf8.RuneCountInString(r.Name) > MaxNameLen:
		return ErrNameTooLong
	case utf8.RuneCountInString(r.Description) > MaxDescriptionLen:
		return ErrDescriptionLong
	case len(r.Slug) > MaxSlugLen || !slugRe.MatchString(r.Slug):
		return ErrBadSlug
	case len(r.TriggerEvent) > MaxTriggerLen || !triggerRe.MatchString(r.TriggerEvent):
		return ErrBadTriggerEvent
	case r.ProgramID != "" && !eval.IsUUID(r.ProgramID):
		return ErrBadProgramID
	case r.Priority < 0 || r.Priority > MaxPriority:
		return ErrBadPriority
	}
	return nil
}

// ValidTriggerEvent reports a well-formed trigger event slug.
func ValidTriggerEvent(s string) bool { return len(s) <= MaxTriggerLen && triggerRe.MatchString(s) }

// Slugify lower-cases and dashes a name.
func Slugify(name string) string {
	s := nonSlugRe.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if len(s) > MaxSlugLen {
		s = strings.Trim(s[:MaxSlugLen], "-")
	}
	return s
}

// Live reports whether the rule is evaluated for activities.
func (r Rule) Live() bool {
	return r.Status == StatusActive && r.DeletedAt == nil && r.CurrentVersionID != ""
}

// AffectsRuleset reports whether a change from old to r alters what the
// evaluator sees, so the tenant's ruleset generation must be bumped.
func (r Rule) AffectsRuleset(old Rule) bool {
	if !r.Live() && !old.Live() {
		return false
	}
	return r.Live() != old.Live() ||
		r.TriggerEvent != old.TriggerEvent ||
		r.Priority != old.Priority ||
		r.ProgramID != old.ProgramID ||
		r.Name != old.Name ||
		r.CurrentVersionID != old.CurrentVersionID
}

// SetStatus applies a PATCH status change. Draft → active is not a status
// change, it is a publish (only publishing sets the live version).
func (r *Rule) SetStatus(target string, now time.Time) error {
	if r.Status == StatusArchived {
		if target == StatusArchived {
			return nil
		}
		return ErrRuleArchived
	}
	if target == r.Status {
		return nil
	}
	switch target {
	case StatusActive:
		if r.CurrentVersionID == "" {
			return ErrPublishRequired
		}
	case StatusInactive:
		if r.Status != StatusActive {
			return ErrInvalidTransition
		}
	case StatusArchived:
	default:
		return ErrBadStatus
	}
	r.Status = target
	r.UpdatedAt = now
	return nil
}

// Publish makes v the one live version (R63): there is exactly one
// current_version_id per rule, so two versions can never both fire (B10).
func (r *Rule) Publish(v Version, now time.Time) error {
	if r.Status == StatusArchived {
		return ErrRuleArchived
	}
	if v.RuleID != r.ID {
		return ErrVersionNotFound
	}
	r.CurrentVersionID = v.ID
	r.Status = StatusActive
	r.UpdatedAt = now
	return nil
}

// Delete soft-deletes the rule.
func (r *Rule) Delete(now time.Time) {
	r.DeletedAt = &now
	r.UpdatedAt = now
}

// Version is one definition of a rule. Draft versions may be edited; once
// published (PublishedAt set) a version is immutable forever.
type Version struct {
	ID             string
	RuleID         string
	TenantID       string
	Version        int
	Conditions     json.RawMessage
	Actions        json.RawMessage
	Limits         json.RawMessage
	Schedule       json.RawMessage // JSON null = always
	StopProcessing bool
	PublishedAt    *time.Time
	CreatedBy      string
	CreatedAt      time.Time
}

// Definition is a version's rule body (what the evaluator compiles).
type Definition struct {
	Conditions     json.RawMessage
	Actions        json.RawMessage
	Limits         json.RawMessage
	Schedule       json.RawMessage
	StopProcessing bool
}

// Spec maps the definition to the evaluator's input.
func (d Definition) Spec() eval.Spec {
	return eval.Spec{Conditions: d.Conditions, Actions: d.Actions, Limits: d.Limits,
		Schedule: d.Schedule, StopProcessing: d.StopProcessing}
}

// DefinitionPatch edits parts of a definition; nil leaves a part unchanged.
type DefinitionPatch struct {
	Conditions     json.RawMessage
	Actions        json.RawMessage
	Limits         json.RawMessage
	Schedule       json.RawMessage
	StopProcessing *bool
}

// IsZero reports a patch that changes nothing.
func (p DefinitionPatch) IsZero() bool {
	return p.Conditions == nil && p.Actions == nil && p.Limits == nil && p.Schedule == nil && p.StopProcessing == nil
}

// Apply returns base with the patch's parts replaced.
func (p DefinitionPatch) Apply(base Definition) Definition {
	if p.Conditions != nil {
		base.Conditions = p.Conditions
	}
	if p.Actions != nil {
		base.Actions = p.Actions
	}
	if p.Limits != nil {
		base.Limits = p.Limits
	}
	if p.Schedule != nil {
		base.Schedule = p.Schedule
	}
	if p.StopProcessing != nil {
		base.StopProcessing = *p.StopProcessing
	}
	return base
}

// Published reports an immutable version.
func (v Version) Published() bool { return v.PublishedAt != nil }

// Definition returns the version's rule body.
func (v Version) Definition() Definition {
	return Definition{Conditions: v.Conditions, Actions: v.Actions, Limits: v.Limits,
		Schedule: v.Schedule, StopProcessing: v.StopProcessing}
}

// ValidateDefinition normalizes and compiles a definition (the write-time
// gate): every compile error is errs.Invalid with per-field errors.
func ValidateDefinition(def Definition, opts eval.Options) (Definition, *eval.Definition, error) {
	def.Conditions = normalizeJSON(def.Conditions)
	def.Actions = normalizeJSON(def.Actions)
	def.Limits = normalizeJSON(def.Limits)
	def.Schedule = normalizeJSON(def.Schedule)
	compiled, err := eval.CompileSpec(def.Spec(), opts)
	if err != nil {
		return Definition{}, nil, err
	}
	return def, compiled, nil
}

// NewVersion validates the definition (compile) and builds a draft version.
func NewVersion(rule Rule, number int, def Definition, createdBy string, opts eval.Options, now time.Time) (Version, error) {
	def, _, err := ValidateDefinition(def, opts)
	if err != nil {
		return Version{}, err
	}
	return Version{
		ID:             id.NewID(),
		RuleID:         rule.ID,
		TenantID:       rule.TenantID,
		Version:        number,
		Conditions:     def.Conditions,
		Actions:        def.Actions,
		Limits:         def.Limits,
		Schedule:       def.Schedule,
		StopProcessing: def.StopProcessing,
		CreatedBy:      createdBy,
		CreatedAt:      now,
	}, nil
}

// Edit replaces parts of a draft's definition.
func (v *Version) Edit(patch DefinitionPatch, opts eval.Options) error {
	if v.Published() {
		return ErrVersionPublished
	}
	def, _, err := ValidateDefinition(patch.Apply(v.Definition()), opts)
	if err != nil {
		return err
	}
	v.Conditions, v.Actions, v.Limits, v.Schedule = def.Conditions, def.Actions, def.Limits, def.Schedule
	v.StopProcessing = def.StopProcessing
	return nil
}

// MarkPublished stamps the first publish time; re-publishing an already
// published version (rollback to it) keeps the original stamp.
func (v *Version) MarkPublished(now time.Time) {
	if v.PublishedAt == nil {
		v.PublishedAt = &now
	}
}

// normalizeJSON maps absent and empty input to JSON null so the stored
// column is always valid JSON.
func normalizeJSON(raw json.RawMessage) json.RawMessage {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return json.RawMessage("null")
	}
	return json.RawMessage(s)
}
