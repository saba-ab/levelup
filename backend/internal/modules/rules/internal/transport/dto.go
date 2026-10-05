package transport

import (
	"encoding/json"
	"time"

	"levelup/internal/modules/rules/internal/app"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
)

// NullableString tells "absent" from "null" in PATCH bodies (fixes B25:
// description and program_id can be cleared).
type NullableString struct {
	Set   bool
	Null  bool
	Value string
}

func (n *NullableString) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		n.Null = true
		return nil
	}
	return json.Unmarshal(b, &n.Value)
}

func (n NullableString) opt() app.OptString {
	return app.OptString{Set: n.Set, Null: n.Null, Value: n.Value}
}

// CreateRuleReq creates a rule and its draft version 1. Conditions, actions
// and limits follow the grammar documented in internal/domain/eval.
type CreateRuleReq struct {
	Name         string          `json:"name" validate:"required,max=255"`
	Slug         string          `json:"slug,omitempty" validate:"omitempty,max=120"`
	Description  string          `json:"description,omitempty" validate:"max=1000"`
	TriggerEvent string          `json:"trigger_event" validate:"required,max=100"`
	ProgramID    string          `json:"program_id,omitempty" validate:"omitempty,uuid"`
	Priority     int             `json:"priority" validate:"min=0,max=1000000"`
	Conditions   json.RawMessage `json:"conditions,omitempty" swaggertype:"object"`
	Actions      json.RawMessage `json:"actions" validate:"required" swaggertype:"array,object"`
	Limits       json.RawMessage `json:"limits,omitempty" swaggertype:"object"`
}

// UpdateRuleReq is a partial update. conditions/actions/limits edit the
// latest version and only while it is a draft (409 no_draft_version).
type UpdateRuleReq struct {
	Name         *string         `json:"name,omitempty" validate:"omitempty,max=255"`
	Description  NullableString  `json:"description" swaggertype:"string"`
	TriggerEvent *string         `json:"trigger_event,omitempty" validate:"omitempty,max=100"`
	ProgramID    NullableString  `json:"program_id" swaggertype:"string"`
	Priority     *int            `json:"priority,omitempty" validate:"omitempty,min=0,max=1000000"`
	Status       *string         `json:"status,omitempty" validate:"omitempty,oneof=active inactive archived"`
	Conditions   json.RawMessage `json:"conditions,omitempty" swaggertype:"object"`
	Actions      json.RawMessage `json:"actions,omitempty" swaggertype:"array,object"`
	Limits       json.RawMessage `json:"limits,omitempty" swaggertype:"object"`
}

// CreateVersionReq creates a new draft version; omitted parts are copied
// from from_version (default: the latest version).
type CreateVersionReq struct {
	FromVersion *int            `json:"from_version,omitempty" validate:"omitempty,min=1"`
	Conditions  json.RawMessage `json:"conditions,omitempty" swaggertype:"object"`
	Actions     json.RawMessage `json:"actions,omitempty" swaggertype:"array,object"`
	Limits      json.RawMessage `json:"limits,omitempty" swaggertype:"object"`
}

// PublishReq names the version to make live.
type PublishReq struct {
	Version int `json:"version" validate:"required,min=1"`
}

