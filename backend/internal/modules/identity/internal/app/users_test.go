package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

var adminPerms = allowKeys{
	contracts.PermUsersViewAny.Key():     true,
	contracts.PermUsersCreate.Key():      true,
	contracts.PermUsersUpdate.Key():      true,
	contracts.PermUsersDelete.Key():      true,
	contracts.PermUsersAssignRoles.Key(): true,
	contracts.PermTenantView.Key():       true,
	contracts.PermTenantUpdate.Key():     true,
	contracts.PermTenantDelete.Key():     true,
}

func ptr[T any](v T) *T { return &v }

func TestUsersRequireTenantPrincipal(t *testing.T) {
	h := newHarness(t, adminPerms)
	platform := authz.Principal{UserID: "staff", RoleIDs: []int64{contracts.RolePlatformAdmin}}
	_, _, err := h.svc.ListUsers(ctxAs(platform), 0, "")
	require.ErrorIs(t, err, authz.ErrNoTenant)
	_, _, err = h.svc.ListUsers(context.Background(), 0, "")
	mustKind(t, err, errs.Unauthenticated)
}

// Fixes doc 02 §10 bug 1: GET /users never shows another tenant's users.
func TestListUsersIsTenantScopedAndPaged(t *testing.T) {
	h := newHarness(t, adminPerms)
	a, ownerA := h.seedTenant(t, "A")
	h.seedUser(t, a.ID, "a1@example.com", "password-1", contracts.RoleDeveloper)
	h.seedUser(t, a.ID, "a2@example.com", "password-1", contracts.RoleDeveloper)
	b, _ := h.seedTenant(t, "B")
	h.seedUser(t, b.ID, "b1@example.com", "password-1")

	ctx := ctxAs(principalOf(ownerA))
	page1, next, err := h.svc.ListUsers(ctx, 2, "")
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, next)
	page2, next2, err := h.svc.ListUsers(ctx, 2, next)
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.Empty(t, next2)
	for _, u := range append(page1, page2...) {
		require.Equal(t, a.ID, u.TenantID)
	}

	_, _, err = h.svc.ListUsers(ctx, 2, "garbage!!")
	mustKind(t, err, errs.Invalid)

	dev := h.seedUser(t, a.ID, "dev@example.com", "password-1", contracts.RoleDeveloper)
	hNoPerm := newHarness(t, allowKeys{})
	hNoPerm.repo = h.repo
	hNoPerm.svc.repo = h.repo
	_, _, err = hNoPerm.svc.ListUsers(ctxAs(principalOf(dev)), 0, "")
	mustKind(t, err, errs.PermissionDenied)
}

func TestGetUserCrossTenantIs404AndSelfNeedsNoPermission(t *testing.T) {
	h := newHarness(t, allowKeys{contracts.PermUsersViewAny.Key(): true})
	_, ownerA := h.seedTenant(t, "A")
	b, _ := h.seedTenant(t, "B")
	foreign := h.seedUser(t, b.ID, "victim@example.com", "password-1")

	_, err := h.svc.GetUser(ctxAs(principalOf(ownerA)), foreign.ID)
	require.ErrorIs(t, err, domain.ErrUserNotFound)
	mustKind(t, err, errs.NotFound)

	hNone := newHarness(t, allowKeys{})
	hNone.svc.repo = h.repo
	self, err := hNone.svc.GetUser(ctxAs(principalOf(foreign)), foreign.ID)
	require.NoError(t, err)
	require.Equal(t, foreign.ID, self.ID)
	_, err = hNone.svc.GetUser(ctxAs(principalOf(foreign)), ownerA.ID)
	mustKind(t, err, errs.PermissionDenied)
}

