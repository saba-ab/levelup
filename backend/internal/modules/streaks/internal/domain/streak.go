// Package domain holds streaks' entities and invariants. No framework tags.
package domain

import (
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"levelup/internal/shared/id"
)

const (
	maxGracePeriods = 30
	maxMilestones   = 50
)

var (
	slugRe        = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,99}$`)
	activityKeyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,99}$`)
	nonSlugChars  = regexp.MustCompile(`[^a-z0-9]+`)
)

// Milestone pays BonusPoints once per run when the run reaches Count periods.
type Milestone struct {
	Count       int
	BonusPoints int64
}

// Streak is a tenant's streak definition.
type Streak struct {
	ID              string
	TenantID        string
	Slug            string
	Name            string
	Description     string
	ActivityKey     string // trigger slug used by rules (record_streak) and record commands
	Period          Period // immutable after creation: buckets already recorded depend on it
	GracePeriods    int    // missed periods tolerated between two buckets of one run
	PointsPerPeriod int64  // paid once per NEW bucket (Laravel paid on every call, S1)
	Milestones      []Milestone
	Active          bool
	// AutoRecord: activity.received.v1 whose event_type equals ActivityKey
	// records a period automatically. Tenants opt out per streak.
	AutoRecord bool
	Version    int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewStreakInput carries creation fields; Slug may be empty (derived from Name).
type NewStreakInput struct {
	Slug            string
	Name            string
	Description     string
	ActivityKey     string
	Period          string
	GracePeriods    int
	PointsPerPeriod int64
	Milestones      []Milestone
	Active          bool
	AutoRecord      *bool // nil = true
}

func NewStreak(tenantID string, in NewStreakInput, now time.Time) (Streak, error) {
	if tenantID == "" {
		return Streak{}, ErrNoTenant
	}
	period, err := ParsePeriod(in.Period)
	if err != nil {
		return Streak{}, err
	}
	s := Streak{
		ID:              id.NewID(),
		TenantID:        tenantID,
		Slug:            strings.TrimSpace(in.Slug),
		Name:            strings.TrimSpace(in.Name),
		Description:     in.Description,
		ActivityKey:     strings.TrimSpace(in.ActivityKey),
		Period:          period,
		GracePeriods:    in.GracePeriods,
		PointsPerPeriod: in.PointsPerPeriod,
		Milestones:      normaliseMilestones(in.Milestones),
		Active:          in.Active,
		AutoRecord:      in.AutoRecord == nil || *in.AutoRecord,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if s.Slug == "" {
		s.Slug = Slugify(s.Name)
	}
	if err := s.validate(); err != nil {
		return Streak{}, err
	}
	return s, nil
}

// StreakPatch is a partial update: nil fields stay untouched.
type StreakPatch struct {
	Slug            *string
	Name            *string
	Description     *string
	ActivityKey     *string
	GracePeriods    *int
	PointsPerPeriod *int64
	Milestones      *[]Milestone
	Active          *bool
	AutoRecord      *bool
}

func (s *Streak) Apply(p StreakPatch, now time.Time) error {
	next := *s
	if p.Slug != nil {
		next.Slug = strings.TrimSpace(*p.Slug)
	}
	if p.Name != nil {
		next.Name = strings.TrimSpace(*p.Name)
	}
	if p.Description != nil {
		next.Description = *p.Description
	}
	if p.ActivityKey != nil {
		next.ActivityKey = strings.TrimSpace(*p.ActivityKey)
	}
	if p.GracePeriods != nil {
		next.GracePeriods = *p.GracePeriods
	}
	if p.PointsPerPeriod != nil {
		next.PointsPerPeriod = *p.PointsPerPeriod
	}
	if p.Milestones != nil {
		next.Milestones = normaliseMilestones(*p.Milestones)
	}
	if p.Active != nil {
		next.Active = *p.Active
	}
	if p.AutoRecord != nil {
		next.AutoRecord = *p.AutoRecord
	}
	if err := next.validate(); err != nil {
		return err
	}
	next.UpdatedAt = now
	*s = next
	return nil
}

func (s Streak) validate() error {
	switch {
	case s.Name == "":
		return ErrNameRequired
	case utf8.RuneCountInString(s.Name) > 255:
		return ErrNameTooLong
	case utf8.RuneCountInString(s.Description) > 1000:
		return ErrDescriptionTooLong
	case !slugRe.MatchString(s.Slug):
		return ErrBadSlug
	case !activityKeyRe.MatchString(s.ActivityKey):
		return ErrBadActivityKey
	case s.GracePeriods < 0 || s.GracePeriods > maxGracePeriods:
		return ErrBadGracePeriods
	case s.PointsPerPeriod < 0:
		return ErrNegativePoints
	case len(s.Milestones) > maxMilestones:
		return ErrTooManyMilestones
	}
	if _, err := ParsePeriod(string(s.Period)); err != nil {
		return err
	}
	for i, m := range s.Milestones {
		if m.Count < 1 {
			return ErrBadMilestoneCount
		}
		if m.BonusPoints < 0 {
			return ErrNegativeBonus
		}
		if i > 0 && s.Milestones[i-1].Count == m.Count {
			return ErrDuplicateMilestone
		}
	}
	return nil
}

// MilestonesUpTo returns the milestones a run of the given length has reached.
func (s Streak) MilestonesUpTo(length int) []Milestone {
	var out []Milestone
	for _, m := range s.Milestones {
		if m.Count <= length {
			out = append(out, m)
		}
	}
	return out
}

func normaliseMilestones(in []Milestone) []Milestone {
	out := slices.Clone(in)
	slices.SortStableFunc(out, func(a, b Milestone) int { return a.Count - b.Count })
	if out == nil {
		out = []Milestone{}
	}
	return out
}

// Slugify derives a slug from a name: lower-case, runs of anything else
// collapsed to '-'.
func Slugify(name string) string {
	s := nonSlugChars.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if len(s) > 100 {
		s = strings.TrimRight(s[:100], "-")
	}
	return s
}
