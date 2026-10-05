// Package domain holds missions' entities and invariants. No tags, no I/O.
package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"levelup/internal/modules/missions/contracts"
	"levelup/internal/shared/id"
)

// Mission is a tenant-defined goal: reach Target units of progress. Criteria
// is opaque to this module: it documents WHICH activities count (for
// example {"event_type": "purchase_completed", "min_amount": 100}) and is
// evaluated by the rules module, which sends job.missions.progress commands.
// Missions only counts.
type Mission struct {
	ID            string
	TenantID      string
	Slug          string
	Name          string
	Description   string
	Type          string
	Status        string
	Target        int64
	Criteria      map[string]any
	PointsReward  int64
	XPReward      int64
	BadgeRewardID string // "" = no badge
	// MaxCompletionsPerPlayer caps completed attempts per player across all
	// periods. nil = unlimited. one_time missions always carry 1.
	MaxCompletionsPerPlayer *int
	StartsAt                *time.Time
	EndsAt                  *time.Time
	Version                 int
	CreatedAt               time.Time
	UpdatedAt               time.Time
	DeletedAt               *time.Time
}

// NewMissionParams are the creatable fields; the service stamps the tenant.
type NewMissionParams struct {
	TenantID                string
	Slug                    string
	Name                    string
	Description             string
	Type                    string
	Status                  string
	Target                  int64
	Criteria                map[string]any
	PointsReward            int64
	XPReward                int64
	BadgeRewardID           string
	MaxCompletionsPerPlayer *int
	StartsAt                *time.Time
	EndsAt                  *time.Time
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// NewMission validates and normalises. An empty slug is derived from the
// name; an empty status is draft; a one_time mission gets max 1.
func NewMission(p NewMissionParams, now time.Time) (Mission, error) {
	if p.TenantID == "" {
		return Mission{}, ErrNoTenant
	}
	if p.Status == "" {
		p.Status = contracts.MissionDraft
	}
	if p.Status != contracts.MissionDraft && p.Status != contracts.MissionActive {
		return Mission{}, ErrBadInitialStatus
	}
	if p.Slug == "" {
		p.Slug = Slugify(p.Name)
	}
	if p.Criteria == nil {
		p.Criteria = map[string]any{}
	}
	m := Mission{
		ID:                      id.NewID(),
		TenantID:                p.TenantID,
		Slug:                    p.Slug,
		Name:                    strings.TrimSpace(p.Name),
		Description:             p.Description,
		Type:                    p.Type,
		Status:                  p.Status,
		Target:                  p.Target,
		Criteria:                p.Criteria,
		PointsReward:            p.PointsReward,
		XPReward:                p.XPReward,
		BadgeRewardID:           p.BadgeRewardID,
		MaxCompletionsPerPlayer: p.MaxCompletionsPerPlayer,
		StartsAt:                utcPtr(p.StartsAt),
		EndsAt:                  utcPtr(p.EndsAt),
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if m.Type == contracts.TypeOneTime && m.MaxCompletionsPerPlayer == nil {
		one := 1
		m.MaxCompletionsPerPlayer = &one
	}
	if err := m.Validate(); err != nil {
		return Mission{}, err
	}
	return m, nil
}

// Validate checks every invariant; called on create and after each update.
func (m Mission) Validate() error {
	switch {
	case m.Name == "":
		return ErrNameRequired
	case utf8.RuneCountInString(m.Name) > 255:
		return ErrNameTooLong
	case utf8.RuneCountInString(m.Description) > 1000:
		return ErrDescriptionLong
	case len(m.Slug) > 120 || !slugRe.MatchString(m.Slug):
		return ErrBadSlug
	case !ValidType(m.Type):
		return ErrBadType
	case !ValidStatus(m.Status):
		return ErrBadStatus
	case m.Target <= 0:
		return ErrBadTarget
	case m.PointsReward < 0 || m.XPReward < 0:
		return ErrNegativeReward
	case m.MaxCompletionsPerPlayer != nil && *m.MaxCompletionsPerPlayer < 1:
		return ErrBadMaxCompletions
	case m.Type == contracts.TypeOneTime && (m.MaxCompletionsPerPlayer == nil || *m.MaxCompletionsPerPlayer != 1):
		return ErrOneTimeOnce
	case m.StartsAt != nil && m.EndsAt != nil && !m.EndsAt.After(*m.StartsAt):
		return ErrBadWindow
	}
	return nil
}

// Available reports whether progress may be made right now: active and
// inside [starts_at, ends_at] (inclusive bounds, like Laravel's isAvailable).
func (m Mission) Available(now time.Time) bool {
	if m.Status != contracts.MissionActive || m.DeletedAt != nil {
		return false
	}
	if m.StartsAt != nil && now.Before(*m.StartsAt) {
		return false
	}
	if m.EndsAt != nil && now.After(*m.EndsAt) {
		return false
	}
	return true
}

// Due reports whether the expire sweep should expire this mission.
func (m Mission) Due(now time.Time) bool {
	return (m.Status == contracts.MissionActive || m.Status == contracts.MissionPaused) &&
		m.EndsAt != nil && now.After(*m.EndsAt)
}

// transitions is the mission state machine. Expired is normally reached by
// the sweep; archived is terminal.
var transitions = map[string]map[string]bool{
	contracts.MissionDraft:    {contracts.MissionActive: true, contracts.MissionArchived: true},
	contracts.MissionActive:   {contracts.MissionPaused: true, contracts.MissionExpired: true, contracts.MissionArchived: true},
	contracts.MissionPaused:   {contracts.MissionActive: true, contracts.MissionExpired: true, contracts.MissionArchived: true},
	contracts.MissionExpired:  {contracts.MissionArchived: true},
	contracts.MissionArchived: {},
}

// TransitionTo moves the mission to status. Same status is a no-op.
func (m *Mission) TransitionTo(status string, now time.Time) error {
	if !ValidStatus(status) {
		return ErrBadStatus
	}
	if status == m.Status {
		return nil
	}
	if !transitions[m.Status][status] {
		return ErrInvalidTransition
	}
	m.Status = status
	m.UpdatedAt = now
	return nil
}

// ChangeType is allowed only while draft: attempts snapshot nothing about
// the type, so changing it under running attempts would re-period them.
func (m *Mission) ChangeType(typ string) error {
	if typ == m.Type {
		return nil
	}
	if m.Status != contracts.MissionDraft {
		return ErrTypeImmutable
	}
	m.Type = typ
	if typ == contracts.TypeOneTime {
		one := 1
		m.MaxCompletionsPerPlayer = &one
	}
	return nil
}

// CanStart decides whether a new attempt may begin for a player.
// completedTotal counts the player's completed attempts across all
// periods; periodCompleted reports a completed attempt in the current
// period (daily/weekly missions complete at most once per period).
func (m Mission) CanStart(completedTotal int, periodCompleted bool) error {
	if m.MaxCompletionsPerPlayer != nil && completedTotal >= *m.MaxCompletionsPerPlayer {
		return ErrLimitReached
	}
	if periodCompleted && (m.Type == contracts.TypeDaily || m.Type == contracts.TypeWeekly) {
		return ErrLimitReached
	}
	return nil
}

func ValidType(t string) bool {
	switch t {
	case contracts.TypeOneTime, contracts.TypeDaily, contracts.TypeWeekly, contracts.TypeRepeating:
		return true
	}
	return false
}

func ValidStatus(s string) bool {
	_, ok := transitions[s]
	return ok
}

// Slugify lower-cases and dashes a name: "Daily Login!" → "daily-login".
func Slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
