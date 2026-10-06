// Package domain holds rewards' entities and invariants: the catalogue item
// with its stock counter, and the claim state machine of the claim saga.
package domain

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/shared/id"
)

var (
	slugPattern  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	valuePattern = regexp.MustCompile(`^\d{1,8}(\.\d{1,2})?$`)
	nonSlug      = regexp.MustCompile(`[^a-z0-9]+`)
)

const (
	maxNameLen = 255
	maxDescLen = 1000
	maxSlugLen = 120
)

// Reward is a catalogue item a player can claim. StockUsed counts every
// claim that holds a unit (pending_payment, claimed, redeemed): it is the
// counter mutated under the reward's row lock, which is what makes the last
// unit impossible to oversell (doc 05 R1/R2).
type Reward struct {
	ID               string
	TenantID         string
	Name             string
	Slug             string
	Description      string
	Type             string
	Status           string
	PointsCost       int64
	Value            *string // decimal(10,2) as text
	ValueType        *string
	BadgeRewardID    *string // bare id in badges
	LevelRewardID    *string // bare id in progression
	MaxRedemptions   *int    // global stock; nil = unlimited
	MaxPerPlayer     *int    // nil = unlimited
	StockUsed        int
	ClaimTTLDays     *int // nil = claims never expire
	StartAt          *time.Time
	EndAt            *time.Time
	LevelRequirement *int
	IsActive         bool
	Metadata         map[string]any
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// RewardPatch is a partial update: nil leaves a field untouched. For the
// nullable fields an empty string (or 0 for numbers) clears the value.
type RewardPatch struct {
	Name             *string
	Slug             *string
	Description      *string
	Type             *string
	Status           *string
	PointsCost       *int64
	Value            *string
	ValueType        *string
	BadgeRewardID    *string
	LevelRewardID    *string
	MaxRedemptions   *int
	MaxPerPlayer     *int
	ClaimTTLDays     *int
	StartAt          *time.Time
	EndAt            *time.Time
	LevelRequirement *int
	IsActive         *bool
	Metadata         map[string]any
}

// NewReward builds a validated reward. Defaults: type points, status draft,
// active flag true (callers pass IsActive explicitly).
func NewReward(tenantID string, p RewardPatch, now time.Time) (Reward, error) {
	if tenantID == "" {
		return Reward{}, ErrNoTenant
	}
	r := Reward{
		ID:        id.NewID(),
		TenantID:  tenantID,
		Type:      contracts.TypePoints,
		Status:    contracts.RewardDraft,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if p.Slug == nil || *p.Slug == "" {
		name := ""
		if p.Name != nil {
			name = *p.Name
		}
		s := Slugify(name)
		p.Slug = &s
	}
	if err := r.Apply(p, now); err != nil {
		return Reward{}, err
	}
	return r, nil
}

// Apply merges a patch and re-validates every invariant.
func (r *Reward) Apply(p RewardPatch, now time.Time) error {
	if p.Name != nil {
		r.Name = strings.TrimSpace(*p.Name)
	}
	if p.Slug != nil {
		r.Slug = *p.Slug
	}
	if p.Description != nil {
		r.Description = *p.Description
	}
	if p.Type != nil {
		r.Type = *p.Type
	}
	if p.Status != nil {
		r.Status = *p.Status
	}
	if p.PointsCost != nil {
		r.PointsCost = *p.PointsCost
	}
	if p.Value != nil {
		r.Value = optString(*p.Value)
	}
	if p.ValueType != nil {
		r.ValueType = optString(*p.ValueType)
	}
	if p.BadgeRewardID != nil {
		r.BadgeRewardID = optString(*p.BadgeRewardID)
	}
	if p.LevelRewardID != nil {
		r.LevelRewardID = optString(*p.LevelRewardID)
	}
	if p.MaxRedemptions != nil {
		r.MaxRedemptions = optInt(*p.MaxRedemptions)
	}
	if p.MaxPerPlayer != nil {
		r.MaxPerPlayer = optInt(*p.MaxPerPlayer)
	}
	if p.ClaimTTLDays != nil {
		r.ClaimTTLDays = optInt(*p.ClaimTTLDays)
	}
	if p.LevelRequirement != nil {
		r.LevelRequirement = optInt(*p.LevelRequirement)
	}
	if p.StartAt != nil {
		t := p.StartAt.UTC()
		r.StartAt = &t
	}
	if p.EndAt != nil {
		t := p.EndAt.UTC()
		r.EndAt = &t
	}
	if p.IsActive != nil {
		r.IsActive = *p.IsActive
	}
	if p.Metadata != nil {
		r.Metadata = p.Metadata
	}
	r.UpdatedAt = now
	return r.validate()
}

func (r *Reward) validate() error {
	switch {
	case r.Name == "":
		return ErrNameRequired
	case utf8.RuneCountInString(r.Name) > maxNameLen:
		return ErrNameTooLong
	case utf8.RuneCountInString(r.Description) > maxDescLen:
		return ErrDescriptionLong
	case len(r.Slug) > maxSlugLen || !slugPattern.MatchString(r.Slug):
		return ErrBadSlug
	case !ValidType(r.Type):
		return ErrBadType
	case !ValidRewardStatus(r.Status):
		return ErrBadStatus
	case r.PointsCost < 0:
		return ErrBadPointsCost
	case r.Value != nil && !valuePattern.MatchString(*r.Value):
		return ErrBadValue
	case r.ValueType != nil && *r.ValueType != "percentage" && *r.ValueType != "fixed":
		return ErrBadValueType
	case r.Value != nil && r.creditsValue() && !wholePositive(*r.Value):
		return ErrBadRewardValue
	case belowOne(r.MaxRedemptions), belowOne(r.MaxPerPlayer), belowOne(r.ClaimTTLDays), belowOne(r.LevelRequirement):
		return ErrBadLimit
	case r.StartAt != nil && r.EndAt != nil && r.EndAt.Before(*r.StartAt):
		return ErrBadWindow
	case r.MaxRedemptions != nil && *r.MaxRedemptions < r.StockUsed:
		return ErrStockBelowUsed
	}
	return nil
}

// CheckClaimable reports whether a new claim may start now. Depletion is
// reported separately so clients can tell "sold out" from "not on sale".
func (r *Reward) CheckClaimable(now time.Time) error {
	if r.Status == contracts.RewardDepleted || r.soldOut() {
		return ErrRewardDepleted
	}
	if !r.IsActive || r.Status != contracts.RewardActive {
		return ErrRewardNotAvailable
	}
	if r.StartAt != nil && now.Before(*r.StartAt) {
		return ErrRewardNotAvailable
	}
	if r.EndAt != nil && !now.Before(*r.EndAt) {
		return ErrRewardNotAvailable
	}
	return nil
}

// CheckPlayerLimit enforces max_per_player over the player's claims that
// hold a unit (pending_payment, claimed, redeemed).
func (r *Reward) CheckPlayerLimit(heldByPlayer int) error {
	if r.MaxPerPlayer != nil && heldByPlayer >= *r.MaxPerPlayer {
		return ErrPlayerLimitReached
	}
	return nil
}

// CheckLevel enforces level_requirement against the player's level.
func (r *Reward) CheckLevel(level int) error {
	if r.LevelRequirement != nil && level < *r.LevelRequirement {
		return ErrLevelTooLow
	}
	return nil
}

// HoldStock takes one unit. The caller holds the row lock. Reaching the cap
// flips the status to depleted.
func (r *Reward) HoldStock(now time.Time) error {
	if r.soldOut() {
		return ErrRewardDepleted
	}
	r.StockUsed++
	if r.soldOut() && r.Status == contracts.RewardActive {
		r.Status = contracts.RewardDepleted
	}
	r.UpdatedAt = now
	return nil
}

// ReleaseStock returns one unit (rejected or cancelled claim). A depleted
// reward with free stock again becomes active.
func (r *Reward) ReleaseStock(now time.Time) {
	if r.StockUsed > 0 {
		r.StockUsed--
	}
	if r.Status == contracts.RewardDepleted && !r.soldOut() {
		r.Status = contracts.RewardActive
	}
	r.UpdatedAt = now
}

// NeedsCode reports whether claims of this reward carry a voucher code.
func (r *Reward) NeedsCode() bool {
	return r.Type == contracts.TypeDiscount || r.Type == contracts.TypeItem
}

// ClaimExpiry is when a claim settled at claimedAt stops being redeemable.
func (r *Reward) ClaimExpiry(claimedAt time.Time) *time.Time {
	if r.ClaimTTLDays == nil {
		return nil
	}
	t := claimedAt.AddDate(0, 0, *r.ClaimTTLDays)
	return &t
}

// ExpireIfEnded flips an active reward past end_at to expired.
func (r *Reward) ExpireIfEnded(now time.Time) bool {
	if r.EndAt == nil || now.Before(*r.EndAt) {
		return false
	}
	if r.Status != contracts.RewardActive && r.Status != contracts.RewardDepleted && r.Status != contracts.RewardPaused {
		return false
	}
	r.Status = contracts.RewardExpired
	r.UpdatedAt = now
	return true
}

// FulfilsByCommand reports whether a claim of this reward is delivered by
// a command to another module (points credit, badge award, XP grant). The
// other types (discount, item, custom) are fulfilled by their voucher code.
func (r *Reward) FulfilsByCommand() bool {
	switch r.Type {
	case contracts.TypePoints, contracts.TypeBadge, contracts.TypeLevel:
		return true
	}
	return false
}

// WholeValue is the reward's value as a positive whole number: the points
// a points reward credits, the XP a level reward grants. ok=false when the
// value is unset or not a positive integer.
func (r *Reward) WholeValue() (int64, bool) {
	if r.Value == nil || !wholePositive(*r.Value) {
		return 0, false
	}
	whole, _, _ := strings.Cut(*r.Value, ".")
	n, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// creditsValue reports whether the value is an amount credited on
// fulfilment (points or XP), which must then be a positive integer.
func (r *Reward) creditsValue() bool {
	return r.Type == contracts.TypePoints || r.Type == contracts.TypeLevel
}

// wholePositive accepts "100" and "100.00" (numeric(10,2) reads back with
// decimals), never "0", "1.5" or "-3".
func wholePositive(v string) bool {
	if !valuePattern.MatchString(v) {
		return false
	}
	whole, frac, _ := strings.Cut(v, ".")
	if strings.Trim(frac, "0") != "" {
		return false
	}
	return strings.Trim(whole, "0") != ""
}

func (r *Reward) soldOut() bool {
	return r.MaxRedemptions != nil && r.StockUsed >= *r.MaxRedemptions
}

// Slugify derives a slug from a name; "reward" when nothing usable is left.
func Slugify(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > maxSlugLen {
		s = strings.Trim(s[:maxSlugLen], "-")
	}
	if s == "" {
		return "reward"
	}
	return s
}

// ValidType reports whether t is a known reward type.
func ValidType(t string) bool {
	switch t {
	case contracts.TypePoints, contracts.TypeDiscount, contracts.TypeItem,
		contracts.TypeBadge, contracts.TypeLevel, contracts.TypeCustom:
		return true
	}
	return false
}

// ValidRewardStatus reports whether s is a known reward status.
func ValidRewardStatus(s string) bool {
	switch s {
	case contracts.RewardDraft, contracts.RewardActive, contracts.RewardPaused,
		contracts.RewardExpired, contracts.RewardDepleted:
		return true
	}
	return false
}

func belowOne(v *int) bool { return v != nil && *v < 1 }

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}
