package domain

import (
	"fmt"
	"strconv"
	"time"

	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/shared/id"
)

// Execution statuses: one row per evaluated rule version.
const (
	ExecFired      = "fired"        // matched and its effects were requested
	ExecNotMatched = "not_matched"  // conditions false
	ExecLimited    = "limited"      // matched but a limit refused it
	ExecOutOfScope = "out_of_scope" // program-scoped, player not enrolled
	ExecInvalid    = "invalid"      // stored definition no longer compiles
	// ExecOutOfSchedule: occurred_at outside the version's schedule.
	ExecOutOfSchedule = "out_of_schedule"
	// ExecSkippedByStop: an earlier stop_processing rule fired.
	ExecSkippedByStop = "skipped_by_stop"
)

// Effect statuses.
const (
	EffectRequested = "requested"
	EffectApplied   = "applied"
	EffectRejected  = "rejected"
)

// Decision is the one record per activity (UNIQUE activity_id).
type Decision struct {
	ID                string
	TenantID          string
	ActivityID        string
	EventID           string
	PlayerID          string
	PlayerExternalID  string
	EventType         string
	Outcome           string
	Reason            string
	RulesetGeneration int64
	CausationDepth    int
	OccurredAt        time.Time
	EvaluatedAt       time.Time
	DurationUS        int64
}

// Execution records one evaluated rule version of a decision.
type Execution struct {
	ID               string
	TenantID         string
	DecisionID       string
	RuleID           string
	RuleVersionID    string
	PlayerID         string
	Status           string
	Matched          bool
	ConditionResults []eval.CondTrace
	EffectsCount     int
	CreatedAt        time.Time
}

// Effect is one requested state change and its settlement.
type Effect struct {
	ID             string
	TenantID       string
	DecisionID     string
	ExecutionID    string
	RuleID         string
	RuleVersionID  string
	PlayerID       string
	ActionIndex    int
	IdempotencyKey string
	Type           string
	Params         map[string]any
	Target         string // job topic, e.g. job.points.credit
	Command        []byte // the exact published command, for re-publish sweeps
	Status         string
	Reason         string
	Attempts       int
	RequestedAt    time.Time
	SettledAt      *time.Time
}

// DecisionID is derived from the activity, so a redelivered activity maps
// to the same decision (golden G28).
func DecisionID(activityID string) string { return id.Derive("rules_decision", activityID) }

// ExecutionID is derived from the decision and the rule version.
func ExecutionID(decisionID, ruleVersionID string) string {
	return id.Derive("rules_execution", decisionID, ruleVersionID)
}

// EffectKey is the idempotency key every target dedupes on:
// uuidv5(activity | rule_version | action_index) (doc 00 F2).
func EffectKey(activityID, ruleVersionID string, actionIndex int) string {
	return id.Derive(activityID, ruleVersionID, strconv.Itoa(actionIndex))
}

// EffectID is the effect row id and the effect.Source id of the command.
func EffectID(activityID, ruleVersionID string, actionIndex int) string {
	return id.Derive("rules_effect", activityID, ruleVersionID, strconv.Itoa(actionIndex))
}

// WindowKeys are the counter rows a limit occupies for an activity time.
// Windows are computed in UTC (the tenant timezone is not known to rules).
func DayWindow(t time.Time) string { return "d:" + t.UTC().Format("2006-01-02") }

func WeekWindow(t time.Time) string {
	y, w := t.UTC().ISOWeek()
	return fmt.Sprintf("w:%04d-W%02d", y, w)
}

const (
	WindowLifetime = "lifetime"
	WindowCooldown = "cooldown"
)
