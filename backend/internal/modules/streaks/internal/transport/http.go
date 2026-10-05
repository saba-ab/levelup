// Package transport is streaks' HTTP layer: DTO shape validation here,
// invariants in the domain and service.
package transport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"levelup/internal/modules/streaks/internal/app"
	"levelup/internal/modules/streaks/internal/domain"
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
	r.Route("/streaks", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
		r.Post("/{id}/record", h.record)
		r.Post("/{id}/players/{playerID}/reset", h.reset)
	})
	// A single pattern, not r.Route("/players"): the player module owns that
	// subtree and chi refuses two mounts on one prefix.
	r.With(httpx.RequireAuth).Get("/players/{playerID}/streaks", h.playerStreaks)
}

// ---- DTOs ----

type MilestoneDTO struct {
	Count       int   `json:"count" validate:"required,min=1"`
	BonusPoints int64 `json:"bonus_points" validate:"min=0"`
}

type CreateReq struct {
	Slug            string         `json:"slug" validate:"omitempty,max=100"`
	Name            string         `json:"name" validate:"required,max=255"`
	Description     string         `json:"description" validate:"max=1000"`
	ActivityKey     string         `json:"activity_key" validate:"required,max=100"`
	Period          string         `json:"period" validate:"required,oneof=daily weekly monthly"`
	GracePeriods    int            `json:"grace_periods" validate:"min=0,max=30"`
	PointsPerPeriod int64          `json:"points_per_period" validate:"min=0"`
	Milestones      []MilestoneDTO `json:"milestones" validate:"max=50,dive"`
	IsActive        *bool          `json:"is_active"`
}

// UpdateReq is a partial update: omitted fields stay untouched. The period
// is immutable (recorded buckets depend on it).
type UpdateReq struct {
	Slug            *string         `json:"slug" validate:"omitempty,min=1,max=100"`
	Name            *string         `json:"name" validate:"omitempty,min=1,max=255"`
	Description     *string         `json:"description" validate:"omitempty,max=1000"`
	ActivityKey     *string         `json:"activity_key" validate:"omitempty,min=1,max=100"`
	GracePeriods    *int            `json:"grace_periods" validate:"omitempty,min=0,max=30"`
	PointsPerPeriod *int64          `json:"points_per_period" validate:"omitempty,min=0"`
	Milestones      *[]MilestoneDTO `json:"milestones" validate:"omitempty,max=50,dive"`
	IsActive        *bool           `json:"is_active"`
}

type RecordReq struct {
	PlayerID   string     `json:"player_id" validate:"required,uuid"`
	OccurredAt *time.Time `json:"occurred_at"` // defaults to now; decides the period bucket
}

type StreakResp struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenant_id"`
	Slug            string         `json:"slug"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	ActivityKey     string         `json:"activity_key"`
	Period          string         `json:"period"`
	GracePeriods    int            `json:"grace_periods"`
	PointsPerPeriod int64          `json:"points_per_period"`
	Milestones      []MilestoneDTO `json:"milestones"`
	IsActive        bool           `json:"is_active"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type StreakListResp struct {
	Data       []StreakResp `json:"data"`
	NextCursor string       `json:"next_cursor"`
}

type PlayerStreakResp struct {
	ID              string      `json:"id"`
	PlayerID        string      `json:"player_id"`
	StreakID        string      `json:"streak_id"`
	CurrentCount    int         `json:"current_count"`
	LongestCount    int         `json:"longest_count"`
	LastPeriodStart *time.Time  `json:"last_period_start"`
	RunStartedAt    *time.Time  `json:"run_started_at"`
	BrokenAt        *time.Time  `json:"broken_at"`
	Streak          *StreakResp `json:"streak,omitempty"`
}

type PlayerStreakListResp struct {
	Data       []PlayerStreakResp `json:"data"`
	NextCursor string             `json:"next_cursor"`
}

type RecordResp struct {
	// Outcome: recorded (new period), noop (period already recorded) or
	// duplicate (Idempotency-Key replay).
	Outcome           string           `json:"outcome"`
	NewPeriod         bool             `json:"new_period"`
	PeriodStart       time.Time        `json:"period_start"`
	MilestonesReached []int            `json:"milestones_reached"`
	PlayerStreak      PlayerStreakResp `json:"player_streak"`
}

// ---- handlers ----

