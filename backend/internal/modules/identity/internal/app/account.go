package app

import (
	"context"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/mail"
	"levelup/internal/shared/errs"
)

// AccountTokenRepository persists the emailed single-use tokens:
// password resets, email verifications and invitations (internal/repo).
// Every lookup is by the token's hash; the plaintext is never stored.
type AccountTokenRepository interface {
	// ReplacePasswordReset deletes the user's earlier resets (used or not)
	// and inserts r: only the newest link works.
	ReplacePasswordReset(ctx context.Context, tx *gorm.DB, r domain.PasswordReset) error
	PasswordResetByHash(ctx context.Context, hash string) (domain.PasswordReset, error)
	PasswordResetByHashForUpdate(ctx context.Context, tx *gorm.DB, hash string) (domain.PasswordReset, error)
	MarkPasswordResetUsed(ctx context.Context, tx *gorm.DB, id string, at time.Time) error

	// ReplaceEmailVerification deletes the user's earlier verifications and
	// inserts v.
	ReplaceEmailVerification(ctx context.Context, tx *gorm.DB, v domain.EmailVerification) error
	EmailVerificationByHashForUpdate(ctx context.Context, tx *gorm.DB, hash string) (domain.EmailVerification, error)
	MarkEmailVerificationUsed(ctx context.Context, tx *gorm.DB, id string, at time.Time) error

	CreateInvitation(ctx context.Context, tx *gorm.DB, inv domain.Invitation) error
	// RevokeOpenInvitations revokes every not-yet-accepted, not-revoked
	// invitation of email in the tenant (a re-invite supersedes them).
	RevokeOpenInvitations(ctx context.Context, tx *gorm.DB, tenantID, email string, at time.Time) error
	// OpenInvitations lists not accepted, not revoked invitations (expired
	// included, so admins see them), keyset-paged.
	OpenInvitations(ctx context.Context, tenantID string, page Page) ([]domain.Invitation, error)
	InvitationInTenantForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Invitation, error)
	InvitationByHash(ctx context.Context, hash string) (domain.Invitation, error)
	InvitationByHashForUpdate(ctx context.Context, tx *gorm.DB, hash string) (domain.Invitation, error)
	// SaveInvitation writes the accepted/revoked fields.
	SaveInvitation(ctx context.Context, tx *gorm.DB, inv domain.Invitation) error
}

// Throttle is a fixed-window counter on redis-core. Callers fail open on
// error (R43: rate limiting fails open).
type Throttle interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

const (
	defaultPasswordResetTTL     = time.Hour
	defaultInvitationTTL        = 7 * 24 * time.Hour
	defaultEmailVerificationTTL = 48 * time.Hour
	defaultResetRequestsPerHour = 3
	defaultPortalURL            = "http://localhost:5173"
	backgroundMailTimeout       = 30 * time.Second
)

func (st Settings) withDefaults() Settings {
	if st.PasswordResetTTL <= 0 {
		st.PasswordResetTTL = defaultPasswordResetTTL
	}
	if st.InvitationTTL <= 0 {
		st.InvitationTTL = defaultInvitationTTL
	}
	if st.EmailVerificationTTL <= 0 {
		st.EmailVerificationTTL = defaultEmailVerificationTTL
	}
	if st.ResetRequestsPerHour <= 0 {
		st.ResetRequestsPerHour = defaultResetRequestsPerHour
	}
	if st.PortalURL == "" {
		st.PortalURL = defaultPortalURL
	}
	st.PortalURL = strings.TrimRight(st.PortalURL, "/")
	return st
}

// WithAccountTokens enables password resets, invitations and email
// verification. A nil throttle never limits; a nil mailer is required to
// be replaced by the log driver by the caller.
func (s *Service) WithAccountTokens(repo AccountTokenRepository, mailer mail.Mailer, throttle Throttle) *Service {
	s.tokens, s.mailer, s.throttle = repo, mailer, throttle
	return s
}

func (s *Service) tokensEnabled() error {
	if s.tokens == nil || s.mailer == nil {
		return errs.New(errs.Unavailable, "account emails are not enabled")
	}
	return nil
}

