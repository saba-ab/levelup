// Package transport is badges' HTTP layer: request shape validation here,
// invariants in the domain and service.
package transport

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"levelup/internal/modules/badges/internal/app"
	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
	"levelup/internal/shared/validate"
)

type Handler struct {
	svc *app.Service
	val *validate.Validator
}

func NewHandler(svc *app.Service, val *validate.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// Mount registers badges' routes. /players/{playerID}/badges is registered
// as a single route (not r.Route("/players")) so it coexists with the player
// module's /players subtree.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/badges", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
		r.Post("/{id}/award", h.award)
		r.Delete("/{id}/players/{playerID}", h.revoke)
	})
	r.With(httpx.RequireAuth).Get("/players/{playerID}/badges", h.listForPlayer)
}

var (
	errMalformedBadgeID  = domain.ErrBadgeNotFound
	errMalformedPlayerID = domain.ErrPlayerNotFound
)

// pathUUID returns the path param when it is a UUID, else notFound (B28: a
// malformed id is a 404, never a 500).
func pathUUID(r *http.Request, name string, notFound error) (string, error) {
	v := chi.URLParam(r, name)
	if _, err := uuid.Parse(v); err != nil {
		return "", notFound
	}
	return v, nil
}

func pageParams(r *http.Request) (limit int, cur app.PageCursor, err error) {
	q := r.URL.Query()
	if raw := q.Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 {
			return 0, cur, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
				map[string]string{"limit": "must be a positive integer"})
		}
	}
	if raw := q.Get("cursor"); raw != "" {
		cur.Before, cur.BeforeID, err = pagination.DecodeCursor(raw)
		if err != nil {
			return 0, cur, err
		}
	}
	return limit, cur, nil
}

// @Summary      List badges
// @Description  Newest first, keyset paginated. Secret and inactive badges are included with their flags.
// @Tags         badges
// @Produce      json
// @Security     BearerAuth
// @Param        tier     query string false "Filter by tier"     Enums(bronze, silver, gold, platinum, diamond)
// @Param        category query string false "Filter by category"
// @Param        active   query bool   false "Filter by is_active"
// @Param        limit    query int    false "Page size (default 25, max 100)"
// @Param        cursor   query string false "Opaque cursor from next_cursor"
// @Success      200 {object} BadgeListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /badges [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, cur, err := pageParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	q := r.URL.Query()
	f := app.BadgeFilter{Tier: q.Get("tier"), Category: q.Get("category"), Limit: limit,
		Before: cur.Before, BeforeID: cur.BeforeID}
	if raw := q.Get("active"); raw != "" {
		active, perr := strconv.ParseBool(raw)
		if perr != nil {
			httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
				map[string]string{"active": "must be a boolean"}))
			return
		}
		f.Active = &active
	}
	rows, more, err := h.svc.ListBadges(r.Context(), f)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := BadgeListResp{Data: make([]BadgeResp, len(rows))}
	for i, b := range rows {
		out.Data[i] = toBadgeResp(b)
	}
	if more {
		last := rows[len(rows)-1]
		out.NextCursor = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a badge
// @Tags         badges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateBadgeReq true "Badge"
// @Success      201 {object} BadgeResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem "badge_slug_taken"
// @Failure      422 {object} httpx.Problem
// @Router       /badges [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateBadgeReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	b, err := h.svc.CreateBadge(r.Context(), req.params())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toBadgeResp(b))
}

// @Summary      Get a badge
// @Tags         badges
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Badge id (uuid)"
// @Success      200 {object} BadgeResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "badge_not_found"
// @Router       /badges/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	badgeID, err := pathUUID(r, "id", errMalformedBadgeID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	b, err := h.svc.GetBadge(r.Context(), badgeID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toBadgeResp(b))
}

// @Summary      Update a badge (partial)
// @Description  Omitted fields stay untouched; max_awards and requirements accept null to clear. The slug never changes on rename.
// @Tags         badges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string         true "Badge id (uuid)"
// @Param        body body UpdateBadgeReq true "Fields to change"
// @Success      200 {object} BadgeResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "badge_not_found"
// @Failure      409 {object} httpx.Problem "badge_slug_taken, version_conflict"
// @Failure      422 {object} httpx.Problem
// @Router       /badges/{id} [patch]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	badgeID, err := pathUUID(r, "id", errMalformedBadgeID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req UpdateBadgeReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	b, err := h.svc.UpdateBadge(r.Context(), badgeID, req.patch())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toBadgeResp(b))
}

// @Summary      Delete a badge (soft)
// @Description  Existing holdings stay; new awards are rejected.
// @Tags         badges
// @Security     BearerAuth
// @Param        id path string true "Badge id (uuid)"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "badge_not_found"
// @Router       /badges/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	badgeID, err := pathUUID(r, "id", errMalformedBadgeID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.DeleteBadge(r.Context(), badgeID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Award a badge to a player
// @Description  Idempotency-Key is optional; with it a retry returns the original outcome (200, replay=true). Points (points_value > 0) are credited asynchronously by the points module.
// @Tags         badges
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id              path   string   true  "Badge id (uuid)"
// @Param        Idempotency-Key header string   false "Client idempotency key"
// @Param        body            body   AwardReq true  "Player"
// @Success      201 {object} AwardResp
// @Success      200 {object} AwardResp "replay"
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "badge_not_found, player_not_found"
// @Failure      409 {object} httpx.Problem "badge_already_earned, badge_max_awards_reached, badge_inactive, player_inactive"
// @Failure      422 {object} httpx.Problem
// @Router       /badges/{id}/award [post]
func (h *Handler) award(w http.ResponseWriter, r *http.Request) {
	badgeID, err := pathUUID(r, "id", errMalformedBadgeID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req AwardReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	out, err := h.svc.AwardManually(r.Context(), badgeID, req.PlayerID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	status := http.StatusCreated
	if out.Replay {
		status = http.StatusOK
	}
	httpx.JSON(w, status, toAwardResp(out))
}

// @Summary      Revoke a badge from a player
// @Description  Removes every stack. Points credited by the badge are NOT taken back. 204 also when the player did not hold it.
// @Tags         badges
// @Security     BearerAuth
// @Param        id       path string true "Badge id (uuid)"
// @Param        playerID path string true "Player id (uuid)"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "badge_not_found, player_not_found"
// @Router       /badges/{id}/players/{playerID} [delete]
func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	badgeID, err := pathUUID(r, "id", errMalformedBadgeID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	playerID, err := pathUUID(r, "playerID", errMalformedPlayerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.Revoke(r.Context(), badgeID, playerID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      List a player's badges
// @Tags         badges
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path  string true  "Player id (uuid)"
// @Param        limit    query int    false "Page size (default 25, max 100)"
// @Param        cursor   query string false "Opaque cursor from next_cursor"
// @Success      200 {object} PlayerBadgeListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "player_not_found"
// @Failure      422 {object} httpx.Problem
// @Router       /players/{playerID}/badges [get]
func (h *Handler) listForPlayer(w http.ResponseWriter, r *http.Request) {
	playerID, err := pathUUID(r, "playerID", errMalformedPlayerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	limit, cur, err := pageParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	rows, more, err := h.svc.ListPlayerBadges(r.Context(), playerID, cur, limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := PlayerBadgeListResp{Data: make([]PlayerBadgeResp, len(rows))}
	for i, v := range rows {
		out.Data[i] = toPlayerBadgeResp(v.PlayerBadge, v.Badge)
	}
	if more {
		last := rows[len(rows)-1].PlayerBadge
		out.NextCursor = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	httpx.JSON(w, http.StatusOK, out)
}
