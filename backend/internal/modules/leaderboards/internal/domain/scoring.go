package domain

import (
	"time"

	"levelup/internal/modules/leaderboards/contracts"
)

// FactKind names the upstream facts that move scores.
type FactKind string

const (
	FactPointsCredited   FactKind = "points.credited"
	FactPointsDebited    FactKind = "points.debited"
	FactPointsRefunded   FactKind = "points.refunded"
	FactBadgeAwarded     FactKind = "badges.awarded"
	FactMissionCompleted FactKind = "missions.completed"
	FactXPGained         FactKind = "progression.xp_gained"
	FactActivity         FactKind = "activity.received"
)

// Fact is leaderboards' own projection of an upstream event.
type Fact struct {
	EventID  string
	TenantID string
	PlayerID string
	Kind     FactKind
	Amount   int64     // positive; the kind gives the direction
	Absolute int64     // balance_after / total_xp, for balance metrics
	At       time.Time // when it happened; decides the period bucket
	// EventType and Properties are set for FactActivity only.
	EventType  string
	Properties map[string]any
	// PlayerExternalID and AutoCreatePlayer let an activity whose player
	// ingest could not resolve wait for the auto-created player.
	PlayerExternalID string
	AutoCreatePlayer bool
}

// BoardType is the leaderboard type a fact kind feeds.
func (k FactKind) BoardType() string {
	switch k {
	case FactPointsCredited, FactPointsDebited, FactPointsRefunded:
		return contracts.TypePoints
	case FactBadgeAwarded:
		return contracts.TypeBadges
	case FactMissionCompleted:
		return contracts.TypeMissions
	case FactXPGained:
		return contracts.TypeXP
	case FactActivity:
		return contracts.TypeActivity
	}
	return ""
}

// OpKind is how a contribution touches a score.
type OpKind int

const (
	// OpIncrement adds Value to the score. Increments commute, so delivery
	// order does not matter; idempotency comes from applied_events.
	OpIncrement OpKind = iota + 1
	// OpSet replaces the score with Value, but only when the fact is newer
	// than the last one applied (last-writer-wins on the fact's time).
	OpSet
)

// Op is one fact's effect on one board.
type Op struct {
	Kind  OpKind
	Value int64
}

// Contribution returns the effect of fact f on board b, or ok=false when the
// fact does not move this board.
func Contribution(b Leaderboard, f Fact) (Op, bool) {
	if f.Kind.BoardType() != b.Type {
		return Op{}, false
	}
	if b.Type == contracts.TypeActivity {
		return activityContribution(b, f)
	}
	switch b.Metric {
	case contracts.MetricEarned:
		switch f.Kind {
		case FactPointsCredited, FactXPGained:
			if f.Amount <= 0 {
				return Op{}, false
			}
			return Op{Kind: OpIncrement, Value: f.Amount}, true
		}
	case contracts.MetricNet:
		if f.Amount <= 0 {
			return Op{}, false
		}
		switch f.Kind {
		case FactPointsCredited, FactPointsRefunded:
			return Op{Kind: OpIncrement, Value: f.Amount}, true
		case FactPointsDebited:
			return Op{Kind: OpIncrement, Value: -f.Amount}, true
		}
	case contracts.MetricBalance:
		switch f.Kind {
		case FactPointsCredited, FactPointsDebited, FactPointsRefunded, FactXPGained:
			return Op{Kind: OpSet, Value: f.Absolute}, true
		}
	case contracts.MetricCount:
		switch f.Kind {
		case FactBadgeAwarded, FactMissionCompleted:
			return Op{Kind: OpIncrement, Value: 1}, true
		}
	}
	return Op{}, false
}

// Standing is one row of a ranking: competition rank (1,2,2,4) ordered by
// score desc, then player id asc for a deterministic list order.
type Standing struct {
	PlayerID string
	Score    int64
	Rank     int64
	Position int64 // 0-based list position
}

// AssignRanks fills Rank for consecutive standings starting at list position
// start, given the rank of the first row. Equal scores share a rank.
func AssignRanks(rows []Standing, start, firstRank int64) {
	for i := range rows {
		rows[i].Position = start + int64(i)
		switch {
		case i == 0:
			rows[i].Rank = firstRank
		case rows[i].Score == rows[i-1].Score:
			rows[i].Rank = rows[i-1].Rank
		default:
			rows[i].Rank = start + int64(i) + 1
		}
	}
}
