// Package repo implements rules' persistence over rules_svc. Models are
// unexported; table names derive from struct names (no TableName()). JSONB
// columns are bound as string: under PgBouncer's simple protocol a []byte
// would be sent as bytea and rejected.
package repo

import (
	"encoding/json"
	"time"

	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
)

// rule → rules_svc.rules
type rule struct {
	ID               string `gorm:"primaryKey;type:uuid"`
	TenantID         string `gorm:"type:uuid"`
	Slug             string
	Name             string
	Description      string
	TriggerEvent     string
	ProgramID        *string `gorm:"type:uuid"`
	Priority         int
	Status           string
	CurrentVersionID *string `gorm:"type:uuid"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

// ruleVersion → rules_svc.rule_versions
type ruleVersion struct {
	ID          string `gorm:"primaryKey;type:uuid"`
	RuleID      string `gorm:"type:uuid"`
	TenantID    string `gorm:"type:uuid"`
	Version     int
	Conditions  string `gorm:"type:jsonb"`
	Actions     string `gorm:"type:jsonb"`
	Limits      string `gorm:"type:jsonb"`
	PublishedAt *time.Time
	CreatedBy   string
	CreatedAt   time.Time
}

// rulesetGeneration → rules_svc.ruleset_generations
type rulesetGeneration struct {
	TenantID   string `gorm:"primaryKey;type:uuid"`
	Generation int64
}

// decision → rules_svc.decisions
type decision struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	TenantID          string `gorm:"type:uuid"`
	ActivityID        string `gorm:"type:uuid"`
	EventID           string
	PlayerID          *string `gorm:"type:uuid"`
	PlayerExternalID  string
	EventType         string
	Outcome           string
	Reason            string
	RulesetGeneration int64
	CausationDepth    int
	OccurredAt        time.Time
	EvaluatedAt       time.Time
	DurationUs        int64
}

// ruleExecution → rules_svc.rule_executions
type ruleExecution struct {
	ID               string  `gorm:"primaryKey;type:uuid"`
	TenantID         string  `gorm:"type:uuid"`
	DecisionID       string  `gorm:"type:uuid"`
	RuleID           string  `gorm:"type:uuid"`
	RuleVersionID    string  `gorm:"type:uuid"`
	PlayerID         *string `gorm:"type:uuid"`
	Status           string
	Matched          bool
	ConditionResults string `gorm:"type:jsonb"`
	EffectsCount     int
	CreatedAt        time.Time
}

// ruleExecutionEffect → rules_svc.rule_execution_effects
type ruleExecutionEffect struct {
	ID             string  `gorm:"primaryKey;type:uuid"`
	TenantID       string  `gorm:"type:uuid"`
	DecisionID     string  `gorm:"type:uuid"`
	ExecutionID    string  `gorm:"type:uuid"`
	RuleID         string  `gorm:"type:uuid"`
	RuleVersionID  string  `gorm:"type:uuid"`
	PlayerID       *string `gorm:"type:uuid"`
	ActionIndex    int
	IdempotencyKey string
	Type           string
	Params         string `gorm:"type:jsonb"`
	Target         string
	Command        string `gorm:"type:jsonb"`
	Status         string
	Reason         string
	Attempts       int
	RequestedAt    time.Time
	LastAttemptAt  *time.Time
	SettledAt      *time.Time
}

// rulePlayerCounter → rules_svc.rule_player_counters (written with raw SQL).
type rulePlayerCounter struct {
	TenantID    string `gorm:"primaryKey;type:uuid"`
	RuleID      string `gorm:"primaryKey;type:uuid"`
	PlayerID    string `gorm:"primaryKey;type:uuid"`
	WindowKey   string `gorm:"primaryKey"`
	Count       int64
	LastFiredAt time.Time
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func val(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func jsonText(raw json.RawMessage, fallback string) string {
	if len(raw) == 0 {
		return fallback
	}
	return string(raw)
}

func fromRule(r domain.Rule) rule {
	return rule{
		ID: r.ID, TenantID: r.TenantID, Slug: r.Slug, Name: r.Name, Description: r.Description,
		TriggerEvent: r.TriggerEvent, ProgramID: ptr(r.ProgramID), Priority: r.Priority, Status: r.Status,
		CurrentVersionID: ptr(r.CurrentVersionID), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt,
	}
}

func (m rule) toDomain() domain.Rule {
	return domain.Rule{
		ID: m.ID, TenantID: m.TenantID, Slug: m.Slug, Name: m.Name, Description: m.Description,
		TriggerEvent: m.TriggerEvent, ProgramID: val(m.ProgramID), Priority: m.Priority, Status: m.Status,
		CurrentVersionID: val(m.CurrentVersionID), CreatedAt: m.CreatedAt.UTC(), UpdatedAt: m.UpdatedAt.UTC(),
		DeletedAt: m.DeletedAt,
	}
}

func fromVersion(v domain.Version) ruleVersion {
	return ruleVersion{
		ID: v.ID, RuleID: v.RuleID, TenantID: v.TenantID, Version: v.Version,
		Conditions: jsonText(v.Conditions, "null"), Actions: jsonText(v.Actions, "null"), Limits: jsonText(v.Limits, "null"),
		PublishedAt: v.PublishedAt, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt,
	}
}

func (m ruleVersion) toDomain() domain.Version {
	return domain.Version{
		ID: m.ID, RuleID: m.RuleID, TenantID: m.TenantID, Version: m.Version,
		Conditions: json.RawMessage(m.Conditions), Actions: json.RawMessage(m.Actions), Limits: json.RawMessage(m.Limits),
		PublishedAt: m.PublishedAt, CreatedBy: m.CreatedBy, CreatedAt: m.CreatedAt.UTC(),
	}
}

func fromDecision(d domain.Decision) decision {
	return decision{
		ID: d.ID, TenantID: d.TenantID, ActivityID: d.ActivityID, EventID: d.EventID, PlayerID: ptr(d.PlayerID),
		PlayerExternalID: d.PlayerExternalID, EventType: d.EventType, Outcome: d.Outcome, Reason: d.Reason,
		RulesetGeneration: d.RulesetGeneration, CausationDepth: d.CausationDepth, OccurredAt: d.OccurredAt,
		EvaluatedAt: d.EvaluatedAt, DurationUs: d.DurationUS,
	}
}

func (m decision) toDomain() domain.Decision {
	return domain.Decision{
		ID: m.ID, TenantID: m.TenantID, ActivityID: m.ActivityID, EventID: m.EventID, PlayerID: val(m.PlayerID),
		PlayerExternalID: m.PlayerExternalID, EventType: m.EventType, Outcome: m.Outcome, Reason: m.Reason,
		RulesetGeneration: m.RulesetGeneration, CausationDepth: m.CausationDepth, OccurredAt: m.OccurredAt.UTC(),
		EvaluatedAt: m.EvaluatedAt.UTC(), DurationUS: m.DurationUs,
	}
}

// traceRow is the stored shape of one condition result.
type traceRow struct {
	Path     string `json:"path"`
	Source   string `json:"source"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
	Present  bool   `json:"present"`
	Result   bool   `json:"result"`
}