// allow consults the throttle and fails open.
func (s *Service) allow(ctx context.Context, key string) bool {
	if s.throttle == nil {
		return true
	}
	ok, err := s.throttle.Allow(ctx, key, s.settings.ResetRequestsPerHour, time.Hour)
	if err != nil {
		s.log.Warn("account email throttle unavailable, failing open", zap.Error(err))
		return true
	}
	return ok
}

func (s *Service) link(path, token string) string {
	return s.settings.PortalURL + path + "?token=" + url.QueryEscape(token)
}

func (s *Service) send(ctx context.Context, m mail.Message) error {
	if err := s.mailer.Send(ctx, m); err != nil {
		s.log.Warn("account email not sent", zap.String("subject", m.Subject), zap.Error(err))
		return err
	}
	return nil
}

// ---- password reset ----

// ForgotPassword always succeeds from the caller's point of view (no
// account enumeration). Past the per-address throttle, the lookup, the
// token write and the mail all run in the background, so neither the
// status nor the response time depends on whether the account exists.
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	if err := s.tokensEnabled(); err != nil {
		return err
	}
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return nil
	}
	if !s.allow(ctx, "pwreset:"+domain.HashAccountToken(normalized)) {
		s.log.Info("password reset throttled")
		return nil
	}
	bg := context.WithoutCancel(ctx)
	s.background(func() {
		ctx, cancel := context.WithTimeout(bg, backgroundMailTimeout)
		defer cancel()
		if err := s.issuePasswordReset(ctx, normalized); err != nil {
			s.log.Warn("password reset not issued", zap.Error(err))
		}
	})
	return nil
}

func (s *Service) issuePasswordReset(ctx context.Context, email string) error {
	u, err := s.repo.UserByEmail(ctx, email)
	if err != nil {
		if errs.KindOf(err) == errs.NotFound {
			return nil
		}
		return err
	}
	if _, err := s.activeSessionSubject(ctx, u); err != nil {
		return nil // inactive user or tenant: same silence as unknown
	}
	plain, hash, err := domain.NewAccountToken()
	if err != nil {
		return err
	}
	r := domain.NewPasswordReset(u.ID, u.Email, hash, s.settings.PasswordResetTTL, s.clock.Now())
	if err := s.tx(ctx, func(tx *gorm.DB) error { return s.tokens.ReplacePasswordReset(ctx, tx, r) }); err != nil {
		return err
	}
	return s.send(ctx, passwordResetEmail(u, s.link("/reset-password", plain), s.settings.PasswordResetTTL))
}

// ResetPassword consumes a reset token: sets the password (same policy as
// everywhere), marks the token used, verifies the email (the link proved
// the mailbox) and revokes every refresh token after commit. Unknown,
// expired, used and superseded tokens are all ErrInvalidResetToken.
func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	if err := s.tokensEnabled(); err != nil {
		return err
	}
	if !domain.WellFormedAccountToken(token) {
		return domain.ErrInvalidResetToken
	}
	hash := domain.HashAccountToken(token)
	// Cheap read first, so an invalid token never costs a bcrypt hash.
	r, err := s.tokens.PasswordResetByHash(ctx, hash)
	if err != nil {
		return asInvalid(err, domain.ErrInvalidResetToken)
	}
	if !r.Usable(s.clock.Now()) {
		return domain.ErrInvalidResetToken
	}
	newHash, err := s.pw.Hash(password)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	var userID string
	err = s.tx(ctx, func(tx *gorm.DB) error {
		r, err := s.tokens.PasswordResetByHashForUpdate(ctx, tx, hash)
		if err != nil {
			return asInvalid(err, domain.ErrInvalidResetToken)
		}
		if !r.Usable(now) {
			return domain.ErrInvalidResetToken
		}
		u, err := s.repo.UserByIDForUpdate(ctx, tx, r.UserID)
		if err != nil {
			return asInvalid(err, domain.ErrInvalidResetToken)
		}
		if !u.Active || u.Email != r.Email {
			return domain.ErrInvalidResetToken
		}
		u.SetPasswordHash(newHash, now)
		if u.EmailVerifiedAt == nil {
			u.EmailVerifiedAt = &now
		}
		if err := s.repo.SaveUser(ctx, tx, u); err != nil {
			return err
		}
		if err := s.tokens.MarkPasswordResetUsed(ctx, tx, r.ID, now); err != nil {
			return err
		}
		userID = u.ID
		return s.outbox.Publish(ctx, tx, contracts.TopicUserUpdated, contracts.UserUpdatedV1{
			UserID: u.ID, TenantID: u.TenantID, Email: u.Email, Name: u.Name, Active: u.Active, At: now,
		})
	})
	if err != nil {
		return err
	}
	s.revokeSessions(ctx, userID)
	return nil
}