// SimPlayerReq is an inline player for simulation.
type SimPlayerReq struct {
	ExternalID string         `json:"external_id,omitempty" validate:"max=255"`
	IsActive   *bool          `json:"is_active,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Level      *int           `json:"level,omitempty" validate:"omitempty,min=0"`
	XP         *int64         `json:"xp,omitempty" validate:"omitempty,min=0"`
	Points     *int64         `json:"points,omitempty"`
}

// SimulateReq is a hypothetical activity evaluated against the live ruleset.
type SimulateReq struct {
	EventType        string          `json:"event_type" validate:"required,max=100"`
	PlayerID         string          `json:"player_id,omitempty" validate:"omitempty,uuid"`
	PlayerExternalID string          `json:"player_external_id,omitempty" validate:"omitempty,max=255"`
	Player           *SimPlayerReq   `json:"player,omitempty"`
	Properties       json.RawMessage `json:"properties,omitempty" swaggertype:"object"`
	Context          json.RawMessage `json:"context,omitempty" swaggertype:"object"`
	CausationDepth   int             `json:"causation_depth,omitempty" validate:"min=0"`
}

// VersionResp is one rule version with its full definition.
type VersionResp struct {
	ID          string          `json:"id"`
	RuleID      string          `json:"rule_id"`
	Version     int             `json:"version"`
	Conditions  json.RawMessage `json:"conditions" swaggertype:"object"`
	Actions     json.RawMessage `json:"actions" swaggertype:"array,object"`
	Limits      json.RawMessage `json:"limits" swaggertype:"object"`
	Published   bool            `json:"published"`
	PublishedAt *time.Time      `json:"published_at"`
	CreatedBy   string          `json:"created_by,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// RuleResp is a rule; current_version and latest_version carry definitions.
type RuleResp struct {
	ID               string       `json:"id"`
	TenantID         string       `json:"tenant_id"`
	Slug             string       `json:"slug"`
	Name             string       `json:"name"`
	Description      string       `json:"description"`
	TriggerEvent     string       `json:"trigger_event"`
	ProgramID        *string      `json:"program_id"`
	Priority         int          `json:"priority"`
	Status           string       `json:"status"`
	CurrentVersionID *string      `json:"current_version_id"`
	CurrentVersion   *VersionResp `json:"current_version,omitempty"`
	LatestVersion    *VersionResp `json:"latest_version,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

type RuleListResp struct {
	Data       []RuleResp `json:"data"`
	NextCursor string     `json:"next_cursor"`
}

type VersionListResp struct {
	Data       []VersionResp `json:"data"`
	NextCursor string        `json:"next_cursor"`
}

// TraceResp is one evaluated condition.
type TraceResp struct {
	Path     string `json:"path"`
	Source   string `json:"source"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
	Present  bool   `json:"present"`
	Result   bool   `json:"result"`
}

// SimEffectResp is an effect the rule would request.
type SimEffectResp struct {
	ActionIndex int            `json:"action_index"`
	Type        string         `json:"type"`
	Params      map[string]any `json:"params"`
}

// SimLimitsResp reports a rule's limits (not enforced in simulation).
type SimLimitsResp struct {
	MaxPerPlayer        int64 `json:"max_per_player,omitempty"`
	MaxPerPlayerPerDay  int64 `json:"max_per_player_per_day,omitempty"`
	MaxPerPlayerPerWeek int64 `json:"max_per_player_per_week,omitempty"`
	CooldownSeconds     int64 `json:"cooldown_seconds,omitempty"`
}

type SimRuleResp struct {
	RuleID        string          `json:"rule_id"`
	RuleVersionID string          `json:"rule_version_id"`
	Name          string          `json:"name"`
	Priority      int             `json:"priority"`
	Status        string          `json:"status"`
	Matched       bool            `json:"matched"`
	Conditions    []TraceResp     `json:"condition_results"`
	Effects       []SimEffectResp `json:"effects"`
	Limits        *SimLimitsResp  `json:"limits,omitempty"`
	Error         string          `json:"error,omitempty"`
}

type SimulateResp struct {
	Outcome           string        `json:"outcome"`
	Reason            string        `json:"reason,omitempty"`
	PlayerID          string        `json:"player_id,omitempty"`
	RulesetGeneration int64         `json:"ruleset_generation"`
	LimitsEnforced    bool          `json:"limits_enforced"`
	Rules             []SimRuleResp `json:"rules"`
}

type DecisionResp struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	ActivityID        string    `json:"activity_id"`
	EventID           string    `json:"event_id,omitempty"`
	PlayerID          *string   `json:"player_id"`
	PlayerExternalID  string    `json:"player_external_id,omitempty"`
	EventType         string    `json:"event_type"`
	Outcome           string    `json:"outcome"`
	Reason            string    `json:"reason,omitempty"`
	RulesetGeneration int64     `json:"ruleset_generation"`
	CausationDepth    int       `json:"causation_depth"`
	OccurredAt        time.Time `json:"occurred_at"`
	EvaluatedAt       time.Time `json:"evaluated_at"`
	DurationUS        int64     `json:"duration_us"`
}

type DecisionListResp struct {
	Data       []DecisionResp `json:"data"`
	NextCursor string         `json:"next_cursor"`
}

type ExecutionResp struct {
	ID            string      `json:"id"`
	RuleID        string      `json:"rule_id"`
	RuleVersionID string      `json:"rule_version_id"`
	Status        string      `json:"status"`
	Matched       bool        `json:"matched"`
	Conditions    []TraceResp `json:"condition_results"`
	EffectsCount  int         `json:"effects_count"`
	CreatedAt     time.Time   `json:"created_at"`
}

type EffectResp struct {
	ID             string         `json:"id"`
	ExecutionID    string         `json:"execution_id"`
	RuleID         string         `json:"rule_id"`
	RuleVersionID  string         `json:"rule_version_id"`
	ActionIndex    int            `json:"action_index"`
	IdempotencyKey string         `json:"idempotency_key"`
	Type           string         `json:"type"`
	Params         map[string]any `json:"params"`
	Target         string         `json:"target"`
	Status         string         `json:"status"`
	Reason         string         `json:"reason,omitempty"`
	Attempts       int            `json:"attempts"`
	RequestedAt    time.Time      `json:"requested_at"`
	SettledAt      *time.Time     `json:"settled_at"`
}

