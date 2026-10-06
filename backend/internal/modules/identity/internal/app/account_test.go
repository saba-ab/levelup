package app

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/mail"
	"levelup/internal/shared/errs"
)

var linkRe = regexp.MustCompile(`https://portal\.test(/[a-z-]+)\?token=([A-Za-z0-9_%-]+)`)

// tokenFrom extracts the token of the link to path in m (text and HTML
// must carry the same link).
func tokenFrom(t *testing.T, m mail.Message, path string) string {
	t.Helper()
	got := linkRe.FindStringSubmatch(m.Text)
	require.NotNil(t, got, "no link in %q", m.Text)
	require.Equal(t, path, got[1])
	require.Contains(t, m.HTML, got[0])
	tok, err := url.QueryUnescape(got[2])
	require.NoError(t, err)
	return tok
}

func (h *harness) lastMail(t *testing.T) mail.Message {
	t.Helper()
	require.NotEmpty(t, h.mail.Sent)
	return h.mail.Sent[len(h.mail.Sent)-1]
}

func mustCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, code, errs.CodeOf(err), "got %v", err)
}

var inviterPerms = allowKeys{contracts.PermUsersCreate.Key(): true}

// ---- password reset ----

func TestForgotPasswordIsEnumerationSafe(t *testing.T) {
	h := newHarness(t, allowKeys{})
	tn, _ := h.seedTenant(t, "A")
	u := h.seedUser(t, tn.ID, "member@example.com", "old-password", contracts.RoleDeveloper)
	inactive := h.seedUser(t, tn.ID, "inactive@example.com", "old-password", contracts.RoleDeveloper)
	in := h.repo.users[inactive.ID]
	in.Active = false
	h.repo.users[inactive.ID] = in
	suspended, _ := h.seedTenant(t, "Suspended")
	su := h.seedUser(t, suspended.ID, "suspended@example.com", "old-password", contracts.RoleDeveloper)
	st := h.repo.tenants[suspended.ID]
	st.Active = false
	h.repo.tenants[suspended.ID] = st

	ctx := context.Background()
	for _, email := range []string{"nobody@example.com", "INACTIVE@example.com", su.Email, "not an email"} {
		require.NoError(t, h.svc.ForgotPassword(ctx, email), email)
	}
	require.Empty(t, h.mail.Sent, "unknown, inactive and suspended accounts get no mail")
	require.Empty(t, h.repo.resets)

	require.NoError(t, h.svc.ForgotPassword(ctx, "  Member@Example.com "))
	require.Len(t, h.mail.Sent, 1)
	m := h.lastMail(t)
	require.Equal(t, u.Email, m.To)
	tok := tokenFrom(t, m, "/reset-password")
	require.True(t, domain.WellFormedAccountToken(tok))
	require.Len(t, h.repo.resets, 1)
	for _, r := range h.repo.resets {
		require.NotEqual(t, tok, r.TokenHash, "only the hash is stored")
		require.Equal(t, domain.HashAccountToken(tok), r.TokenHash)
		require.Equal(t, h.clock.Now().Add(time.Hour), r.ExpiresAt)
	}
}

func TestResetPasswordIsSingleUseAndRevokesSessions(t *testing.T) {
	h := newHarness(t, allowKeys{})
	tn, _ := h.seedTenant(t, "A")
	u := h.seedUser(t, tn.ID, "member@example.com", "old-password", contracts.RoleDeveloper)
	_, err := h.refresh.Issue(context.Background(), u.ID)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, h.svc.ForgotPassword(ctx, u.Email))
	tok := tokenFrom(t, h.lastMail(t), "/reset-password")

	// A policy failure does not burn the token.
	mustCode(t, h.svc.ResetPassword(ctx, tok, "short"), "password_too_short")

	require.NoError(t, h.svc.ResetPassword(ctx, tok, "brand-new-password"))
	require.Contains(t, h.refresh.revoked, u.ID, "every session is revoked")
	require.Equal(t, []string{contracts.TopicUserUpdated}, h.outbox.topics())
	got := h.repo.users[u.ID]
	require.NotNil(t, got.PasswordChangedAt)
	require.NotNil(t, got.EmailVerifiedAt, "the link proved the mailbox")

	_, err = h.svc.Login(ctx, u.Email, "old-password")
	require.ErrorIs(t, err, domain.ErrBadCredentials)
	_, err = h.svc.Login(ctx, u.Email, "brand-new-password")
	require.NoError(t, err)

	err = h.svc.ResetPassword(ctx, tok, "another-password")
	require.ErrorIs(t, err, domain.ErrInvalidResetToken, "single use")
	mustKind(t, err, errs.Invalid)
	mustCode(t, err, "invalid_reset_token")
}