// asInvalid maps "no such row" to the token's single failure error.
func asInvalid(err, invalid error) error {
	if errs.KindOf(err) == errs.NotFound {
		return invalid
	}
	return err
}

// ---- email verification ----

// stageVerification creates the token row inside the caller's transaction
// and returns the mail to send after commit.
func (s *Service) stageVerification(ctx context.Context, tx *gorm.DB, u domain.User, now time.Time) (mail.Message, error) {
	plain, hash, err := domain.NewAccountToken()
	if err != nil {
		return mail.Message{}, err
	}
	v := domain.NewEmailVerification(u.ID, u.Email, hash, s.settings.EmailVerificationTTL, now)
	if err := s.tokens.ReplaceEmailVerification(ctx, tx, v); err != nil {
		return mail.Message{}, err
	}
	return verificationEmail(u, s.link("/verify-email", plain), s.settings.EmailVerificationTTL), nil
}

// VerifyEmail consumes a verification token. A token for an address the
// user no longer has is invalid. Verifying an already verified address
// with a valid token is a no-op success.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	if err := s.tokensEnabled(); err != nil {
		return err
	}
	if !domain.WellFormedAccountToken(token) {
		return domain.ErrInvalidVerificationToken
	}
	hash := domain.HashAccountToken(token)
	now := s.clock.Now()
	return s.tx(ctx, func(tx *gorm.DB) error {
		v, err := s.tokens.EmailVerificationByHashForUpdate(ctx, tx, hash)
		if err != nil {
			return asInvalid(err, domain.ErrInvalidVerificationToken)
		}
		if !v.Usable(now) {
			return domain.ErrInvalidVerificationToken
		}
		u, err := s.repo.UserByIDForUpdate(ctx, tx, v.UserID)
		if err != nil {
			return asInvalid(err, domain.ErrInvalidVerificationToken)
		}
		if u.Email != v.Email {
			return domain.ErrInvalidVerificationToken
		}
		if err := s.tokens.MarkEmailVerificationUsed(ctx, tx, v.ID, now); err != nil {
			return err
		}
		if u.EmailVerifiedAt != nil {
			return nil
		}
		u.EmailVerifiedAt, u.UpdatedAt = &now, now
		return s.repo.SaveUser(ctx, tx, u)
	})
}

// ResendVerification mails a fresh link to the caller (earlier links stop
// working). Throttled per user like forgot-password; a throttled request is
// still accepted, silently.
func (s *Service) ResendVerification(ctx context.Context) error {
	if err := s.tokensEnabled(); err != nil {
		return err
	}
	p, ok := authz.From(ctx)
	if !ok {
		return errs.New(errs.Unauthenticated, "authentication required")
	}
	if err := requireHuman(p); err != nil {
		return err
	}
	u, err := s.repo.UserByID(ctx, p.UserID)
	if err != nil {
		return err
	}
	if u.EmailVerifiedAt != nil {
		return domain.ErrEmailAlreadyVerified
	}
	if !s.allow(ctx, "verify:"+u.ID) {
		s.log.Info("verification resend throttled", zap.String("user_id", u.ID))
		return nil
	}
	var msg mail.Message
	err = s.tx(ctx, func(tx *gorm.DB) error {
		var err error
		msg, err = s.stageVerification(ctx, tx, u, s.clock.Now())
		return err
	})
	if err != nil {
		return err
	}
	return s.send(ctx, msg)
}

