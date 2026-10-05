// Package transport is identity's HTTP layer: DTO shape validation here,
// invariants in the domain, authorization in the service.
package transport

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/httpx"
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
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", h.register)
		r.Post("/login", h.login)
		r.Post("/refresh", h.refresh)
		r.Group(func(r chi.Router) {
			r.Use(httpx.RequireAuth)
			r.Post("/logout", h.logout)
			r.Get("/me", h.me)
		})
	})
	r.Route("/users", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.listUsers)
		r.Post("/", h.createUser)
		r.Get("/{id}", h.getUser)
		r.Patch("/{id}", h.updateUser)
		r.Delete("/{id}", h.deleteUser)
		r.Put("/{id}/roles", h.assignRoles)
	})
	r.Route("/tenant", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.getTenant)
		r.Patch("/", h.updateTenant)
		r.Delete("/", h.deleteTenant)
	})
	r.Route("/platform/tenants", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.platformListTenants)
		r.Patch("/{id}", h.platformUpdateTenant)
	})
}

func pageParams(r *http.Request) (int, string) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	return limit, r.URL.Query().Get("cursor")
}

// ---- auth ----

// @Summary      Register a tenant and its owner
// @Description  Atomically creates the tenant, the owner user and the owner role, and returns a session.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body RegisterReq true "Registration"
// @Success      201 {object} SessionResp
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /auth/register [post]
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	s, err := h.svc.Register(r.Context(), app.RegisterCmd{
		TenantName: req.TenantName, Name: req.Name, Email: req.Email,
		Password: req.Password, Timezone: req.Timezone,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toSessionResp(s))
}

// @Summary      Log in with email and password
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body LoginReq true "Credentials"
// @Success      200 {object} SessionResp
// @Failure      401 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /auth/login [post]
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	s, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toSessionResp(s))
}

// @Summary      Rotate a refresh token
// @Description  Consumes the refresh token and returns a new pair; a replayed token revokes every session of the user.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body RefreshReq true "Refresh token"
// @Success      200 {object} SessionResp
// @Failure      401 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /auth/refresh [post]
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	s, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toSessionResp(s))
}

// @Summary      Log out everywhere
// @Description  Deny-lists the presented access token and revokes every refresh token of the user.
// @Tags         auth
// @Security     BearerAuth
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Router       /auth/logout [post]
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Logout(r.Context()); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      The authenticated user, roles and tenant
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} MeResp
// @Failure      401 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /auth/me [get]
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u, t, err := h.svc.Me(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, MeResp{User: toUserResp(u), Tenant: toTenantPtr(t)})
}

// ---- users ----

// @Summary      List the users of the caller's tenant
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from the previous page"
// @Success      200 {object} UserListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /users [get]
func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	limit, cursor := pageParams(r)
	users, next, err := h.svc.ListUsers(r.Context(), limit, cursor)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]UserResp, len(users))
	for i, u := range users {
		out[i] = toUserResp(u)
	}
	httpx.JSON(w, http.StatusOK, UserListResp{Data: out, NextCursor: next})
}

// @Summary      Create a user in the caller's tenant
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateUserReq true "User"
// @Success      201 {object} UserResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /users [post]
func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, err := h.svc.CreateUser(r.Context(), app.CreateUserCmd{
		Name: req.Name, Email: req.Email, Password: req.Password, RoleIDs: req.RoleIDs,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toUserResp(u))
}

// @Summary      Get a user of the caller's tenant
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "User id"
// @Success      200 {object} UserResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /users/{id} [get]
func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.GetUser(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toUserResp(u))
}

// @Summary      Update a user (partial)
// @Description  Self-service is always allowed; changing your own password requires current_password.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string        true "User id"
// @Param        body body UpdateUserReq true "Fields to change"
// @Success      200 {object} UserResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /users/{id} [patch]
func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var req UpdateUserReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, err := h.svc.UpdateUser(r.Context(), chi.URLParam(r, "id"), app.UpdateUserCmd{
		Name: req.Name, Email: req.Email, Password: req.Password,
		CurrentPassword: req.CurrentPassword, Active: req.Active,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toUserResp(u))
}

// @Summary      Delete a user of the caller's tenant
// @Tags         users
// @Security     BearerAuth
// @Param        id path string true "User id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Router       /users/{id} [delete]
func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteUser(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Replace a user's roles
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string         true "User id"
// @Param        body body AssignRolesReq true "Role ids"
// @Success      200 {object} UserResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /users/{id}/roles [put]
func (h *Handler) assignRoles(w http.ResponseWriter, r *http.Request) {
	var req AssignRolesReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	u, err := h.svc.AssignRoles(r.Context(), chi.URLParam(r, "id"), req.RoleIDs)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toUserResp(u))
}

// ---- tenant ----

// @Summary      The caller's tenant
// @Tags         tenant
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} TenantResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /tenant [get]
func (h *Handler) getTenant(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.CurrentTenant(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toTenantResp(t))
}

// @Summary      Update the caller's tenant (partial)
// @Tags         tenant
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body UpdateTenantReq true "Fields to change; settings replaces the whole object"
// @Success      200 {object} TenantResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /tenant [patch]
func (h *Handler) updateTenant(w http.ResponseWriter, r *http.Request) {
	var req UpdateTenantReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	t, err := h.svc.UpdateCurrentTenant(r.Context(), domain.TenantChanges{
		Name: req.Name, Timezone: req.Timezone, Settings: req.Settings,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toTenantResp(t))
}

// @Summary      Delete the caller's tenant (owner only)
// @Description  Soft delete; every module purges its data on tenant.deleted.v1.
// @Tags         tenant
// @Security     BearerAuth
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /tenant [delete]
func (h *Handler) deleteTenant(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteCurrentTenant(r.Context()); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- platform ----

// @Summary      List tenants (platform admins)
// @Tags         platform
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from the previous page"
// @Success      200 {object} TenantListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /platform/tenants [get]
func (h *Handler) platformListTenants(w http.ResponseWriter, r *http.Request) {
	limit, cursor := pageParams(r)
	tenants, next, err := h.svc.PlatformListTenants(r.Context(), limit, cursor)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]TenantResp, len(tenants))
	for i, t := range tenants {
		out[i] = toTenantResp(t)
	}
	httpx.JSON(w, http.StatusOK, TenantListResp{Data: out, NextCursor: next})
}

// @Summary      Activate or deactivate a tenant (platform admins)
// @Tags         platform
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string                  true "Tenant id"
// @Param        body body PlatformUpdateTenantReq true "Active flag"
// @Success      200 {object} TenantResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /platform/tenants/{id} [patch]
func (h *Handler) platformUpdateTenant(w http.ResponseWriter, r *http.Request) {
	var req PlatformUpdateTenantReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	t, err := h.svc.PlatformSetTenantActive(r.Context(), chi.URLParam(r, "id"), *req.Active)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toTenantResp(t))
}