func TestResetPasswordRejectsExpiredGarbageSupersededAndStaleEmail(t *testing.T) {
	h := newHarness(t, allowKeys{})
	tn, _ := h.seedTenant(t, "A")
	u := h.seedUser(t, tn.ID, "member@example.com", "old-password", contracts.RoleDeveloper)
	ctx := context.Background()

	for _, garbage := range []string{"", "x", strings.Repeat("A", 43), strings.Repeat("!", 43)} {
		require.ErrorIs(t, h.svc.ResetPassword(ctx, garbage, "new-password-1"), domain.ErrInvalidResetToken)
	}

	// Expiry.
	require.NoError(t, h.svc.ForgotPassword(ctx, u.Email))
	expired := tokenFrom(t, h.lastMail(t), "/reset-password")
	h.clock.Advance(time.Hour)
	require.ErrorIs(t, h.svc.ResetPassword(ctx, expired, "new-password-1"), domain.ErrInvalidResetToken)

	// A newer request invalidates the older link.
	require.NoError(t, h.svc.ForgotPassword(ctx, u.Email))
	older := tokenFrom(t, h.lastMail(t), "/reset-password")
	require.NoError(t, h.svc.ForgotPassword(ctx, u.Email))
	newer := tokenFrom(t, h.lastMail(t), "/reset-password")
	require.ErrorIs(t, h.svc.ResetPassword(ctx, older, "new-password-1"), domain.ErrInvalidResetToken)

	// The address changed after the link was sent: the link is dead.
	moved := h.repo.users[u.ID]
	moved.Email = "moved@example.com"
	h.repo.users[u.ID] = moved
	require.ErrorIs(t, h.svc.ResetPassword(ctx, newer, "new-password-1"), domain.ErrInvalidResetToken)
	require.Empty(t, h.outbox.published)
	require.Empty(t, h.refresh.revoked)
}

func TestForgotPasswordIsRateLimitedPerAddressAndFailsOpen(t *testing.T) {
	h := newHarness(t, allowKeys{})
	tn, _ := h.seedTenant(t, "A")
	a := h.seedUser(t, tn.ID, "a@example.com", "old-password", contracts.RoleDeveloper)
	b := h.seedUser(t, tn.ID, "b@example.com", "old-password", contracts.RoleDeveloper)
	ctx := context.Background()

	for range 5 {
		require.NoError(t, h.svc.ForgotPassword(ctx, a.Email), "throttled requests still succeed")
	}
	require.Len(t, h.mail.Sent, 3, "max 3 mails per address per hour")
	require.NoError(t, h.svc.ForgotPassword(ctx, "A@EXAMPLE.COM"))
	require.Len(t, h.mail.Sent, 3, "the key is the normalised address")

	require.NoError(t, h.svc.ForgotPassword(ctx, b.Email))
	require.Len(t, h.mail.Sent, 4, "other addresses are unaffected")

	h.throttle.fail = errors.New("redis down")
	require.NoError(t, h.svc.ForgotPassword(ctx, a.Email))
	require.Len(t, h.mail.Sent, 5, "a broken throttle fails open")
}

// ---- invitations ----

