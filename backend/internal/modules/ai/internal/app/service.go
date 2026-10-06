// Package app holds ai's use cases: authorization, the daily quota, the
// model call and server-side validation of what the model returns.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/ai/contracts"
	"levelup/internal/modules/ai/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/errs"
)

const (
	DefaultUsageDays = 30
	MaxUsageDays     = 90
	recordTimeout    = 5 * time.Second
)

// Repository is implemented by internal/repo. Write methods take the tx.
type Repository interface {
	// Reserve counts one request against (tenant, day) unless the day
	// already holds limit requests (limit <= 0: unlimited). It returns the
	// day's request count after the reservation and whether it was made.
	Reserve(ctx context.Context, tx *gorm.DB, tenantID string, day time.Time, limit int, at time.Time) (int64, bool, error)
	AddTokens(ctx context.Context, tx *gorm.DB, tenantID string, day time.Time, input, output int64, at time.Time) error
	UsageBetween(ctx context.Context, tenantID string, from, to time.Time) ([]domain.UsageDay, error)
	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
}

// GenerateRequest is one structured-output model call.
type GenerateRequest struct {
	System string
	User   string
	Schema map[string]any
}

// GenerateResult is the model's JSON text plus token usage. Usage is filled
// whenever the provider reported it, also alongside an error.
type GenerateResult struct {
	JSON         []byte
	Model        string
	InputTokens  int64
	OutputTokens int64
}

// Generator calls the model. Errors carry errs kinds and contracts.Code*.
type Generator interface {
	Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error)
}

// Settings are the service's knobs, from the module Config.
type Settings struct {
	Enabled    bool
	Model      string
	DailyLimit int
}

type Service struct {
	repo  Repository
	gen   Generator
	authz authz.Enforcer
	clock clock.Clock
	log   *zap.Logger
	cfg   Settings
	db    *gorm.DB

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, gen Generator, enf authz.Enforcer, db *gorm.DB, c clock.Clock, log *zap.Logger, cfg Settings) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{repo: repo, gen: gen, authz: enf, clock: c, log: log, cfg: cfg, db: db}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

// SetTxRunner replaces the transaction runner; tests outside this package
// that run without a database pass one that calls fn(nil).
func (s *Service) SetTxRunner(run func(ctx context.Context, fn func(tx *gorm.DB) error) error) {
	s.tx = run
}

func (s *Service) guard(ctx context.Context) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermUse, nil); err != nil {
		return authz.Principal{}, err
	}
	return p, nil
}

// Quota is the tenant's request headroom for the current UTC day.
type Quota struct {
	DailyLimit int   // <= 0: unlimited
	Used       int64 // requests today, including this one
	Remaining  int64 // -1 when unlimited
}

// DraftOutput is what POST /ai/drafts returns.
type DraftOutput struct {
	Kind         string
	Model        string
	Drafts       []map[string]any
	Rejected     []domain.Rejection
	InputTokens  int64
	OutputTokens int64
	Quota        Quota
}

