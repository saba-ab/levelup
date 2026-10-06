package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/rules/contracts"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/shared/errs"
)

// OptString is a PATCH field that distinguishes "absent" from "null".
type OptString struct {
	Set   bool
	Null  bool
	Value string
}

// CreateRuleInput is a new rule plus its first (draft) version.
type CreateRuleInput struct {
	Slug         string
	Name         string
	Description  string
	TriggerEvent string
	ProgramID    string
	Priority     int
	Conditions   json.RawMessage
	Actions      json.RawMessage
	Limits       json.RawMessage
	// Schedule (JSON object or null) and StopProcessing are version parts.
	Schedule       json.RawMessage
	StopProcessing bool
}

// UpdateRuleInput is a partial update. Definition parts (Conditions,
// Actions, Limits, Schedule, StopProcessing; nil = untouched) edit the
// latest version and only while it is a draft: published versions are
// immutable.
type UpdateRuleInput struct {
	Name           *string
	Description    OptString
	TriggerEvent   *string
	ProgramID      OptString
	Priority       *int
	Status         *string
	Conditions     json.RawMessage
	Actions        json.RawMessage
	Limits         json.RawMessage
	Schedule       json.RawMessage
	StopProcessing *bool
}

func (in UpdateRuleInput) definitionPatch() domain.DefinitionPatch {
	return domain.DefinitionPatch{Conditions: in.Conditions, Actions: in.Actions, Limits: in.Limits,
		Schedule: in.Schedule, StopProcessing: in.StopProcessing}
}

// CreateVersionInput creates a new draft. Parts left nil are copied from
// FromVersion (or the latest version when FromVersion is nil).
type CreateVersionInput struct {
	FromVersion    *int
	Conditions     json.RawMessage
	Actions        json.RawMessage
	Limits         json.RawMessage
	Schedule       json.RawMessage
	StopProcessing *bool
}

// RuleView is a rule with its live and latest versions (fixes B11: the
// definition is always returned).
type RuleView struct {
	Rule    domain.Rule
	Current *domain.Version
	Latest  *domain.Version
}

func (s *Service) CreateRule(ctx context.Context, in CreateRuleInput) (RuleView, error) {
	p, err := s.authorize(ctx, contracts.PermCreate)
	if err != nil {
		return RuleView{}, err
	}
	now := s.clock.Now()
	rule, err := domain.NewRule(domain.NewRuleParams{
		TenantID: p.TenantID, Slug: in.Slug, Name: in.Name, Description: in.Description,
		TriggerEvent: in.TriggerEvent, ProgramID: in.ProgramID, Priority: in.Priority,
	}, now)
	if err != nil {
		return RuleView{}, err
	}
	v, err := domain.NewVersion(rule, 1, domain.Definition{Conditions: in.Conditions, Actions: in.Actions,
		Limits: in.Limits, Schedule: in.Schedule, StopProcessing: in.StopProcessing}, p.UserID, s.cfg.Compile, now)
	if err != nil {
		return RuleView{}, err
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.CreateRule(ctx, tx, rule); err != nil {
			return err
		}
		return s.repo.CreateVersion(ctx, tx, v)
	})
	if err != nil {
		return RuleView{}, err
	}
	return RuleView{Rule: rule, Latest: &v}, nil
}

func (s *Service) GetRule(ctx context.Context, ruleID string) (RuleView, error) {
	p, err := s.authorize(ctx, contracts.PermView)
	if err != nil {
		return RuleView{}, err
	}
	rule, err := s.repo.RuleByID(ctx, p.TenantID, ruleID)
	if err != nil {
		return RuleView{}, err
	}
	return s.view(ctx, rule)
}

func (s *Service) view(ctx context.Context, rule domain.Rule) (RuleView, error) {
	out := RuleView{Rule: rule}
	latest, err := s.repo.LatestVersion(ctx, rule.TenantID, rule.ID)
	if err != nil && errs.KindOf(err) != errs.NotFound {
		return RuleView{}, err
	}
	if err == nil {
		out.Latest = &latest
	}
	if rule.CurrentVersionID != "" {
		if out.Latest != nil && out.Latest.ID == rule.CurrentVersionID {
			out.Current = out.Latest
		} else {
			vs, err := s.repo.VersionsByIDs(ctx, rule.TenantID, []string{rule.CurrentVersionID})
			if err != nil {
				return RuleView{}, err
			}
			if v, ok := vs[rule.CurrentVersionID]; ok {
				out.Current = &v
			}
		}
	}
	return out, nil
}