// @Summary      List streak definitions
// @Tags         streaks
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from next_cursor"
// @Param        active query bool   false "Filter by is_active"
// @Param        period query string false "Filter by period (daily|weekly|monthly)"
// @Success      200 {object} StreakListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /streaks [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := app.ListFilter{Period: q.Get("period")}
	if v := q.Get("active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			httpx.Error(w, r, errs.New(errs.Invalid, "active must be a boolean"))
			return
		}
		f.Active = &b
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, next, err := h.svc.List(r.Context(), f, q.Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := StreakListResp{Data: make([]StreakResp, len(rows)), NextCursor: next}
	for i, s := range rows {
		out.Data[i] = toStreakResp(s)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a streak definition
// @Tags         streaks
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateReq true "Streak"
// @Success      201 {object} StreakResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /streaks [post]
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
	st, err := h.svc.Create(r.Context(), domain.NewStreakInput{
		Slug:            req.Slug,
		Name:            req.Name,
		Description:     req.Description,
		ActivityKey:     req.ActivityKey,
		Period:          req.Period,
		GracePeriods:    req.GracePeriods,
		PointsPerPeriod: req.PointsPerPeriod,
		Milestones:      toMilestones(req.Milestones),
		Active:          active,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toStreakResp(st))
}

// @Summary      Get a streak definition
// @Tags         streaks
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Streak id"
// @Success      200 {object} StreakResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /streaks/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toStreakResp(st))
}

// @Summary      Update a streak definition (partial)
// @Tags         streaks
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string    true "Streak id"
// @Param        body body UpdateReq true "Fields to change"
// @Success      200 {object} StreakResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /streaks/{id} [patch]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	patch := domain.StreakPatch{
		Slug:            req.Slug,
		Name:            req.Name,
		Description:     req.Description,
		ActivityKey:     req.ActivityKey,
		GracePeriods:    req.GracePeriods,
		PointsPerPeriod: req.PointsPerPeriod,
		Active:          req.IsActive,
	}
	if req.Milestones != nil {
		ms := toMilestones(*req.Milestones)
		patch.Milestones = &ms
	}
	st, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toStreakResp(st))
}

// @Summary      Delete a streak definition
// @Tags         streaks
// @Security     BearerAuth
// @Param        id path string true "Streak id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /streaks/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Record activity for a player on a streak
// @Description  The period bucket is computed from occurred_at in the tenant timezone. A second record in the same period is a no-op (outcome "noop"): no points, no events.
// @Tags         streaks
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id              path   string    true  "Streak id"
// @Param        Idempotency-Key header string    false "Replay protection"
// @Param        body            body   RecordReq true  "Record"
// @Success      200 {object} RecordResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /streaks/{id}/record [post]
func (h *Handler) record(w http.ResponseWriter, r *http.Request) {
	var req RecordReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Record(r.Context(), app.RecordInput{
		StreakID:       chi.URLParam(r, "id"),
		PlayerID:       req.PlayerID,
		OccurredAt:     req.OccurredAt,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	reached := res.MilestonesReached
	if reached == nil {
		reached = []int{}
	}
	httpx.JSON(w, http.StatusOK, RecordResp{
		Outcome:           res.Outcome,
		NewPeriod:         res.Outcome == app.OutcomeRecorded,
		PeriodStart:       res.PeriodStart,
		MilestonesReached: reached,
		PlayerStreak:      toPlayerStreakResp(res.PlayerStreak, nil),
	})
}

// @Summary      Reset a player's streak run
// @Tags         streaks
// @Produce      json
// @Security     BearerAuth
// @Param        id       path string true "Streak id"
// @Param        playerID path string true "Player id"
// @Success      200 {object} PlayerStreakResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /streaks/{id}/players/{playerID}/reset [post]
func (h *Handler) reset(w http.ResponseWriter, r *http.Request) {
	ps, err := h.svc.Reset(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "playerID"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toPlayerStreakResp(ps, nil))
}

// @Summary      List a player's streaks
// @Tags         streaks
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path string true "Player id"
// @Success      200 {object} PlayerStreakListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /players/{playerID}/streaks [get]
func (h *Handler) playerStreaks(w http.ResponseWriter, r *http.Request) {
	views, err := h.svc.ListPlayerStreaks(r.Context(), chi.URLParam(r, "playerID"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := PlayerStreakListResp{Data: make([]PlayerStreakResp, len(views))}
	for i, v := range views {
		st := toStreakResp(v.Streak)
		out.Data[i] = toPlayerStreakResp(v.PlayerStreak, &st)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ---- mapping ----

func toMilestones(in []MilestoneDTO) []domain.Milestone {
	out := make([]domain.Milestone, len(in))
	for i, m := range in {
		out[i] = domain.Milestone{Count: m.Count, BonusPoints: m.BonusPoints}
	}
	return out
}

func toStreakResp(s domain.Streak) StreakResp {
	ms := make([]MilestoneDTO, len(s.Milestones))
	for i, m := range s.Milestones {
		ms[i] = MilestoneDTO{Count: m.Count, BonusPoints: m.BonusPoints}
	}
	return StreakResp{
		ID:              s.ID,
		TenantID:        s.TenantID,
		Slug:            s.Slug,
		Name:            s.Name,
		Description:     s.Description,
		ActivityKey:     s.ActivityKey,
		Period:          string(s.Period),
		GracePeriods:    s.GracePeriods,
		PointsPerPeriod: s.PointsPerPeriod,
		Milestones:      ms,
		IsActive:        s.Active,
		CreatedAt:       s.CreatedAt.UTC(),
		UpdatedAt:       s.UpdatedAt.UTC(),
	}
}

func toPlayerStreakResp(ps domain.PlayerStreak, st *StreakResp) PlayerStreakResp {
	return PlayerStreakResp{
		ID:              ps.ID,
		PlayerID:        ps.PlayerID,
		StreakID:        ps.StreakID,
		CurrentCount:    ps.CurrentCount,
		LongestCount:    ps.LongestCount,
		LastPeriodStart: ps.LastPeriodStart,
		RunStartedAt:    ps.RunStartedAt,
		BrokenAt:        ps.BrokenAt,
		Streak:          st,
	}
}
