// Package transport is program's HTTP layer: DTO shape validation here,
// invariants in the domain, authorization and tenancy in the service.
package transport

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"levelup/internal/modules/program/internal/app"
	"levelup/internal/modules/program/internal/domain"
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
	r.Route("/programs", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
		r.Post("/{id}/activate", h.activate)
		r.Post("/{id}/pause", h.pause)
		r.Post("/{id}/end", h.end)
		r.Get("/{id}/players", h.listPlayers)
		r.Post("/{id}/players", h.enroll)
		r.Delete("/{id}/players/{playerID}", h.unenroll)
	})
}

// @Summary      List programs
// @Tags         program
// @Produce      json
// @Security     BearerAuth
// @Param        status query string false "Filter by status" Enums(draft, active, paused, ended)
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from next_cursor"
// @Success      200 {object} ProgramListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /programs [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	q := r.URL.Query()
	items, next, err := h.svc.List(r.Context(), q.Get("status"), q.Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := ProgramListResp{Data: make([]ProgramResp, len(items)), NextCursor: next}
	for i, p := range items {
		out.Data[i] = toProgramResp(p)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a program (always starts as draft)
// @Tags         program
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateReq true "Program"
// @Success      201 {object} ProgramResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem "slug_taken"
// @Failure      422 {object} httpx.Problem
// @Router       /programs [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Create(r.Context(), app.CreateCmd{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		StartsAt:    req.StartsAt,
		EndsAt:      req.EndsAt,
		Settings:    req.Settings,
		Mechanics:   req.Mechanics,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toProgramResp(p))
}

// @Summary      Get a program
// @Tags         program
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Program id"
// @Success      200 {object} ProgramResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "program_not_found"
// @Router       /programs/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := programID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toProgramResp(p))
}

// @Summary      Partially update a program (status is not editable here)
// @Tags         program
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string   true "Program id"
// @Param        body body PatchReq true "Fields to change; omitted fields stay untouched"
// @Success      200 {object} ProgramResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "program_not_found"
// @Failure      409 {object} httpx.Problem "slug_taken | version_conflict"
// @Failure      422 {object} httpx.Problem
// @Router       /programs/{id} [patch]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := programID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req PatchReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Update(r.Context(), id, req.toPatch())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toProgramResp(p))
}

// @Summary      Delete a program (soft delete)
// @Tags         program
// @Security     BearerAuth
// @Param        id path string true "Program id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "program_not_found"
// @Router       /programs/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := programID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Activate a program (draft|paused → active)
// @Tags         program
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Program id"
// @Success      200 {object} ProgramResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "program_not_found"
// @Failure      409 {object} httpx.Problem "invalid_status_transition | program_window_elapsed"
// @Router       /programs/{id}/activate [post]
func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.svc.Activate)
}

// @Summary      Pause a program (active → paused)
// @Tags         program
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Program id"
// @Success      200 {object} ProgramResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "program_not_found"
// @Failure      409 {object} httpx.Problem "invalid_status_transition"
// @Router       /programs/{id}/pause [post]
func (h *Handler) pause(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.svc.Pause)
}

// @Summary      End a program (active|paused → ended; terminal)
// @Tags         program
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Program id"
// @Success      200 {object} ProgramResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "program_not_found"
// @Failure      409 {object} httpx.Problem "invalid_status_transition"
// @Router       /programs/{id}/end [post]
func (h *Handler) end(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.svc.End)
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request,
	move func(ctx context.Context, id string) (domain.Program, error)) {
	id, err := programID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := move(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toProgramResp(p))
}

// @Summary      List a program's enrolled players
// @Tags         program
// @Produce      json
// @Security     BearerAuth
// @Param        id     path  string true  "Program id"
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from next_cursor"
// @Success      200 {object} MemberListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "program_not_found"
// @Failure      422 {object} httpx.Problem
// @Router       /programs/{id}/players [get]
func (h *Handler) listPlayers(w http.ResponseWriter, r *http.Request) {
	id, err := programID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	limit, err := parseLimit(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	members, next, err := h.svc.ListMembers(r.Context(), id, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := MemberListResp{Data: make([]MemberResp, len(members)), NextCursor: next}
	for i, m := range members {
		out.Data[i] = toMemberResp(m)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Enrol a player (idempotent: 200 with the existing enrolment)
// @Tags         program
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string    true "Program id"
// @Param        body body EnrollReq true "Player"
// @Success      201 {object} EnrollmentResp
// @Success      200 {object} EnrollmentResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "program_not_found | player_not_found"
// @Failure      409 {object} httpx.Problem "program_not_accepting_players"
// @Failure      422 {object} httpx.Problem "player_inactive"
// @Router       /programs/{id}/players [post]
func (h *Handler) enroll(w http.ResponseWriter, r *http.Request) {
	id, err := programID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req EnrollReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	e, created, err := h.svc.Enroll(r.Context(), id, req.PlayerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	httpx.JSON(w, status, toEnrollmentResp(e))
}

// @Summary      Remove a player from a program (idempotent)
// @Tags         program
// @Security     BearerAuth
// @Param        id       path string true "Program id"
// @Param        playerID path string true "Player id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "program_not_found"
// @Router       /programs/{id}/players/{playerID} [delete]
func (h *Handler) unenroll(w http.ResponseWriter, r *http.Request) {
	id, err := programID(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	playerID := chi.URLParam(r, "playerID")
	if _, perr := uuid.Parse(playerID); perr != nil {
		httpx.Error(w, r, domain.ErrPlayerNotFound)
		return
	}
	if err := h.svc.Unenroll(r.Context(), id, playerID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// programID rejects non-uuid ids as 404 before they reach SQL.
func programID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		return "", domain.ErrNotFound
	}
	return id, nil
}

func parseLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return app.DefaultPageSize, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
			map[string]string{"limit": "must be a positive integer"})
	}
	if n > app.MaxPageSize {
		n = app.MaxPageSize
	}
	return n, nil
}
