package app

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authn"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// Session is what register, login and refresh hand back.
type Session struct {
	User         domain.User
	Tenant       *domain.Tenant // nil for platform users
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64 // seconds (fixes doc 02 §10 bug 14)
}

type RegisterCmd struct {
	TenantName string
	Name       string // optional; defaults to the email's local part
	Email      string
	Password   string
	Timezone   string // optional; defaults to UTC
}

const slugAttempts = 3

// Register atomically creates tenant + owner user + owner role and records
// tenant.created.v1 and user.registered.v1 in the same transaction (fixes doc
// 02 §10 bug 17). The slug suffix is regenerated on a collision.
func (s *Service) Register(ctx context.Context, cmd RegisterCmd) (Session, error) {
	if !s.settings.AllowSelfSignup {
		return Session{}, domain.ErrSignupDisabled
	}
	if err := s.tokensWired(); err != nil {
		return Session{}, err
	}
	now := s.clock.Now()

	t, err := domain.NewTenant(cmd.TenantName, cmd.Timezone, randomSuffix(), now)
	if err != nil {
		return Session{}, err
	}
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		name = domain.NameFromEmail(cmd.Email)
	}
	u, err := domain.NewUser(t.ID, name, cmd.Email, "", []int64{contracts.RoleOwner}, now)
	if err != nil {
		return Session{}, err
	}
	// Hash before the transaction: bcrypt must not hold a connection.
	if u.PasswordHash, err = s.pw.Hash(cmd.Password); err != nil {
		return Session{}, err
	}
	t.OwnerUserID = u.ID

	for attempt := range slugAttempts {
		if attempt > 0 {
			t.Slug = domain.SlugFor(t.Name, randomSuffix())
		}
		err = s.tx(ctx, func(tx *gorm.DB) error {
			// Tenant first: its owner FK is DEFERRABLE INITIALLY DEFERRED.
			if err := s.repo.CreateTenant(ctx, tx, t); err != nil {
				return err
			}
			if err := s.repo.CreateUser(ctx, tx, u); err != nil {
				return err
			}
			if err := s.outbox.Publish(ctx, tx, contracts.TopicTenantCreated, contracts.TenantCreatedV1{
				TenantID: t.ID, Name: t.Name, Slug: t.Slug, OwnerUserID: u.ID, Timezone: t.Timezone, At: now,
			}); err != nil {
				return err
			}
			return s.outbox.Publish(ctx, tx, contracts.TopicUserRegistered, contracts.UserRegisteredV1{
				UserID: u.ID, TenantID: t.ID, Email: u.Email, Name: u.Name, At: now,
			})
		})
		if !errors.Is(err, domain.ErrSlugTaken) {
			break
		}
	}
	if err != nil {
		return Session{}, err
	}
	return s.issueSession(ctx, u, &t)
}

// Login checks credentials. Unknown email, wrong password, inactive user and
// inactive or deleted tenant all yield domain.ErrBadCredentials, and the
// unknown-email path still spends one bcrypt compare.
func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	if err := s.tokensWired(); err != nil {
		return Session{}, err
	}
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		s.pw.Burn(password)
		return Session{}, domain.ErrBadCredentials
	}
	u, err := s.repo.UserByEmail(ctx, normalized)
	if err != nil {
		if errs.KindOf(err) == errs.NotFound {
			s.pw.Burn(password)
			return Session{}, domain.ErrBadCredentials
		}
		return Session{}, err
	}
	if u.PasswordHash == "" || !s.pw.Matches(u.PasswordHash, password) {
		return Session{}, domain.ErrBadCredentials
	}
	tenant, err := s.activeSessionSubject(ctx, u)
	if err != nil {
		return Session{}, err
	}
	s.maybeRehash(ctx, u, password)
	return s.issueSession(ctx, u, tenant)
}

