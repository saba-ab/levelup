// Package domain holds activity's entity and its invariants. No framework
// tags: shape validation is transport's job, invariants live here.
package domain

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"levelup/internal/shared/id"
)

// Statuses of the decision projection.
const (
	StatusPending  = "pending"
	StatusDecided  = "decided"
	StatusRejected = "rejected"
)

const (
	MaxEventIDLength    = 128
	MaxEventTypeLength  = 100
	MaxExternalIDLength = 255
)

var eventTypePattern = regexp.MustCompile(`^[a-z0-9_.:-]+$`)

// Activity is one fact a tenant system (or, for internal triggers, another
// module) reported about a player. Stored once per (TenantID, EventID).
type Activity struct {
	ID               string
	TenantID         string
	EventID          string
	EventType        string
	PlayerExternalID string
	PlayerID         string // "" when the player was unknown at ingest
	Properties       map[string]any
	Context          map[string]any
	OccurredAt       time.Time
	ReceivedAt       time.Time

	Status     string
	DecisionID string
	Outcome    string
	Reason     string
	DecidedAt  time.Time

	CausationDepth int
	SourceEventID  string // set for internal activities only
	RepublishCount int

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Limits are the acceptance window and size caps. A zero value disables
// the corresponding check.
type Limits struct {
	MaxAge          time.Duration
	MaxFutureSkew   time.Duration
	MaxPayloadBytes int
}

// NewInput is everything a caller may set on a fresh activity.
type NewInput struct {
	TenantID         string
	EventID          string
	EventType        string
	PlayerExternalID string
	PlayerID         string
	Properties       map[string]any
	Context          map[string]any
	OccurredAt       time.Time // zero = now
	CausationDepth   int
	SourceEventID    string
}

// NewActivity validates the invariants and returns a pending activity.
func NewActivity(in NewInput, now time.Time, lim Limits) (Activity, error) {
	now = normalizeTime(now)
	if in.TenantID == "" {
		return Activity{}, ErrNoTenant
	}

	eventID := strings.TrimSpace(in.EventID)
	switch {
	case eventID == "":
		return Activity{}, ErrEventIDRequired
	case len(eventID) > MaxEventIDLength:
		return Activity{}, ErrEventIDTooLong
	}

	eventType := strings.TrimSpace(in.EventType)
	if eventType == "" || len(eventType) > MaxEventTypeLength || !eventTypePattern.MatchString(eventType) {
		return Activity{}, ErrBadEventType
	}

	extID := strings.TrimSpace(in.PlayerExternalID)
	switch {
	case extID == "" && in.PlayerID == "":
		return Activity{}, ErrPlayerRequired
	case len(extID) > MaxExternalIDLength:
		return Activity{}, ErrPlayerIDTooLong
	}

	occurredAt := now
	if !in.OccurredAt.IsZero() {
		occurredAt = normalizeTime(in.OccurredAt)
	}
	if lim.MaxFutureSkew > 0 && occurredAt.After(now.Add(lim.MaxFutureSkew)) {
		return Activity{}, ErrOccurredInFuture
	}
	if lim.MaxAge > 0 && occurredAt.Before(now.Add(-lim.MaxAge)) {
		return Activity{}, ErrOccurredTooOld
	}

	if in.CausationDepth < 0 {
		return Activity{}, ErrBadCausationDepth
	}

	props, err := capped(in.Properties, lim.MaxPayloadBytes, ErrPropertiesTooLarge)
	if err != nil {
		return Activity{}, err
	}
	ctxData, err := capped(in.Context, lim.MaxPayloadBytes, ErrContextTooLarge)
	if err != nil {
		return Activity{}, err
	}

	return Activity{
		ID:               id.NewID(),
		TenantID:         in.TenantID,
		EventID:          eventID,
		EventType:        eventType,
		PlayerExternalID: extID,
		PlayerID:         in.PlayerID,
		Properties:       props,
		Context:          ctxData,
		OccurredAt:       occurredAt,
		ReceivedAt:       now,
		Status:           StatusPending,
		CausationDepth:   in.CausationDepth,
		SourceEventID:    in.SourceEventID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

// Pending reports whether no decision has been projected yet.
func (a Activity) Pending() bool { return a.Status == StatusPending }

// ValidStatus reports whether s is a known status (list filters).
func ValidStatus(s string) bool {
	switch s {
	case StatusPending, StatusDecided, StatusRejected:
		return true
	}
	return false
}

// capped normalises nil to an empty object and enforces the serialized
// size cap: the cap is on what gets stored and shipped to rules.
func capped(m map[string]any, maxBytes int, tooLarge error) (map[string]any, error) {
	if m == nil {
		return map[string]any{}, nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, ErrPayloadNotJSON
	}
	if maxBytes > 0 && len(raw) > maxBytes {
		return nil, tooLarge
	}
	return m, nil
}

// normalizeTime pins times to UTC at Postgres precision, so the stored row
// and the published event carry the identical instant.
func normalizeTime(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }
