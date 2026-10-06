// Package transport is leaderboards' HTTP layer: DTO shape validation here,
// invariants in the domain, authorization in the service.
package transport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"levelup/internal/modules/leaderboards/internal/app"
	"levelup/internal/modules/leaderboards/internal/domain"
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
	r.Route("/leaderboards", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
		r.Get("/{id}/entries", h.entries)
		r.Get("/{id}/players/{playerID}", h.playerRank)
		r.Post("/{id}/rebuild", h.rebuild)
	})
}

type CreateReq struct {
	Name           string `json:"name" validate:"required,max=255"`
	Slug           string `json:"slug" validate:"omitempty,max=120"`
	Description    string `json:"description" validate:"max=2000"`
	Type           string `json:"type" validate:"required,oneof=points badges missions xp activity"`
	Metric         string `json:"metric" validate:"omitempty,oneof=earned net balance count"`
	ResetFrequency string `json:"reset_frequency" validate:"omitempty,oneof=never daily weekly monthly"`
	ProgramID      string `json:"program_id" validate:"omitempty,uuid"`
	MaxEntries     int    `json:"max_entries" validate:"omitempty,min=1,max=1000"`
	IsActive       *bool  `json:"is_active"`
	// Config is required for type activity and refused for every other.
	Config *ConfigReq `json:"config"`
}

// ConfigReq configures an activity board: count matching events (value
// count, metric count) or sum a numeric property of them (value property,
// metric earned; property e.g. "amount").
type ConfigReq struct {
	EventType string `json:"event_type" validate:"required,max=100"`
	Value     string `json:"value" validate:"required,oneof=count property"`
	Property  string `json:"property" validate:"omitempty,max=100"`
}

// ConfigResp is an activity board's config; null for other types.
type ConfigResp struct {
	EventType string `json:"event_type"`
	Value     string `json:"value"`
	Property  string `json:"property,omitempty"`
}

// UpdateReq is a partial update: omitted fields stay untouched. Type,
// metric, reset frequency and program are fixed at creation.
type UpdateReq struct {
	Name        *string `json:"name" validate:"omitempty,min=1,max=255"`
	Slug        *string `json:"slug" validate:"omitempty,min=1,max=120"`
	Description *string `json:"description" validate:"omitempty,max=2000"`
	MaxEntries  *int    `json:"max_entries" validate:"omitempty,min=1,max=1000"`
	IsActive    *bool   `json:"is_active"`
}

