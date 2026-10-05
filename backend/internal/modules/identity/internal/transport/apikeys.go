package transport

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/httpx"
)

type CreateAPIKeyReq struct {
	Name      string     `json:"name"       validate:"required,min=1,max=100"`
	RoleIDs   []int64    `json:"role_ids"   validate:"omitempty,max=3,dive,min=1"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// APIKeyResp never contains the secret: only the clear prefix, so a key
// can be recognised ("lvl_live_<prefix>_…") without being usable.
type APIKeyResp struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	RoleIDs    []int64    `json:"role_ids"`
	Roles      []RoleResp `json:"roles"`
	CreatedBy  *string    `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// CreatedAPIKeyResp is the one response that carries the plaintext key.
type CreatedAPIKeyResp struct {
	APIKey APIKeyResp `json:"api_key"`
	// Secret is shown once; LevelUp stores only its hash.
	Secret string `json:"secret"`
}

type APIKeyListResp struct {
	Data       []APIKeyResp `json:"data"`
	NextCursor string       `json:"next_cursor"`
}

func (h *Handler) mountAPIKeys(r chi.Router) {
	r.Route("/api-keys", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.listAPIKeys)
		r.Post("/", h.createAPIKey)
		r.Delete("/{id}", h.revokeAPIKey)
	})
}

func toAPIKeyResp(k domain.APIKey) APIKeyResp {
	roles := make([]RoleResp, 0, len(k.RoleIDs))
	for _, r := range domain.Roles(k.RoleIDs) {
		roles = append(roles, RoleResp{ID: r.ID, Key: r.Key, Label: r.Label})
	}
	out := APIKeyResp{
		ID: k.ID, Name: k.Name, Prefix: k.Prefix, RoleIDs: k.RoleIDs, Roles: roles,
		CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt, ExpiresAt: k.ExpiresAt, RevokedAt: k.RevokedAt,
	}
	if k.CreatedBy != "" {
		out.CreatedBy = &k.CreatedBy
	}
	return out
}

// @Summary      Create an API key for the tenant's backend
// @Description  The secret is returned once. Keys authenticate with "Authorization: Bearer lvl_live_…" or "X-API-Key". Requires a signed-in admin (not another key).
// @Tags         api-keys
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateAPIKeyReq true "Key"
// @Success      201 {object} CreatedAPIKeyResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /api-keys [post]
func (h *Handler) createAPIKey(w http.ResponseWriter, r *http.Request) {
	var req CreateAPIKeyReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	k, secret, err := h.svc.CreateAPIKey(r.Context(), app.CreateAPIKeyCmd{Name: req.Name, RoleIDs: req.RoleIDs, ExpiresAt: req.ExpiresAt})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, CreatedAPIKeyResp{APIKey: toAPIKeyResp(k), Secret: secret})
}

// @Summary      List the tenant's API keys (secrets are never returned)
// @Tags         api-keys
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from the previous page"
// @Success      200 {object} APIKeyListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /api-keys [get]
func (h *Handler) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	limit, cursor := pageParams(r)
	keys, next, err := h.svc.ListAPIKeys(r.Context(), limit, cursor)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]APIKeyResp, len(keys))
	for i, k := range keys {
		out[i] = toAPIKeyResp(k)
	}
	httpx.JSON(w, http.StatusOK, APIKeyListResp{Data: out, NextCursor: next})
}

// @Summary      Revoke an API key (idempotent; effective on the next request)
// @Tags         api-keys
// @Security     BearerAuth
// @Param        id path string true "API key id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /api-keys/{id} [delete]
func (h *Handler) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		httpx.Error(w, r, domain.ErrAPIKeyNotFound)
		return
	}
	if err := h.svc.RevokeAPIKey(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