// Refresh rotates the refresh token (a replay revokes the whole family in
// the platform store) and re-reads user, roles and tenant so a suspension
// or role change takes effect at the next refresh.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (Session, error) {
	if err := s.tokensWired(); err != nil {
		return Session{}, err
	}
	next, err := s.refresh.Rotate(ctx, refreshToken)
	if err != nil {
		return Session{}, err
	}
	userID, _, err := authn.DecodeUserID(next)
	if err != nil {
		return Session{}, err
	}
	u, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		if errs.KindOf(err) == errs.NotFound {
			s.revokeSessions(ctx, userID)
			return Session{}, domain.ErrBadCredentials
		}
		return Session{}, err
	}
	tenant, err := s.activeSessionSubject(ctx, u)
	if err != nil {
		s.revokeSessions(ctx, userID)
		return Session{}, err
	}
	access, _, err := s.issuer.IssueAccess(u.ID, u.TenantID, u.RoleIDs)
	if err != nil {
		return Session{}, err
	}
	return Session{
		User: u, Tenant: tenant, AccessToken: access, RefreshToken: next,
		ExpiresIn: int64(s.issuer.AccessTTL().Seconds()),
	}, nil
}

// Logout deny-lists the presented access token for its remaining TTL and
// revokes every refresh token of the user (logout everywhere).
func (s *Service) Logout(ctx context.Context) error {
	if err := s.tokensWired(); err != nil {
		return err
	}
	p, ok := authz.From(ctx)
	if !ok {
		return errs.New(errs.Unauthenticated, "authentication required")
	}
	var denyErr error
	if jti, ok := authn.JTIFrom(ctx); ok && jti != "" {
		denyErr = s.refresh.Deny(ctx, jti, s.issuer.AccessTTL())
	}
	if err := s.refresh.RevokeAll(ctx, p.UserID); err != nil {
		return err
	}
	return denyErr
}

// Me returns the caller with roles and (for tenant users) the tenant.
func (s *Service) Me(ctx context.Context) (domain.User, *domain.Tenant, error) {
	p, ok := authz.From(ctx)
	if !ok {
		return domain.User{}, nil, errs.New(errs.Unauthenticated, "authentication required")
	}
	if p.TenantID == "" {
		u, err := s.repo.UserByID(ctx, p.UserID)
		if err != nil {
			return domain.User{}, nil, err
		}
		if !u.IsPlatform() {
			return domain.User{}, nil, domain.ErrUserNotFound
		}
		return u, nil, nil
	}
	u, err := s.repo.UserInTenant(ctx, p.TenantID, p.UserID)
	if err != nil {
		return domain.User{}, nil, err
	}
	t, err := s.repo.TenantByID(ctx, p.TenantID)
	if err != nil {
		return domain.User{}, nil, err
	}
	return u, &t, nil
}

// activeSessionSubject enforces "active user in an active tenant" and
// returns the tenant (nil for platform users).
func (s *Service) activeSessionSubject(ctx context.Context, u domain.User) (*domain.Tenant, error) {
	if !u.Active {
		return nil, domain.ErrBadCredentials
	}
	if u.IsPlatform() {
		return nil, nil
	}
	t, err := s.repo.TenantByID(ctx, u.TenantID)
	if err != nil {
		if errs.KindOf(err) == errs.NotFound {
			return nil, domain.ErrBadCredentials
		}
		return nil, err
	}
	if !t.IsActive() {
		return nil, domain.ErrBadCredentials
	}
	return &t, nil
}

func (s *Service) issueSession(ctx context.Context, u domain.User, t *domain.Tenant) (Session, error) {
	access, _, err := s.issuer.IssueAccess(u.ID, u.TenantID, u.RoleIDs)
	if err != nil {
		return Session{}, err
	}
	refresh, err := s.refresh.Issue(ctx, u.ID)
	if err != nil {
		return Session{}, err
	}
	return Session{
		User: u, Tenant: t, AccessToken: access, RefreshToken: refresh,
		ExpiresIn: int64(s.issuer.AccessTTL().Seconds()),
	}, nil
}

// maybeRehash upgrades a weaker hash after a successful login. Best effort:
// a failure is logged and the login proceeds. Passwords over 72 bytes keep
// their legacy hash (new passwords cannot exceed the limit).
func (s *Service) maybeRehash(ctx context.Context, u domain.User, password string) {
	if !s.pw.NeedsRehash(u.PasswordHash) || len(password) > domain.MaxPasswordBytes {
		return
	}
	hash, err := s.pw.Hash(password)
	if err != nil {
		return
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.UpdatePasswordHash(ctx, tx, u.ID, hash)
	}); err != nil {
		s.log.Warn("password rehash failed", zap.String("user_id", u.ID), zap.Error(err))
	}
}
