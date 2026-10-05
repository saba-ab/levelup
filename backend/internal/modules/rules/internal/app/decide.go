package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	activitycontracts "levelup/internal/modules/activity/contracts"
	badgescontracts "levelup/internal/modules/badges/contracts"
	missionscontracts "levelup/internal/modules/missions/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	rewardscontracts "levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rules/contracts"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/modules/rules/internal/ports"
	streakscontracts "levelup/internal/modules/streaks/contracts"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
)

// Decide turns one activity into one decision (flow F2, doc 06 §11.7).
//
//   - Idempotent: the decision id is derived from the activity id and
//     decisions.activity_id is UNIQUE; a redelivery finds the row and does
//     nothing, and a racing delivery loses the INSERT ... ON CONFLICT and
//     publishes nothing. Effect keys are derived, so even a delivery whose
//     first attempt crashed before commit re-issues identical keys (G28).
//   - Business outcomes (unknown or inactive player, causation depth) are
//     recorded decisions, never errors. Only transient failures return an
//     error (retry ladder); an undecodable or incomplete payload is
//     errs.Invalid (immediate DLQ).
func (s *Service) Decide(ctx context.Context, ev activitycontracts.ReceivedV1) error {
	if err := validateActivity(ev); err != nil {
		return err
	}
	exists, err := s.repo.DecisionExists(ctx, ev.ActivityID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	start := s.clock.Now()
	d := domain.Decision{
		ID:               domain.DecisionID(ev.ActivityID),
		TenantID:         ev.TenantID,
		ActivityID:       ev.ActivityID,
		EventID:          ev.EventID,
		PlayerID:         ev.PlayerID,
		PlayerExternalID: ev.PlayerExternalID,
		EventType:        ev.EventType,
		CausationDepth:   ev.CausationDepth,
		OccurredAt:       ev.OccurredAt.UTC(),
	}

	if ev.CausationDepth > s.cfg.MaxCausationDepth {
		d.Outcome, d.Reason = contracts.OutcomeRejected, contracts.ReasonCausationDepthExceeded
		return s.record(ctx, d, start, ev, eval.Result{})
	}

	player, reason, err := s.resolvePlayer(ctx, ev)
	if err != nil {
		return err
	}
	if reason != "" {
		d.Outcome, d.Reason = contracts.OutcomeRejected, reason
		return s.record(ctx, d, start, ev, eval.Result{})
	}
	d.PlayerID = player.ID
	d.PlayerExternalID = player.ExternalID

	prog, gen, err := s.ruleset(ctx, ev.TenantID, ev.EventType)
	if err != nil {
		return err
	}
	d.RulesetGeneration = gen

	facts, err := s.facts(ctx, prog, ev, player)
	if err != nil {
		return err
	}
	res := eval.Evaluate(prog, facts)
	return s.record(ctx, d, start, ev, res)
}

func validateActivity(ev activitycontracts.ReceivedV1) error {
	switch {
	case !eval.IsUUID(ev.ActivityID):
		return errs.New(errs.Invalid, "activity.received.v1: activity_id must be a UUID")
	case ev.TenantID == "":
		return errs.New(errs.Invalid, "activity.received.v1: tenant_id is required")
	case ev.EventType == "":
		return errs.New(errs.Invalid, "activity.received.v1: event_type is required")
	case ev.PlayerID == "" && ev.PlayerExternalID == "":
		return errs.New(errs.Invalid, "activity.received.v1: player_id or player_external_id is required")
	}
	return nil
}

// resolvePlayer returns the player or a rejection reason (G25/G26). A port
// failure is returned as an error so the retry ladder handles it.
func (s *Service) resolvePlayer(ctx context.Context, ev activitycontracts.ReceivedV1) (eval.Player, string, error) {
	var (
		snap  ports.PlayerSnapshot
		found bool
	)
	if ev.PlayerID != "" {
		got, err := s.players.ByIDs(ctx, ev.TenantID, []string{ev.PlayerID})
		if err != nil {
			return eval.Player{}, "", err
		}
		p, ok := got[ev.PlayerID]
		snap, found = p, ok
	} else {
		got, err := s.players.ByExternalIDs(ctx, ev.TenantID, []string{ev.PlayerExternalID})
		if err != nil {
			return eval.Player{}, "", err
		}
		p, ok := got[ev.PlayerExternalID]
		snap, found = p, ok
	}
	// A provider must never hand back another tenant's player; treat it as unknown.
	if !found || (snap.TenantID != "" && snap.TenantID != ev.TenantID) {
		return eval.Player{}, effect.ReasonPlayerNotFound, nil
	}
	if !snap.Active {
		return eval.Player{}, effect.ReasonPlayerInactive, nil
	}
	return eval.Player{
		ID: snap.ID, ExternalID: snap.ExternalID, DisplayName: snap.DisplayName,
		Email: snap.Email, Active: snap.Active, Attributes: snap.Attributes,
	}, "", nil
}

// facts builds the single pre-activity snapshot every rule sees. Level/XP
// and balance are read only when some rule references them.
func (s *Service) facts(ctx context.Context, prog *eval.Program, ev activitycontracts.ReceivedV1, player eval.Player) (eval.Facts, error) {
	if err := s.enrich(ctx, prog, ev.TenantID, &player); err != nil {
		return eval.Facts{}, err
	}
	f := eval.Facts{
		Activity: eval.Activity{
			EventID: ev.EventID, EventType: ev.EventType,
			Properties: ev.Properties, Context: ev.Context, CausationDepth: ev.CausationDepth,
		},
		Player: &player,
	}
	if s.programs != nil && prog.ProgramScoped() {
		ids, err := s.programs.EnrolledProgramIDs(ctx, ev.TenantID, player.ID)
		if err != nil {
			return eval.Facts{}, err
		}
		f.ProgramScoping, f.EnrolledPrograms = true, ids
	}
	return f, nil
}

func (s *Service) enrich(ctx context.Context, prog *eval.Program, tenantID string, player *eval.Player) error {
	if prog.NeedsProgress() && s.progress != nil {
		got, err := s.progress.ByPlayerIDs(ctx, tenantID, []string{player.ID})
		if err != nil {
			return err
		}
		pr := got[player.ID] // absent: no XP yet → level 0, xp 0
		player.Level, player.XP, player.HasProgress = pr.Level, pr.TotalXP, true
	}
	if prog.NeedsPoints() && s.points != nil {
		got, err := s.points.ByPlayerIDs(ctx, tenantID, []string{player.ID})
		if err != nil {
			return err
		}
		player.Points, player.HasPoints = got[player.ID].Balance, true
	}
	return nil
}

// record writes the decision, its executions, limit counters and effects,
// and publishes one job command per effect plus rules.decision_made.v1 — all
// in one transaction.
func (s *Service) record(ctx context.Context, d domain.Decision, start time.Time,
	ev activitycontracts.ReceivedV1, res eval.Result) error {

	return s.tx(ctx, func(tx *gorm.DB) error {
		now := s.clock.Now()
		d.EvaluatedAt = now
		d.DurationUS = now.Sub(start).Microseconds()
		pendingOutcome := d.Outcome == ""
		if pendingOutcome {
			d.Outcome = contracts.OutcomeNoMatch
		}
		inserted, err := s.repo.InsertDecision(ctx, tx, d)
		if err != nil {
			return err
		}
		if !inserted {
			return nil // another delivery decided this activity first
		}

		var (
			execs        []domain.Execution
			effects      []domain.Effect
			matchedRules = []string{}
			published    = []contracts.EffectV1{}
			fired        int
			limited      int
		)
		for _, rr := range res.Rules {
			ex := domain.Execution{
				ID:               domain.ExecutionID(d.ID, rr.RuleVersionID),
				TenantID:         d.TenantID,
				DecisionID:       d.ID,
				RuleID:           rr.RuleID,
				RuleVersionID:    rr.RuleVersionID,
				PlayerID:         d.PlayerID,
				Matched:          rr.Matched,
				ConditionResults: rr.Trace,
				CreatedAt:        now,
			}
			switch rr.Status {
			case eval.StatusInvalid:
				ex.Status = domain.ExecInvalid
				s.log.Warn("stored rule version does not compile; skipped",
					zap.String("rule_version_id", rr.RuleVersionID), zap.String("error", rr.Error))
			case eval.StatusOutOfScope:
				ex.Status = domain.ExecOutOfScope
			case eval.StatusNotMatched:
				ex.Status = domain.ExecNotMatched
			default:
				ex.Status = domain.ExecFired
				if !rr.Limits.IsZero() {
					ok, err := s.repo.ApplyLimits(ctx, tx, LimitCheck{
						TenantID: d.TenantID, RuleID: rr.RuleID, PlayerID: d.PlayerID,
						Limits: rr.Limits, At: d.OccurredAt,
					})
					if err != nil {
						return err
					}
					if !ok {
						ex.Status = domain.ExecLimited
					}
				}
			}
			if ex.Status == domain.ExecLimited {
				limited++
			}
			if ex.Status == domain.ExecFired {
				fired++
				matchedRules = append(matchedRules, rr.RuleID)
				for i, a := range rr.Actions {
					e, cmd, err := s.effectFor(d, ev, rr, ex.ID, i, a, now)
					if err != nil {
						return err
					}
					if err := s.outbox.Publish(ctx, tx, e.Target, cmd); err != nil {
						return err
					}
					effects = append(effects, e)
					published = append(published, contracts.EffectV1{
						EffectID: e.ID, IdempotencyKey: e.IdempotencyKey, RuleID: e.RuleID,
						RuleVersionID: e.RuleVersionID, ActionIndex: i, Type: e.Type, Params: e.Params,
					})
				}
				ex.EffectsCount = len(rr.Actions)
			}
			execs = append(execs, ex)
		}

		if pendingOutcome {
			switch {
			case fired > 0:
				d.Outcome = contracts.OutcomeMatched
			case limited > 0:
				d.Outcome = contracts.OutcomeLimitReached
			default:
				d.Outcome = contracts.OutcomeNoMatch
			}
			if d.Outcome != contracts.OutcomeNoMatch {
				if err := s.repo.SetDecisionOutcome(ctx, tx, d); err != nil {
					return err
				}
			}
		}
		if err := s.repo.InsertExecutions(ctx, tx, execs); err != nil {
			return err
		}
		if err := s.repo.InsertEffects(ctx, tx, effects); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicDecisionMade, contracts.DecisionMadeV1{
			DecisionID:        d.ID,
			ActivityID:        d.ActivityID,
			TenantID:          d.TenantID,
			PlayerID:          d.PlayerID,
			EventType:         d.EventType,
			Outcome:           d.Outcome,
			Reason:            d.Reason,
			MatchedRules:      matchedRules,
			Effects:           published,
			At:                now,
			RulesetGeneration: d.RulesetGeneration,
		})
	})
}