func (s *Service) ListRules(ctx context.Context, f RuleFilter, cursor string, limit int) ([]domain.Rule, string, error) {
	p, err := s.authorize(ctx, contracts.PermViewAny)
	if err != nil {
		return nil, "", err
	}
	page, err := pageFrom(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.repo.ListRules(ctx, p.TenantID, f, page)
	if err != nil {
		return nil, "", err
	}
	rows, next := trimPage(rows, page, func(r domain.Rule) (time.Time, string) { return r.CreatedAt, r.ID })
	return rows, next, nil
}

func (s *Service) UpdateRule(ctx context.Context, ruleID string, in UpdateRuleInput) (RuleView, error) {
	p, err := s.authorize(ctx, contracts.PermUpdate)
	if err != nil {
		return RuleView{}, err
	}
	now := s.clock.Now()
	var (
		old, rule domain.Rule
		gen       int64
	)
	err = s.tx(ctx, func(tx *gorm.DB) error {
		var err error
		rule, err = s.repo.RuleForUpdate(ctx, tx, p.TenantID, ruleID)
		if err != nil {
			return err
		}
		old = rule
		if rule.Status == domain.StatusArchived && !onlyArchive(in) {
			return domain.ErrRuleArchived
		}
		if in.Name != nil {
			rule.Name = strings.TrimSpace(*in.Name)
		}
		if in.Description.Set {
			rule.Description = in.Description.Value
		}
		if in.TriggerEvent != nil {
			rule.TriggerEvent = *in.TriggerEvent
		}
		if in.ProgramID.Set {
			rule.ProgramID = strings.ToLower(in.ProgramID.Value)
		}
		if in.Priority != nil {
			rule.Priority = *in.Priority
		}
		if err := rule.Validate(); err != nil {
			return err
		}
		if in.Status != nil {
			if err := rule.SetStatus(*in.Status, now); err != nil {
				return err
			}
		}
		if patch := in.definitionPatch(); !patch.IsZero() {
			latest, err := s.repo.LatestVersionTx(ctx, tx, p.TenantID, rule.ID)
			if err != nil {
				return err
			}
			if latest.Published() {
				return domain.ErrNoDraft
			}
			if err := latest.Edit(patch, s.cfg.Compile); err != nil {
				return err
			}
			if err := s.repo.SaveDraftVersion(ctx, tx, latest); err != nil {
				return err
			}
		}
		rule.UpdatedAt = now
		if err := s.repo.SaveRule(ctx, tx, rule); err != nil {
			return err
		}
		if rule.AffectsRuleset(old) {
			gen, err = s.repo.BumpGeneration(ctx, tx, p.TenantID)
			return err
		}
		return nil
	})
	if err != nil {
		return RuleView{}, err
	}
	s.evictRuleset(ctx, p.TenantID, gen, triggersOf(old, rule)...)
	return s.view(ctx, rule)
}

func onlyArchive(in UpdateRuleInput) bool {
	return in.Status != nil && *in.Status == domain.StatusArchived &&
		in.Name == nil && !in.Description.Set && in.TriggerEvent == nil && !in.ProgramID.Set &&
		in.Priority == nil && in.definitionPatch().IsZero()
}

func triggersOf(a, b domain.Rule) []string {
	if a.TriggerEvent == b.TriggerEvent {
		return []string{a.TriggerEvent}
	}
	return []string{a.TriggerEvent, b.TriggerEvent}
}

func (s *Service) DeleteRule(ctx context.Context, ruleID string) error {
	p, err := s.authorize(ctx, contracts.PermDelete)
	if err != nil {
		return err
	}
	var (
		rule domain.Rule
		gen  int64
	)
	err = s.tx(ctx, func(tx *gorm.DB) error {
		var err error
		rule, err = s.repo.RuleForUpdate(ctx, tx, p.TenantID, ruleID)
		if err != nil {
			return err
		}
		old := rule
		rule.Delete(s.clock.Now())
		if err := s.repo.SaveRule(ctx, tx, rule); err != nil {
			return err
		}
		if rule.AffectsRuleset(old) {
			gen, err = s.repo.BumpGeneration(ctx, tx, p.TenantID)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.evictRuleset(ctx, p.TenantID, gen, rule.TriggerEvent)
	return nil
}

func (s *Service) CreateVersion(ctx context.Context, ruleID string, in CreateVersionInput) (domain.Version, error) {
	p, err := s.authorize(ctx, contracts.PermUpdate)
	if err != nil {
		return domain.Version{}, err
	}
	now := s.clock.Now()
	var v domain.Version
	err = s.tx(ctx, func(tx *gorm.DB) error {
		rule, err := s.repo.RuleForUpdate(ctx, tx, p.TenantID, ruleID)
		if err != nil {
			return err
		}
		if rule.Status == domain.StatusArchived {
			return domain.ErrRuleArchived
		}
		latest, err := s.repo.LatestVersionTx(ctx, tx, p.TenantID, rule.ID)
		if err != nil {
			return err
		}
		base := latest
		if in.FromVersion != nil {
			if base, err = s.repo.VersionByNumberTx(ctx, tx, p.TenantID, rule.ID, *in.FromVersion); err != nil {
				return err
			}
		}
		def := domain.DefinitionPatch{Conditions: in.Conditions, Actions: in.Actions, Limits: in.Limits,
			Schedule: in.Schedule, StopProcessing: in.StopProcessing}.Apply(base.Definition())
		v, err = domain.NewVersion(rule, latest.Version+1, def, p.UserID, s.cfg.Compile, now)
		if err != nil {
			return err
		}
		return s.repo.CreateVersion(ctx, tx, v)
	})
	if err != nil {
		return domain.Version{}, err
	}
	return v, nil
}

func (s *Service) ListVersions(ctx context.Context, ruleID, cursor string, limit int) ([]domain.Version, string, error) {
	p, err := s.authorize(ctx, contracts.PermView)
	if err != nil {
		return nil, "", err
	}
	if _, err := s.repo.RuleByID(ctx, p.TenantID, ruleID); err != nil {
		return nil, "", err
	}
	page, err := pageFrom(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.repo.ListVersions(ctx, p.TenantID, ruleID, page)
	if err != nil {
		return nil, "", err
	}
	rows, next := trimPage(rows, page, func(v domain.Version) (time.Time, string) { return v.CreatedAt, v.ID })
	return rows, next, nil
}

// Publish makes version the rule's one live version, atomically: compile,
// set current_version_id + status active, stamp published_at, bump the
// ruleset generation and record rules.version_published.v1 in ONE tx.
// Publishing the version that is already live is a no-op (idempotent).
func (s *Service) Publish(ctx context.Context, ruleID string, version int) (RuleView, error) {
	p, err := s.authorize(ctx, contracts.PermPublish)
	if err != nil {
		return RuleView{}, err
	}
	now := s.clock.Now()
	var (
		rule domain.Rule
		gen  int64
	)
	err = s.tx(ctx, func(tx *gorm.DB) error {
		var err error
		rule, err = s.repo.RuleForUpdate(ctx, tx, p.TenantID, ruleID)
		if err != nil {
			return err
		}
		v, err := s.repo.VersionByNumberTx(ctx, tx, p.TenantID, rule.ID, version)
		if err != nil {
			return err
		}
		if rule.CurrentVersionID == v.ID && rule.Status == domain.StatusActive {
			return nil
		}
		// Re-validate under today's grammar: a draft written under a looser
		// limit must not go live.
		if _, _, err := domain.ValidateDefinition(v.Definition(), s.cfg.Compile); err != nil {
			return err
		}
		if err := rule.Publish(v, now); err != nil {
			return err
		}
		v.MarkPublished(now)
		if err := s.repo.PublishVersion(ctx, tx, v); err != nil {
			return err
		}
		if err := s.repo.SaveRule(ctx, tx, rule); err != nil {
			return err
		}
		if gen, err = s.repo.BumpGeneration(ctx, tx, p.TenantID); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicVersionPublished, contracts.VersionPublishedV1{
			RuleID:        rule.ID,
			RuleVersionID: v.ID,
			TenantID:      rule.TenantID,
			Version:       v.Version,
			TriggerEvent:  rule.TriggerEvent,
			At:            now,
		})
	})
	if err != nil {
		return RuleView{}, err
	}
	s.evictRuleset(ctx, p.TenantID, gen, rule.TriggerEvent)
	return s.view(ctx, rule)
}
