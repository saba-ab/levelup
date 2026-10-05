package domain

import (
	"math"
	"time"

	"levelup/internal/shared/id"
)

// Progress is one player's XP counter and the level derived from it.
// LevelID/LevelNumber are a denormalised placement ("" / 0 when below the
// first level or the tenant has no ladder) refreshed on every grant.
type Progress struct {
	TenantID    string
	PlayerID    string
	TotalXP     int64
	LevelID     string
	LevelNumber int
	// Version is the optimistic-concurrency token behind the row lock.
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewProgress is the zero state of a player who has never gained XP.
func NewProgress(tenantID, playerID string, now time.Time) Progress {
	return Progress{TenantID: tenantID, PlayerID: playerID, CreatedAt: now, UpdatedAt: now}
}

// Gain adds amount to total XP, re-places the player on the active ladder
// and returns the levels whose thresholds this grant crossed, in order.
// It may return several levels (multi-level jump) or none.
func (p *Progress) Gain(amount int64, ladder Ladder, now time.Time) ([]Level, error) {
	if amount <= 0 {
		return nil, ErrNonPositiveAmount
	}
	if p.TotalXP > math.MaxInt64-amount {
		return nil, ErrXPOverflow
	}
	from := p.TotalXP
	p.TotalXP += amount
	p.Place(ladder)
	p.UpdatedAt = now
	return ladder.Crossed(from, p.TotalXP), nil
}

// Place sets the level derived from total XP. It reports whether the
// placement changed.
func (p *Progress) Place(ladder Ladder) bool {
	levelID, number := "", 0
	if l := ladder.LevelFor(p.TotalXP); l != nil {
		levelID, number = l.ID, l.Number
	}
	changed := levelID != p.LevelID || number != p.LevelNumber
	p.LevelID, p.LevelNumber = levelID, number
	return changed
}

// View is the read model of a player's progress against the live ladder.
type View struct {
	PlayerID        string
	TotalXP         int64
	Current         *Level
	Next            *Level
	XPToNext        *int64 // nil at max level or without a ladder
	ProgressPercent float64
}

// ViewOf derives current and next level from total XP (formulas of
// PlayerLevel.php:102-135, with "no ladder" handled instead of a 500).
func ViewOf(playerID string, totalXP int64, ladder Ladder) View {
	v := View{PlayerID: playerID, TotalXP: totalXP}
	v.Current = ladder.LevelFor(totalXP)
	v.Next = ladder.NextAfter(totalXP)
	switch {
	case v.Next == nil && v.Current == nil:
		v.ProgressPercent = 0
	case v.Next == nil:
		v.ProgressPercent = 100
	default:
		base := int64(0)
		if v.Current != nil {
			base = v.Current.XPRequired
		}
		toNext := v.Next.XPRequired - totalXP
		v.XPToNext = &toNext
		span := v.Next.XPRequired - base
		pct := float64(totalXP-base) / float64(span) * 100
		v.ProgressPercent = math.Round(math.Max(0, math.Min(100, pct))*100) / 100
	}
	return v
}

// XPGrant is one immutable XP ledger row. The (tenant, idempotency key)
// pair is unique: a redelivered command inserts nothing.
type XPGrant struct {
	ID             string
	TenantID       string
	PlayerID       string
	IdempotencyKey string
	Amount         int64
	Description    string
	SourceKind     string
	SourceID       string
	ActivityID     string
	OccurredAt     time.Time
	CreatedBy      string
	CreatedAt      time.Time
}

// GrantSpec is everything a caller supplies for a grant.
type GrantSpec struct {
	TenantID       string
	PlayerID       string
	IdempotencyKey string
	Amount         int64
	Description    string
	SourceKind     string
	SourceID       string
	ActivityID     string
	OccurredAt     time.Time
	CreatedBy      string
}

func NewXPGrant(s GrantSpec, now time.Time) (XPGrant, error) {
	switch {
	case s.TenantID == "":
		return XPGrant{}, ErrMissingTenant
	case s.PlayerID == "":
		return XPGrant{}, ErrMissingPlayer
	case s.IdempotencyKey == "":
		return XPGrant{}, ErrMissingIdempotencyKey
	case s.Amount <= 0:
		return XPGrant{}, ErrNonPositiveAmount
	}
	occurred := s.OccurredAt
	if occurred.IsZero() {
		occurred = now
	}
	return XPGrant{
		ID:             id.NewID(),
		TenantID:       s.TenantID,
		PlayerID:       s.PlayerID,
		IdempotencyKey: s.IdempotencyKey,
		Amount:         s.Amount,
		Description:    s.Description,
		SourceKind:     s.SourceKind,
		SourceID:       s.SourceID,
		ActivityID:     s.ActivityID,
		OccurredAt:     occurred,
		CreatedBy:      s.CreatedBy,
		CreatedAt:      now,
	}, nil
}

// LevelReward records that a player reached a level, once per
// (tenant, player, level) ever: it is what makes level rewards exactly-once.
type LevelReward struct {
	ID        string
	TenantID  string
	PlayerID  string
	LevelID   string
	GrantedAt time.Time
}

func NewLevelReward(tenantID, playerID, levelID string, now time.Time) LevelReward {
	return LevelReward{ID: id.NewID(), TenantID: tenantID, PlayerID: playerID, LevelID: levelID, GrantedAt: now}
}

// GrantRejection records a rejected grant command so a redelivery does not
// publish progression.grant_rejected.v1 twice.
type GrantRejection struct {
	TenantID       string
	IdempotencyKey string
	PlayerID       string
	Reason         string
	CreatedAt      time.Time
}
