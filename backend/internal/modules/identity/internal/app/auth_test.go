package app

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authn"
	"levelup/internal/shared/errs"
)

// Real PHP fixtures, generated with
//
//	php -r 'echo password_hash("secret-password", PASSWORD_BCRYPT, ["cost"=>12]);'
//	php -r 'echo password_hash(str_repeat("a",72)."-tail-beyond-seventy-two", PASSWORD_BCRYPT, ["cost"=>12]);'
const (
	phpHashSecretPassword = `$2y$12$4.ZLrSYmDvLgdg4C8UevvewMdaK5X90p4AvfNkMWF3zUgvpZyteRi`
	phpHashLongPassword   = `$2y$12$VHmi93Cromc9ejekCbNaQ.42BTda6OZ.7vo68zNqqVri1mFpZXW/y`
)

func register(t *testing.T, h *harness, email string) Session {
	t.Helper()
	s, err := h.svc.Register(context.Background(), RegisterCmd{
		TenantName: "Acme Corp", Email: email, Password: "correct-horse",
	})
	require.NoError(t, err)
	return s
}

func TestRegisterCreatesTenantOwnerAndPublishesBothEventsAtomically(t *testing.T) {
	h := newHarness(t, allowKeys{})
	s := register(t, h, "Founder@Example.com")

	require.NotNil(t, s.Tenant)
	require.Equal(t, "founder@example.com", s.User.Email)
	require.Equal(t, "Founder", s.User.Name, "name defaults to the email local part")
	require.Equal(t, s.Tenant.ID, s.User.TenantID)
	require.Equal(t, s.User.ID, s.Tenant.OwnerUserID)
	require.Equal(t, []int64{contracts.RoleOwner}, s.User.RoleIDs)
	require.True(t, strings.HasPrefix(s.Tenant.Slug, "acme-corp-"))
	require.Len(t, s.Tenant.Slug, len("acme-corp-")+6)
	require.Equal(t, "UTC", s.Tenant.Timezone)
	require.EqualValues(t, 900, s.ExpiresIn, "expires_in is seconds")

	require.Equal(t, []string{contracts.TopicTenantCreated, contracts.TopicUserRegistered}, h.outbox.topics())
	created := h.outbox.published[0].payload.(contracts.TenantCreatedV1)
	require.Equal(t, s.Tenant.ID, created.TenantID)
	require.Equal(t, s.User.ID, created.OwnerUserID)
	registered := h.outbox.published[1].payload.(contracts.UserRegisteredV1)
	require.Equal(t, s.Tenant.ID, registered.TenantID)

	// The access token carries tid and the owner role.
	p, _, err := h.issuer.Verify(s.AccessToken)
	require.NoError(t, err)
	require.Equal(t, s.Tenant.ID, p.TenantID)
	require.Equal(t, []int64{contracts.RoleOwner}, p.RoleIDs)
	require.NotEmpty(t, s.RefreshToken)
}

func TestRegisterRollsBackTenantAndEventsWhenUserInsertFails(t *testing.T) {
	h := newHarness(t, allowKeys{})
	register(t, h, "taken@example.com")
	h.outbox.published = nil

	_, err := h.svc.Register(context.Background(), RegisterCmd{
		TenantName: "Second", Email: "TAKEN@example.com", Password: "correct-horse",
	})
	require.ErrorIs(t, err, domain.ErrEmailTaken)
	require.Equal(t, "email_taken", errs.CodeOf(err))
	require.Len(t, h.repo.tenants, 1, "the second tenant must roll back with the user")
	require.Empty(t, h.outbox.published, "no event survives a rolled-back registration")
}

func TestRegisterRetriesSlugCollision(t *testing.T) {
	h := newHarness(t, allowKeys{})
	h.repo.slugCollisions = 2
	s := register(t, h, "a@example.com")
	require.Len(t, h.repo.tenants, 1)
	require.Equal(t, []string{contracts.TopicTenantCreated, contracts.TopicUserRegistered}, h.outbox.topics())
	require.NotEmpty(t, s.Tenant.Slug)

	h2 := newHarness(t, allowKeys{})
	h2.repo.slugCollisions = slugAttempts
	_, err := h2.svc.Register(context.Background(), RegisterCmd{TenantName: "X", Email: "b@example.com", Password: "correct-horse"})
	require.ErrorIs(t, err, domain.ErrSlugTaken)
	require.Empty(t, h2.outbox.published)
}