// Fixes doc 02 §10 bug 2: tenant comes from the principal, never the body.
func TestCreateUserUsesPrincipalTenantAndPublishes(t *testing.T) {
	h := newHarness(t, adminPerms)
	a, owner := h.seedTenant(t, "A")

	u, err := h.svc.CreateUser(ctxAs(principalOf(owner)), CreateUserCmd{
		Name: "New", Email: "New@Example.com", Password: "password-1",
		RoleIDs: []int64{contracts.RoleDeveloper, contracts.RoleAdmin},
	})
	require.NoError(t, err)
	require.Equal(t, a.ID, u.TenantID)
	require.Equal(t, "new@example.com", u.Email)
	require.Equal(t, []int64{contracts.RoleAdmin, contracts.RoleDeveloper}, u.RoleIDs)
	require.Equal(t, []string{contracts.TopicUserCreated}, h.outbox.topics())
	ev := h.outbox.published[0].payload.(contracts.UserCreatedV1)
	require.Equal(t, owner.ID, ev.CreatedBy)
	require.Equal(t, a.ID, ev.TenantID)

	_, err = h.svc.CreateUser(ctxAs(principalOf(owner)), CreateUserCmd{
		Name: "Dup", Email: "NEW@example.com", Password: "password-1",
	})
	require.ErrorIs(t, err, domain.ErrEmailTaken)
	mustKind(t, err, errs.AlreadyExists)
}

func TestCannotAssignPlatformAdminRole(t *testing.T) {
	h := newHarness(t, adminPerms)
	_, owner := h.seedTenant(t, "A")
	member := h.seedUser(t, owner.TenantID, "m@example.com", "password-1", contracts.RoleDeveloper)
	ctx := ctxAs(principalOf(owner))

	_, err := h.svc.CreateUser(ctx, CreateUserCmd{
		Name: "X", Email: "x@example.com", Password: "password-1",
		RoleIDs: []int64{contracts.RolePlatformAdmin},
	})
	require.ErrorIs(t, err, domain.ErrPlatformRoleForbidden)
	mustKind(t, err, errs.PermissionDenied)

	_, err = h.svc.AssignRoles(ctx, member.ID, []int64{contracts.RolePlatformAdmin})
	require.ErrorIs(t, err, domain.ErrPlatformRoleForbidden)
	require.Empty(t, h.outbox.published)
	require.Equal(t, []int64{contracts.RoleDeveloper}, h.repo.users[member.ID].RoleIDs)
}

func TestRoleEscalationRefused(t *testing.T) {
	h := newHarness(t, adminPerms)
	_, owner := h.seedTenant(t, "A")
	admin := h.seedUser(t, owner.TenantID, "admin@example.com", "password-1", contracts.RoleAdmin)
	dev := h.seedUser(t, owner.TenantID, "dev@example.com", "password-1", contracts.RoleDeveloper)
	ctx := ctxAs(principalOf(admin))

	_, err := h.svc.CreateUser(ctx, CreateUserCmd{Name: "X", Email: "x@example.com", Password: "password-1",
		RoleIDs: []int64{contracts.RoleOwner}})
	require.ErrorIs(t, err, domain.ErrRoleEscalation)

	_, err = h.svc.AssignRoles(ctx, dev.ID, []int64{contracts.RoleSuperAdmin})
	require.ErrorIs(t, err, domain.ErrRoleEscalation)

	// An admin cannot touch the owner, who outranks them.
	_, err = h.svc.AssignRoles(ctx, owner.ID, []int64{contracts.RoleDeveloper})
	require.ErrorIs(t, err, domain.ErrInsufficientRank)
	_, err = h.svc.UpdateUser(ctx, owner.ID, UpdateUserCmd{Name: ptr("Hijacked")})
	require.ErrorIs(t, err, domain.ErrInsufficientRank)
	require.Empty(t, h.outbox.published)
}

