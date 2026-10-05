// Package domain holds progression's entities and invariants: the level
// ladder, a player's progress and the XP grant ledger. A player's level is
// derived from total XP against the tenant's active ladder, so grants
// commute: any delivery order reaches the same level.
package domain

import (
	"sort"
	"strconv"
	"time"

	"levelup/internal/shared/id"
)

// Level is one rung of a tenant's ladder. XPRequired is a cumulative
// threshold that strictly increases with Number.
type Level struct {
	ID            string
	TenantID      string
	Number        int
	Name          string
	Description   string
	XPRequired    int64
	PointsReward  int64
	BadgeRewardID string // "" = none; bare uuid into badges_svc
	Perks         map[string]any
	IconURL       string
	Active        bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// LevelSpec is the full set of caller-supplied level fields.
type LevelSpec struct {
	Number        int
	Name          string
	Description   string
	XPRequired    int64
	PointsReward  int64
	BadgeRewardID string
	Perks         map[string]any
	IconURL       string
	Active        bool
}

// LevelPatch carries partial updates: nil leaves a field untouched. For
// BadgeRewardID a pointer to "" clears the reward (Laravel could not, B20).
type LevelPatch struct {
	Number        *int
	Name          *string
	Description   *string
	XPRequired    *int64
	PointsReward  *int64
	BadgeRewardID *string
	Perks         *map[string]any
	IconURL       *string
	Active        *bool
}

func NewLevel(tenantID string, spec LevelSpec, now time.Time) (Level, error) {
	if tenantID == "" {
		return Level{}, ErrMissingTenant
	}
	l := Level{
		ID:            id.NewID(),
		TenantID:      tenantID,
		Number:        spec.Number,
		Name:          spec.Name,
		Description:   spec.Description,
		XPRequired:    spec.XPRequired,
		PointsReward:  spec.PointsReward,
		BadgeRewardID: spec.BadgeRewardID,
		Perks:         spec.Perks,
		IconURL:       spec.IconURL,
		Active:        spec.Active,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := l.validate(); err != nil {
		return Level{}, err
	}
	return l, nil
}

// Apply merges a patch and re-checks the level's own invariants. Ladder
// ordering is checked separately against the other levels (Ladder.CheckPlacement).
func (l *Level) Apply(p LevelPatch, now time.Time) error {
	next := *l
	if p.Number != nil {
		next.Number = *p.Number
	}
	if p.Name != nil {
		next.Name = *p.Name
	}
	if p.Description != nil {
		next.Description = *p.Description
	}
	if p.XPRequired != nil {
		next.XPRequired = *p.XPRequired
	}
	if p.PointsReward != nil {
		next.PointsReward = *p.PointsReward
	}
	if p.BadgeRewardID != nil {
		next.BadgeRewardID = *p.BadgeRewardID
	}
	if p.Perks != nil {
		next.Perks = *p.Perks
	}
	if p.IconURL != nil {
		next.IconURL = *p.IconURL
	}
	if p.Active != nil {
		next.Active = *p.Active
	}
	if err := next.validate(); err != nil {
		return err
	}
	next.UpdatedAt = now
	*l = next
	return nil
}

func (l *Level) validate() error {
	if l.Number < 1 {
		return ErrInvalidLevelNumber
	}
	if l.XPRequired < 0 {
		return ErrNegativeXPRequired
	}
	if l.PointsReward < 0 {
		return ErrNegativePointsReward
	}
	if l.Name == "" {
		l.Name = "Level " + strconv.Itoa(l.Number)
	}
	return nil
}

// Ladder is a tenant's levels sorted by Number.
type Ladder []Level

// NewLadder copies and sorts levels by Number.
func NewLadder(levels []Level) Ladder {
	out := make(Ladder, len(levels))
	copy(out, levels)
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// Active returns the active levels, still sorted.
func (lad Ladder) Active() Ladder {
	out := make(Ladder, 0, len(lad))
	for _, l := range lad {
		if l.Active {
			out = append(out, l)
		}
	}
	return out
}

// CheckPlacement verifies that c fits the ladder: its number is unused and
// xp_required strictly increases with level_number. lad is every
// non-deleted level of the tenant (active or not); c itself is skipped by id.
func (lad Ladder) CheckPlacement(c Level) error {
	for _, o := range lad {
		if o.ID == c.ID {
			continue
		}
		switch {
		case o.Number == c.Number:
			return ErrLevelNumberTaken
		case o.Number < c.Number && o.XPRequired >= c.XPRequired:
			return ErrXPNotIncreasing
		case o.Number > c.Number && o.XPRequired <= c.XPRequired:
			return ErrXPNotIncreasing
		}
	}
	return nil
}

// LevelFor is the highest active level whose threshold total reaches; nil
// when total is below the first level or the tenant has no ladder.
func (lad Ladder) LevelFor(total int64) *Level {
	var best *Level
	for i := range lad {
		l := lad[i]
		if !l.Active || l.XPRequired > total {
			continue
		}
		if best == nil || l.Number > best.Number {
			best = &lad[i]
		}
	}
	return best
}

// NextAfter is the lowest active level whose threshold is above total.
func (lad Ladder) NextAfter(total int64) *Level {
	var best *Level
	for i := range lad {
		l := lad[i]
		if !l.Active || l.XPRequired <= total {
			continue
		}
		if best == nil || l.Number < best.Number {
			best = &lad[i]
		}
	}
	return best
}

// Crossed returns the active levels whose threshold lies in (from, to],
// ordered by Number: the levels a grant moving total XP from→to reaches.
// Summing grants in any order crosses every threshold exactly once, which
// is what makes level rewards commutative. A level at threshold 0 is the
// starting level and is never crossed, so it carries no reward (doc 04 Q4).
func (lad Ladder) Crossed(from, to int64) []Level {
	var out []Level
	for _, l := range lad {
		if l.Active && l.XPRequired > from && l.XPRequired <= to {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}
