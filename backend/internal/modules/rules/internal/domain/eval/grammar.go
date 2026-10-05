// Package eval is the rules engine's pure evaluator (doc 06 §11.5).
//
// Compile turns a rule's raw JSON (conditions, actions, limits) into an
// immutable AST and rejects anything it does not understand: an unknown
// source, field, operator, action type or parameter is a compile error at
// write time, never "true" at run time (fixes Laravel's B4/B8). Evaluate
// walks a compiled Program against Facts. It does no I/O, reads no clock and
// touches no globals, so the same inputs always give the same Result.
//
// # Grammar v1 (a backward-compatible superset of the Laravel grammar)
//
//	conditions := null | [] | [node, ...]          // a list is an implicit AND
//	            | node
//	node       := {"all": [node, ...]} | {"any": [node, ...]} | {"not": node} | leaf
//	leaf       := {"source": src, "field": path, "operator": op, "value": v}
//	src        := "trigger" | "player" | "activity"  // "type" is accepted as an alias of "source"
//	op         := eq | neq | gt | gte | lt | lte | in | not_in | contains | exists | not_exists
//	              (Laravel names equals, not_equals, greater_than, less_than,
//	               greater_than_or_equal, less_than_or_equal are aliases)
//
// Fields:
//
//	trigger.<dot.path>                 activity properties (Laravel trigger_data), dot paths + array indexes
//	activity.event_type | event_id | causation_depth
//	activity.properties.<path> | activity.context.<path>
//	player.id | external_id | display_name | email | is_active
//	player.level | xp (xp_total) | points (points_balance, balance)
//	player.attributes.<path> (metadata.<path>)
//
// # Typed semantics (deliberate divergences from PHP loose comparison, B18)
//
//   - Numbers compare numerically; integers exactly (int64), otherwise as float64.
//   - A numeric string is coerced to a number when compared with a number
//     ("100" eq 100 is true, golden G09). Two numeric strings compare
//     numerically; two other strings compare byte-wise for ordering operators.
//   - Any other type mismatch is false, never an error: true is not 1, and
//     "abc" > 5 is false (PHP: true).
//   - A missing or null fact makes every operator false except neq, not_in
//     and not_exists, which are true (G10: missing < 5 is false; PHP: true).
//   - contains is substring on strings and membership on arrays; numbers are
//     never stringified (PHP: str_contains((string) actual, ...)). An empty
//     needle is a compile error (G11).
//   - in accepts a scalar value and wraps it, like Laravel's (array) cast; an
//     empty list is a compile error.
//
// Actions: credit_points{amount, description?}, grant_xp{amount, description?},
// award_badge{badge_id}, record_streak{streak_id | activity_key},
// progress_mission{mission_id, increment=1}, grant_reward{reward_id}.
// Amounts and increments are JSON integers > 0 (G17-G19).
//
// Limits: {max_per_player, max_per_player_per_day, max_per_player_per_week,
// cooldown_seconds}, each a positive integer. The evaluator only carries them;
// the service enforces them against counters in the decision transaction.
package eval

// Condition sources.
const (
	SourceTrigger  = "trigger"
	SourcePlayer   = "player"
	SourceActivity = "activity"
)

// Operators.
const (
	OpEq        = "eq"
	OpNeq       = "neq"
	OpGt        = "gt"
	OpGte       = "gte"
	OpLt        = "lt"
	OpLte       = "lte"
	OpIn        = "in"
	OpNotIn     = "not_in"
	OpContains  = "contains"
	OpExists    = "exists"
	OpNotExists = "not_exists"
)

// Action types (mirrors rules/contracts Action* constants).
const (
	ActionCreditPoints    = "credit_points"
	ActionGrantXP         = "grant_xp"
	ActionAwardBadge      = "award_badge"
	ActionRecordStreak    = "record_streak"
	ActionProgressMission = "progress_mission"
	ActionGrantReward     = "grant_reward"
)

// Rule statuses inside a Result.
const (
	StatusMatched    = "matched"
	StatusNotMatched = "not_matched"
	StatusOutOfScope = "out_of_scope" // program-scoped rule, player not enrolled
	StatusInvalid    = "invalid"      // stored definition no longer compiles
)

// Compile-time caps. They bound evaluation cost (doc 06 §11.10: < 1 ms for
// 200 rules x 20 conditions).
const (
	DefaultMaxActions    = 20
	DefaultMaxConditions = 100
	MaxNestingDepth      = 5
	MaxPathSegments      = 10
	MaxListValues        = 500
	MaxStringValue       = 1000
	MaxAmount            = 1_000_000_000
	MaxLimitCount        = 1_000_000_000
	MaxCooldownSeconds   = 10 * 365 * 24 * 3600
	MaxDescription       = 255
	MaxActivityKey       = 100
)

var operatorAliases = map[string]string{
	"equals":                OpEq,
	"not_equals":            OpNeq,
	"greater_than":          OpGt,
	"less_than":             OpLt,
	"greater_than_or_equal": OpGte,
	"less_than_or_equal":    OpLte,
}

var knownOperators = map[string]bool{
	OpEq: true, OpNeq: true, OpGt: true, OpGte: true, OpLt: true, OpLte: true,
	OpIn: true, OpNotIn: true, OpContains: true, OpExists: true, OpNotExists: true,
}

// playerField identifies a fixed player fact.
type playerField uint8

const (
	pfNone playerField = iota
	pfID
	pfExternalID
	pfDisplayName
	pfEmail
	pfActive
	pfLevel
	pfXP
	pfPoints
	pfAttributes // followed by a path
)

var playerFields = map[string]playerField{
	"id":             pfID,
	"external_id":    pfExternalID,
	"display_name":   pfDisplayName,
	"email":          pfEmail,
	"is_active":      pfActive,
	"active":         pfActive,
	"level":          pfLevel,
	"xp":             pfXP,
	"xp_total":       pfXP,
	"total_xp":       pfXP,
	"points":         pfPoints,
	"points_balance": pfPoints,
	"balance":        pfPoints,
}

// activityField identifies a fixed activity fact.
type activityField uint8

const (
	afNone activityField = iota
	afEventType
	afEventID
	afCausationDepth
	afProperties // followed by a path
	afContext    // followed by a path
)

var activityFields = map[string]activityField{
	"event_type":      afEventType,
	"event_id":        afEventID,
	"causation_depth": afCausationDepth,
}
