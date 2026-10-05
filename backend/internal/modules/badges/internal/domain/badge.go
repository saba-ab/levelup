// Package domain holds badges' entities and invariants. No framework tags.
package domain

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"levelup/internal/modules/badges/contracts"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/id"
)

// Tier is a badge rarity tier (Laravel BadgeTier).
type Tier string

const (
	TierBronze   Tier = contracts.TierBronze
	TierSilver   Tier = contracts.TierSilver
	TierGold     Tier = contracts.TierGold
	TierPlatinum Tier = contracts.TierPlatinum
	TierDiamond  Tier = contracts.TierDiamond
)

var tierRank = map[Tier]int{TierBronze: 1, TierSilver: 2, TierGold: 3, TierPlatinum: 4, TierDiamond: 5}

var tierDefaultPoints = map[Tier]int64{TierBronze: 10, TierSilver: 25, TierGold: 50, TierPlatinum: 100, TierDiamond: 250}

// Valid reports whether t is one of the five known tiers.
func (t Tier) Valid() bool { _, ok := tierRank[t]; return ok }

// Rank orders tiers: bronze 1 … diamond 5.
func (t Tier) Rank() int { return tierRank[t] }

// DefaultPoints is the points_value used when a create omits it (parity
// with CreateBadgeAction).
func (t Tier) DefaultPoints() int64 { return tierDefaultPoints[t] }

// Category is a closed, purely presentational set (Laravel BadgeCategory).
type Category string

var categories = map[Category]bool{
	contracts.CategoryAchievement: true,
	contracts.CategoryMilestone:   true,
	contracts.CategorySkill:       true,
	contracts.CategorySocial:      true,
	contracts.CategoryExploration: true,
	contracts.CategoryCollection:  true,
	contracts.CategorySpecial:     true,
	contracts.CategorySeasonal:    true,
}

// Valid reports whether c is a known category.
func (c Category) Valid() bool { return categories[c] }

