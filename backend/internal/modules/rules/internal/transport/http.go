// Package transport is rules' HTTP layer: DTO shape validation here,
// invariants and grammar in the domain, authorization in the service.
package transport

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"

	"levelup/internal/modules/rules/contracts"
	"levelup/internal/modules/rules/internal/app"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/validate"
)

type Handler struct {
	svc *app.Service
	val *validate.Validator
}

func NewHandler(svc *app.Service, val *validate.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/rules", func(r chi.Router) {
		// Deprecated Laravel ingest path: answered for everyone, no auth.
		r.Post("/execute", h.execute)

		r.Group(func(r chi.Router) {
			r.Use(httpx.RequireAuth)
			r.Get("/", h.list)
			r.Post("/", h.create)
			r.Post("/simulate", h.simulate)
			r.Get("/stats", h.stats)
			r.Get("/decisions", h.listDecisions)
			r.Get("/decisions/{id}", h.getDecision)
			r.Get("/{id}", h.get)
			r.Patch("/{id}", h.update)
			r.Delete("/{id}", h.delete)
			r.Get("/{id}/versions", h.listVersions)
			r.Post("/{id}/versions", h.createVersion)
			r.Post("/{id}/publish", h.publish)
		})
	})
}

func limitParam(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return n
}

// @Summary      List rules
// @Tags         rules
// @Produce      json
// @Security     BearerAuth
// @Param        status        query string false "draft | active | inactive | archived"
// @Param        trigger_event query string false "Trigger event slug"
// @Param        program_id    query string false "Program id"
// @Param        limit         query int    false "Page size (default 25, max 100)"
// @Param        cursor        query string false "Opaque cursor"
// @Success      200 {object} RuleListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /rules [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rows, next, err := h.svc.ListRules(r.Context(), app.RuleFilter{
		Status: q.Get("status"), TriggerEvent: q.Get("trigger_event"), ProgramID: q.Get("program_id"),
	}, q.Get("cursor"), limitParam(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := RuleListResp{Data: make([]RuleResp, len(rows)), NextCursor: next}
	for i, rule := range rows {
		out.Data[i] = toRule(rule)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a rule (draft, version 1)
// @Description  The definition is compiled at write time: an unknown source, field, operator, action or parameter is 422 invalid_rule_definition with per-field errors.
// @Tags         rules
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateRuleReq true "Rule"
// @Success      201 {object} RuleResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rules [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRuleReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	view, err := h.svc.CreateRule(r.Context(), app.CreateRuleInput{
		Slug: req.Slug, Name: req.Name, Description: req.Description, TriggerEvent: req.TriggerEvent,
		ProgramID: req.ProgramID, Priority: req.Priority, Conditions: req.Conditions, Actions: req.Actions, Limits: req.Limits,
		Schedule: req.Schedule, StopProcessing: req.StopProcessing,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toRuleView(view))
}

// @Summary      Get a rule with its current and latest versions
// @Tags         rules
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Rule id"
// @Success      200 {object} RuleResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /rules/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.GetRule(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toRuleView(view))
}

// @Summary      Update a rule
// @Description  Partial update. conditions/actions/limits/schedule/stop_processing edit the latest version only while it is a draft (409 no_draft_version otherwise: published versions are immutable). status may be active (needs a published version), inactive or archived.
// @Tags         rules
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string        true "Rule id"
// @Param        body body UpdateRuleReq true "Fields to change"
// @Success      200 {object} RuleResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rules/{id} [patch]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateRuleReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if req.Name != nil && *req.Name == "" {
		httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "validation failed"), map[string]string{"name": "must not be empty"}))
		return
	}
	view, err := h.svc.UpdateRule(r.Context(), chi.URLParam(r, "id"), app.UpdateRuleInput{
		Name: req.Name, Description: req.Description.opt(), TriggerEvent: req.TriggerEvent,
		ProgramID: req.ProgramID.opt(), Priority: req.Priority, Status: req.Status,
		Conditions: req.Conditions, Actions: req.Actions, Limits: req.Limits,
		Schedule: req.Schedule, StopProcessing: req.StopProcessing,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toRuleView(view))
}