func TestRegisterValidation(t *testing.T) {
	h := newHarness(t, allowKeys{})
	for name, cmd := range map[string]RegisterCmd{
		"short password": {TenantName: "A", Email: "a@example.com", Password: "short"},
		"long password":  {TenantName: "A", Email: "a@example.com", Password: strings.Repeat("x", 73)},
		"bad timezone":   {TenantName: "A", Email: "a@example.com", Password: "correct-horse", Timezone: "Mars/Base"},
		"bad email":      {TenantName: "A", Email: "nope", Password: "correct-horse"},
		"empty tenant":   {TenantName: " ", Email: "a@example.com", Password: "correct-horse"},
	} {
		_, err := h.svc.Register(context.Background(), cmd)
		mustKind(t, err, errs.Invalid)
		require.Empty(t, h.repo.tenants, name)
	}

	h.svc.settings.AllowSelfSignup = false
	_, err := h.svc.Register(context.Background(), RegisterCmd{TenantName: "A", Email: "a@example.com", Password: "correct-horse"})
	require.ErrorIs(t, err, domain.ErrSignupDisabled)
}

// R53: a Laravel $2y$12$ hash verifies unchanged.
func TestLoginWithLegacyPHPHash(t *testing.T) {
	t.Parallel()
	h := newHarness(t, allowKeys{})
	tn, _ := h.seedTenant(t, "Legacy")
	u := h.seedUser(t, tn.ID, "legacy@example.com", "placeholder-pw", contracts.RoleAdmin)
	u.PasswordHash = phpHashSecretPassword
	h.repo.users[u.ID] = u
	h.svc.pw = newPasswords(12) // equal cost: no rehash

	s, err := h.svc.Login(context.Background(), "LEGACY@example.com", "secret-password")
	require.NoError(t, err)
	require.Equal(t, u.ID, s.User.ID)
	require.Equal(t, tn.ID, s.Tenant.ID)
	require.Empty(t, h.repo.rehashed, "cost-12 hash must not be rewritten")

	_, err = h.svc.Login(context.Background(), "legacy@example.com", "wrong-password")
	require.ErrorIs(t, err, domain.ErrBadCredentials)
}

// PHP hashed only the first 72 bytes; the Go side truncates before compare
// so those users can still log in with their full password.
func TestLoginWithLegacyPasswordOver72Bytes(t *testing.T) {
	t.Parallel()
	h := newHarness(t, allowKeys{})
	tn, _ := h.seedTenant(t, "Legacy")
	u := h.seedUser(t, tn.ID, "long@example.com", "placeholder-pw")
	u.PasswordHash = phpHashLongPassword
	h.repo.users[u.ID] = u
	h.svc.pw = newPasswords(12)

	full := strings.Repeat("a", 72) + "-tail-beyond-seventy-two"
	_, err := h.svc.Login(context.Background(), "long@example.com", full)
	require.NoError(t, err)

	// Same first 72 bytes: equivalent, exactly as under PHP.
	_, err = h.svc.Login(context.Background(), "long@example.com", strings.Repeat("a", 72)+"anything")
	require.NoError(t, err)

	_, err = h.svc.Login(context.Background(), "long@example.com", strings.Repeat("a", 71))
	require.ErrorIs(t, err, domain.ErrBadCredentials)
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	h := newHarness(t, allowKeys{})
	tn, _ := h.seedTenant(t, "Acme")
	inactive := h.seedUser(t, tn.ID, "inactive@example.com", "good-password")
	inactive.Active = false
	h.repo.users[inactive.ID] = inactive

	suspended, _ := h.seedTenant(t, "Suspended")
	inSuspended := h.seedUser(t, suspended.ID, "member@suspended.com", "good-password")
	suspended.Active = false
	h.repo.tenants[suspended.ID] = suspended

	deleted, _ := h.seedTenant(t, "Deleted")
	inDeleted := h.seedUser(t, deleted.ID, "member@deleted.com", "good-password")
	deleted.SoftDelete(t0)
	h.repo.tenants[deleted.ID] = deleted
	_ = inSuspended
	_ = inDeleted

	for name, creds := range map[string][2]string{
		"unknown email":   {"ghost@example.com", "good-password"},
		"malformed email": {"not-an-email", "good-password"},
		"wrong password":  {"inactive@example.com", "bad-password"},
		"inactive user":   {"inactive@example.com", "good-password"},
		"inactive tenant": {"member@suspended.com", "good-password"},
		"deleted tenant":  {"member@deleted.com", "good-password"},
	} {
		_, err := h.svc.Login(context.Background(), creds[0], creds[1])
		require.ErrorIs(t, err, domain.ErrBadCredentials, name)
		require.Equal(t, "invalid_credentials", errs.CodeOf(err), name)
	}
	require.Empty(t, h.refresh.live, "no session for any failed login")
}

