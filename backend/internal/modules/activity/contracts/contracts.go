// Package contracts is activity's public surface. An activity is one fact a
// tenant system reports about a player ("purchase_completed"). It is stored
// once per (tenant, event_id) and handed to rules through the outbox.
package contracts

import (
	"time"

	"levelup/internal/platform/authz"
)

const TopicReceived = "activity.received.v1"

// Activity statuses (projection updated from rules.decision_made.v1).
const (
	StatusPending  = "pending"
	StatusDecided  = "decided"
	StatusRejected = "rejected"
)

type ReceivedV1 struct {
	ActivityID       string         `json:"activity_id"`
	TenantID         string         `json:"tenant_id"`
	EventID          string         `json:"event_id"` // tenant-supplied, unique per tenant
	EventType        string         `json:"event_type"`
	PlayerExternalID string         `json:"player_external_id"`
	PlayerID         string         `json:"player_id,omitempty"` // set when resolved at ingest
	Properties       map[string]any `json:"properties,omitempty"`
	Context          map[string]any `json:"context,omitempty"`
	OccurredAt       time.Time      `json:"occurred_at"`
	ReceivedAt       time.Time      `json:"received_at"`
	// CausationDepth > 0 marks internal activities derived from other facts
	// (level reached, badge awarded); rules caps it to stop loops.
	CausationDepth int `json:"causation_depth"`
	// SourceEventID is the envelope event_id of the fact an internal
	// activity was derived from (empty for tenant-reported activities).
	SourceEventID string `json:"source_event_id,omitempty"`
	// AutoCreatePlayer is set when ACTIVITY_AUTO_CREATE_PLAYERS is on and
	// the player could not be resolved at ingest: a consumer that owns
	// players may create it from PlayerExternalID before rules decide.
	AutoCreatePlayer bool `json:"auto_create_player,omitempty"`
}

// Internal activity event types (doc 06 §11.9), emitted only when
// ACTIVITY_INTERNAL_TRIGGERS is enabled.
const (
	EventTypeLevelUp          = "level_up"
	EventTypeBadgeEarned      = "badge_earned"
	EventTypeMissionCompleted = "mission_completed"
)

// InternalEventIDPrefix marks activities derived from other modules'
// facts: event_id = InternalEventIDPrefix + source envelope event_id.
const InternalEventIDPrefix = "sys:"

const Module = "activity"

var (
	PermIngest  = authz.Permission{Module: Module, Action: "ingest"}
	PermViewAny = authz.Permission{Module: Module, Action: "view_any"}
	PermView    = authz.Permission{Module: Module, Action: "view"}
)

var AllPermissions = []authz.Permission{PermIngest, PermViewAny, PermView}
