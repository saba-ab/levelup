// Package contracts is rules' public surface. Rules turn activities into
// effects: rules evaluates, records a decision, and issues one job command
// per effect to the module that owns the state (points, progression, ...).
package contracts

import (
	"time"

	"levelup/internal/platform/authz"
)

const (
	TopicDecisionMade     = "rules.decision_made.v1"
	TopicVersionPublished = "rules.version_published.v1"
)

// Action types of the rule grammar (doc 06 §2.4 + §11.5).
const (
	ActionCreditPoints    = "credit_points"
	ActionGrantXP         = "grant_xp"
	ActionAwardBadge      = "award_badge"
	ActionRecordStreak    = "record_streak"
	ActionProgressMission = "progress_mission"
	ActionGrantReward     = "grant_reward"
)

// Decision outcomes.
const (
	OutcomeMatched      = "matched"
	OutcomeNoMatch      = "no_match"
	OutcomeRejected     = "rejected" // e.g. unknown player
	OutcomeLimitReached = "limit_reached"
)

// Decision reasons (DecisionMadeV1.Reason) beside effect.ReasonPlayerNotFound
// and effect.ReasonPlayerInactive. Additive.
const (
	ReasonCausationDepthExceeded = "causation_depth_exceeded"
)

// Effect settlement statuses (rule_execution_effects.status). Additive.
const (
	EffectRequested = "requested"
	EffectApplied   = "applied"
	EffectRejected  = "rejected"
)

// Job names declared by rules' Jobs() (crons). Additive.
const (
	JobEffectsReconcile = "rules.effects_reconcile"
)

// Problem codes clients may branch on (ADR-0016). Additive.
const (
	CodeRuleNotFound          = "rule_not_found"
	CodeInvalidRuleDefinition = "invalid_rule_definition"
	CodeEndpointGone          = "endpoint_gone"
)

type EffectV1 struct {
	EffectID       string         `json:"effect_id"`
	IdempotencyKey string         `json:"idempotency_key"`
	RuleID         string         `json:"rule_id"`
	RuleVersionID  string         `json:"rule_version_id"`
	ActionIndex    int            `json:"action_index"`
	Type           string         `json:"type"`
	Params         map[string]any `json:"params"`
}

type DecisionMadeV1 struct {
	DecisionID   string     `json:"decision_id"`
	ActivityID   string     `json:"activity_id"`
	TenantID     string     `json:"tenant_id"`
	PlayerID     string     `json:"player_id,omitempty"`
	EventType    string     `json:"event_type"`
	Outcome      string     `json:"outcome"`
	Reason       string     `json:"reason,omitempty"`
	MatchedRules []string   `json:"matched_rules"`
	Effects      []EffectV1 `json:"effects"`
	At           time.Time  `json:"at"`
	// RulesetGeneration is the ruleset the decision was evaluated against. Additive.
	RulesetGeneration int64 `json:"ruleset_generation,omitempty"`
}

type VersionPublishedV1 struct {
	RuleID        string    `json:"rule_id"`
	RuleVersionID string    `json:"rule_version_id"`
	TenantID      string    `json:"tenant_id"`
	Version       int       `json:"version"`
	TriggerEvent  string    `json:"trigger_event"`
	At            time.Time `json:"at"`
}

const Module = "rules"

var (
	PermViewAny       = authz.Permission{Module: Module, Action: "view_any"}
	PermView          = authz.Permission{Module: Module, Action: "view"}
	PermCreate        = authz.Permission{Module: Module, Action: "create"}  // admin roles
	PermUpdate        = authz.Permission{Module: Module, Action: "update"}  // admin roles
	PermDelete        = authz.Permission{Module: Module, Action: "delete"}  // admin roles
	PermPublish       = authz.Permission{Module: Module, Action: "publish"} // admin roles
	PermSimulate      = authz.Permission{Module: Module, Action: "simulate"}
	PermViewDecisions = authz.Permission{Module: Module, Action: "view_decisions"}
)

var AllPermissions = []authz.Permission{
	PermViewAny, PermView, PermCreate, PermUpdate, PermDelete, PermPublish, PermSimulate, PermViewDecisions,
}
