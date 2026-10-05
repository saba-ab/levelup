// Package transport is progression's HTTP layer: DTO shape validation
// here, invariants in the domain and service.
package transport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"levelup/internal/modules/progression/internal/app"
	"levelup/internal/modules/progression/internal/domain"
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
	r.Route("/levels", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.listLevels)
		r.Post("/", h.createLevel)
		r.Get("/{levelID}", h.getLevel)
		r.Patch("/{levelID}", h.updateLevel)
		r.Delete("/{levelID}", h.deleteLevel)
	})
	// Leaf routes, not a /players sub-router: the player module owns that
	// prefix, and a second Mount on it would shadow or conflict with it.
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/players/{playerID}/progress", h.getProgress)
		r.Post("/players/{playerID}/xp", h.grantXP)
		r.Get("/players/{playerID}/xp-grants", h.listGrants)
	})
}

// ---- DTOs ----

type CreateLevelReq struct {
	LevelNumber   int            `json:"level_number" validate:"required,min=1"`
	Name          string         `json:"name" validate:"max=255"`
	Description   string         `json:"description" validate:"max=1000"`
	XPRequired    int64          `json:"xp_required" validate:"min=0"`
	PointsReward  int64          `json:"points_reward" validate:"min=0"`
	BadgeRewardID string         `json:"badge_reward_id" validate:"omitempty,uuid"`
	Perks         map[string]any `json:"perks"`
	IconURL       string         `json:"icon_url" validate:"omitempty,url,max=500"`
	IsActive      *bool          `json:"is_active"`
}

// UpdateLevelReq is a partial update: omitted fields stay untouched.
// badge_reward_id: null clears the reward.
type UpdateLevelReq struct {
	LevelNumber   *int            `json:"level_number" validate:"omitempty,min=1"`
	Name          *string         `json:"name" validate:"omitempty,min=1,max=255"`
	Description   *string         `json:"description" validate:"omitempty,max=1000"`
	XPRequired    *int64          `json:"xp_required" validate:"omitempty,min=0"`
	PointsReward  *int64          `json:"points_reward" validate:"omitempty,min=0"`
	BadgeRewardID NullableString  `json:"badge_reward_id" swaggertype:"string"`
	Perks         *map[string]any `json:"perks"`
	IconURL       *string         `json:"icon_url" validate:"omitempty,max=500"`
	IsActive      *bool           `json:"is_active"`
}

// NullableString tells "absent" from "null" (JSON merge-patch semantics).
type NullableString struct {
	Present bool
	Value   *string
}

func (n *NullableString) UnmarshalJSON(b []byte) error {
	n.Present = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		n.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	n.Value = &s
	return nil
}

type GrantXPReq struct {
	Amount      int64  `json:"amount" validate:"required,gt=0"`
	Description string `json:"description" validate:"max=500"`
}