func TestAssignRolesPublishesAndRevokesSessions(t *testing.T) {
	h := newHarness(t, adminPerms)
	_, owner := h.seedTenant(t, "A")
	member := h.seedUser(t, owner.TenantID, "m@example.com", "password-1", contracts.RoleDeveloper)
	ctx := ctxAs(principalOf(owner))

	u, err := h.svc.AssignRoles(ctx, member.ID, []int64{contracts.RoleProgramManager, contracts.RoleAdmin})
	require.NoError(t, err)
	require.Equal(t, []int64{contracts.RoleAdmin, contracts.RoleProgramManager}, u.RoleIDs)
	require.Equal(t, []int64{contracts.RoleAdmin, contracts.RoleProgramManager}, h.repo.users[member.ID].RoleIDs)
	require.Equal(t, []string{contracts.TopicUserRolesChanged}, h.outbox.topics())
	ev := h.outbox.published[0].payload.(contracts.UserRolesChangedV1)
	require.Equal(t, member.ID, ev.UserID)
	require.Equal(t, owner.TenantID, ev.TenantID)
	require.Contains(t, h.refresh.revoked, member.ID)

	// Same set again: no event, no revoke.
	h.outbox.published, h.refresh.revoked = nil, nil
	_, err = h.svc.AssignRoles(ctx, member.ID, []int64{contracts.RoleAdmin, contracts.RoleProgramManager})
	require.NoError(t, err)
	require.Empty(t, h.outbox.published)
	require.Empty(t, h.refresh.revoked)
}

func TestTenantOwnerKeepsOwnerRole(t *testing.T) {
	h := newHarness(t, adminPerms)
	_, owner := h.seedTenant(t, "A")
	_, err := h.svc.AssignRoles(ctxAs(principalOf(owner)), owner.ID, []int64{contracts.RoleAdmin})
	require.ErrorIs(t, err, domain.ErrOwnerRoleRequired)
}

func TestAssignRolesCrossTenantIs404(t *testing.T) {
	h := newHarness(t, adminPerms)
	_, ownerA := h.seedTenant(t, "A")
	_, ownerB := h.seedTenant(t, "B")
	_, err := h.svc.AssignRoles(ctxAs(principalOf(ownerA)), ownerB.ID, []int64{contracts.RoleDeveloper})
	require.ErrorIs(t, err, domain.ErrUserNotFound)
}

func TestUpdateSelfPasswordRequiresCurrentAndRevokes(t *testing.T) {
	h := newHarness(t, allowKeys{}) // self-service needs no permission
	_, owner := h.seedTenant(t, "A")
	dev := h.seedUser(t, owner.TenantID, "dev@example.com", "old-password", contracts.RoleDeveloper)
	ctx := ctxAs(principalOf(dev))

	_, err := h.svc.UpdateUser(ctx, dev.ID, UpdateUserCmd{Password: ptr("new-password")})
	require.ErrorIs(t, err, domain.ErrCurrentPasswordRequired)
	_, err = h.svc.UpdateUser(ctx, dev.ID, UpdateUserCmd{Password: ptr("new-password"), CurrentPassword: ptr("wrong-one")})
	require.ErrorIs(t, err, domain.ErrCurrentPasswordWrong)
	require.Empty(t, h.outbox.published)

	u, err := h.svc.UpdateUser(ctx, dev.ID, UpdateUserCmd{
		Password: ptr("new-password"), CurrentPassword: ptr("old-password"), Name: ptr("Renamed"),
	})
	require.NoError(t, err)
	require.Equal(t, "Renamed", u.Name)
	require.NotNil(t, u.PasswordChangedAt)
	require.True(t, h.svc.pw.Matches(h.repo.users[dev.ID].PasswordHash, "new-password"))
	require.Equal(t, []string{contracts.TopicUserUpdated}, h.outbox.topics())
	require.Contains(t, h.refresh.revoked, dev.ID)

	_, err = h.svc.UpdateUser(ctx, dev.ID, UpdateUserCmd{Active: ptr(false)})
	require.ErrorIs(t, err, domain.ErrCannotDeactivateSelf)

	// A developer cannot update anyone else.
	_, err = h.svc.UpdateUser(ctx, owner.ID, UpdateUserCmd{Name: ptr("x")})
	mustKind(t, err, errs.PermissionDenied)
}