func TestInviteAcceptCreatesUserInInviteTenantWithRoles(t *testing.T) {
	h := newHarness(t, inviterPerms)
	tn, owner := h.seedTenant(t, "Acme")
	ctx := ctxAs(principalOf(owner))

	inv, err := h.svc.Invite(ctx, InviteCmd{Email: " New@Example.com ", Name: "Nina", RoleIDs: []int64{contracts.RoleDeveloper, contracts.RoleAdmin}})
	require.NoError(t, err)
	require.Equal(t, "new@example.com", inv.Email)
	require.Equal(t, tn.ID, inv.TenantID)
	require.Equal(t, owner.ID, inv.InvitedBy)
	require.Equal(t, []int64{contracts.RoleAdmin, contracts.RoleDeveloper}, inv.RoleIDs)
	require.Equal(t, h.clock.Now().Add(7*24*time.Hour), inv.ExpiresAt)
	require.Empty(t, h.outbox.published, "no user exists yet")

	m := h.lastMail(t)
	require.Equal(t, "new@example.com", m.To)
	require.Contains(t, m.Subject, "Acme")
	tok := tokenFrom(t, m, "/accept-invite")

	list, next, err := h.svc.ListInvitations(ctx, 0, "")
	require.NoError(t, err)
	require.Empty(t, next)
	require.Len(t, list, 1)

	preview, err := h.svc.PreviewInvitation(context.Background(), tok)
	require.NoError(t, err)
	require.Equal(t, InvitationPreview{
		Email: "new@example.com", Name: "Nina", TenantName: "Acme", InviterName: owner.Name,
		RoleIDs: inv.RoleIDs, ExpiresAt: inv.ExpiresAt,
	}, preview)

	s, err := h.svc.AcceptInvitation(context.Background(), AcceptInviteCmd{Token: tok, Password: "chosen-password"})
	require.NoError(t, err)
	require.Equal(t, tn.ID, s.User.TenantID)
	require.NotNil(t, s.Tenant)
	require.Equal(t, tn.ID, s.Tenant.ID)
	require.Equal(t, "Nina", s.User.Name, "name falls back to the invitation's")
	require.Equal(t, inv.RoleIDs, s.User.RoleIDs)
	require.NotNil(t, s.User.EmailVerifiedAt)
	require.NotEmpty(t, s.AccessToken)
	require.NotEmpty(t, s.RefreshToken)
	p, _, err := h.issuer.Verify(s.AccessToken)
	require.NoError(t, err)
	require.Equal(t, tn.ID, p.TenantID)
	require.Equal(t, inv.RoleIDs, p.RoleIDs)

	require.Equal(t, []string{contracts.TopicUserCreated}, h.outbox.topics())
	created := h.outbox.published[0].payload.(contracts.UserCreatedV1)
	require.Equal(t, s.User.ID, created.UserID)
	require.Equal(t, tn.ID, created.TenantID)
	require.Equal(t, owner.ID, created.CreatedBy)
	require.Equal(t, inv.RoleIDs, created.RoleIDs)

	_, err = h.svc.Login(context.Background(), "new@example.com", "chosen-password")
	require.NoError(t, err)

	// Single use; the accept page no longer resolves it.
	_, err = h.svc.AcceptInvitation(context.Background(), AcceptInviteCmd{Token: tok, Password: "chosen-password"})
	mustCode(t, err, "invalid_invitation_token")
	_, err = h.svc.PreviewInvitation(context.Background(), tok)
	require.ErrorIs(t, err, domain.ErrInvitationNotFound)
	list, _, err = h.svc.ListInvitations(ctx, 0, "")
	require.NoError(t, err)
	require.Empty(t, list, "accepted invitations leave the open list")
	require.ErrorIs(t, h.svc.RevokeInvitation(ctx, inv.ID), domain.ErrInvitationAccepted)
}