// @Summary      Delete a rule (soft)
// @Tags         rules
// @Security     BearerAuth
// @Param        id path string true "Rule id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /rules/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteRule(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      List a rule's versions (newest first)
// @Tags         rules
// @Produce      json
// @Security     BearerAuth
// @Param        id     path  string true  "Rule id"
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor"
// @Success      200 {object} VersionListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /rules/{id}/versions [get]
func (h *Handler) listVersions(w http.ResponseWriter, r *http.Request) {
	rows, next, err := h.svc.ListVersions(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("cursor"), limitParam(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := VersionListResp{Data: make([]VersionResp, len(rows)), NextCursor: next}
	for i := range rows {
		out.Data[i] = *toVersion(&rows[i])
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a new draft version
// @Tags         rules
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string           true "Rule id"
// @Param        body body CreateVersionReq true "Definition parts (omitted parts are copied)"
// @Success      201 {object} VersionResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rules/{id}/versions [post]
func (h *Handler) createVersion(w http.ResponseWriter, r *http.Request) {
	var req CreateVersionReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	v, err := h.svc.CreateVersion(r.Context(), chi.URLParam(r, "id"), app.CreateVersionInput{
		FromVersion: req.FromVersion, Conditions: req.Conditions, Actions: req.Actions, Limits: req.Limits,
		Schedule: req.Schedule, StopProcessing: req.StopProcessing,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toVersion(&v))
}

// @Summary      Publish a version
// @Description  Atomically makes the version the rule's ONE live version (status active), stamps published_at, bumps the ruleset generation and records rules.version_published.v1. Publishing the live version again is a no-op.
// @Tags         rules
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string     true "Rule id"
// @Param        body body PublishReq true "Version number"
// @Success      200 {object} RuleResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rules/{id}/publish [post]
func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	var req PublishReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	view, err := h.svc.Publish(r.Context(), chi.URLParam(r, "id"), req.Version)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toRuleView(view))
}

// @Summary      Simulate an activity against the live ruleset or a draft definition
// @Description  Synchronous evaluation with no writes: no decision, no counters, no history, no effects. Limits are reported, not enforced (stop_processing treats every match as a firing). With definition, only that unpublished body is evaluated (event_type defaults to definition.trigger_event); compile errors are 422 invalid_rule_definition with field errors under definition.*. occurred_at (default now) drives schedules and history windows; history facts are loaded for stored players only.
// @Tags         rules
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body SimulateReq true "Hypothetical activity"
// @Success      200 {object} SimulateResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rules/simulate [post]
func (h *Handler) simulate(w http.ResponseWriter, r *http.Request) {
	var req SimulateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	in := app.SimulateInput{
		EventType: req.EventType, PlayerID: req.PlayerID, PlayerExternalID: req.PlayerExternalID,
		Properties: req.Properties, Context: req.Context, CausationDepth: req.CausationDepth, OccurredAt: req.OccurredAt,
	}
	if d := req.Definition; d != nil {
		in.Definition = &app.DraftDefinition{TriggerEvent: d.TriggerEvent, Definition: domain.Definition{
			Conditions: d.Conditions, Actions: d.Actions, Limits: d.Limits, Schedule: d.Schedule, StopProcessing: d.StopProcessing,
		}}
	}
	if req.Player != nil {
		in.Player = &app.SimPlayer{ExternalID: req.Player.ExternalID, IsActive: req.Player.IsActive,
			Attributes: req.Player.Attributes, Level: req.Player.Level, XP: req.Player.XP, Points: req.Player.Points}
	}
	res, err := h.svc.Simulate(r.Context(), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toSimulate(res))
}

// @Summary      Rule statistics
// @Description  Per rule over [from, to): executions by status (fired, not_matched, limited, out_of_schedule, skipped_by_stop), effects applied/rejected, and points_awarded / xp_awarded (sums of credit_points / grant_xp amounts of fired executions, whatever their settlement), plus totals. Rules are ordered by fired DESC. Defaults: to = now, from = to - 30 days; the range is at most 366 days.
// @Tags         rules
// @Produce      json
// @Security     BearerAuth
// @Param        from query string false "RFC 3339 timestamp or YYYY-MM-DD (UTC midnight)"
// @Param        to   query string false "RFC 3339 timestamp or YYYY-MM-DD (UTC midnight), exclusive"
// @Success      200 {object} RuleStatsResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rules/stats [get]
func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, err := timeParam(q.Get("from"), "from")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	to, err := timeParam(q.Get("to"), "to")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Stats(r.Context(), from, to)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toStats(res))
}

// timeParam parses an optional RFC 3339 timestamp or a YYYY-MM-DD date.
func timeParam(v, name string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if t, err := time.Parse(layout, v); err == nil {
			return &t, nil
		}
	}
	return nil, errs.WithFields(errs.New(errs.Invalid, name+" must be an RFC 3339 timestamp or YYYY-MM-DD"),
		map[string]string{name: "must be an RFC 3339 timestamp or YYYY-MM-DD"})
}

// @Summary      List decisions
// @Tags         rules
// @Produce      json
// @Security     BearerAuth
// @Param        activity_id query string false "Activity id"
// @Param        player_id   query string false "Player id"
// @Param        limit       query int    false "Page size (default 25, max 100)"
// @Param        cursor      query string false "Opaque cursor"
// @Success      200 {object} DecisionListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /rules/decisions [get]
func (h *Handler) listDecisions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rows, next, err := h.svc.ListDecisions(r.Context(), app.DecisionFilter{
		ActivityID: q.Get("activity_id"), PlayerID: q.Get("player_id"),
	}, q.Get("cursor"), limitParam(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := DecisionListResp{Data: make([]DecisionResp, len(rows)), NextCursor: next}
	for i, d := range rows {
		out.Data[i] = toDecision(d)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Get a decision with its executions and effects
// @Tags         rules
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Decision id"
// @Success      200 {object} DecisionDetailResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /rules/decisions/{id} [get]
func (h *Handler) getDecision(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.GetDecision(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toDecisionDetail(d))
}

// @Summary      Deprecated: synchronous rule execution
// @Description  Removed. Report activities to POST /api/v1/activities (202, idempotent on event_id); preview outcomes with POST /api/v1/rules/simulate.
// @Tags         rules
// @Produce      json
// @Failure      410 {object} httpx.Problem
// @Router       /rules/execute [post]
// @Deprecated
func (h *Handler) execute(w http.ResponseWriter, r *http.Request) {
	p := httpx.Problem{
		Type:   "about:blank",
		Title:  "gone",
		Status: http.StatusGone,
		Detail: "POST /api/v1/rules/execute was removed: report activities to POST /api/v1/activities " +
			"(asynchronous, idempotent on event_id) and preview outcomes with POST /api/v1/rules/simulate",
		Code: contracts.CodeEndpointGone,
	}
	if sc := trace.SpanContextFromContext(r.Context()); sc.HasTraceID() {
		p.TraceID = sc.TraceID().String()
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Link", `</api/v1/activities>; rel="successor-version"`)
	w.WriteHeader(http.StatusGone)
	_ = json.NewEncoder(w).Encode(p)
}
