package app

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/rules/contracts"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
)

// SimPlayer is an inline player for simulation (no lookup).
type SimPlayer struct {
	ExternalID string
	IsActive   *bool
	Attributes map[string]any
	Level      *int
	XP         *int64
	Points     *int64
}

// SimulateInput is a hypothetical activity. Player is resolved from
// PlayerID / PlayerExternalID through the port, or taken from Player inline.
type SimulateInput struct {
	EventType        string
	PlayerID         string
	PlayerExternalID string
	Player           *SimPlayer
	Properties       json.RawMessage
	Context          json.RawMessage
	CausationDepth   int
}

// SimulateResult is what the live ruleset would decide. Limits are reported
// per rule but not enforced: counters are only touched by real decisions.
type SimulateResult struct {
	Outcome           string
	Reason            string
	PlayerID          string
	RulesetGeneration int64
	Rules             []eval.RuleResult
}

// Simulate evaluates the live ruleset synchronously with no writes (the
// PRD's "sync decision", doc 06 §11.1).
func (s *Service) Simulate(ctx context.Context, in SimulateInput) (SimulateResult, error) {
	p, err := s.authorize(ctx, contracts.PermSimulate)
	if err != nil {
		return SimulateResult{}, err
	}
	props, err := decodeObject("properties", in.Properties)
	if err != nil {
		return SimulateResult{}, err
	}
	ctxProps, err := decodeObject("context", in.Context)
	if err != nil {
		return SimulateResult{}, err
	}

	prog, gen, err := s.ruleset(ctx, p.TenantID, in.EventType)
	if err != nil {
		return SimulateResult{}, err
	}
	out := SimulateResult{RulesetGeneration: gen}
	if in.CausationDepth > s.cfg.MaxCausationDepth {
		out.Outcome, out.Reason = contracts.OutcomeRejected, contracts.ReasonCausationDepthExceeded
		return out, nil
	}

	var player *eval.Player
	switch {
	case in.PlayerID != "" || in.PlayerExternalID != "":
		pl, reason, err := s.resolvePlayerForSim(ctx, p.TenantID, in)
		if err != nil {
			return SimulateResult{}, err
		}
		if reason != "" {
			out.Outcome, out.Reason = contracts.OutcomeRejected, reason
			return out, nil
		}
		if err := s.enrich(ctx, prog, p.TenantID, &pl); err != nil {
			return SimulateResult{}, err
		}
		player = &pl
	case in.Player != nil:
		player = inlinePlayer(in.Player)
	}

	facts := eval.Facts{
		Activity: eval.Activity{EventID: "simulation", EventType: in.EventType, Properties: props,
			Context: ctxProps, CausationDepth: in.CausationDepth},
		Player: player,
	}
	if s.programs != nil && prog.ProgramScoped() && player != nil && player.ID != "" {
		ids, err := s.programs.EnrolledProgramIDs(ctx, p.TenantID, player.ID)
		if err != nil {
			return SimulateResult{}, err
		}
		facts.ProgramScoping, facts.EnrolledPrograms = true, ids
	}
	res := eval.Evaluate(prog, facts)
	out.Rules = res.Rules
	if player != nil {
		out.PlayerID = player.ID
	}
	out.Outcome = contracts.OutcomeNoMatch
	if res.MatchedCount() > 0 {
		out.Outcome = contracts.OutcomeMatched
	}
	return out, nil
}

func (s *Service) resolvePlayerForSim(ctx context.Context, tenantID string, in SimulateInput) (eval.Player, string, error) {
	pl, reason, err := s.resolvePlayer(ctx, activityLike(tenantID, in))
	if err != nil {
		return eval.Player{}, "", err
	}
	if reason == effect.ReasonPlayerNotFound {
		return eval.Player{}, "", domain.ErrPlayerNotFound
	}
	return pl, reason, nil
}

func inlinePlayer(sp *SimPlayer) *eval.Player {
	pl := &eval.Player{ExternalID: sp.ExternalID, Active: true, Attributes: sp.Attributes}
	if sp.IsActive != nil {
		pl.Active = *sp.IsActive
	}
	if sp.Level != nil || sp.XP != nil {
		pl.HasProgress = true
		if sp.Level != nil {
			pl.Level = *sp.Level
		}
		if sp.XP != nil {
			pl.XP = *sp.XP
		}
	}
	if sp.Points != nil {
		pl.HasPoints, pl.Points = true, *sp.Points
	}
	return pl
}

// decodeObject decodes a JSON object keeping numbers exact (json.Number),
// exactly as the activity subscriber sees them.
func decodeObject(field string, raw json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, errs.WithFields(errs.New(errs.Invalid, field+" must be a JSON object"),
			map[string]string{field: "must be a JSON object"})
	}
	return m, nil
}

// DecisionDetail is a decision with its executions and effects.
type DecisionDetail struct {
	Decision   domain.Decision
	Executions []domain.Execution
	Effects    []domain.Effect
}

func (s *Service) ListDecisions(ctx context.Context, f DecisionFilter, cursor string, limit int) ([]domain.Decision, string, error) {
	p, err := s.authorize(ctx, contracts.PermViewDecisions)
	if err != nil {
		return nil, "", err
	}
	page, err := pageFrom(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.repo.ListDecisions(ctx, p.TenantID, f, page)
	if err != nil {
		return nil, "", err
	}
	rows, next := trimPage(rows, page, func(d domain.Decision) (time.Time, string) { return d.EvaluatedAt, d.ID })
	return rows, next, nil
}

func (s *Service) GetDecision(ctx context.Context, id string) (DecisionDetail, error) {
	p, err := s.authorize(ctx, contracts.PermViewDecisions)
	if err != nil {
		return DecisionDetail{}, err
	}
	d, err := s.repo.DecisionByID(ctx, p.TenantID, id)
	if err != nil {
		return DecisionDetail{}, err
	}
	execs, err := s.repo.ExecutionsByDecision(ctx, p.TenantID, d.ID)
	if err != nil {
		return DecisionDetail{}, err
	}
	effects, err := s.repo.EffectsByDecision(ctx, p.TenantID, d.ID)
	if err != nil {
		return DecisionDetail{}, err
	}
	return DecisionDetail{Decision: d, Executions: execs, Effects: effects}, nil
}

func activityLike(tenantID string, in SimulateInput) activitycontracts.ReceivedV1 {
	return activitycontracts.ReceivedV1{TenantID: tenantID, PlayerID: in.PlayerID, PlayerExternalID: in.PlayerExternalID}
}
