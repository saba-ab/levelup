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
//	src        := "trigger" | "player" | "activity" | "history"  // "type" is accepted as an alias of "source"
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
//
// # stop_processing
//
// A version flag. Rules run in (priority DESC, rule id ASC) order; once a
// stop_processing rule FIRES, every later rule of the same activity is
// recorded as skipped_by_stop and neither evaluated for limits nor allowed
// to emit effects. "Fires" means matched and not refused by a limit, so a
// limited stop rule does not stop anything. Evaluate itself never cuts the
// list (it cannot see limits); StopGate applies the cut in order, and
// Result.ApplyStop applies it with "matched = fired" (simulation, where
// limits are reported but not enforced).
//
// # Schedule
//
//	schedule := null | {"starts_at"?: RFC3339, "ends_at"?: RFC3339,
//	                    "days_of_week"?: [0-6, ...],          // 0 = Sunday
//	                    "hours"?: {"from": "HH:MM", "to": "HH:MM"},
//	                    "timezone"?: IANA name}               // default UTC
//
// Evaluated against activity.occurred_at (never the clock). starts_at is
// inclusive, ends_at exclusive; hours is [from, to) in the schedule's
// timezone and wraps midnight when from > to (22:00-02:00); days_of_week is
// the local weekday of the activity instant. Outside the schedule the rule is
// recorded as out_of_schedule (it is not evaluated and does not stop
// processing). Everything is validated at compile time: unknown keys, a
// malformed time, ends_at <= starts_at, a day outside 0-6, from == to and an
// unknown timezone are errors.
//
// # Grammar v2: history leaves (aggregates over the player's past)
//
//	leaf := ... | {"source": "history", "field": "first_time",
//	               "operator"?: eq | neq, "value"?: bool}   // default: eq true
//	            | {"source": "history", "field": "count",
//	               "event_type"?: slug,                       // default: the trigger
//	               "window": "1d" | "7d" | "30d" | "90d" | "all",
//	               "operator": eq | neq | gt | gte | lt | lte | in | not_in,
//	               "value": number | [number, ...]}
//
// History is read from rules' own projection (rule_player_event_days: one
// counter per tenant, player, event type and UTC day of occurred_at, bumped
// in the decision transaction of every evaluated activity). The values are
// loaded by the service in one query and handed in as Facts.History, so the
// evaluator stays pure.
//
//   - count is the number of PRIOR activities: the current activity is never
//     counted (the projection is bumped after evaluation, in the same tx).
//   - Windows are UTC calendar days ending on the activity's day: "1d" is
//     the activity's UTC day, "7d" that day and the 6 before it, ..., "all"
//     every day up to and including it. Activities recorded for later days
//     (out-of-order delivery) are not counted; activities of the same day are
//     counted when they were decided before this one (day granularity).
//   - first_time is true when the player has no prior activity of the
//     trigger event type, i.e. count(trigger, all) == 0. Like conditions on
//     player state it is read from a snapshot, so two concurrent first
//     activities can both see it true: pair it with limits.max_per_player = 1
//     when it must fire exactly once (counters are concurrency-safe).
//   - Facts.History == nil (not loaded) makes every history fact missing.
package eval

// Condition sources.
const (
	SourceTrigger  = "trigger"
	SourcePlayer   = "player"
	SourceActivity = "activity"
	SourceHistory  = "history"
)

// History fields and windows (grammar v2).
const (
	HistoryFirstTime = "first_time"
	HistoryCount     = "count"

	Window1d  = "1d"
	Window7d  = "7d"
	Window30d = "30d"
	Window90d = "90d"
	WindowAll = "all"
)

// HistoryWindows lists every window; WindowDays gives each one's length in
// days (0 = unbounded).
var HistoryWindows = []string{Window1d, Window7d, Window30d, Window90d, WindowAll}

var WindowDays = map[string]int{Window1d: 1, Window7d: 7, Window30d: 30, Window90d: 90, WindowAll: 0}

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
	// StatusOutOfSchedule: the activity's occurred_at is outside the
	// rule's schedule; the conditions were not evaluated.
	StatusOutOfSchedule = "out_of_schedule"
	// StatusSkippedByStop: an earlier stop_processing rule fired.
	StatusSkippedByStop = "skipped_by_stop"
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
	MaxEventType         = 100
	MaxTimezone          = 64
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
