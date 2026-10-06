// Package transport is rewards' HTTP layer: DTO shape validation here,
// invariants in the domain, rules and authorization in the service.
package transport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/app"
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
	r.Route("/rewards", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/claims", h.listClaims)
		r.Get("/stats", h.stats)
		r.Get("/claims/{claimID}", h.getClaim)
		r.Post("/claims/{claimID}/redeem", h.redeem)
		r.Post("/claims/{claimID}/cancel", h.cancel)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
		r.Post("/{id}/claim", h.claim)
	})
	// /players is the player module's prefix: register a single route
	// rather than mounting a sub-router on it.
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/players/{playerID}/reward-claims", h.playerClaims)
	})
}

// @Summary      List rewards
// @Tags         rewards
// @Produce      json
// @Security     BearerAuth
// @Param        status    query string false "Filter by status"
// @Param        type      query string false "Filter by type"
// @Param        is_active query bool   false "Filter by active flag"
// @Param        limit     query int    false "Page size (default 25, max 100)"
// @Param        cursor    query string false "Opaque cursor from next_cursor"
// @Success      200 {object} RewardListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rewards [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := app.RewardFilter{Status: q.Get("status"), Type: q.Get("type")}
	if v := q.Get("is_active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "validation failed"), map[string]string{"is_active": "must be a boolean"}))
			return
		}
		f.IsActive = &b
	}
	limit, err := limitOf(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	rows, next, err := h.svc.ListRewards(r.Context(), f, q.Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := RewardListResp{Data: make([]RewardResp, len(rows)), NextCursor: next}
	for i, rw := range rows {
		out.Data[i] = toRewardResp(rw)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a reward
// @Tags         rewards
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateRewardReq true "Reward"
// @Success      201 {object} RewardResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rewards [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRewardReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	rw, err := h.svc.CreateReward(r.Context(), req.toPatch())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toRewardResp(rw))
}

// @Summary      Get a reward
// @Tags         rewards
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Reward id"
// @Success      200 {object} RewardResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /rewards/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	rw, err := h.svc.GetReward(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toRewardResp(rw))
}

// @Summary      Update a reward (partial)
// @Tags         rewards
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string true "Reward id"
// @Param        body body UpdateRewardReq true "Fields to change; omitted fields stay untouched"
// @Success      200 {object} RewardResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rewards/{id} [patch]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateRewardReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	rw, err := h.svc.UpdateReward(r.Context(), chi.URLParam(r, "id"), req.toPatch())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toRewardResp(rw))
}

