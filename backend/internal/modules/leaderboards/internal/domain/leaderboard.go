// Package domain holds leaderboards' entities and invariants: board
// definitions, period bucketing and how a fact contributes to a score.
package domain

import (
	"regexp"
	"strings"
	"time"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/shared/id"
)

const (
	DefaultMaxEntries = 100
	MaxMaxEntries     = 1000
)

// Leaderboard is a tenant's board definition. Type, metric, reset frequency
// and program scope are fixed at creation: changing them would silently
// reinterpret every score already accumulated.
type Leaderboard struct {
	ID             string
	TenantID       string
	Slug           string
	Name           string
	Description    string
	Type           string
	Metric         string
	ResetFrequency string
	ProgramID      string // empty = every player of the tenant
	// Activity is the config of a type "activity" board; nil otherwise.
	Activity   *ActivityConfig
	MaxEntries int
	Active     bool
	Version    int
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  *time.Time
}

// NewLeaderboardInput is what a creator supplies; tenant comes from the
// principal, never from the request body.
type NewLeaderboardInput struct {
	TenantID       string
	Name           string
	Slug           string
	Description    string
	Type           string
	Metric         string
	ResetFrequency string
	ProgramID      string
	// Activity is required for type activity and refused for every other.
	Activity   *ActivityConfig
	MaxEntries int
	Active     bool
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify derives a slug from a name ("Weekly Points Race" → "weekly-points-race").
func Slugify(name string) string {
	s := nonSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	s = strings.Trim(s, "-")
	if len(s) > 120 {
		s = strings.Trim(s[:120], "-")
	}
	return s
}

// DefaultMetric is the metric a board gets when the creator names none.
func DefaultMetric(typ string) string {
	switch typ {
	case contracts.TypeBadges, contracts.TypeMissions:
		return contracts.MetricCount
	default:
		return contracts.MetricEarned
	}
}

// validMetric is the type × metric matrix.
func validMetric(typ, metric string) bool {
	switch typ {
	case contracts.TypePoints:
		return metric == contracts.MetricEarned || metric == contracts.MetricNet || metric == contracts.MetricBalance
	case contracts.TypeXP:
		return metric == contracts.MetricEarned || metric == contracts.MetricBalance
	case contracts.TypeBadges, contracts.TypeMissions:
		return metric == contracts.MetricCount
	case contracts.TypeActivity:
		return metric == contracts.MetricCount || metric == contracts.MetricEarned
	}
	return false
}

func validType(typ string) bool {
	switch typ {
	case contracts.TypePoints, contracts.TypeBadges, contracts.TypeMissions, contracts.TypeXP, contracts.TypeActivity:
		return true
	}
	return false
}

func validReset(r string) bool {
	switch r {
	case contracts.ResetNever, contracts.ResetDaily, contracts.ResetWeekly, contracts.ResetMonthly:
		return true
	}
	return false
}

// NewLeaderboard validates and normalises a definition.
func NewLeaderboard(in NewLeaderboardInput, now time.Time) (Leaderboard, error) {
	if in.TenantID == "" {
		return Leaderboard{}, ErrTenantRequired
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Leaderboard{}, ErrNameRequired
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = Slugify(name)
	}
	if !slugRe.MatchString(slug) {
		return Leaderboard{}, ErrSlugInvalid
	}
	if !validType(in.Type) {
		return Leaderboard{}, ErrInvalidType
	}
	metric := in.Metric
	var activity *ActivityConfig
	if in.Type == contracts.TypeActivity {
		cfg, m, err := normaliseActivity(in.Activity, metric)
		if err != nil {
			return Leaderboard{}, err
		}
		activity, metric = &cfg, m
	} else if in.Activity != nil {
		return Leaderboard{}, ErrConfigNotAllowed
	}
	if metric == "" {
		metric = DefaultMetric(in.Type)
	}
	if !validMetric(in.Type, metric) {
		return Leaderboard{}, ErrInvalidMetric
	}
	reset := in.ResetFrequency
	if reset == "" {
		reset = contracts.ResetNever
	}
	if !validReset(reset) {
		return Leaderboard{}, ErrInvalidReset
	}
	if metric == contracts.MetricBalance && reset != contracts.ResetNever {
		return Leaderboard{}, ErrBalancePeriodic
	}
	maxEntries := in.MaxEntries
	if maxEntries == 0 {
		maxEntries = DefaultMaxEntries
	}
	if maxEntries < 1 || maxEntries > MaxMaxEntries {
		return Leaderboard{}, ErrMaxEntries
	}
	return Leaderboard{
		ID:             id.NewID(),
		TenantID:       in.TenantID,
		Slug:           slug,
		Name:           name,
		Description:    strings.TrimSpace(in.Description),
		Type:           in.Type,
		Metric:         metric,
		ResetFrequency: reset,
		ProgramID:      in.ProgramID,
		Activity:       activity,
		MaxEntries:     maxEntries,
		Active:         in.Active,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// Patch is a partial update: nil fields stay untouched (fixes the Laravel
// update that mass-assigned the raw request, L3).
type Patch struct {
	Name        *string
	Slug        *string
	Description *string
	MaxEntries  *int
	Active      *bool
}

// Apply validates and applies a patch.
func (l *Leaderboard) Apply(p Patch, now time.Time) error {
	next := *l
	if p.Name != nil {
		n := strings.TrimSpace(*p.Name)
		if n == "" {
			return ErrNameRequired
		}
		next.Name = n
	}
	if p.Slug != nil {
		s := strings.TrimSpace(*p.Slug)
		if !slugRe.MatchString(s) {
			return ErrSlugInvalid
		}
		next.Slug = s
	}
	if p.Description != nil {
		next.Description = strings.TrimSpace(*p.Description)
	}
	if p.MaxEntries != nil {
		if *p.MaxEntries < 1 || *p.MaxEntries > MaxMaxEntries {
			return ErrMaxEntries
		}
		next.MaxEntries = *p.MaxEntries
	}
	if p.Active != nil {
		next.Active = *p.Active
	}
	next.UpdatedAt = now
	*l = next
	return nil
}

// Periodic reports whether the board resets.
func (l Leaderboard) Periodic() bool { return l.ResetFrequency != contracts.ResetNever }