func TestInviteFollowsCreateUserRules(t *testing.T) {
	h := newHarness(t, inviterPerms)
	tn, owner := h.seedTenant(t, "Acme")
	admin := h.seedUser(t, tn.ID, "admin@example.com", "password-1", contracts.RoleAdmin)
	existing := h.seedUser(t, tn.ID, "taken@example.com", "password-1", contracts.RoleDeveloper)
	dev := principalOf(h.seedUser(t, tn.ID, "dev@example.com", "password-1", contracts.RoleDeveloper))

	cmd := func(roles ...int64) InviteCmd { return InviteCmd{Email: "x@example.com", RoleIDs: roles} }
	_, err := h.svc.Invite(ctxAs(principalOf(admin)), cmd(contracts.RoleOwner))
	require.ErrorIs(t, err, domain.ErrRoleEscalation)
	_, err = h.svc.Invite(ctxAs(principalOf(owner)), cmd(contracts.RolePlatformAdmin))
	require.ErrorIs(t, err, domain.ErrPlatformRoleForbidden)
	_, err = h.svc.Invite(ctxAs(principalOf(owner)), cmd(99))
	require.ErrorIs(t, err, domain.ErrUnknownRole)

	key := principalOf(owner)
	key.APIKeyID = "key-1"
	_, err = h.svc.Invite(ctxAs(key), cmd(contracts.RoleDeveloper))
	require.ErrorIs(t, err, domain.ErrHumanPrincipalRequired)

	_, err = h.svc.Invite(ctxAs(authz.Principal{UserID: "staff", RoleIDs: []int64{contracts.RolePlatformAdmin}}), cmd(contracts.RoleDeveloper))
	require.ErrorIs(t, err, authz.ErrNoTenant)

	hNone := newHarness(t, allowKeys{})
	hNone.repo, hNone.svc.repo, hNone.svc.tokens = h.repo, h.repo, h.repo
	_, err = hNone.svc.Invite(ctxAs(dev), cmd(contracts.RoleDeveloper))
	mustKind(t, err, errs.PermissionDenied)
	_, _, err = hNone.svc.ListInvitations(ctxAs(dev), 0, "")
	mustKind(t, err, errs.PermissionDenied)

	_, err = h.svc.Invite(ctxAs(principalOf(owner)), InviteCmd{Email: "TAKEN@example.com", RoleIDs: []int64{contracts.RoleDeveloper}})
	require.ErrorIs(t, err, domain.ErrEmailTaken)
	mustCode(t, err, "email_taken")
	require.Equal(t, existing.Email, h.repo.users[existing.ID].Email)

	require.Empty(t, h.repo.invites)
	require.Empty(t, h.mail.Sent)
}

func TestReinviteRevokesEarlierAndRevokeKillsTheLink(t *testing.T) {
	h := newHarness(t, inviterPerms)
	_, owner := h.seedTenant(t, "Acme")
	other, otherOwner := h.seedTenant(t, "Other")
	ctx := ctxAs(principalOf(owner))
	cmd := InviteCmd{Email: "new@example.com", RoleIDs: []int64{contracts.RoleDeveloper}}

	_, err := h.svc.Invite(ctx, cmd)
	require.NoError(t, err)
	first := tokenFrom(t, h.lastMail(t), "/accept-invite")
	second, err := h.svc.Invite(ctx, cmd)
	require.NoError(t, err)
	secondTok := tokenFrom(t, h.lastMail(t), "/accept-invite")

	_, err = h.svc.PreviewInvitation(context.Background(), first)
	require.ErrorIs(t, err, domain.ErrInvitationNotFound, "re-invite supersedes")
	list, _, err := h.svc.ListInvitations(ctx, 0, "")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, second.ID, list[0].ID)

	// Another tenant may invite the same address independently.
	_, err = h.svc.Invite(ctxAs(principalOf(otherOwner)), cmd)
	require.NoError(t, err)
	require.ErrorIs(t, h.svc.RevokeInvitation(ctx, func() string {
		for _, i := range h.repo.invites {
			if i.TenantID == other.ID {
				return i.ID
			}
		}
		return ""
	}()), domain.ErrInvitationNotFound, "cross-tenant revoke is 404")

	require.NoError(t, h.svc.RevokeInvitation(ctx, second.ID))
	require.NoError(t, h.svc.RevokeInvitation(ctx, second.ID), "idempotent")
	_, err = h.svc.AcceptInvitation(context.Background(), AcceptInviteCmd{Token: secondTok, Name: "N", Password: "chosen-password"})
	require.ErrorIs(t, err, domain.ErrInvalidInvitationToken)
}

