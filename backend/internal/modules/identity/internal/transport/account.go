package transport

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/httpx"
)

// Tokens are 43-character base64url strings; the bounds below only keep
// garbage out, the service decides validity.

type ForgotPasswordReq struct {
	Email string `json:"email" validate:"required,email,max=255"`
}

type ResetPasswordReq struct {
	Token                string `json:"token"                 validate:"required,max=128"`
	Password             string `json:"password"              validate:"required,min=8,max=72"`
	PasswordConfirmation string `json:"password_confirmation" validate:"omitempty,eqfield=Password"`
}

type VerifyEmailReq struct {
	Token string `json:"token" validate:"required,max=128"`
}

type CreateInvitationReq struct {
	Email   string  `json:"email"    validate:"required,email,max=255"`
	Name    string  `json:"name"     validate:"omitempty,max=255"`
	RoleIDs []int64 `json:"role_ids" validate:"required,min=1,max=10,dive,min=1"`
}

type AcceptInvitationReq struct {
	Token                string `json:"token"                 validate:"required,max=128"`
	Name                 string `json:"name"                  validate:"omitempty,max=255"`
	Password             string `json:"password"              validate:"required,min=8,max=72"`
	PasswordConfirmation string `json:"password_confirmation" validate:"omitempty,eqfield=Password"`
}

// InvitationResp never contains the token (only its hash is stored).
type InvitationResp struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Name      string     `json:"name"`
	RoleIDs   []int64    `json:"role_ids"`
	Roles     []RoleResp `json:"roles"`
	Status    string     `json:"status"` // pending | expired (lists show open invitations only)
	InvitedBy *string    `json:"invited_by"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type InvitationListResp struct {
	Data       []InvitationResp `json:"data"`
	NextCursor string           `json:"next_cursor"`
}

// InvitationPreviewResp feeds the accept-invite page.
type InvitationPreviewResp struct {
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	TenantName  string     `json:"tenant_name"`
	InviterName string     `json:"inviter_name"`
	RoleIDs     []int64    `json:"role_ids"`
	Roles       []RoleResp `json:"roles"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

func rolesResp(ids []int64) []RoleResp {
	out := make([]RoleResp, 0, len(ids))
	for _, r := range domain.Roles(ids) {
		out = append(out, RoleResp{ID: r.ID, Key: r.Key, Label: r.Label})
	}
	return out
}

func toInvitationResp(i domain.Invitation, now time.Time) InvitationResp {
	ids := i.RoleIDs
	if ids == nil {
		ids = []int64{}
	}
	out := InvitationResp{
		ID: i.ID, Email: i.Email, Name: i.Name, RoleIDs: ids, Roles: rolesResp(ids),
		Status: string(i.Status(now)), ExpiresAt: i.ExpiresAt.UTC(), CreatedAt: i.CreatedAt.UTC(),
	}
	if i.InvitedBy != "" {
		v := i.InvitedBy
		out.InvitedBy = &v
	}
	return out
}

func (h *Handler) mountAccountAuth(r chi.Router) {
	r.Post("/forgot-password", h.forgotPassword)
	r.Post("/reset-password", h.resetPassword)
	r.Post("/verify-email", h.verifyEmail)
	r.Get("/invitations/{token}", h.previewInvitation)
	r.Post("/accept-invite", h.acceptInvitation)
	r.With(httpx.RequireAuth).Post("/resend-verification", h.resendVerification)
}

// mountInvitations is called inside the authenticated /users route; the
// static "invitations" segment wins over /users/{id} in chi.
func (h *Handler) mountInvitations(r chi.Router) {
	r.Get("/invitations", h.listInvitations)
	r.Post("/invitations", h.createInvitation)
	r.Delete("/invitations/{id}", h.revokeInvitation)
}