type LeaderboardResp struct {
	ID             string      `json:"id"`
	TenantID       string      `json:"tenant_id"`
	Slug           string      `json:"slug"`
	Name           string      `json:"name"`
	Description    string      `json:"description"`
	Type           string      `json:"type"`
	Metric         string      `json:"metric"`
	ResetFrequency string      `json:"reset_frequency"`
	ProgramID      *string     `json:"program_id"`
	Config         *ConfigResp `json:"config"`
	MaxEntries     int         `json:"max_entries"`
	IsActive       bool        `json:"is_active"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

type ListResp struct {
	Data       []LeaderboardResp `json:"data"`
	NextCursor string            `json:"next_cursor"`
}

type EntryResp struct {
	Rank        int64  `json:"rank"`
	PlayerID    string `json:"player_id"`
	ExternalID  string `json:"external_id"`
	DisplayName string `json:"display_name"`
	Score       int64  `json:"score"`
}

type EntriesResp struct {
	Data        []EntryResp `json:"data"`
	NextCursor  string      `json:"next_cursor"`
	PeriodStart time.Time   `json:"period_start"`
	PeriodEnd   *time.Time  `json:"period_end"`
}

type PlayerRankResp struct {
	Entry       EntryResp   `json:"entry"`
	Neighbours  []EntryResp `json:"neighbours"`
	PeriodStart time.Time   `json:"period_start"`
	PeriodEnd   *time.Time  `json:"period_end"`
}

type RebuildResp struct {
	Periods int `json:"periods"`
	Entries int `json:"entries"`
}

func toResp(lb domain.Leaderboard) LeaderboardResp {
	out := LeaderboardResp{
		ID: lb.ID, TenantID: lb.TenantID, Slug: lb.Slug, Name: lb.Name, Description: lb.Description,
		Type: lb.Type, Metric: lb.Metric, ResetFrequency: lb.ResetFrequency, MaxEntries: lb.MaxEntries,
		IsActive: lb.Active, CreatedAt: lb.CreatedAt.UTC(), UpdatedAt: lb.UpdatedAt.UTC(),
	}
	if lb.ProgramID != "" {
		p := lb.ProgramID
		out.ProgramID = &p
	}
	if lb.Activity != nil {
		out.Config = &ConfigResp{EventType: lb.Activity.EventType, Value: lb.Activity.Value, Property: lb.Activity.Property}
	}
	return out
}

func toEntry(e app.Entry) EntryResp {
	return EntryResp{Rank: e.Rank, PlayerID: e.PlayerID, ExternalID: e.ExternalID, DisplayName: e.DisplayName, Score: e.Score}
}

func periodEnd(p domain.Period) *time.Time {
	if !p.HasEnd() {
		return nil
	}
	e := p.End.UTC()
	return &e
}

func queryInt(r *http.Request, name string, def int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, errs.WithFields(errs.New(errs.Invalid, "invalid query parameter"), map[string]string{name: "must be a non-negative integer"})
	}
	return n, nil
}

// @Summary      List leaderboards
// @Tags         leaderboards
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor"
// @Param        type   query string false "Filter by type"
// @Param        active query bool   false "Filter by active flag"
// @Success      200 {object} ListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /leaderboards [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 0)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	f := app.ListFilter{Type: r.URL.Query().Get("type")}
	if raw := r.URL.Query().Get("active"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "invalid query parameter"), map[string]string{"active": "must be a boolean"}))
			return
		}
		f.Active = &b
	}
	rows, next, err := h.svc.List(r.Context(), f, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := ListResp{Data: make([]LeaderboardResp, len(rows)), NextCursor: next}
	for i, lb := range rows {
		out.Data[i] = toResp(lb)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a leaderboard
// @Tags         leaderboards
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateReq true "Leaderboard"
// @Success      201 {object} LeaderboardResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem "leaderboard_invalid_type, leaderboard_invalid_metric, leaderboard_invalid_config"
// @Router       /leaderboards [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	in := app.CreateInput{
		Name: req.Name, Slug: req.Slug, Description: req.Description, Type: req.Type, Metric: req.Metric,
		ResetFrequency: req.ResetFrequency, ProgramID: req.ProgramID, MaxEntries: req.MaxEntries, Active: active,
	}
	if req.Config != nil {
		in.Activity = &domain.ActivityConfig{EventType: req.Config.EventType, Value: req.Config.Value, Property: req.Config.Property}
	}
	lb, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toResp(lb))
}

// @Summary      Get a leaderboard
// @Tags         leaderboards
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Leaderboard id"
// @Success      200 {object} LeaderboardResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /leaderboards/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	lb, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResp(lb))
}

// @Summary      Update a leaderboard (partial)
// @Tags         leaderboards
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string    true "Leaderboard id"
// @Param        body body UpdateReq true "Fields to change"
// @Success      200 {object} LeaderboardResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /leaderboards/{id} [patch]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	lb, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), domain.Patch{
		Name: req.Name, Slug: req.Slug, Description: req.Description, MaxEntries: req.MaxEntries, Active: req.IsActive,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResp(lb))
}

// @Summary      Delete a leaderboard
// @Tags         leaderboards
// @Security     BearerAuth
// @Param        id path string true "Leaderboard id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /leaderboards/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Ranking entries of a leaderboard period
// @Tags         leaderboards
// @Produce      json
// @Security     BearerAuth
// @Param        id     path  string true  "Leaderboard id"
// @Param        period query string false "current (default) or an RFC 3339 time inside the period"
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor"
// @Success      200 {object} EntriesResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /leaderboards/{id}/entries [get]
func (h *Handler) entries(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 0)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	page, err := h.svc.Entries(r.Context(), chi.URLParam(r, "id"), r.URL.Query().Get("period"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := EntriesResp{
		Data: make([]EntryResp, len(page.Entries)), NextCursor: page.NextCursor,
		PeriodStart: page.Period.Start.UTC(), PeriodEnd: periodEnd(page.Period),
	}
	for i, e := range page.Entries {
		out.Data[i] = toEntry(e)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      A player's rank, score and neighbours
// @Tags         leaderboards
// @Produce      json
// @Security     BearerAuth
// @Param        id       path  string true  "Leaderboard id"
// @Param        playerID path  string true  "Player id"
// @Param        period   query string false "current (default) or an RFC 3339 time inside the period"
// @Param        around   query int    false "Neighbours on each side (default 2, max 10)"
// @Success      200 {object} PlayerRankResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /leaderboards/{id}/players/{playerID} [get]
func (h *Handler) playerRank(w http.ResponseWriter, r *http.Request) {
	around, err := queryInt(r, "around", 2)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.PlayerStanding(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "playerID"), r.URL.Query().Get("period"), around)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := PlayerRankResp{
		Entry: toEntry(res.Entry), Neighbours: make([]EntryResp, len(res.Neighbours)),
		PeriodStart: res.Period.Start.UTC(), PeriodEnd: periodEnd(res.Period),
	}
	for i, e := range res.Neighbours {
		out.Neighbours[i] = toEntry(e)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Rebuild a leaderboard's redis read model from Postgres
// @Tags         leaderboards
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Leaderboard id"
// @Success      200 {object} RebuildResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      503 {object} httpx.Problem
// @Router       /leaderboards/{id}/rebuild [post]
func (h *Handler) rebuild(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Rebuild(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, RebuildResp{Periods: res.Periods, Entries: res.Entries})
}