func TestUpdateUserEmailRules(t *testing.T) {
	h := newHarness(t, adminPerms)
	_, owner := h.seedTenant(t, "A")
	dev := h.seedUser(t, owner.TenantID, "dev@example.com", "password-1", contracts.RoleDeveloper)
	ctx := ctxAs(principalOf(owner))

	// Fixes doc 02 §10 bug 13: duplicate email is 409, not 500.
	_, err := h.svc.UpdateUser(ctx, dev.ID, UpdateUserCmd{Email: ptr("OWNER-" + "x@example.com")})
	require.NoError(t, err)
	_, err = h.svc.UpdateUser(ctx, dev.ID, UpdateUserCmd{Email: ptr(owner.Email)})
	require.ErrorIs(t, err, domain.ErrEmailTaken)

	// An admin may set another user's password without current_password.
	_, err = h.svc.UpdateUser(ctx, dev.ID, UpdateUserCmd{Password: ptr("reset-password")})
	require.NoError(t, err)
	require.Contains(t, h.refresh.revoked, dev.ID)

	_, err = h.svc.UpdateUser(ctx, dev.ID, UpdateUserCmd{Password: ptr("short")})
	require.ErrorIs(t, err, domain.ErrPasswordTooShort)
}

func TestDeactivateUser(t *testing.T) {
	h := newHarness(t, adminPerms)
	_, owner := h.seedTenant(t, "A")
	superAdmin := h.seedUser(t, owner.TenantID, "sa@example.com", "password-1", contracts.RoleSuperAdmin)
	dev := h.seedUser(t, owner.TenantID, "dev@example.com", "password-1", contracts.RoleDeveloper)

	u, err := h.svc.UpdateUser(ctxAs(principalOf(superAdmin)), dev.ID, UpdateUserCmd{Active: ptr(false)})
	require.NoError(t, err)
	require.False(t, u.Active)
	require.Contains(t, h.refresh.revoked, dev.ID)
	ev := h.outbox.published[0].payload.(contracts.UserUpdatedV1)
	require.False(t, ev.Active)

	// The owner cannot be deactivated, even by another owner-role holder.
	coOwner := h.seedUser(t, owner.TenantID, "co@example.com", "password-1", contracts.RoleOwner)
	_, err = h.svc.UpdateUser(ctxAs(principalOf(coOwner)), owner.ID, UpdateUserCmd{Active: ptr(false)})
	require.ErrorIs(t, err, domain.ErrOwnerCannotBeDeactivated)
}

func TestDeleteUser(t *testing.T) {
	h := newHarness(t, adminPerms)
	_, owner := h.seedTenant(t, "A")
	dev := h.seedUser(t, owner.TenantID, "dev@example.com", "password-1", contracts.RoleDeveloper)
	_, ownerB := h.seedTenant(t, "B")
	ctx := ctxAs(principalOf(owner))

	err := h.svc.DeleteUser(ctx, ownerB.ID)
	require.ErrorIs(t, err, domain.ErrUserNotFound, "cross-tenant delete is 404")

	err = h.svc.DeleteUser(ctx, owner.ID)
	require.ErrorIs(t, err, domain.ErrOwnerCannotBeDeleted)

	require.NoError(t, h.svc.DeleteUser(ctx, dev.ID))
	require.NotContains(t, h.repo.users, dev.ID)
	require.Equal(t, []string{contracts.TopicUserDeleted}, h.outbox.topics())
	require.Contains(t, h.refresh.revoked, dev.ID)

	hNone := newHarness(t, allowKeys{})
	hNone.svc.repo = h.repo
	err = hNone.svc.DeleteUser(ctxAs(principalOf(owner)), ownerB.ID)
	mustKind(t, err, errs.PermissionDenied)
}
