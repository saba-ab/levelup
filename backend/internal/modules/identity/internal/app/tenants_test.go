package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

func TestUpdateCurrentTenant(t *testing.T) {
	h := newHarness(t, adminPerms)
	tn, owner := h.seedTenant(t, "Acme")
	ctx := ctxAs(principalOf(owner))

	got, err := h.svc.UpdateCurrentTenant(ctx, domain.TenantChanges{
		Name: ptr("Acme Ltd"), Timezone: ptr("Asia/Tbilisi"), Settings: map[string]any{"theme": "dark"},
	})
	require.NoError(t, err)
	require.Equal(t, "Acme Ltd", got.Name)
	require.Equal(t, tn.Slug, got.Slug, "slug does not follow renames")
	require.Equal(t, "Asia/Tbilisi", h.repo.tenants[tn.ID].Timezone)
	require.Equal(t, []string{contracts.TopicTenantUpdated}, h.outbox.topics())
	ev := h.outbox.published[0].payload.(contracts.TenantUpdatedV1)
	require.Equal(t, "Asia/Tbilisi", ev.Timezone)
	require.True(t, ev.Active)

	h.outbox.published = nil
	_, err = h.svc.UpdateCurrentTenant(ctx, domain.TenantChanges{Name: ptr("Acme Ltd")})
	require.NoError(t, err)
	require.Empty(t, h.outbox.published, "no-op update publishes nothing")

	_, err = h.svc.UpdateCurrentTenant(ctx, domain.TenantChanges{Timezone: ptr("Moon/Crater")})
	require.ErrorIs(t, err, domain.ErrInvalidTimezone)
	mustKind(t, err, errs.Invalid)

	dev := h.seedUser(t, tn.ID, "dev@example.com", "password-1", contracts.RoleDeveloper)
	hView := newHarness(t, allowKeys{contracts.PermTenantView.Key(): true})
	hView.svc.repo = h.repo
	cur, err := hView.svc.CurrentTenant(ctxAs(principalOf(dev)))
	require.NoError(t, err)
	require.Equal(t, tn.ID, cur.ID)
	_, err = hView.svc.UpdateCurrentTenant(ctxAs(principalOf(dev)), domain.TenantChanges{Name: ptr("x")})
	mustKind(t, err, errs.PermissionDenied)
}

func TestDeleteCurrentTenantOwnerOnly(t *testing.T) {
	h := newHarness(t, adminPerms)
	tn, owner := h.seedTenant(t, "Acme")
	member := h.seedUser(t, tn.ID, "m@example.com", "password-1", contracts.RoleOwner)

	err := h.svc.DeleteCurrentTenant(ctxAs(principalOf(member)))
	require.ErrorIs(t, err, domain.ErrNotTenantOwner, "holding the owner role is not owning the tenant")
	require.Empty(t, h.outbox.published)

	require.NoError(t, h.svc.DeleteCurrentTenant(ctxAs(principalOf(owner))))
	require.NotNil(t, h.repo.tenants[tn.ID].DeletedAt)
	require.False(t, h.repo.tenants[tn.ID].Active)
	require.Equal(t, []string{contracts.TopicTenantDeleted}, h.outbox.topics())
	require.Equal(t, tn.ID, h.outbox.published[0].payload.(contracts.TenantDeletedV1).TenantID)
	require.ElementsMatch(t, []string{owner.ID, member.ID}, h.refresh.revoked)

	_, err = h.svc.Login(context.Background(), owner.Email, "owner-password")
	require.ErrorIs(t, err, domain.ErrBadCredentials, "members of a deleted tenant cannot log in")
}

func TestPlatformRoutesRequirePlatformPrincipal(t *testing.T) {
	h := newHarness(t, allowKeys{contracts.PermPlatformTenantsManage.Key(): true})
	tn, owner := h.seedTenant(t, "Acme")

	// A tenant principal is refused even when holding the permission.
	_, _, err := h.svc.PlatformListTenants(ctxAs(principalOf(owner)), 0, "")
	require.ErrorIs(t, err, domain.ErrPlatformOnly)

	hNone := newHarness(t, allowKeys{})
	hNone.svc.repo = h.repo
	_, _, err = hNone.svc.PlatformListTenants(ctxAs(authz.Principal{UserID: "x"}), 0, "")
	mustKind(t, err, errs.PermissionDenied)

	staff := authz.Principal{UserID: id.NewID(), RoleIDs: []int64{contracts.RolePlatformAdmin}}
	list, next, err := h.svc.PlatformListTenants(ctxAs(staff), 0, "")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Empty(t, next)

	got, err := h.svc.PlatformSetTenantActive(ctxAs(staff), tn.ID, false)
	require.NoError(t, err)
	require.False(t, got.Active)
	require.Equal(t, []string{contracts.TopicTenantUpdated}, h.outbox.topics())
	require.False(t, h.outbox.published[0].payload.(contracts.TenantUpdatedV1).Active)
	require.Contains(t, h.refresh.revoked, owner.ID)

	h.outbox.published = nil
	_, err = h.svc.PlatformSetTenantActive(ctxAs(staff), tn.ID, false)
	require.NoError(t, err)
	require.Empty(t, h.outbox.published, "idempotent: no change, no event")

	_, err = h.svc.PlatformSetTenantActive(ctxAs(staff), id.NewID(), true)
	require.ErrorIs(t, err, domain.ErrTenantNotFound)
}

func TestTenantReader(t *testing.T) {
	h := newHarness(t, allowKeys{})
	a, _ := h.seedTenant(t, "A")
	b, _ := h.seedTenant(t, "B")
	gone, _ := h.seedTenant(t, "Gone")
	gone.SoftDelete(t0)
	h.repo.tenants[gone.ID] = gone
	b.Active = false
	h.repo.tenants[b.ID] = b

	snaps, err := h.svc.TenantsByIDs(context.Background(), []string{a.ID, a.ID, b.ID, gone.ID, "not-a-uuid", id.NewID()})
	require.NoError(t, err)
	require.Len(t, snaps, 2)
	byID := map[string]contracts.TenantSnapshot{}
	for _, s := range snaps {
		byID[s.ID] = s
	}
	require.True(t, byID[a.ID].Active)
	require.Equal(t, "UTC", byID[a.ID].Timezone)
	require.False(t, byID[b.ID].Active)

	empty, err := h.svc.TenantsByIDs(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, empty)

	var _ contracts.TenantReader = h.svc
}
