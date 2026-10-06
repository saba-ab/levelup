// Package transport is segments' HTTP layer: DTO shape validation here,
// condition semantics in the domain.
package transport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"levelup/internal/modules/segments/internal/app"
	"levelup/internal/modules/segments/internal/domain"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/validate"
)

type Handler struct {
	svc *app.Service
	val *validate.Validator
	now func() time.Time
}

func NewHandler(svc *app.Service, val *validate.Validator, now func() time.Time) *Handler {
	return &Handler{svc: svc, val: val, now: now}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/segments", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Post("/preview", h.preview)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
		r.Post("/{id}/refresh", h.refresh)
		r.Get("/{id}/players", h.players)
	})
}

// ---- DTOs ----

// Conditions is {"all"|"any": [item, ...]} where each item is a condition
// {"field", "op", "value"} or a nested group (max depth 3, max 50
// conditions). Fields and operators:
//   - attributes.<path>: eq, neq, gt, gte, lt, lte, in, not_in, contains, exists, not_exists
//   - is_active: eq, neq (boolean)
//   - created_at: before, after (RFC 3339 or YYYY-MM-DD)
//   - level, balance, lifetime_earned, last_seen_days: eq, neq, gt, gte, lt, lte (number)
//   - badges_earned: eq, neq, gt, gte, lt, lte (count of distinct badges) or has, not_has (badge id)
//
// The DTO fields below are typed map[string]any so swag renders an object.

type CreateReq struct {
	Name        string         `json:"name" validate:"required,max=255"`
	Description string         `json:"description" validate:"max=1000"`
	Conditions  map[string]any `json:"conditions" validate:"required"`
}

// UpdateReq is a partial update: omitted fields stay untouched. Changing
// conditions queues a refresh.
type UpdateReq struct {
	Name        *string        `json:"name" validate:"omitempty,min=1,max=255"`
	Description *string        `json:"description" validate:"omitempty,max=1000"`
	Conditions  map[string]any `json:"conditions"`
}

type PreviewReq struct {
	Conditions map[string]any `json:"conditions" validate:"required"`
}

type SegmentResp struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	Conditions      map[string]any `json:"conditions"`
	MemberCount     int            `json:"member_count"`
	LastRefreshedAt *time.Time     `json:"last_refreshed_at"`
	Refreshing      bool           `json:"refreshing"`
	CreatedBy       string         `json:"created_by"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type SegmentListResp struct {
	Data       []SegmentResp `json:"data"`
	NextCursor string        `json:"next_cursor"`
}

type PlayerSummary struct {
	ID          string `json:"id"`
	ExternalID  string `json:"external_id"`
	DisplayName string `json:"display_name"`
}

type PreviewResp struct {
	// CountEstimate is the number of matching players among the first
	// Scanned players (ascending id, at most 1000). Exact when Complete.
	CountEstimate int             `json:"count_estimate"`
	Scanned       int             `json:"scanned"`
	Complete      bool            `json:"complete"`
	Sample        []PlayerSummary `json:"sample"`
}

type RefreshResp struct {
	Status  string      `json:"status"` // "queued"
	Segment SegmentResp `json:"segment"`
}

type MemberResp struct {
	PlayerID    string    `json:"player_id"`
	ExternalID  string    `json:"external_id"`
	DisplayName string    `json:"display_name"`
	AddedAt     time.Time `json:"added_at"`
}

type MemberListResp struct {
	Data       []MemberResp `json:"data"`
	NextCursor string       `json:"next_cursor"`
}

// ---- handlers ----

// @Summary      List segments
// @Tags         segments
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from next_cursor"
// @Success      200 {object} SegmentListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /segments [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, next, err := h.svc.List(r.Context(), q.Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := SegmentListResp{Data: make([]SegmentResp, len(rows)), NextCursor: next}
	for i, s := range rows {
		out.Data[i] = h.toResp(s)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a segment
// @Description  Stores the definition and queues its first membership refresh.
// @Tags         segments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateReq true "Segment"
// @Success      201 {object} SegmentResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /segments [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	s, err := h.svc.Create(r.Context(), domain.NewSegmentInput{
		Name: req.Name, Description: req.Description, Conditions: req.Conditions,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, h.toResp(s))
}

// @Summary      Preview segment conditions
// @Description  Evaluates unsaved conditions over the tenant's first 1000 players (ascending id) without changing any membership.
// @Tags         segments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body PreviewReq true "Conditions"
// @Success      200 {object} PreviewResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /segments/preview [post]
func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	var req PreviewReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Preview(r.Context(), req.Conditions)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := PreviewResp{CountEstimate: p.Matched, Scanned: p.Scanned, Complete: p.Complete,
		Sample: make([]PlayerSummary, len(p.Sample))}
	for i, s := range p.Sample {
		out.Sample[i] = PlayerSummary{ID: s.ID, ExternalID: s.ExternalID, DisplayName: s.DisplayName}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Get a segment
// @Tags         segments
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Segment id"
// @Success      200 {object} SegmentResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /segments/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, h.toResp(s))
}

// @Summary      Update a segment (partial)
// @Description  Changing conditions queues a membership refresh.
// @Tags         segments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string    true "Segment id"
// @Param        body body UpdateReq true "Fields to change"
// @Success      200 {object} SegmentResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /segments/{id} [patch]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	s, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), domain.SegmentPatch{
		Name: req.Name, Description: req.Description, Conditions: req.Conditions,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, h.toResp(s))
}

// @Summary      Delete a segment
// @Description  Drops its membership and publishes segments.deleted.v1.
// @Tags         segments
// @Security     BearerAuth
// @Param        id path string true "Segment id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /segments/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Refresh a segment's membership
// @Description  Queues a recomputation over every player (asynchronous). Poll GET /segments/{id} for last_refreshed_at and member_count.
// @Tags         segments
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Segment id"
// @Success      202 {object} RefreshResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /segments/{id}/refresh [post]
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.RequestRefresh(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, RefreshResp{Status: "queued", Segment: h.toResp(s)})
}

// @Summary      List a segment's players
// @Description  Materialized members as of the last refresh, newest first.
// @Tags         segments
// @Produce      json
// @Security     BearerAuth
// @Param        id     path  string true  "Segment id"
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from next_cursor"
// @Success      200 {object} MemberListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /segments/{id}/players [get]
func (h *Handler) players(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, next, err := h.svc.Members(r.Context(), chi.URLParam(r, "id"), q.Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := MemberListResp{Data: make([]MemberResp, len(rows)), NextCursor: next}
	for i, m := range rows {
		out.Data[i] = MemberResp{
			PlayerID: m.Member.PlayerID, ExternalID: m.Player.ExternalID,
			DisplayName: m.Player.DisplayName, AddedAt: m.Member.AddedAt.UTC(),
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) toResp(s domain.Segment) SegmentResp {
	return SegmentResp{
		ID:              s.ID,
		Name:            s.Name,
		Description:     s.Description,
		Conditions:      s.Conditions.ToMap(),
		MemberCount:     s.MemberCount,
		LastRefreshedAt: s.LastRefreshedAt,
		Refreshing:      s.Refreshing(h.now()),
		CreatedBy:       s.CreatedBy,
		CreatedAt:       s.CreatedAt.UTC(),
		UpdatedAt:       s.UpdatedAt.UTC(),
	}
}