type LevelResp struct {
	ID            string         `json:"id"`
	LevelNumber   int            `json:"level_number"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	XPRequired    int64          `json:"xp_required"`
	PointsReward  int64          `json:"points_reward"`
	BadgeRewardID *string        `json:"badge_reward_id"`
	Perks         map[string]any `json:"perks"`
	IconURL       string         `json:"icon_url"`
	IsActive      bool           `json:"is_active"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type LevelListResp struct {
	Data       []LevelResp `json:"data"`
	NextCursor string      `json:"next_cursor"`
}

type LevelRefResp struct {
	ID          string `json:"id"`
	LevelNumber int    `json:"level_number"`
	Name        string `json:"name"`
	XPRequired  int64  `json:"xp_required"`
}

type ProgressResp struct {
	PlayerID        string        `json:"player_id"`
	TotalXP         int64         `json:"total_xp"`
	CurrentLevel    *LevelRefResp `json:"current_level"`
	NextLevel       *LevelRefResp `json:"next_level"`
	XPToNext        *int64        `json:"xp_to_next"`
	ProgressPercent float64       `json:"progress_percent"`
}

type GrantResp struct {
	ID             string    `json:"id"`
	PlayerID       string    `json:"player_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Amount         int64     `json:"amount"`
	Description    string    `json:"description"`
	SourceKind     string    `json:"source_kind"`
	SourceID       string    `json:"source_id"`
	ActivityID     string    `json:"activity_id,omitempty"`
	OccurredAt     time.Time `json:"occurred_at"`
	CreatedBy      string    `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type GrantListResp struct {
	Data       []GrantResp `json:"data"`
	NextCursor string      `json:"next_cursor"`
}

type GrantXPResp struct {
	Grant         GrantResp      `json:"grant"`
	Replayed      bool           `json:"replayed"`
	TotalXP       int64          `json:"total_xp"`
	LevelNumber   int            `json:"level_number"`
	LevelsReached []LevelRefResp `json:"levels_reached"`
}

// ---- levels ----

// @Summary      List the tenant's level ladder, ordered by level_number
// @Tags         progression
// @Produce      json
// @Security     BearerAuth
// @Param        active query bool false "Only active levels"
// @Success      200 {object} LevelListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /levels [get]
func (h *Handler) listLevels(w http.ResponseWriter, r *http.Request) {
	activeOnly, _ := strconv.ParseBool(r.URL.Query().Get("active"))
	levels, err := h.svc.ListLevels(r.Context(), activeOnly)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := LevelListResp{Data: make([]LevelResp, len(levels))}
	for i, l := range levels {
		out.Data[i] = toLevelResp(l)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a level
// @Tags         progression
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateLevelReq true "Level"
// @Success      201 {object} LevelResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem "level_number_taken"
// @Failure      422 {object} httpx.Problem "validation, xp_required_not_increasing"
// @Router       /levels [post]
func (h *Handler) createLevel(w http.ResponseWriter, r *http.Request) {
	var req CreateLevelReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	l, err := h.svc.CreateLevel(r.Context(), domain.LevelSpec{
		Number:        req.LevelNumber,
		Name:          req.Name,
		Description:   req.Description,
		XPRequired:    req.XPRequired,
		PointsReward:  req.PointsReward,
		BadgeRewardID: req.BadgeRewardID,
		Perks:         req.Perks,
		IconURL:       req.IconURL,
		Active:        active,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toLevelResp(l))
}

// @Summary      Get a level
// @Tags         progression
// @Produce      json
// @Security     BearerAuth
// @Param        levelID path string true "Level id (uuid)"
// @Success      200 {object} LevelResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "level_not_found"
// @Router       /levels/{levelID} [get]
func (h *Handler) getLevel(w http.ResponseWriter, r *http.Request) {
	levelID, ok := pathUUID(w, r, "levelID", domain.ErrLevelNotFound)
	if !ok {
		return
	}
	l, err := h.svc.GetLevel(r.Context(), levelID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toLevelResp(l))
}

// @Summary      Update a level (partial; omitted fields stay untouched)
// @Tags         progression
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        levelID path string true "Level id (uuid)"
// @Param        body body UpdateLevelReq true "Fields to change"
// @Success      200 {object} LevelResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "level_not_found"
// @Failure      409 {object} httpx.Problem "level_number_taken"
// @Failure      422 {object} httpx.Problem "validation, xp_required_not_increasing"
// @Router       /levels/{levelID} [patch]
func (h *Handler) updateLevel(w http.ResponseWriter, r *http.Request) {
	levelID, ok := pathUUID(w, r, "levelID", domain.ErrLevelNotFound)
	if !ok {
		return
	}
	var req UpdateLevelReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	patch := domain.LevelPatch{
		Number:       req.LevelNumber,
		Name:         req.Name,
		Description:  req.Description,
		XPRequired:   req.XPRequired,
		PointsReward: req.PointsReward,
		Perks:        req.Perks,
		IconURL:      req.IconURL,
		Active:       req.IsActive,
	}
	if req.BadgeRewardID.Present {
		badge := ""
		if req.BadgeRewardID.Value != nil {
			badge = *req.BadgeRewardID.Value
			if _, err := uuid.Parse(badge); err != nil {
				httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
					map[string]string{"badge_reward_id": "must be a valid uuid"}))
				return
			}
		}
		patch.BadgeRewardID = &badge
	}
	l, err := h.svc.UpdateLevel(r.Context(), levelID, patch)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toLevelResp(l))
}

// @Summary      Delete a level (soft delete)
// @Tags         progression
// @Security     BearerAuth
// @Param        levelID path string true "Level id (uuid)"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "level_not_found"
// @Router       /levels/{levelID} [delete]
func (h *Handler) deleteLevel(w http.ResponseWriter, r *http.Request) {
	levelID, ok := pathUUID(w, r, "levelID", domain.ErrLevelNotFound)
	if !ok {
		return
	}
	if err := h.svc.DeleteLevel(r.Context(), levelID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- players ----

// @Summary      Get a player's XP and level
// @Tags         progression
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path string true "Player id (uuid)"
// @Success      200 {object} ProgressResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "player_not_found"
// @Router       /players/{playerID}/progress [get]
func (h *Handler) getProgress(w http.ResponseWriter, r *http.Request) {
	playerID, ok := pathUUID(w, r, "playerID", domain.ErrPlayerNotFound)
	if !ok {
		return
	}
	v, err := h.svc.GetProgress(r.Context(), playerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, ProgressResp{
		PlayerID:        v.PlayerID,
		TotalXP:         v.TotalXP,
		CurrentLevel:    toLevelRef(v.Current),
		NextLevel:       toLevelRef(v.Next),
		XPToNext:        v.XPToNext,
		ProgressPercent: v.ProgressPercent,
	})
}

// @Summary      Grant XP to a player
// @Description  Idempotent with the Idempotency-Key header: a repeated key is a no-op (replayed=true).
// @Tags         progression
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path string true "Player id (uuid)"
// @Param        Idempotency-Key header string false "Client idempotency key"
// @Param        body body GrantXPReq true "Grant"
// @Success      200 {object} GrantXPResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "player_not_found"
// @Failure      409 {object} httpx.Problem "player_inactive"
// @Failure      422 {object} httpx.Problem
// @Router       /players/{playerID}/xp [post]
func (h *Handler) grantXP(w http.ResponseWriter, r *http.Request) {
	playerID, ok := pathUUID(w, r, "playerID", domain.ErrPlayerNotFound)
	if !ok {
		return
	}
	var req GrantXPReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.GrantXP(r.Context(), app.ManualGrant{
		PlayerID:       playerID,
		Amount:         req.Amount,
		Description:    req.Description,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := GrantXPResp{
		Grant:         toGrantResp(res.Grant),
		Replayed:      res.Replayed,
		TotalXP:       res.Progress.TotalXP,
		LevelNumber:   res.Progress.LevelNumber,
		LevelsReached: make([]LevelRefResp, 0, len(res.LevelsReached)),
	}
	for i := range res.LevelsReached {
		out.LevelsReached = append(out.LevelsReached, *toLevelRef(&res.LevelsReached[i]))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      List a player's XP grants, newest first
// @Tags         progression
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path string true "Player id (uuid)"
// @Param        limit query int false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from next_cursor"
// @Success      200 {object} GrantListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "player_not_found"
// @Failure      422 {object} httpx.Problem "malformed cursor"
// @Router       /players/{playerID}/xp-grants [get]
func (h *Handler) listGrants(w http.ResponseWriter, r *http.Request) {
	playerID, ok := pathUUID(w, r, "playerID", domain.ErrPlayerNotFound)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := h.svc.ListGrants(r.Context(), playerID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := GrantListResp{Data: make([]GrantResp, len(page.Grants)), NextCursor: page.NextCursor}
	for i, g := range page.Grants {
		out.Data[i] = toGrantResp(g)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ---- helpers ----

// pathUUID rejects a non-uuid id as not found (Laravel answered 500, B28).
func pathUUID(w http.ResponseWriter, r *http.Request, param string, notFound error) (string, bool) {
	raw := chi.URLParam(r, param)
	if _, err := uuid.Parse(raw); err != nil {
		httpx.Error(w, r, notFound)
		return "", false
	}
	return raw, true
}

func toLevelResp(l domain.Level) LevelResp {
	out := LevelResp{
		ID:           l.ID,
		LevelNumber:  l.Number,
		Name:         l.Name,
		Description:  l.Description,
		XPRequired:   l.XPRequired,
		PointsReward: l.PointsReward,
		Perks:        l.Perks,
		IconURL:      l.IconURL,
		IsActive:     l.Active,
		CreatedAt:    l.CreatedAt,
		UpdatedAt:    l.UpdatedAt,
	}
	if l.BadgeRewardID != "" {
		b := l.BadgeRewardID
		out.BadgeRewardID = &b
	}
	return out
}

func toLevelRef(l *domain.Level) *LevelRefResp {
	if l == nil {
		return nil
	}
	return &LevelRefResp{ID: l.ID, LevelNumber: l.Number, Name: l.Name, XPRequired: l.XPRequired}
}

func toGrantResp(g domain.XPGrant) GrantResp {
	return GrantResp{
		ID:             g.ID,
		PlayerID:       g.PlayerID,
		IdempotencyKey: g.IdempotencyKey,
		Amount:         g.Amount,
		Description:    g.Description,
		SourceKind:     g.SourceKind,
		SourceID:       g.SourceID,
		ActivityID:     g.ActivityID,
		OccurredAt:     g.OccurredAt,
		CreatedBy:      g.CreatedBy,
		CreatedAt:      g.CreatedAt,
	}
}
