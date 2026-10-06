package domain

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"levelup/internal/modules/ai/contracts"
	"levelup/internal/shared/errs"
)

// Caps on client-supplied tenant context. They bound prompt size (and cost).
const (
	MaxContextItems = 200
	MaxContextLevel = 100
	MaxRefNameLen   = 255
	MaxRefDescLen   = 500
)

// EventTypeRef is an event type the tenant can trigger rules and missions on.
type EventTypeRef struct {
	Slug        string
	Name        string
	Description string
}

// EntityRef is an existing tenant entity a draft may reference by id.
type EntityRef struct {
	ID   string
	Name string
}

// LevelRef is an existing level of the tenant's ladder.
type LevelRef struct {
	LevelNumber int
	Name        string
	XPRequired  int64
}

// Context is what the portal tells the model about the tenant. The ai module
// never reads other modules: the client lists what it already loaded. It is
// prompt DATA, and it is also the allow-list for ids a draft may reference.
type Context struct {
	EventTypes []EventTypeRef
	Badges     []EntityRef
	Missions   []EntityRef
	Rewards    []EntityRef
	Levels     []LevelRef
}

func (c Context) Validate() error {
	fields := map[string]string{}
	check := func(name string, n, maxN int) {
		if n > maxN {
			fields[name] = fmt.Sprintf("must have at most %d items", maxN)
		}
	}
	check("context.event_types", len(c.EventTypes), MaxContextItems)
	check("context.badges", len(c.Badges), MaxContextItems)
	check("context.missions", len(c.Missions), MaxContextItems)
	check("context.rewards", len(c.Rewards), MaxContextItems)
	check("context.levels", len(c.Levels), MaxContextLevel)
	for i, e := range c.EventTypes {
		if !IsEventSlug(e.Slug) {
			fields[fmt.Sprintf("context.event_types[%d].slug", i)] = "must be an event type slug"
		}
	}
	refs := map[string][]EntityRef{"badges": c.Badges, "missions": c.Missions, "rewards": c.Rewards}
	for name, list := range refs {
		for i, r := range list {
			if !isUUID(r.ID) {
				fields[fmt.Sprintf("context.%s[%d].id", name, i)] = "must be a UUID"
			}
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return errs.WithCode(errs.WithFields(errs.New(errs.Invalid, "invalid context"), fields), contracts.CodeInvalidContext)
}

// hasEventTypes reports whether the client listed event types; when it did,
// drafts may only trigger on those.
func (c Context) hasEventType(slug string) bool {
	for _, e := range c.EventTypes {
		if e.Slug == slug {
			return true
		}
	}
	return false
}

func hasRef(list []EntityRef, id string) bool {
	for _, r := range list {
		if strings.EqualFold(r.ID, id) {
			return true
		}
	}
	return false
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}
