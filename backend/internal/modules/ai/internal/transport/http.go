// Package transport is ai's HTTP layer: DTO shape validation here, request
// invariants in the domain, quota and model call in the service.
package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"

	"levelup/internal/modules/ai/contracts"
	"levelup/internal/modules/ai/internal/app"
	"levelup/internal/modules/ai/internal/domain"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/validate"
)

type Handler struct {
	svc *app.Service
	val *validate.Validator
	now func() time.Time
}

func NewHandler(svc *app.Service, val *validate.Validator) *Handler {
	return &Handler{svc: svc, val: val, now: time.Now}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/ai", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Post("/drafts", h.draft)
		r.Get("/templates", h.templates)
		r.Get("/usage", h.usage)
	})
}

// ---- DTOs ----

type EventTypeDTO struct {
	Slug        string `json:"slug" validate:"required,max=100"`
	Name        string `json:"name" validate:"max=255"`
	Description string `json:"description" validate:"max=500"`
}

type RefDTO struct {
	ID   string `json:"id" validate:"required,uuid"`
	Name string `json:"name" validate:"max=255"`
}

type LevelDTO struct {
	LevelNumber int    `json:"level_number" validate:"required,min=1"`
	Name        string `json:"name" validate:"max=255"`
	XPRequired  int64  `json:"xp_required" validate:"min=0"`
}

// ContextDTO is what the portal already knows about the tenant. It is
// prompt data and the allow-list of ids drafts may reference; the ai module
// never reads other modules itself.
type ContextDTO struct {
	EventTypes []EventTypeDTO `json:"event_types" validate:"max=200,dive"`
	Badges     []RefDTO       `json:"badges" validate:"max=200,dive"`
	Missions   []RefDTO       `json:"missions" validate:"max=200,dive"`
	Rewards    []RefDTO       `json:"rewards" validate:"max=200,dive"`
	Levels     []LevelDTO     `json:"levels" validate:"max=100,dive"`
}

type DraftReq struct {
	Kind    string      `json:"kind" validate:"required,oneof=badge level mission reward rule segment"`
	Prompt  string      `json:"prompt" validate:"required,max=2000"`
	Count   int         `json:"count" validate:"omitempty,min=1,max=5"` // default 3
	Context *ContextDTO `json:"context"`
}

type RejectionResp struct {
	Index   int      `json:"index"`
	Reasons []string `json:"reasons"`
}

type TokenUsageResp struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

type QuotaResp struct {
	DailyLimit int   `json:"daily_limit"` // 0 = unlimited
	Used       int64 `json:"used"`
	Remaining  int64 `json:"remaining"` // -1 = unlimited
}

// DraftResp carries drafts that passed validation. Each draft is exactly the
// JSON body of the owning module's create endpoint (POST /badges, /levels,
// /missions, /rewards, /rules, /segments).
type DraftResp struct {
	Kind     string           `json:"kind"`
	Model    string           `json:"model"`
	Drafts   []map[string]any `json:"drafts" swaggertype:"array,object"`
	Rejected []RejectionResp  `json:"rejected"`
	Usage    TokenUsageResp   `json:"usage"`
	Quota    QuotaResp        `json:"quota"`
}

type TemplateResp struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	Description   string `json:"description"`
	ExamplePrompt string `json:"example_prompt"`
	DefaultCount  int    `json:"default_count"`
}

type TemplateListResp struct {
	Data       []TemplateResp `json:"data"`
	NextCursor string         `json:"next_cursor"`
}