// Draft asks the model for drafts of one kind and returns those that pass
// validation. Nothing is persisted except usage accounting.
func (s *Service) Draft(ctx context.Context, kind, prompt string, count int, c domain.Context) (DraftOutput, error) {
	p, err := s.guard(ctx)
	if err != nil {
		return DraftOutput{}, err
	}
	if !s.cfg.Enabled {
		return DraftOutput{}, domain.ErrNotConfigured
	}
	req, err := domain.NewDraftRequest(kind, prompt, count, c)
	if err != nil {
		return DraftOutput{}, err
	}
	system, err := systemPrompt(req)
	if err != nil {
		return DraftOutput{}, errs.Wrap(errs.Internal, "build prompt", err)
	}

	now := s.clock.Now()
	day := domain.DayOf(now)
	var used int64
	var reserved bool
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		var e error
		used, reserved, e = s.repo.Reserve(ctx, tx, p.TenantID, day, s.cfg.DailyLimit, now)
		return e
	}); err != nil {
		return DraftOutput{}, err
	}
	if !reserved {
		return DraftOutput{}, domain.ErrQuotaExceeded
	}

	res, genErr := s.gen.Generate(ctx, GenerateRequest{System: system, User: userPrompt(req), Schema: draftSchema(req.Kind)})
	s.recordTokens(ctx, p.TenantID, day, res)
	if genErr != nil {
		return DraftOutput{}, genErr
	}

	var parsed struct {
		Drafts []any `json:"drafts"`
	}
	dec := json.NewDecoder(bytes.NewReader(res.JSON))
	dec.UseNumber()
	if err := dec.Decode(&parsed); err != nil {
		s.log.Warn("ai: undecodable model output", zap.Error(err), zap.String("kind", req.Kind))
		return DraftOutput{}, domain.ErrBadOutput
	}
	set := domain.NormalizeDrafts(req, parsed.Drafts)
	if len(set.Rejected) > 0 {
		s.log.Info("ai: drafts rejected", zap.String("kind", req.Kind), zap.Int("rejected", len(set.Rejected)))
	}
	return DraftOutput{
		Kind:         req.Kind,
		Model:        res.Model,
		Drafts:       set.Drafts,
		Rejected:     set.Rejected,
		InputTokens:  res.InputTokens,
		OutputTokens: res.OutputTokens,
		Quota:        Quota{DailyLimit: s.cfg.DailyLimit, Used: used, Remaining: domain.Remaining(s.cfg.DailyLimit, used)},
	}, nil
}

// recordTokens adds reported token usage. It survives a cancelled request
// context (the tokens were spent) and never fails the request.
func (s *Service) recordTokens(ctx context.Context, tenantID string, day time.Time, res GenerateResult) {
	if res.InputTokens == 0 && res.OutputTokens == 0 {
		return
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	err := s.tx(rctx, func(tx *gorm.DB) error {
		return s.repo.AddTokens(rctx, tx, tenantID, day, res.InputTokens, res.OutputTokens, s.clock.Now())
	})
	if err != nil {
		s.log.Warn("ai: record token usage", zap.Error(err), zap.String("tenant_id", tenantID))
	}
}

// Templates returns the AI Hub's static prompt catalogue.
func (s *Service) Templates(ctx context.Context) ([]domain.Template, error) {
	if _, err := s.guard(ctx); err != nil {
		return nil, err
	}
	return domain.Templates, nil
}

// UsageReport is the tenant's AI consumption over the last Days days.
type UsageReport struct {
	Enabled    bool
	Model      string
	DailyLimit int
	Today      domain.UsageDay
	Remaining  int64
	Days       []domain.UsageDay // newest first, days without usage omitted
}

// Usage reports today's usage and quota plus the history of the last days
// (1..90, default 30), today included.
func (s *Service) Usage(ctx context.Context, days int) (UsageReport, error) {
	p, err := s.guard(ctx)
	if err != nil {
		return UsageReport{}, err
	}
	if days == 0 {
		days = DefaultUsageDays
	}
	if days < 1 || days > MaxUsageDays {
		return UsageReport{}, errs.WithCode(errs.New(errs.Invalid, "days must be between 1 and 90"), "invalid_days")
	}
	today := domain.DayOf(s.clock.Now())
	rows, err := s.repo.UsageBetween(ctx, p.TenantID, today.AddDate(0, 0, -(days-1)), today)
	if err != nil {
		return UsageReport{}, err
	}
	rep := UsageReport{Enabled: s.cfg.Enabled, Model: s.cfg.Model, DailyLimit: s.cfg.DailyLimit, Today: domain.UsageDay{Day: today}, Days: rows}
	for _, r := range rows {
		if r.Day.Equal(today) {
			rep.Today = r
		}
	}
	rep.Remaining = domain.Remaining(s.cfg.DailyLimit, rep.Today.Requests)
	return rep, nil
}

// PurgeTenant deletes a deleted tenant's usage rows (tenant.deleted.v1).
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	return s.tx(ctx, func(tx *gorm.DB) error { return s.repo.PurgeTenant(ctx, tx, tenantID) })
}