// effectFor builds the effect row and the target module's job command.
func (s *Service) effectFor(d domain.Decision, ev activitycontracts.ReceivedV1, rr eval.RuleResult,
	executionID string, idx int, a eval.Action, now time.Time) (domain.Effect, any, error) {

	key := domain.EffectKey(d.ActivityID, rr.RuleVersionID, idx)
	effectID := domain.EffectID(d.ActivityID, rr.RuleVersionID, idx)
	src := effect.Source{Kind: effect.SourceRule, ID: effectID, ActivityID: d.ActivityID}
	occurred := ev.OccurredAt.UTC()

	var (
		topic string
		cmd   any
	)
	switch a.Type {
	case eval.ActionCreditPoints:
		desc := a.Description
		if desc == "" {
			desc = strings.TrimSpace("Rule: " + rr.Name)
		}
		topic = pointscontracts.Topic(pointscontracts.JobCredit)
		cmd = pointscontracts.CreditCmdV1{IdempotencyKey: key, TenantID: d.TenantID, PlayerID: d.PlayerID,
			Amount: a.Amount, Kind: pointscontracts.KindEarn, Description: desc, Source: src, OccurredAt: occurred}
	case eval.ActionGrantXP:
		topic = progressioncontracts.Topic(progressioncontracts.JobGrantXP)
		cmd = progressioncontracts.GrantXPCmdV1{IdempotencyKey: key, TenantID: d.TenantID, PlayerID: d.PlayerID,
			Amount: a.Amount, Description: a.Description, Source: src, OccurredAt: occurred}
	case eval.ActionAwardBadge:
		topic = badgescontracts.Topic(badgescontracts.JobAward)
		cmd = badgescontracts.AwardCmdV1{IdempotencyKey: key, TenantID: d.TenantID, PlayerID: d.PlayerID,
			BadgeID: a.BadgeID, Source: src, OccurredAt: occurred}
	case eval.ActionRecordStreak:
		topic = streakscontracts.Topic(streakscontracts.JobRecord)
		cmd = streakscontracts.RecordCmdV1{IdempotencyKey: key, TenantID: d.TenantID, PlayerID: d.PlayerID,
			StreakID: a.StreakID, ActivityKey: a.ActivityKey, Source: src, OccurredAt: occurred}
	case eval.ActionProgressMission:
		topic = missionscontracts.Topic(missionscontracts.JobProgress)
		cmd = missionscontracts.ProgressCmdV1{IdempotencyKey: key, TenantID: d.TenantID, PlayerID: d.PlayerID,
			MissionID: a.MissionID, Increment: a.Increment, Source: src, OccurredAt: occurred}
	case eval.ActionGrantReward:
		topic = rewardscontracts.Topic(rewardscontracts.JobGrant)
		cmd = rewardscontracts.GrantCmdV1{IdempotencyKey: key, TenantID: d.TenantID, PlayerID: d.PlayerID,
			RewardID: a.RewardID, Source: src, OccurredAt: occurred}
	default:
		// Unreachable: Compile rejects unknown action types.
		return domain.Effect{}, nil, errs.New(errs.Internal, "unknown action type "+a.Type)
	}
	body, err := json.Marshal(cmd)
	if err != nil {
		return domain.Effect{}, nil, errs.Wrap(errs.Internal, "marshal effect command", err)
	}
	return domain.Effect{
		ID: effectID, TenantID: d.TenantID, DecisionID: d.ID, ExecutionID: executionID,
		RuleID: rr.RuleID, RuleVersionID: rr.RuleVersionID, PlayerID: d.PlayerID, ActionIndex: idx,
		IdempotencyKey: key, Type: a.Type, Params: a.Params(), Target: topic, Command: body,
		Status: domain.EffectRequested, RequestedAt: now,
	}, cmd, nil
}