type UsageDayResp struct {
	Day          string `json:"day"` // YYYY-MM-DD (UTC)
	Requests     int64  `json:"requests"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

type UsageResp struct {
	Enabled    bool           `json:"enabled"`
	Model      string         `json:"model"`
	DailyLimit int            `json:"daily_limit"` // 0 = unlimited
	Remaining  int64          `json:"remaining"`   // -1 = unlimited
	Today      UsageDayResp   `json:"today"`
	Data       []UsageDayResp `json:"data"` // newest first; days without usage omitted
}

// ---- handlers ----

// @Summary      Draft gamification entities with AI
// @Description  Returns up to count validated drafts shaped exactly like the owning module's create request. Drafts are not persisted. Counts against the tenant's daily AI request limit.
// @Tags         ai
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body DraftReq true "Draft request"
// @Success      200 {object} DraftResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem "invalid input, ai_refused, ai_output_truncated, invalid_context"
// @Failure      429 {object} httpx.Problem "ai_quota_exceeded"
// @Failure      503 {object} httpx.Problem "ai_not_configured, ai_unavailable, ai_bad_output"
// @Router       /ai/drafts [post]
func (h *Handler) draft(w http.ResponseWriter, r *http.Request) {
	var req DraftReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	out, err := h.svc.Draft(r.Context(), req.Kind, req.Prompt, req.Count, toContext(req.Context))
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	resp := DraftResp{
		Kind:     out.Kind,
		Model:    out.Model,
		Drafts:   out.Drafts,
		Rejected: make([]RejectionResp, len(out.Rejected)),
		Usage:    TokenUsageResp{InputTokens: out.InputTokens, OutputTokens: out.OutputTokens},
		Quota:    QuotaResp{DailyLimit: max(out.Quota.DailyLimit, 0), Used: out.Quota.Used, Remaining: out.Quota.Remaining},
	}
	for i, rj := range out.Rejected {
		resp.Rejected[i] = RejectionResp{Index: rj.Index, Reasons: rj.Reasons}
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// @Summary      List AI Hub prompt templates
// @Tags         ai
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} TemplateListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /ai/templates [get]
func (h *Handler) templates(w http.ResponseWriter, r *http.Request) {
	ts, err := h.svc.Templates(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := TemplateListResp{Data: make([]TemplateResp, len(ts))}
	for i, t := range ts {
		out.Data[i] = TemplateResp{
			ID: t.ID, Name: t.Name, Kind: t.Kind, Description: t.Description,
			ExamplePrompt: t.ExamplePrompt, DefaultCount: t.DefaultCount,
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      AI usage and daily quota of the current tenant
// @Tags         ai
// @Produce      json
// @Security     BearerAuth
// @Param        days query int false "History window in days, today included (default 30, max 90)"
// @Success      200 {object} UsageResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /ai/usage [get]
func (h *Handler) usage(w http.ResponseWriter, r *http.Request) {
	days := 0
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			httpx.Error(w, r, errs.WithCode(errs.New(errs.Invalid, "days must be an integer"), "invalid_days"))
			return
		}
		days = n
	}
	rep, err := h.svc.Usage(r.Context(), days)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := UsageResp{
		Enabled:    rep.Enabled,
		Model:      rep.Model,
		DailyLimit: max(rep.DailyLimit, 0),
		Remaining:  rep.Remaining,
		Today:      toUsageDay(rep.Today),
		Data:       make([]UsageDayResp, len(rep.Days)),
	}
	for i, d := range rep.Days {
		out.Data[i] = toUsageDay(d)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// unavailableDetail is the fixed, safe detail of each 503 code: httpx.Error
// drops codes on 5xx, but clients must tell "not configured" from "try
// again", so these are rendered here without echoing the provider's cause.
var unavailableDetail = map[string]string{
	contracts.CodeNotConfigured: "AI drafting is not configured",
	contracts.CodeUnavailable:   "the AI provider is temporarily unavailable; try again",
	contracts.CodeBadOutput:     "the AI returned an unreadable response; try again",
}

// renderError renders the quota error as 429 with Retry-After until the next
// UTC day (errs has no rate-limit kind) and the module's 503s with their
// code; everything else goes through httpx.Error.
func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, err error) {
	code := errs.CodeOf(err)
	switch {
	case errors.Is(err, domain.ErrQuotaExceeded):
		now := h.now().UTC()
		reset := domain.DayOf(now).AddDate(0, 0, 1)
		w.Header().Set("Retry-After", strconv.Itoa(int(reset.Sub(now).Seconds())+1))
		writeProblem(w, r, http.StatusTooManyRequests, "too_many_requests", err.Error(), code)
	case errs.KindOf(err) == errs.Unavailable && unavailableDetail[code] != "":
		writeProblem(w, r, http.StatusServiceUnavailable, errs.Unavailable.String(), unavailableDetail[code], code)
	default:
		httpx.Error(w, r, err)
	}
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail, code string) {
	p := httpx.Problem{Type: "about:blank", Title: title, Status: status, Detail: detail, Code: code}
	if sc := trace.SpanContextFromContext(r.Context()); sc.HasTraceID() {
		p.TraceID = sc.TraceID().String()
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

func toContext(c *ContextDTO) domain.Context {
	if c == nil {
		return domain.Context{}
	}
	out := domain.Context{}
	for _, e := range c.EventTypes {
		out.EventTypes = append(out.EventTypes, domain.EventTypeRef(e))
	}
	refs := func(in []RefDTO) []domain.EntityRef {
		var res []domain.EntityRef
		for _, r := range in {
			res = append(res, domain.EntityRef(r))
		}
		return res
	}
	out.Badges, out.Missions, out.Rewards = refs(c.Badges), refs(c.Missions), refs(c.Rewards)
	for _, l := range c.Levels {
		out.Levels = append(out.Levels, domain.LevelRef(l))
	}
	return out
}

func toUsageDay(d domain.UsageDay) UsageDayResp {
	return UsageDayResp{
		Day:          d.Day.Format(time.DateOnly),
		Requests:     d.Requests,
		InputTokens:  d.InputTokens,
		OutputTokens: d.OutputTokens,
	}
}