func fromExecution(e domain.Execution) (ruleExecution, error) {
	rows := make([]traceRow, len(e.ConditionResults))
	for i, t := range e.ConditionResults {
		rows[i] = traceRow(t)
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return ruleExecution{}, err
	}
	return ruleExecution{
		ID: e.ID, TenantID: e.TenantID, DecisionID: e.DecisionID, RuleID: e.RuleID, RuleVersionID: e.RuleVersionID,
		PlayerID: ptr(e.PlayerID), Status: e.Status, Matched: e.Matched, ConditionResults: string(b),
		EffectsCount: e.EffectsCount, CreatedAt: e.CreatedAt,
	}, nil
}

func (m ruleExecution) toDomain() domain.Execution {
	var rows []traceRow
	_ = json.Unmarshal([]byte(m.ConditionResults), &rows)
	trace := make([]eval.CondTrace, len(rows))
	for i, t := range rows {
		trace[i] = eval.CondTrace(t)
	}
	return domain.Execution{
		ID: m.ID, TenantID: m.TenantID, DecisionID: m.DecisionID, RuleID: m.RuleID, RuleVersionID: m.RuleVersionID,
		PlayerID: val(m.PlayerID), Status: m.Status, Matched: m.Matched, ConditionResults: trace,
		EffectsCount: m.EffectsCount, CreatedAt: m.CreatedAt.UTC(),
	}
}

func fromEffect(e domain.Effect) (ruleExecutionEffect, error) {
	params, err := json.Marshal(e.Params)
	if err != nil {
		return ruleExecutionEffect{}, err
	}
	return ruleExecutionEffect{
		ID: e.ID, TenantID: e.TenantID, DecisionID: e.DecisionID, ExecutionID: e.ExecutionID, RuleID: e.RuleID,
		RuleVersionID: e.RuleVersionID, PlayerID: ptr(e.PlayerID), ActionIndex: e.ActionIndex,
		IdempotencyKey: e.IdempotencyKey, Type: e.Type, Params: string(params), Target: e.Target,
		Command: jsonText(e.Command, "{}"), Status: e.Status, Reason: e.Reason, Attempts: e.Attempts,
		RequestedAt: e.RequestedAt, SettledAt: e.SettledAt,
	}, nil
}

func (m ruleExecutionEffect) toDomain() domain.Effect {
	var params map[string]any
	_ = json.Unmarshal([]byte(m.Params), &params)
	return domain.Effect{
		ID: m.ID, TenantID: m.TenantID, DecisionID: m.DecisionID, ExecutionID: m.ExecutionID, RuleID: m.RuleID,
		RuleVersionID: m.RuleVersionID, PlayerID: val(m.PlayerID), ActionIndex: m.ActionIndex,
		IdempotencyKey: m.IdempotencyKey, Type: m.Type, Params: params, Target: m.Target,
		Command: []byte(m.Command), Status: m.Status, Reason: m.Reason, Attempts: m.Attempts,
		RequestedAt: m.RequestedAt.UTC(), SettledAt: m.SettledAt,
	}
}