// ---- invitations ----

type InviteCmd struct {
	Email   string
	Name    string
	RoleIDs []int64
}

func (s *Service) inviteAdmin(ctx context.Context) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return p, err
	}
	if err := requireHuman(p); err != nil {
		return p, err
	}
	if err := s.tokensEnabled(); err != nil {
		return p, err
	}
	return p, s.authz.Authorize(ctx, p, contracts.PermUsersCreate, nil)
}

// Invite creates an invitation into the CALLER's tenant with the same role
// rules as CreateUser and mails the accept link after commit. Inviting an
// address that already has an account is email_taken; re-inviting an
// address with an open invitation revokes the earlier one. A mail failure
// is logged, not returned: the invitation exists and a re-invite resends.
func (s *Service) Invite(ctx context.Context, cmd InviteCmd) (domain.Invitation, error) {
	p, err := s.inviteAdmin(ctx)
	if err != nil {
		return domain.Invitation{}, err
	}
	roles := domain.NormalizeRoleIDs(cmd.RoleIDs)
	if err := domain.CheckGrantable(p.RoleIDs, roles); err != nil {
		return domain.Invitation{}, err
	}
	plain, hash, err := domain.NewAccountToken()
	if err != nil {
		return domain.Invitation{}, err
	}
	now := s.clock.Now()
	inv, err := domain.NewInvitation(p.TenantID, cmd.Email, cmd.Name, p.UserID, hash, roles, s.settings.InvitationTTL, now)
	if err != nil {
		return domain.Invitation{}, err
	}
	if _, err := s.repo.UserByEmail(ctx, inv.Email); err == nil {
		return domain.Invitation{}, domain.ErrEmailTaken
	} else if errs.KindOf(err) != errs.NotFound {
		return domain.Invitation{}, err
	}
	tenant, err := s.repo.TenantByID(ctx, p.TenantID)
	if err != nil {
		return domain.Invitation{}, err
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.tokens.RevokeOpenInvitations(ctx, tx, inv.TenantID, inv.Email, now); err != nil {
			return err
		}
		return s.tokens.CreateInvitation(ctx, tx, inv)
	})
	if err != nil {
		return domain.Invitation{}, err
	}
	inviter := ""
	if u, err := s.repo.UserByID(ctx, p.UserID); err == nil {
		inviter = u.Name
	}
	_ = s.send(ctx, invitationEmail(inv, tenant.Name, inviter, s.link("/accept-invite", plain)))
	return inv, nil
}

// ListInvitations returns the tenant's open invitations, newest first.
func (s *Service) ListInvitations(ctx context.Context, limit int, cursor string) ([]domain.Invitation, string, error) {
	p, err := s.inviteAdmin(ctx)
	if err != nil {
		return nil, "", err
	}
	page, err := pageFrom(limit, cursor)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.tokens.OpenInvitations(ctx, p.TenantID, page)
	if err != nil {
		return nil, "", err
	}
	out, next := trimPage(rows, page.Limit, func(i domain.Invitation) (time.Time, string) { return i.CreatedAt, i.ID })
	return out, next, nil
}

// RevokeInvitation is idempotent; another tenant's invitation is 404 and
// an accepted one is 409.
func (s *Service) RevokeInvitation(ctx context.Context, invitationID string) error {
	p, err := s.inviteAdmin(ctx)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	return s.tx(ctx, func(tx *gorm.DB) error {
		inv, err := s.tokens.InvitationInTenantForUpdate(ctx, tx, p.TenantID, invitationID)
		if err != nil {
			return err
		}
		changed, err := inv.Revoke(now)
		if err != nil || !changed {
			return err
		}
		return s.tokens.SaveInvitation(ctx, tx, inv)
	})
}

// InvitationPreview is what the accept page shows before sign-up.
type InvitationPreview struct {
	Email       string
	Name        string
	TenantName  string
	InviterName string
	RoleIDs     []int64
	ExpiresAt   time.Time
}

