// Package transport is missions' HTTP layer: DTO shape validation here,
// invariants in the domain and service (ADR-0016 conventions).
package transport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"levelup/internal/modules/missions/internal/app"
	"levelup/internal/modules/missions/internal/domain"
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
	r.Route("/missions", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/stats", h.listStats)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
		r.Post("/{id}/start", h.start)
		r.Post("/{id}/progress", h.progress)
		r.Post("/{id}/complete", h.complete)
		r.Get("/{id}/attempts", h.attempts)
		r.Get("/{id}/stats", h.stats)
	})
	// Registered as a single route (not a /players subrouter) so it coexists
	// with the player module's /players mount.
	r.With(httpx.RequireAuth).Get("/players/{playerID}/missions", h.playerMissions)
}

// ---- DTOs ----

type CreateMissionReq struct {
	Slug        string `json:"slug" validate:"omitempty,max=120"`
	Name        string `json:"name" validate:"required,max=255"`
	Description string `json:"description" validate:"max=1000"`
	Type        string `json:"type" validate:"required,oneof=one_time daily weekly repeating"`
	Status      string `json:"status" validate:"omitempty,oneof=draft active"`
	Target      int64  `json:"target" validate:"required,gt=0"`
	// Criteria grammar: {"event_type": "...", "where": [{"field", "operator", "value"}],
	// "increment": {"by": "count"} | {"by": "property", "field": "..."}}.
	// With event_type, matching activities progress the mission automatically.
	Criteria                map[string]any `json:"criteria"`
	PointsReward            int64          `json:"points_reward" validate:"gte=0"`
	XPReward                int64          `json:"xp_reward" validate:"gte=0"`
	BadgeRewardID           string         `json:"badge_reward_id" validate:"omitempty,uuid"`
	MaxCompletionsPerPlayer *int           `json:"max_completions_per_player" validate:"omitempty,gte=1"`
	StartsAt                *time.Time     `json:"starts_at"`
	EndsAt                  *time.Time     `json:"ends_at"`
}

// UpdateMissionReq is a partial update: omitted fields stay untouched.
// badge_reward_id "" clears the badge; max_completions_per_player 0 means
// unlimited; starts_at / ends_at "" clear the bound (else RFC 3339).
type UpdateMissionReq struct {
	Slug                    *string        `json:"slug" validate:"omitempty,max=120"`
	Name                    *string        `json:"name" validate:"omitempty,max=255"`
	Description             *string        `json:"description" validate:"omitempty,max=1000"`
	Type                    *string        `json:"type" validate:"omitempty,oneof=one_time daily weekly repeating"`
	Status                  *string        `json:"status" validate:"omitempty,oneof=draft active paused expired archived"`
	Target                  *int64         `json:"target" validate:"omitempty,gt=0"`
	Criteria                map[string]any `json:"criteria"`
	PointsReward            *int64         `json:"points_reward" validate:"omitempty,gte=0"`
	XPReward                *int64         `json:"xp_reward" validate:"omitempty,gte=0"`
	BadgeRewardID           *string        `json:"badge_reward_id" validate:"omitempty,uuid"`
	MaxCompletionsPerPlayer *int           `json:"max_completions_per_player" validate:"omitempty,gte=0"`
	StartsAt                *string        `json:"starts_at"`
	EndsAt                  *string        `json:"ends_at"`
}

type PlayerReq struct {
	PlayerID string `json:"player_id" validate:"required,uuid"`
}

type ProgressReq struct {
	PlayerID  string `json:"player_id" validate:"required,uuid"`
	Increment int64  `json:"increment" validate:"required,gt=0,lte=1000000"`
}

type MissionResp struct {
	ID                      string         `json:"id"`
	Slug                    string         `json:"slug"`
	Name                    string         `json:"name"`
	Description             string         `json:"description"`
	Type                    string         `json:"type"`
	Status                  string         `json:"status"`
	Target                  int64          `json:"target"`
	Criteria                map[string]any `json:"criteria"`
	PointsReward            int64          `json:"points_reward"`
	XPReward                int64          `json:"xp_reward"`
	BadgeRewardID           *string        `json:"badge_reward_id"`
	MaxCompletionsPerPlayer *int           `json:"max_completions_per_player"`
	StartsAt                *time.Time     `json:"starts_at"`
	EndsAt                  *time.Time     `json:"ends_at"`
	Version                 int            `json:"version"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
}

type MissionSummaryResp struct {
	ID      string `json:"id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	Deleted bool   `json:"deleted"`
}

