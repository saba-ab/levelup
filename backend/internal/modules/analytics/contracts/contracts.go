// Package contracts is analytics' public surface. Analytics is a pure
// projection: it consumes other modules' facts into per-tenant daily
// counters and player activity days (UTC), and serves the portal's
// Analytics page. It publishes nothing and offers no Reader.
package contracts

import "levelup/internal/platform/authz"

const Module = "analytics"

// Cron jobs (Schedule set in Jobs(); enqueued by the scheduler).
const (
	// JobPrune drops projection rows older than the retention (2 years by
	// default) and applied_events idempotency rows past their retention.
	JobPrune = "analytics.prune"
)

// Metric names stored in daily_counters. The dimension column carries the
// breakdown named in each comment ("" when there is none).
const (
	MetricActivities        = "activities"         // dimension: event_type
	MetricPointsCredited    = "points_credited"    // dimension: ledger kind; value: points
	MetricPointsDebited     = "points_debited"     // dimension: ledger kind; value: points
	MetricBadgesAwarded     = "badges_awarded"     // dimension: badge_id
	MetricMissionsStarted   = "missions_started"   // dimension: mission_id
	MetricMissionsCompleted = "missions_completed" // dimension: mission_id
	MetricLevelsReached     = "levels_reached"     // dimension: level_number
	MetricRewardsClaimed    = "rewards_claimed"    // dimension: reward_id
	MetricPlayersCreated    = "players_created"    // dimension: ""
)

var PermView = authz.Permission{Module: Module, Action: "view"}

var AllPermissions = []authz.Permission{PermView}