// PreviewInvitation is anonymous: the token is the credential. Anything but
// a pending invitation of an active tenant is ErrInvitationNotFound.
func (s *Service) PreviewInvitation(ctx context.Context, token string) (InvitationPreview, error) {
	if err := s.tokensEnabled(); err != nil {
		return InvitationPreview{}, err
	}
	inv, tenant, err := s.openInvitation(ctx, token, domain.ErrInvitationNotFound)
	if err != nil {
		return InvitationPreview{}, err
	}
	inviter := ""
	if u, err := s.repo.UserByID(ctx, inv.InvitedBy); err == nil {
		inviter = u.Name
	}
	return InvitationPreview{
		Email: inv.Email, Name: inv.Name, TenantName: tenant.Name, InviterName: inviter,
		RoleIDs: roleIDsOrEmpty(inv.RoleIDs), ExpiresAt: inv.ExpiresAt,
	}, nil
}

// openInvitation resolves a token to a pending invitation of an active
// tenant, or returns invalid.
func (s *Service) openInvitation(ctx context.Context, token string, invalid error) (domain.Invitation, domain.Tenant, error) {
	if !domain.WellFormedAccountToken(token) {
		return domain.Invitation{}, domain.Tenant{}, invalid
	}
	inv, err := s.tokens.InvitationByHash(ctx, domain.HashAccountToken(token))
	if err != nil {
		return domain.Invitation{}, domain.Tenant{}, asInvalid(err, invalid)
	}
	if inv.Status(s.clock.Now()) != domain.InvitationPending {
		return domain.Invitation{}, domain.Tenant{}, invalid
	}
	tenant, err := s.repo.TenantByID(ctx, inv.TenantID)
	if err != nil {
		return domain.Invitation{}, domain.Tenant{}, asInvalid(err, invalid)
	}
	if !tenant.IsActive() {
		return domain.Invitation{}, domain.Tenant{}, invalid
	}
	return inv, tenant, nil
}

type AcceptInviteCmd struct {
	Token    string
	Name     string // optional; defaults to the invitation's name, then the email's local part
	Password string
}

// AcceptInvitation creates the user in the invitation's tenant with its
// roles (email already verified: the link proved the mailbox), marks the
// invitation accepted, publishes user.created.v1 and returns a session.
func (s *Service) AcceptInvitation(ctx context.Context, cmd AcceptInviteCmd) (Session, error) {
	if err := s.tokensEnabled(); err != nil {
		return Session{}, err
	}
	if err := s.tokensWired(); err != nil {
		return Session{}, err
	}
	inv, tenant, err := s.openInvitation(ctx, cmd.Token, domain.ErrInvalidInvitationToken)
	if err != nil {
		return Session{}, err
	}
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		name = inv.Name
	}
	if name == "" {
		name = domain.NameFromEmail(inv.Email)
	}
	now := s.clock.Now()
	u, err := domain.NewUser(inv.TenantID, name, inv.Email, "", inv.RoleIDs, now)
	if err != nil {
		return Session{}, err
	}
	u.EmailVerifiedAt = &now
	if u.PasswordHash, err = s.pw.Hash(cmd.Password); err != nil {
		return Session{}, err
	}
	hash := domain.HashAccountToken(cmd.Token)
	err = s.tx(ctx, func(tx *gorm.DB) error {
		locked, err := s.tokens.InvitationByHashForUpdate(ctx, tx, hash)
		if err != nil {
			return asInvalid(err, domain.ErrInvalidInvitationToken)
		}
		if err := locked.Accept(u.ID, now); err != nil {
			return err
		}
		if err := s.repo.CreateUser(ctx, tx, u); err != nil {
			return err
		}
		if err := s.tokens.SaveInvitation(ctx, tx, locked); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicUserCreated, contracts.UserCreatedV1{
			UserID: u.ID, TenantID: u.TenantID, Email: u.Email, Name: u.Name,
			RoleIDs: roleIDsOrEmpty(u.RoleIDs), CreatedBy: locked.InvitedBy, At: now,
		})
	})
	if err != nil {
		return Session{}, err
	}
	return s.issueSession(ctx, u, &tenant)
}