// @Summary      Delete a reward (soft delete)
// @Tags         rewards
// @Security     BearerAuth
// @Param        id path string true "Reward id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /rewards/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteReward(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Claim a reward for a player
// @Description  Free rewards settle immediately (201, status claimed). Paid rewards return 202 with status pending_payment while points are debited; poll GET /rewards/claims/{claimID}. The Idempotency-Key header makes the call replayable: the same key returns the same claim (200).
// @Tags         rewards
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id              path   string   true  "Reward id"
// @Param        Idempotency-Key header string   false "Client request id"
// @Param        body            body   ClaimReq true  "Claim"
// @Success      200 {object} ClaimResp "Replay of an earlier claim with the same Idempotency-Key"
// @Success      201 {object} ClaimResp "Claimed (free reward)"
// @Success      202 {object} ClaimResp "pending_payment (paid reward)"
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rewards/{id}/claim [post]
func (h *Handler) claim(w http.ResponseWriter, r *http.Request) {
	var req ClaimReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	c, created, err := h.svc.Claim(r.Context(), chi.URLParam(r, "id"), req.PlayerID, r.Header.Get("Idempotency-Key"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	status := http.StatusOK
	switch {
	case created && c.Status == contracts.ClaimPendingPayment:
		status = http.StatusAccepted
	case created:
		status = http.StatusCreated
	}
	httpx.JSON(w, status, toClaimResp(c))
}

// @Summary      Get a reward claim (poll its status)
// @Tags         rewards
// @Produce      json
// @Security     BearerAuth
// @Param        claimID path string true "Claim id"
// @Success      200 {object} ClaimResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /rewards/claims/{claimID} [get]
func (h *Handler) getClaim(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.GetClaim(r.Context(), chi.URLParam(r, "claimID"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toClaimResp(c))
}

// @Summary      Redeem a claimed reward
// @Tags         rewards
// @Produce      json
// @Security     BearerAuth
// @Param        claimID path string true "Claim id"
// @Success      200 {object} ClaimResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Router       /rewards/claims/{claimID}/redeem [post]
func (h *Handler) redeem(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Redeem(r.Context(), chi.URLParam(r, "claimID"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toClaimResp(c))
}

// @Summary      Cancel a claim (admin); paid claims are refunded
// @Tags         rewards
// @Produce      json
// @Security     BearerAuth
// @Param        claimID path string true "Claim id"
// @Success      200 {object} ClaimResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Router       /rewards/claims/{claimID}/cancel [post]
func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Cancel(r.Context(), chi.URLParam(r, "claimID"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toClaimResp(c))
}

// @Summary      List a player's reward claims
// @Tags         rewards
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path  string true  "Player id"
// @Param        limit    query int    false "Page size (default 25, max 100)"
// @Param        cursor   query string false "Opaque cursor from next_cursor"
// @Success      200 {object} ClaimListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /players/{playerID}/reward-claims [get]
func (h *Handler) playerClaims(w http.ResponseWriter, r *http.Request) {
	limit, err := limitOf(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	rows, next, err := h.svc.ListPlayerClaims(r.Context(), chi.URLParam(r, "playerID"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := ClaimListResp{Data: make([]ClaimResp, len(rows)), NextCursor: next}
	for i, c := range rows {
		out.Data[i] = toClaimResp(c)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      List the tenant's reward claims (redemption history)
// @Description  Newest first, keyset paginated. from (inclusive) and to (exclusive) bound created_at, RFC 3339.
// @Tags         rewards
// @Produce      json
// @Security     BearerAuth
// @Param        status    query string false "Claim status" Enums(pending_payment, claimed, rejected, redeemed, expired, cancelled, refund_pending, refunded)
// @Param        reward_id query string false "Reward id (uuid)"
// @Param        player_id query string false "Player id (uuid)"
// @Param        from      query string false "Created at or after (RFC 3339)"
// @Param        to        query string false "Created before (RFC 3339)"
// @Param        limit     query int    false "Page size (default 25, max 100)"
// @Param        cursor    query string false "Opaque cursor from next_cursor"
// @Success      200 {object} ClaimListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /rewards/claims [get]
func (h *Handler) listClaims(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := app.ClaimFilter{Status: q.Get("status"), RewardID: q.Get("reward_id"), PlayerID: q.Get("player_id")}
	fields := map[string]string{}
	for name, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		v := q.Get(name)
		if v == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			fields[name] = "must be an RFC 3339 timestamp"
			continue
		}
		t = t.UTC()
		*dst = &t
	}
	if len(fields) > 0 {
		httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "validation failed"), fields))
		return
	}
	limit, err := limitOf(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	rows, next, err := h.svc.ListClaims(r.Context(), f, q.Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := ClaimListResp{Data: make([]ClaimResp, len(rows)), NextCursor: next}
	for i, c := range rows {
		out.Data[i] = toClaimResp(c)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Claim statistics per reward
// @Description  Every live reward (and deleted rewards that have claims) with claimed, redeemed, expired, cancelled and points_spent, plus tenant totals.
// @Tags         rewards
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} StatsResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /rewards/stats [get]
func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	rep, err := h.svc.Stats(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toStatsResp(rep))
}

func limitOf(r *http.Request) (int, error) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 100 {
		return 0, errs.WithFields(errs.New(errs.Invalid, "validation failed"), map[string]string{"limit": "must be between 1 and 100"})
	}
	return n, nil
}