func TestInvitationExpiresAndDiesWithItsTenant(t *testing.T) {
	h := newHarness(t, inviterPerms)
	tn, owner := h.seedTenant(t, "Acme")
	ctx := ctxAs(principalOf(owner))
	_, err := h.svc.Invite(ctx, InviteCmd{Email: "late@example.com", RoleIDs: []int64{contracts.RoleDeveloper}})
	require.NoError(t, err)
	late := tokenFrom(t, h.lastMail(t), "/accept-invite")
	_, err = h.svc.Invite(ctx, InviteCmd{Email: "suspended@example.com", RoleIDs: []int64{contracts.RoleDeveloper}})
	require.NoError(t, err)
	suspendedTok := tokenFrom(t, h.lastMail(t), "/accept-invite")

	t2 := h.repo.tenants[tn.ID]
	t2.Active = false
	h.repo.tenants[tn.ID] = t2
	_, err = h.svc.PreviewInvitation(context.Background(), suspendedTok)
	require.ErrorIs(t, err, domain.ErrInvitationNotFound)
	_, err = h.svc.AcceptInvitation(context.Background(), AcceptInviteCmd{Token: suspendedTok, Password: "chosen-password"})
	require.ErrorIs(t, err, domain.ErrInvalidInvitationToken)
	t2.Active = true
	h.repo.tenants[tn.ID] = t2

	h.clock.Advance(7 * 24 * time.Hour)
	_, err = h.svc.PreviewInvitation(context.Background(), late)
	require.ErrorIs(t, err, domain.ErrInvitationNotFound)
	_, err = h.svc.AcceptInvitation(context.Background(), AcceptInviteCmd{Token: late, Password: "chosen-password"})
	require.ErrorIs(t, err, domain.ErrInvalidInvitationToken)

	list, _, err := h.svc.ListInvitations(ctx, 0, "")
	require.NoError(t, err)
	require.Len(t, list, 2, "expired invitations stay listed")
	for _, i := range list {
		require.Equal(t, domain.InvitationExpired, i.Status(h.clock.Now()))
	}
	require.Empty(t, h.outbox.published)
}

func TestAcceptInvitationWithTakenEmailIs409AndKeepsTheInvitation(t *testing.T) {
	h := newHarness(t, inviterPerms)
	_, owner := h.seedTenant(t, "Acme")
	_, err := h.svc.Invite(ctxAs(principalOf(owner)), InviteCmd{Email: "race@example.com", RoleIDs: []int64{contracts.RoleDeveloper}})
	require.NoError(t, err)
	tok := tokenFrom(t, h.lastMail(t), "/accept-invite")

	// The address got an account elsewhere after the invite was sent.
	other, _ := h.seedTenant(t, "Other")
	h.seedUser(t, other.ID, "race@example.com", "password-1")

	_, err = h.svc.AcceptInvitation(context.Background(), AcceptInviteCmd{Token: tok, Name: "R", Password: "chosen-password"})
	require.ErrorIs(t, err, domain.ErrEmailTaken)
	mustKind(t, err, errs.AlreadyExists)
	require.Empty(t, h.outbox.published)
	_, err = h.svc.PreviewInvitation(context.Background(), tok)
	require.NoError(t, err, "the failed accept rolled back")
	for _, u := range h.repo.users {
		if u.Email == "race@example.com" {
			require.Equal(t, other.ID, u.TenantID)
		}
	}

	_, err = h.svc.AcceptInvitation(context.Background(), AcceptInviteCmd{Token: tok, Password: "short"})
	require.ErrorIs(t, err, domain.ErrPasswordTooShort)
}

// ---- email verification ----