type DecisionDetailResp struct {
	DecisionResp
	Executions []ExecutionResp `json:"executions"`
	Effects    []EffectResp    `json:"effects"`
}

func optPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toVersion(v *domain.Version) *VersionResp {
	if v == nil {
		return nil
	}
	return &VersionResp{
		ID: v.ID, RuleID: v.RuleID, Version: v.Version, Conditions: v.Conditions, Actions: v.Actions,
		Limits: v.Limits, Published: v.Published(), PublishedAt: v.PublishedAt, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt,
	}
}

func toRule(r domain.Rule) RuleResp {
	return RuleResp{
		ID: r.ID, TenantID: r.TenantID, Slug: r.Slug, Name: r.Name, Description: r.Description,
		TriggerEvent: r.TriggerEvent, ProgramID: optPtr(r.ProgramID), Priority: r.Priority, Status: r.Status,
		CurrentVersionID: optPtr(r.CurrentVersionID), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toRuleView(v app.RuleView) RuleResp {
	out := toRule(v.Rule)
	out.CurrentVersion = toVersion(v.Current)
	out.LatestVersion = toVersion(v.Latest)
	return out
}

func toTrace(ts []eval.CondTrace) []TraceResp {
	out := make([]TraceResp, len(ts))
	for i, t := range ts {
		out[i] = TraceResp(t)
	}
	return out
}

func toSimulate(r app.SimulateResult) SimulateResp {
	out := SimulateResp{Outcome: r.Outcome, Reason: r.Reason, PlayerID: r.PlayerID,
		RulesetGeneration: r.RulesetGeneration, Rules: make([]SimRuleResp, 0, len(r.Rules))}
	for _, rr := range r.Rules {
		sr := SimRuleResp{RuleID: rr.RuleID, RuleVersionID: rr.RuleVersionID, Name: rr.Name, Priority: rr.Priority,
			Status: rr.Status, Matched: rr.Matched, Conditions: toTrace(rr.Trace), Effects: []SimEffectResp{}, Error: rr.Error}
		for i, a := range rr.Actions {
			sr.Effects = append(sr.Effects, SimEffectResp{ActionIndex: i, Type: a.Type, Params: a.Params()})
		}
		if !rr.Limits.IsZero() {
			sr.Limits = &SimLimitsResp{MaxPerPlayer: rr.Limits.MaxPerPlayer, MaxPerPlayerPerDay: rr.Limits.MaxPerPlayerPerDay,
				MaxPerPlayerPerWeek: rr.Limits.MaxPerPlayerPerWeek, CooldownSeconds: rr.Limits.CooldownSeconds}
		}
		out.Rules = append(out.Rules, sr)
	}
	return out
}

func toDecision(d domain.Decision) DecisionResp {
	return DecisionResp{
		ID: d.ID, TenantID: d.TenantID, ActivityID: d.ActivityID, EventID: d.EventID, PlayerID: optPtr(d.PlayerID),
		PlayerExternalID: d.PlayerExternalID, EventType: d.EventType, Outcome: d.Outcome, Reason: d.Reason,
		RulesetGeneration: d.RulesetGeneration, CausationDepth: d.CausationDepth, OccurredAt: d.OccurredAt,
		EvaluatedAt: d.EvaluatedAt, DurationUS: d.DurationUS,
	}
}

func toDecisionDetail(d app.DecisionDetail) DecisionDetailResp {
	out := DecisionDetailResp{DecisionResp: toDecision(d.Decision),
		Executions: make([]ExecutionResp, len(d.Executions)), Effects: make([]EffectResp, len(d.Effects))}
	for i, e := range d.Executions {
		out.Executions[i] = ExecutionResp{ID: e.ID, RuleID: e.RuleID, RuleVersionID: e.RuleVersionID, Status: e.Status,
			Matched: e.Matched, Conditions: toTrace(e.ConditionResults), EffectsCount: e.EffectsCount, CreatedAt: e.CreatedAt}
	}
	for i, e := range d.Effects {
		out.Effects[i] = EffectResp{ID: e.ID, ExecutionID: e.ExecutionID, RuleID: e.RuleID, RuleVersionID: e.RuleVersionID,
			ActionIndex: e.ActionIndex, IdempotencyKey: e.IdempotencyKey, Type: e.Type, Params: e.Params, Target: e.Target,
			Status: e.Status, Reason: e.Reason, Attempts: e.Attempts, RequestedAt: e.RequestedAt, SettledAt: e.SettledAt}
	}
	return out
}