func TestLoginRehashesWeakerHash(t *testing.T) {
	h := newHarness(t, allowKeys{})
	tn, _ := h.seedTenant(t, "Acme")
	u := h.seedUser(t, tn.ID, "weak@example.com", "placeholder-pw")
	weak, err := bcrypt.GenerateFromPassword([]byte("good-password"), 4)
	require.NoError(t, err)
	u.PasswordHash = string(weak)
	h.repo.users[u.ID] = u
	h.svc.pw = newPasswords(5)

	_, err = h.svc.Login(context.Background(), "weak@example.com", "good-password")
	require.NoError(t, err)
	require.Contains(t, h.repo.rehashed, u.ID)
	cost, err := bcrypt.Cost([]byte(h.repo.rehashed[u.ID]))
	require.NoError(t, err)
	require.Equal(t, 5, cost)
}

func TestPlatformUserLoginHasNoTenant(t *testing.T) {
	h := newHarness(t, allowKeys{})
	admin := h.seedUser(t, "", "staff@levelup.test", "good-password", contracts.RolePlatformAdmin)
	s, err := h.svc.Login(context.Background(), "staff@levelup.test", "good-password")
	require.NoError(t, err)
	require.Nil(t, s.Tenant)
	p, _, err := h.issuer.Verify(s.AccessToken)
	require.NoError(t, err)
	require.Empty(t, p.TenantID)
	require.Equal(t, admin.ID, p.UserID)
}

func TestRefreshRotatesAndReReadsRoles(t *testing.T) {
	h := newHarness(t, allowKeys{})
	s := register(t, h, "owner@example.com")

	// Role change between login and refresh shows up in the new token.
	u := h.repo.users[s.User.ID]
	u.RoleIDs = []int64{contracts.RoleOwner, contracts.RoleAdmin}
	h.repo.users[u.ID] = u

	next, err := h.svc.Refresh(context.Background(), s.RefreshToken)
	require.NoError(t, err)
	require.NotEqual(t, s.RefreshToken, next.RefreshToken)
	p, _, err := h.issuer.Verify(next.AccessToken)
	require.NoError(t, err)
	require.Equal(t, []int64{contracts.RoleOwner, contracts.RoleAdmin}, p.RoleIDs)
	require.Equal(t, s.Tenant.ID, p.TenantID)

	// Replay of the consumed token is rejected and revokes the family.
	_, err = h.svc.Refresh(context.Background(), s.RefreshToken)
	require.ErrorIs(t, err, authn.ErrTokenReused)
	_, err = h.svc.Refresh(context.Background(), next.RefreshToken)
	mustKind(t, err, errs.Unauthenticated)
}

func TestRefreshRejectsDeactivatedUser(t *testing.T) {
	h := newHarness(t, allowKeys{})
	s := register(t, h, "owner@example.com")
	tn := h.repo.tenants[s.Tenant.ID]
	tn.Active = false
	h.repo.tenants[tn.ID] = tn

	_, err := h.svc.Refresh(context.Background(), s.RefreshToken)
	require.ErrorIs(t, err, domain.ErrBadCredentials)
	require.Contains(t, h.refresh.revoked, s.User.ID)
}

func TestLogoutDeniesJTIAndRevokesRefreshTokens(t *testing.T) {
	h := newHarness(t, allowKeys{})
	s := register(t, h, "owner@example.com")
	ctx := h.ctxWithToken(t, s.AccessToken)
	_, jti, err := h.issuer.Verify(s.AccessToken)
	require.NoError(t, err)

	require.NoError(t, h.svc.Logout(ctx))
	require.Contains(t, h.refresh.denied, jti)
	require.Equal(t, h.issuer.AccessTTL(), h.refresh.denied[jti])
	require.Contains(t, h.refresh.revoked, s.User.ID)

	_, err = h.svc.Refresh(context.Background(), s.RefreshToken)
	mustKind(t, err, errs.Unauthenticated)
}

func TestMeReturnsUserRolesAndTenant(t *testing.T) {
	h := newHarness(t, allowKeys{})
	s := register(t, h, "owner@example.com")
	u, tn, err := h.svc.Me(h.ctxWithToken(t, s.AccessToken))
	require.NoError(t, err)
	require.Equal(t, s.User.ID, u.ID)
	require.Equal(t, []int64{contracts.RoleOwner}, u.RoleIDs)
	require.NotNil(t, tn)
	require.Equal(t, s.Tenant.ID, tn.ID)

	_, _, err = h.svc.Me(context.Background())
	mustKind(t, err, errs.Unauthenticated)
}