func TestRegisterSendsVerificationAndVerifyIsSingleUse(t *testing.T) {
	h := newHarness(t, allowKeys{})
	s := register(t, h, "founder@example.com")
	require.Nil(t, s.User.EmailVerifiedAt)
	m := h.lastMail(t)
	require.Equal(t, "founder@example.com", m.To)
	tok := tokenFrom(t, m, "/verify-email")

	_, err := h.svc.Login(context.Background(), "founder@example.com", "correct-horse")
	require.NoError(t, err, "login never waits on verification")

	require.NoError(t, h.svc.VerifyEmail(context.Background(), tok))
	require.NotNil(t, h.repo.users[s.User.ID].EmailVerifiedAt)
	err = h.svc.VerifyEmail(context.Background(), tok)
	require.ErrorIs(t, err, domain.ErrInvalidVerificationToken)
	mustCode(t, err, "invalid_verification_token")
	require.ErrorIs(t, h.svc.VerifyEmail(context.Background(), "garbage"), domain.ErrInvalidVerificationToken)
}

func TestResendVerificationRotatesLinksAndIsGuarded(t *testing.T) {
	h := newHarness(t, allowKeys{})
	s := register(t, h, "founder@example.com")
	first := tokenFrom(t, h.lastMail(t), "/verify-email")
	ctx := ctxAs(principalOf(s.User))

	require.NoError(t, h.svc.ResendVerification(ctx))
	second := tokenFrom(t, h.lastMail(t), "/verify-email")
	require.NotEqual(t, first, second)
	require.ErrorIs(t, h.svc.VerifyEmail(context.Background(), first), domain.ErrInvalidVerificationToken, "older links stop working")

	key := principalOf(s.User)
	key.APIKeyID = "key-1"
	require.ErrorIs(t, h.svc.ResendVerification(ctxAs(key)), domain.ErrHumanPrincipalRequired)
	mustKind(t, h.svc.ResendVerification(context.Background()), errs.Unauthenticated)

	sent := len(h.mail.Sent)
	for range 4 {
		require.NoError(t, h.svc.ResendVerification(ctx))
	}
	require.Len(t, h.mail.Sent, sent+2, "3 resends per hour in total")

	// Expiry, then a stale address.
	latest := tokenFrom(t, h.lastMail(t), "/verify-email")
	h.clock.Advance(48 * time.Hour)
	require.ErrorIs(t, h.svc.VerifyEmail(context.Background(), latest), domain.ErrInvalidVerificationToken)

	h.throttle.counts = map[string]int{}
	require.NoError(t, h.svc.ResendVerification(ctx))
	fresh := tokenFrom(t, h.lastMail(t), "/verify-email")
	moved := h.repo.users[s.User.ID]
	moved.Email = "moved@example.com"
	h.repo.users[s.User.ID] = moved
	require.ErrorIs(t, h.svc.VerifyEmail(context.Background(), fresh), domain.ErrInvalidVerificationToken)
	moved.Email = "founder@example.com"
	h.repo.users[s.User.ID] = moved
	require.NoError(t, h.svc.VerifyEmail(context.Background(), fresh))

	require.ErrorIs(t, h.svc.ResendVerification(ctx), domain.ErrEmailAlreadyVerified)
}

func TestAccountEmailsEscapeHTMLAndKeepHeadersOnOneLine(t *testing.T) {
	inv := domain.Invitation{Email: "x@example.com", ExpiresAt: t0}
	m := invitationEmail(inv, "Evil\r\nBcc: <b>co</b>", "<script>", "https://portal.test/accept-invite?token=a&b")
	require.NotContains(t, m.Subject, "\n")
	require.NotContains(t, m.HTML, "<script>")
	require.NotContains(t, m.HTML, "<b>co</b>")
	require.Contains(t, m.HTML, "token=a&amp;b")
	require.Equal(t, "1 hour", humanDuration(time.Hour))
	require.Equal(t, "2 days", humanDuration(48*time.Hour))
	require.Equal(t, "30 minutes", humanDuration(30*time.Minute))
}
