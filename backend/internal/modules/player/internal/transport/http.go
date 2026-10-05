// Package transport is player's HTTP layer: DTO shape validation here,
// invariants in the domain, authorization and tenancy in the service.
package transport

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"levelup/internal/modules/player/internal/app"
	"levelup/internal/modules/player/internal/domain"
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
	r.Route("/players", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/by-external-id/{externalID}", h.getByExternalID)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Post("/{id}/activate", h.activate)
		r.Post("/{id}/deactivate", h.deactivate)
		r.Delete("/{id}", h.delete)
	})
}

type CreateReq struct {
	ExternalID  string         `json:"external_id"  validate:"required,max=255"`
	DisplayName string         `json:"display_name" validate:"omitempty,max=255"`
	Email       string         `json:"email"        validate:"omitempty,email,max=255"`
	Attributes  map[string]any `json:"attributes"   validate:"omitempty,max=100"`
}

// UpdateReq is a true PATCH: an omitted field stays untouched. An empty
// string clears display_name / email. attributes is a merge patch: listed
// keys are set, a null value removes the key, unlisted keys stay.
type UpdateReq struct {
	DisplayName *string        `json:"display_name" validate:"omitempty,max=255"`
	Email       *string        `json:"email"        validate:"omitempty,max=255"`
	Attributes  map[string]any `json:"attributes"   validate:"omitempty,max=100"`
	IsActive    *bool          `json:"is_active"`
}

type PlayerResp struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	ExternalID  string         `json:"external_id"`
	DisplayName *string        `json:"display_name"`
	Email       *string        `json:"email"`
	Attributes  map[string]any `json:"attributes"`
	IsActive    bool           `json:"is_active"`
	CreatedBy   *string        `json:"created_by"`
	Version     int            `json:"version"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type ListResp struct {
	Data       []PlayerResp `json:"data"`
	NextCursor string       `json:"next_cursor"`
}

func toResp(p domain.Player) PlayerResp {
	attrs := p.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	return PlayerResp{
		ID:          p.ID,
		TenantID:    p.TenantID,
		ExternalID:  p.ExternalID,
		DisplayName: optional(p.DisplayName),
		Email:       optional(p.Email),
		Attributes:  attrs,
		IsActive:    p.Active,
		CreatedBy:   optional(p.CreatedBy),
		Version:     p.Version,
		CreatedAt:   p.CreatedAt.UTC(),
		UpdatedAt:   p.UpdatedAt.UTC(),
	}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// @Summary      List players
// @Tags         player
// @Produce      json
// @Security     BearerAuth
// @Param        limit     query int    false "Page size (default 25, max 100)"
// @Param        cursor    query string false "Opaque cursor from next_cursor"
// @Param        is_active query bool   false "Filter by active flag"
// @Param        search    query string false "Case-insensitive prefix of external_id, display_name or email"
// @Success      200 {object} ListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /players [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := app.ListQuery{Cursor: q.Get("cursor"), Search: q.Get("search")}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
				map[string]string{"limit": "must be a positive integer"}))
			return
		}
		query.Limit = n
	}
	if raw := q.Get("is_active"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
				map[string]string{"is_active": "must be true or false"}))
			return
		}
		query.Active = &b
	}
	page, err := h.svc.List(r.Context(), query)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := ListResp{Data: make([]PlayerResp, len(page.Players)), NextCursor: page.NextCursor}
	for i, p := range page.Players {
		out.Data[i] = toResp(p)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a player
// @Tags         player
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        Idempotency-Key header string false "Replays the stored response for a repeated key"
// @Param        body body CreateReq true "Player"
// @Success      201 {object} PlayerResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem "code player_external_id_taken"
// @Failure      422 {object} httpx.Problem
// @Router       /players [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Create(r.Context(), app.CreateCmd{
		ExternalID:  req.ExternalID,
		DisplayName: req.DisplayName,
		Email:       req.Email,
		Attributes:  req.Attributes,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toResp(p))
}

// @Summary      Get a player
// @Tags         player
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Player id (uuid)"
// @Success      200 {object} PlayerResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "code player_not_found"
// @Router       /players/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResp(p))
}

// @Summary      Get a player by the tenant's external id
// @Tags         player
// @Produce      json
// @Security     BearerAuth
// @Param        externalID path string true "External id (URL-encoded)"
// @Success      200 {object} PlayerResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "code player_not_found"
// @Router       /players/by-external-id/{externalID} [get]
func (h *Handler) getByExternalID(w http.ResponseWriter, r *http.Request) {
	// chi routes on RawPath when the request carried escapes the default
	// encoding would not produce (e.g. %2F); only then is the param still
	// escaped.
	ext := chi.URLParam(r, "externalID")
	if r.URL.RawPath != "" {
		unescaped, err := url.PathUnescape(ext)
		if err != nil {
			httpx.Error(w, r, errs.Wrap(errs.Invalid, "malformed external id", err))
			return
		}
		ext = unescaped
	}
	p, err := h.svc.GetByExternalID(r.Context(), ext)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResp(p))
}

// @Summary      Partially update a player
// @Description  Omitted fields stay untouched; "" clears display_name/email; attributes is a merge patch (null removes a key).
// @Tags         player
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string    true "Player id (uuid)"
// @Param        body body UpdateReq true "Fields to change"
// @Success      200 {object} PlayerResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "code player_not_found"
// @Failure      409 {object} httpx.Problem "code player_version_conflict"
// @Failure      422 {object} httpx.Problem
// @Router       /players/{id} [patch]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	p, err := h.svc.Update(r.Context(), chi.URLParam(r, "id"), domain.Patch{
		DisplayName: req.DisplayName,
		Email:       req.Email,
		Attributes:  req.Attributes,
		Active:      req.IsActive,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResp(p))
}

// @Summary      Activate a player
// @Tags         player
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Player id (uuid)"
// @Success      200 {object} PlayerResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "code player_not_found"
// @Router       /players/{id}/activate [post]
func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Activate(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResp(p))
}

// @Summary      Deactivate a player
// @Tags         player
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Player id (uuid)"
// @Success      200 {object} PlayerResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "code player_not_found"
// @Router       /players/{id}/deactivate [post]
func (h *Handler) deactivate(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Deactivate(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toResp(p))
}

// @Summary      Delete a player (soft delete, admin only)
// @Tags         player
// @Security     BearerAuth
// @Param        id path string true "Player id (uuid)"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "code player_not_found"
// @Router       /players/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
