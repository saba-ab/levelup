// Package domain holds notifications' entities and invariants. No framework
// tags here.
package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/shared/id"
)

// Template renders a notification for every player fact of its trigger.
type Template struct {
	ID            string
	TenantID      string
	Name          string
	Trigger       string
	Channels      []string
	TitleTemplate string
	BodyTemplate  string
	Active        bool
	CreatedBy     string
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

func (t Template) Deleted() bool { return t.DeletedAt != nil }

// HasChannel reports whether the template sends on ch.
func (t Template) HasChannel(ch string) bool {
	for _, c := range t.Channels {
		if c == ch {
			return true
		}
	}
	return false
}

// NewTemplateParams is the create input; TenantID and CreatedBy come from
// the principal.
type NewTemplateParams struct {
	TenantID      string
	CreatedBy     string
	Name          string
	Trigger       string
	Channels      []string
	TitleTemplate string
	BodyTemplate  string
	Active        bool
}

// NewTemplate validates and builds a template; the sources must compile and
// dry-run against SampleData.
func NewTemplate(p NewTemplateParams, now time.Time, renderTimeout time.Duration) (Template, error) {
	t := Template{
		ID:            id.NewID(),
		TenantID:      p.TenantID,
		Name:          strings.TrimSpace(p.Name),
		Trigger:       p.Trigger,
		Channels:      normalizeChannels(p.Channels),
		TitleTemplate: p.TitleTemplate,
		BodyTemplate:  p.BodyTemplate,
		Active:        p.Active,
		CreatedBy:     p.CreatedBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := t.Validate(renderTimeout); err != nil {
		return Template{}, err
	}
	return t, nil
}

// Patch is a partial update; nil fields stay untouched.
type Patch struct {
	Name          *string
	Trigger       *string
	Channels      *[]string
	TitleTemplate *string
	BodyTemplate  *string
	Active        *bool
}

// Apply patches and revalidates.
func (t *Template) Apply(p Patch, now time.Time, renderTimeout time.Duration) error {
	next := *t
	if p.Name != nil {
		next.Name = strings.TrimSpace(*p.Name)
	}
	if p.Trigger != nil {
		next.Trigger = *p.Trigger
	}
	if p.Channels != nil {
		next.Channels = normalizeChannels(*p.Channels)
	}
	if p.TitleTemplate != nil {
		next.TitleTemplate = *p.TitleTemplate
	}
	if p.BodyTemplate != nil {
		next.BodyTemplate = *p.BodyTemplate
	}
	if p.Active != nil {
		next.Active = *p.Active
	}
	if err := next.Validate(renderTimeout); err != nil {
		return err
	}
	next.UpdatedAt = now
	*t = next
	return nil
}

// normalizeChannels lowercases, trims and de-duplicates, keeping order.
func normalizeChannels(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, c := range in {
		c = strings.ToLower(strings.TrimSpace(c))
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// Validate checks every invariant and reports all failing fields at once.
func (t Template) Validate(renderTimeout time.Duration) error {
	fields := map[string]string{}
	if t.Name == "" {
		fields["name"] = "is required"
	} else if utf8.RuneCountInString(t.Name) > 255 {
		fields["name"] = "must be at most 255 characters"
	}
	if !IsTrigger(t.Trigger) {
		fields["trigger"] = "must be one of " + strings.Join(contracts.AllTriggers, ", ")
	}
	if len(t.Channels) == 0 {
		fields["channels"] = "at least one channel is required"
	}
	for _, c := range t.Channels {
		if c != contracts.ChannelInApp && c != contracts.ChannelEmail {
			fields["channels"] = "channels must be in_app or email"
			break
		}
	}
	checkSource := func(field, src string, maxLen int, required bool) {
		switch {
		case required && strings.TrimSpace(src) == "":
			fields[field] = "is required"
			return
		case utf8.RuneCountInString(src) > maxLen:
			fields[field] = "is too long"
			return
		}
		c, err := Compile(src)
		if err != nil {
			fields[field] = err.Error()
			return
		}
		// Dry run: an unknown field ({{.Player.Email}}) is an exec error.
		if _, err := c.Execute(SampleData(t.Trigger), renderTimeout, MaxRenderOutput); err != nil {
			fields[field] = err.Error()
		}
	}
	checkSource("title_template", t.TitleTemplate, MaxTitleSource, true)
	checkSource("body_template", t.BodyTemplate, MaxBodySource, false)
	if len(fields) > 0 {
		return invalid(fields)
	}
	return nil
}