type AttemptResp struct {
	ID          string              `json:"id"`
	MissionID   string              `json:"mission_id"`
	PlayerID    string              `json:"player_id"`
	Status      string              `json:"status"`
	Progress    int64               `json:"progress"`
	Target      int64               `json:"target"`
	PeriodKey   string              `json:"period_key"`
	StartedAt   time.Time           `json:"started_at"`
	CompletedAt *time.Time          `json:"completed_at"`
	Mission     *MissionSummaryResp `json:"mission,omitempty"`
}

type ProgressResp struct {
	Attempt   *AttemptResp `json:"attempt"`
	Completed bool         `json:"completed"`
	// Duplicate is true when the Idempotency-Key was already applied: the
	// increment was not counted again.
	Duplicate bool `json:"duplicate"`
}

// MissionStatsResp is one mission's completion analytics.
// completion_rate = completed / started (0..1, 0 when nothing started);
// avg_hours_to_complete is null when no attempt completed.
type MissionStatsResp struct {
	MissionID          string   `json:"mission_id"`
	Name               string   `json:"name"`
	Slug               string   `json:"slug"`
	Status             string   `json:"status"`
	Started            int64    `json:"started"`
	InProgress         int64    `json:"in_progress"`
	Completed          int64    `json:"completed"`
	CompletionRate     float64  `json:"completion_rate"`
	AvgHoursToComplete *float64 `json:"avg_hours_to_complete"`
}

type MissionStatsListResp struct {
	Data       []MissionStatsResp `json:"data"`
	NextCursor string             `json:"next_cursor"`
}

type MissionListResp struct {
	Data       []MissionResp `json:"data"`
	NextCursor string        `json:"next_cursor"`
}

type AttemptListResp struct {
	Data       []AttemptResp `json:"data"`
	NextCursor string        `json:"next_cursor"`
}

// ---- handlers ----

// @Summary      List missions
// @Tags         missions
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from next_cursor"
// @Param        status query string false "Filter by status"
// @Param        type   query string false "Filter by type"
// @Success      200 {object} MissionListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /missions [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.svc.ListMissions(r.Context(),
		app.MissionFilter{Status: q.Get("status"), Type: q.Get("type")},
		q.Get("cursor"), limitParam(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := MissionListResp{Data: make([]MissionResp, len(page.Items)), NextCursor: page.NextCursor}
	for i, m := range page.Items {
		out.Data[i] = toMissionResp(m)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a mission
// @Tags         missions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateMissionReq true "Mission"
// @Success      201 {object} MissionResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem "slug_taken"
// @Failure      422 {object} httpx.Problem "invalid_mission_criteria (fields keyed criteria.*)"
// @Router       /missions [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateMissionReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	m, err := h.svc.CreateMission(r.Context(), app.CreateMissionCmd{
		Slug:                    req.Slug,
		Name:                    req.Name,
		Description:             req.Description,
		Type:                    req.Type,
		Status:                  req.Status,
		Target:                  req.Target,
		Criteria:                req.Criteria,
		PointsReward:            req.PointsReward,
		XPReward:                req.XPReward,
		BadgeRewardID:           req.BadgeRewardID,
		MaxCompletionsPerPlayer: req.MaxCompletionsPerPlayer,
		StartsAt:                req.StartsAt,
		EndsAt:                  req.EndsAt,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toMissionResp(m))
}

// @Summary      Get a mission
// @Tags         missions
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Mission id"
// @Success      200 {object} MissionResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "mission_not_found"
// @Router       /missions/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	m, err := h.svc.GetMission(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toMissionResp(m))
}

// @Summary      Update a mission (partial)
// @Tags         missions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string           true "Mission id"
// @Param        body body UpdateMissionReq true "Fields to change"
// @Success      200 {object} MissionResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "mission_not_found"
// @Failure      409 {object} httpx.Problem "invalid_status_transition, slug_taken, version_conflict, mission_type_immutable"
// @Failure      422 {object} httpx.Problem "invalid_mission_criteria (fields keyed criteria.*)"
// @Router       /missions/{id} [patch]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateMissionReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	cmd := app.UpdateMissionCmd{
		Slug:                    req.Slug,
		Name:                    req.Name,
		Description:             req.Description,
		Type:                    req.Type,
		Status:                  req.Status,
		Target:                  req.Target,
		Criteria:                req.Criteria,
		PointsReward:            req.PointsReward,
		XPReward:                req.XPReward,
		BadgeRewardID:           req.BadgeRewardID,
		MaxCompletionsPerPlayer: req.MaxCompletionsPerPlayer,
	}
	var err error
	if cmd.StartsAt, cmd.ClearStartsAt, err = optionalTime("starts_at", req.StartsAt); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if cmd.EndsAt, cmd.ClearEndsAt, err = optionalTime("ends_at", req.EndsAt); err != nil {
		httpx.Error(w, r, err)
		return
	}
	m, err := h.svc.UpdateMission(r.Context(), chi.URLParam(r, "id"), cmd)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toMissionResp(m))
}