// Badge is a tenant's badge definition.
//
// Requirements is stored and returned verbatim but NEVER evaluated: rules
// (and missions, rewards, progression) decide awards and issue
// job.badges.award. This matches Laravel, where the column was opaque.
type Badge struct {
	ID           string
	TenantID     string
	Slug         string
	Name         string
	Description  string
	IconURL      string
	Tier         Tier
	Category     Category
	PointsValue  int64
	Stackable    bool
	MaxAwards    *int // nil = unlimited; only meaningful for stackable badges
	Requirements map[string]any
	Active       bool
	Secret       bool
	SortOrder    int
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

// NewBadgeParams carries a create request after shape validation.
type NewBadgeParams struct {
	TenantID     string
	Name         string
	Slug         string // optional; derived from Name when empty
	Description  string
	IconURL      string
	Tier         string
	Category     string
	PointsValue  *int64 // nil → tier default
	Stackable    bool
	MaxAwards    *int
	Requirements map[string]any
	Active       bool
	Secret       bool
	SortOrder    int
}

// NewBadge builds a valid badge. The slug is stable: renaming never
// regenerates it (fixes Laravel B18).
func NewBadge(p NewBadgeParams, now time.Time) (Badge, error) {
	b := Badge{
		ID:           id.NewID(),
		TenantID:     p.TenantID,
		Name:         strings.TrimSpace(p.Name),
		Slug:         p.Slug,
		Description:  p.Description,
		IconURL:      p.IconURL,
		Tier:         Tier(p.Tier),
		Category:     Category(p.Category),
		Stackable:    p.Stackable,
		MaxAwards:    p.MaxAwards,
		Requirements: p.Requirements,
		Active:       p.Active,
		Secret:       p.Secret,
		SortOrder:    p.SortOrder,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if b.Slug == "" {
		b.Slug = Slugify(b.Name)
	}
	if !b.Tier.Valid() {
		return Badge{}, ErrInvalidTier
	}
	if p.PointsValue != nil {
		b.PointsValue = *p.PointsValue
	} else {
		b.PointsValue = b.Tier.DefaultPoints()
	}
	if err := b.validate(); err != nil {
		return Badge{}, err
	}
	return b, nil
}

// Patch is a partial update: nil pointers leave a field untouched. The
// *Set flags distinguish "absent" from "explicit null" for nullable fields
// (fixes Laravel B20: null meant unchanged, so nothing could be cleared).
type Patch struct {
	Name            *string
	Slug            *string
	Description     *string
	IconURL         *string
	Tier            *string
	Category        *string
	PointsValue     *int64
	Stackable       *bool
	MaxAwardsSet    bool
	MaxAwards       *int
	RequirementsSet bool
	Requirements    map[string]any
	Active          *bool
	Secret          *bool
	SortOrder       *int
}

// Apply mutates b with p and re-checks every invariant.
func (b *Badge) Apply(p Patch, now time.Time) error {
	next := *b
	if p.Name != nil {
		next.Name = strings.TrimSpace(*p.Name)
	}
	if p.Slug != nil {
		next.Slug = *p.Slug
	}
	if p.Description != nil {
		next.Description = *p.Description
	}
	if p.IconURL != nil {
		next.IconURL = *p.IconURL
	}
	if p.Tier != nil {
		next.Tier = Tier(*p.Tier)
	}
	if p.Category != nil {
		next.Category = Category(*p.Category)
	}
	if p.PointsValue != nil {
		next.PointsValue = *p.PointsValue
	}
	if p.Stackable != nil {
		next.Stackable = *p.Stackable
	}
	if p.MaxAwardsSet {
		next.MaxAwards = p.MaxAwards
	}
	if p.RequirementsSet {
		next.Requirements = p.Requirements
	}
	if p.Active != nil {
		next.Active = *p.Active
	}
	if p.Secret != nil {
		next.Secret = *p.Secret
	}
	if p.SortOrder != nil {
		next.SortOrder = *p.SortOrder
	}
	if err := next.validate(); err != nil {
		return err
	}
	next.UpdatedAt = now
	*b = next
	return nil
}

func (b Badge) validate() error {
	if b.Name == "" {
		return ErrNameRequired
	}
	if !slugPattern.MatchString(b.Slug) {
		return ErrInvalidSlug
	}
	if !b.Tier.Valid() {
		return ErrInvalidTier
	}
	if !b.Category.Valid() {
		return ErrInvalidCategory
	}
	if b.PointsValue < 0 {
		return ErrNegativePoints
	}
	if b.MaxAwards != nil && *b.MaxAwards < 1 {
		return ErrInvalidMaxAwards
	}
	return nil
}

// Deleted reports whether the badge was soft-deleted.
func (b Badge) Deleted() bool { return b.DeletedAt != nil }

// CanAward decides whether one more award is allowed given the player's
// current earned count (0 = never earned). It returns "" when allowed,
// otherwise an effect.Reason* — a business RESULT, never an error, so a
// non-stackable badge no longer makes later rule runs fail forever (fixes
// Laravel's exception-based flow that rolled back whole rule batches).
//
// max_awards applies to stackable badges only; a non-stackable badge is
// capped at one award by the "already earned" rule.
func (b Badge) CanAward(earnedCount int) string {
	switch {
	case !b.Active || b.Deleted():
		return effect.ReasonTargetInactive
	case earnedCount == 0:
		return ""
	case !b.Stackable:
		return effect.ReasonAlreadyEarned
	case b.MaxAwards != nil && earnedCount >= *b.MaxAwards:
		return effect.ReasonLimitReached
	default:
		return ""
	}
}

var (
	slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	dashRun     = regexp.MustCompile(`-+`)
)

// Slugify lower-cases name and replaces every run of non-alphanumerics with a
// single dash. Non-ASCII letters are dropped; an all-symbol name yields
// "badge" so the slug invariant still holds.
func Slugify(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			sb.WriteRune(r)
			continue
		}
		sb.WriteRune('-')
	}
	s := strings.Trim(dashRun.ReplaceAllString(sb.String(), "-"), "-")
	if s == "" {
		return "badge"
	}
	if len(s) > 200 {
		s = strings.TrimRight(s[:200], "-")
	}
	return s
}