// @Summary      Request a password reset email
// @Description  Always 202, whether or not the address has an account (no enumeration). At most 3 emails per address per hour.
// @Tags         auth
// @Accept       json
// @Param        body body ForgotPasswordReq true "Email"
// @Success      202
// @Failure      422 {object} httpx.Problem
// @Router       /auth/forgot-password [post]
func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), req.Email); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// @Summary      Set a new password with a reset token
// @Description  Single use. Revokes every session of the user. Unknown, expired, used or superseded tokens are 422 invalid_reset_token.
// @Tags         auth
// @Accept       json
// @Param        body body ResetPasswordReq true "Token and new password"
// @Success      204
// @Failure      422 {object} httpx.Problem
// @Router       /auth/reset-password [post]
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Verify an email address with a token
// @Description  Unknown, expired or used tokens, or a token for an address the user no longer has, are 422 invalid_verification_token.
// @Tags         auth
// @Accept       json
// @Param        body body VerifyEmailReq true "Token"
// @Success      204
// @Failure      422 {object} httpx.Problem
// @Router       /auth/verify-email [post]
func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req VerifyEmailReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), req.Token); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Email a fresh verification link to the caller
// @Description  Earlier links stop working. Throttled requests are accepted silently.
// @Tags         auth
// @Security     BearerAuth
// @Success      202
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem "email_already_verified"
// @Router       /auth/resend-verification [post]
func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ResendVerification(r.Context()); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// @Summary      Describe an invitation for the accept page
// @Description  Anonymous; the token is the credential. Anything but a pending invitation of an active tenant is 404 invitation_not_found.
// @Tags         auth
// @Produce      json
// @Param        token path string true "Invitation token from the email link"
// @Success      200 {object} InvitationPreviewResp
// @Failure      404 {object} httpx.Problem
// @Router       /auth/invitations/{token} [get]
func (h *Handler) previewInvitation(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.PreviewInvitation(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, InvitationPreviewResp{
		Email: p.Email, Name: p.Name, TenantName: p.TenantName, InviterName: p.InviterName,
		RoleIDs: p.RoleIDs, Roles: rolesResp(p.RoleIDs), ExpiresAt: p.ExpiresAt.UTC(),
	})
}

// @Summary      Accept an invitation and sign in
// @Description  Creates the user in the invitation's tenant with its roles (email verified) and returns a session like login.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body AcceptInvitationReq true "Token, name and password"
// @Success      201 {object} SessionResp
// @Failure      409 {object} httpx.Problem "email_taken"
// @Failure      422 {object} httpx.Problem "invalid_invitation_token"
// @Router       /auth/accept-invite [post]
func (h *Handler) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	var req AcceptInvitationReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	s, err := h.svc.AcceptInvitation(r.Context(), app.AcceptInviteCmd{
		Token: req.Token, Name: req.Name, Password: req.Password,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toSessionResp(s))
}

// @Summary      Invite a person to the caller's tenant
// @Description  Emails an accept link (valid 7 days). Same role rules as creating a user. Re-inviting an address revokes its earlier open invitation. Requires a signed-in user (not an API key).
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateInvitationReq true "Invitation"
// @Success      201 {object} InvitationResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem "email_taken"
// @Failure      422 {object} httpx.Problem
// @Router       /users/invitations [post]
func (h *Handler) createInvitation(w http.ResponseWriter, r *http.Request) {
	var req CreateInvitationReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	inv, err := h.svc.Invite(r.Context(), app.InviteCmd{Email: req.Email, Name: req.Name, RoleIDs: req.RoleIDs})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toInvitationResp(inv, time.Now()))
}

// @Summary      List the tenant's open invitations
// @Description  Not accepted and not revoked, newest first; expired ones are included with status "expired".
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from the previous page"
// @Success      200 {object} InvitationListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /users/invitations [get]
func (h *Handler) listInvitations(w http.ResponseWriter, r *http.Request) {
	limit, cursor := pageParams(r)
	rows, next, err := h.svc.ListInvitations(r.Context(), limit, cursor)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	now := time.Now()
	out := make([]InvitationResp, len(rows))
	for i, inv := range rows {
		out[i] = toInvitationResp(inv, now)
	}
	httpx.JSON(w, http.StatusOK, InvitationListResp{Data: out, NextCursor: next})
}

// @Summary      Revoke an invitation
// @Description  Idempotent. An accepted invitation is 409 invitation_already_accepted.
// @Tags         users
// @Security     BearerAuth
// @Param        id path string true "Invitation id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Router       /users/invitations/{id} [delete]
func (h *Handler) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RevokeInvitation(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