// @Summary      Delete a mission (soft)
// @Tags         missions
// @Security     BearerAuth
// @Param        id path string true "Mission id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "mission_not_found"
// @Router       /missions/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteMission(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Start a mission for a player (current period)
// @Tags         missions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string    true "Mission id"
// @Param        body body PlayerReq true "Player"
// @Success      201 {object} AttemptResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "mission_not_found, player_not_found"
// @Failure      409 {object} httpx.Problem "mission_already_started, mission_not_available, mission_limit_reached, player_inactive"
// @Failure      422 {object} httpx.Problem
// @Router       /missions/{id}/start [post]
func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	var req PlayerReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	a, err := h.svc.StartMission(r.Context(), chi.URLParam(r, "id"), req.PlayerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toAttemptResp(a))
}

// @Summary      Add progress to a player's mission attempt
// @Description  Starts the current period's attempt when none is open and completes it when the target is reached. With an Idempotency-Key the increment is counted at most once (key namespace "manual:").
// @Tags         missions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id              path   string      true  "Mission id"
// @Param        Idempotency-Key header string      false "Client idempotency key"
// @Param        body            body   ProgressReq true  "Progress"
// @Success      200 {object} ProgressResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "mission_not_found, player_not_found"
// @Failure      409 {object} httpx.Problem "mission_not_available, mission_limit_reached, player_inactive"
// @Failure      422 {object} httpx.Problem
// @Router       /missions/{id}/progress [post]
func (h *Handler) progress(w http.ResponseWriter, r *http.Request) {
	var req ProgressReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.ManualProgress(r.Context(), chi.URLParam(r, "id"), req.PlayerID, req.Increment,
		r.Header.Get("Idempotency-Key"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := ProgressResp{Completed: res.Completed, Duplicate: res.Outcome == app.OutcomeDuplicate}
	if res.HasAttempt {
		a := toAttemptResp(res.Attempt)
		out.Attempt = &a
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Complete a player's mission attempt
// @Description  Completes the open attempt only when progress reached the target. Repeating it after completion returns the completed attempt and grants nothing again.
// @Tags         missions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string    true "Mission id"
// @Param        body body PlayerReq true "Player"
// @Success      200 {object} AttemptResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "mission_not_found, player_not_found, mission_not_started"
// @Failure      409 {object} httpx.Problem "mission_not_completed, player_inactive"
// @Failure      422 {object} httpx.Problem
// @Router       /missions/{id}/complete [post]
func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	var req PlayerReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	a, err := h.svc.CompleteMission(r.Context(), chi.URLParam(r, "id"), req.PlayerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAttemptResp(a))
}

// @Summary      List a mission's attempts
// @Tags         missions
// @Produce      json
// @Security     BearerAuth
// @Param        id        path  string true  "Mission id"
// @Param        player_id query string false "Filter by player"
// @Param        status    query string false "Filter by attempt status"
// @Param        limit     query int    false "Page size (default 25, max 100)"
// @Param        cursor    query string false "Opaque cursor from next_cursor"
// @Success      200 {object} AttemptListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "mission_not_found"
// @Failure      422 {object} httpx.Problem
// @Router       /missions/{id}/attempts [get]
func (h *Handler) attempts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.svc.ListMissionAttempts(r.Context(), chi.URLParam(r, "id"),
		app.AttemptFilter{PlayerID: q.Get("player_id"), Status: q.Get("status")},
		q.Get("cursor"), limitParam(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := AttemptListResp{Data: make([]AttemptResp, len(page.Items)), NextCursor: page.NextCursor}
	for i, a := range page.Items {
		out.Data[i] = toAttemptResp(a)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      List a player's mission attempts with mission summaries
// @Tags         missions
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path  string true  "Player id"
// @Param        status   query string false "Filter by attempt status"
// @Param        limit    query int    false "Page size (default 25, max 100)"
// @Param        cursor   query string false "Opaque cursor from next_cursor"
// @Success      200 {object} AttemptListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "player_not_found"
// @Failure      422 {object} httpx.Problem
// @Router       /players/{playerID}/missions [get]
func (h *Handler) playerMissions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.svc.ListPlayerAttempts(r.Context(), chi.URLParam(r, "playerID"),
		app.AttemptFilter{Status: q.Get("status")}, q.Get("cursor"), limitParam(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := AttemptListResp{Data: make([]AttemptResp, len(page.Items)), NextCursor: page.NextCursor}
	for i, it := range page.Items {
		a := toAttemptResp(it.Attempt)
		if it.HasMission {
			a.Mission = &MissionSummaryResp{
				ID:      it.Mission.ID,
				Slug:    it.Mission.Slug,
				Name:    it.Mission.Name,
				Type:    it.Mission.Type,
				Status:  it.Mission.Status,
				Deleted: it.Mission.DeletedAt != nil,
			}
		}
		out.Data[i] = a
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Completion analytics of every mission
// @Description  One page of missions (newest first) with started / in_progress / completed attempt counts, completion_rate and avg_hours_to_complete. Missions without attempts report zeros.
// @Tags         missions
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from next_cursor"
// @Param        status query string false "Filter by mission status"
// @Param        type   query string false "Filter by mission type"
// @Success      200 {object} MissionStatsListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /missions/stats [get]
func (h *Handler) listStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, err := h.svc.ListMissionStats(r.Context(),
		app.MissionFilter{Status: q.Get("status"), Type: q.Get("type")},
		q.Get("cursor"), limitParam(r))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := MissionStatsListResp{Data: make([]MissionStatsResp, len(page.Items)), NextCursor: page.NextCursor}
	for i, st := range page.Items {
		out.Data[i] = toStatsResp(st)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Completion analytics of one mission
// @Tags         missions
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Mission id"
// @Success      200 {object} MissionStatsResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "mission_not_found"
// @Router       /missions/{id}/stats [get]
func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.GetMissionStats(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toStatsResp(st))
}

// ---- mapping ----

func toStatsResp(st app.MissionStats) MissionStatsResp {
	return MissionStatsResp{
		MissionID:          st.MissionID,
		Name:               st.Name,
		Slug:               st.Slug,
		Status:             st.Status,
		Started:            st.Started,
		InProgress:         st.InProgress,
		Completed:          st.Completed,
		CompletionRate:     st.CompletionRate,
		AvgHoursToComplete: st.AvgHoursToComplete,
	}
}

func toMissionResp(m domain.Mission) MissionResp {
	out := MissionResp{
		ID:                      m.ID,
		Slug:                    m.Slug,
		Name:                    m.Name,
		Description:             m.Description,
		Type:                    m.Type,
		Status:                  m.Status,
		Target:                  m.Target,
		Criteria:                m.Criteria,
		PointsReward:            m.PointsReward,
		XPReward:                m.XPReward,
		MaxCompletionsPerPlayer: m.MaxCompletionsPerPlayer,
		StartsAt:                m.StartsAt,
		EndsAt:                  m.EndsAt,
		Version:                 m.Version,
		CreatedAt:               m.CreatedAt,
		UpdatedAt:               m.UpdatedAt,
	}
	if out.Criteria == nil {
		out.Criteria = map[string]any{}
	}
	if m.BadgeRewardID != "" {
		b := m.BadgeRewardID
		out.BadgeRewardID = &b
	}
	return out
}

func toAttemptResp(a domain.Attempt) AttemptResp {
	return AttemptResp{
		ID:          a.ID,
		MissionID:   a.MissionID,
		PlayerID:    a.PlayerID,
		Status:      a.Status,
		Progress:    a.Progress,
		Target:      a.Target,
		PeriodKey:   a.PeriodKey,
		StartedAt:   a.StartedAt,
		CompletedAt: a.CompletedAt,
	}
}

func limitParam(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return n
}

// optionalTime maps a PATCH time field: nil → untouched, "" → clear,
// otherwise RFC 3339.
func optionalTime(field string, raw *string) (*time.Time, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	if *raw == "" {
		return nil, true, nil
	}
	t, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return nil, false, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
			map[string]string{field: "must be an RFC 3339 timestamp or empty to clear"})
	}
	t = t.UTC()
	return &t, false, nil
}
