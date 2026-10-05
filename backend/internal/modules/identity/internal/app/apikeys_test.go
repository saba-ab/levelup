package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authn"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// fakeKeys is an in-memory APIKeyRepository. activeTenants mimics the
// repository's "tenant still active" join.
type fakeKeys struct {
	byID          map[string]domain.APIKey
	activeTenants map[string]bool
	touches       int
}

func newFakeKeys() *fakeKeys {
	return &fakeKeys{byID: map[string]domain.APIKey{}, activeTenants: map[string]bool{}}
}

func (f *fakeKeys) CreateAPIKey(_ context.Context, _ *gorm.DB, k domain.APIKey) error {
	f.byID[k.ID] = k
	return nil
}

func (f *fakeKeys) APIKeysInTenant(_ context.Context, tenantID string, page Page) ([]domain.APIKey, error) {
	var all []domain.APIKey
	for _, k := range f.byID {
		if k.TenantID == tenantID {
			all = append(all, k)
		}
	}
	return pageOf(all, page, func(k domain.APIKey) (time.Time, string) { return k.CreatedAt, k.ID }), nil
}

func (f *fakeKeys) APIKeyInTenantForUpdate(_ context.Context, _ *gorm.DB, tenantID, id string) (domain.APIKey, error) {
	k, ok := f.byID[id]
	if !ok || k.TenantID != tenantID {
		return domain.APIKey{}, domain.ErrAPIKeyNotFound
	}
	return k, nil
}

func (f *fakeKeys) APIKeyByPrefix(_ context.Context, prefix string) (domain.APIKey, error) {
	for _, k := range f.byID {
		if k.Prefix == prefix && f.activeTenants[k.TenantID] {
			return k, nil
		}
	}
	return domain.APIKey{}, domain.ErrAPIKeyNotFound
}

func (f *fakeKeys) RevokeAPIKey(_ context.Context, _ *gorm.DB, id string, at time.Time) error {
	k := f.byID[id]
	k.RevokedAt = &at
	f.byID[id] = k
	return nil
}

func (f *fakeKeys) TouchAPIKey(_ context.Context, id string, at time.Time, _ time.Duration) error {
	k := f.byID[id]
	k.LastUsedAt = &at
	f.byID[id] = k
	f.touches++
	return nil
}

// fakeKeyCache records evictions; it never serves stale data (pass-through).
type fakeKeyCache struct{ evicted []string }

func (c *fakeKeyCache) GetOrLoad(ctx context.Context, _ string, dst any, load func(context.Context) (any, error)) error {
	v, err := load(ctx)
	if err != nil {
		return err
	}
	*(dst.(*cachedKey)) = v.(cachedKey)
	return nil
}

func (c *fakeKeyCache) Del(_ context.Context, keys ...string) error {
	c.evicted = append(c.evicted, keys...)
	return nil
}

func keyHarness(t *testing.T) (*harness, *fakeKeys, *fakeKeyCache, domain.Tenant, domain.User) {
	t.Helper()
	h := newHarness(t, allowKeys{contracts.PermAPIKeysManage.Key(): true, contracts.PermUsersCreate.Key(): true})
	keys, cache := newFakeKeys(), &fakeKeyCache{}
	h.svc.WithAPIKeys(keys, cache)
	tn, owner := h.seedTenant(t, "Keys Co")
	keys.activeTenants[tn.ID] = true
	return h, keys, cache, tn, owner
}

func TestAPIKeyCreateAndVerify(t *testing.T) {
	h, keys, _, tn, owner := keyHarness(t)

	k, secret, err := h.svc.CreateAPIKey(ctxAs(principalOf(owner)), CreateAPIKeyCmd{Name: "Checkout backend"})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(secret, "lvl_live_"+k.Prefix+"_"))
	require.Equal(t, domain.DefaultAPIKeyRoles, k.RoleIDs)
	require.NotContains(t, keys.byID[k.ID].SecretHash, secret, "only the hash is stored")

	p, err := h.svc.VerifyAPIKey(context.Background(), secret)
	require.NoError(t, err)
	require.Equal(t, authz.Principal{UserID: k.ID, TenantID: tn.ID, RoleIDs: k.RoleIDs, APIKeyID: k.ID}, p)
	require.True(t, p.IsAPIKey())
	require.Equal(t, 1, keys.touches)
}

func TestAPIKeyVerifyRejectsEveryBadShapeTheSameWay(t *testing.T) {
	h, keys, _, tn, owner := keyHarness(t)
	_, secret, err := h.svc.CreateAPIKey(ctxAs(principalOf(owner)), CreateAPIKeyCmd{Name: "k"})
	require.NoError(t, err)

	wrongSecret := secret[:len(secret)-1] + map[bool]string{true: "a", false: "b"}[!strings.HasSuffix(secret, "a")]
	for name, raw := range map[string]string{
		"garbage":      "lvl_nope",
		"wrong secret": wrongSecret,
		"unknown":      "lvl_live_aaaaaaaaaa_" + strings.Repeat("a", 32),
		"bad charset":  strings.ToUpper(secret),
	} {
		_, err := h.svc.VerifyAPIKey(context.Background(), raw)
		require.ErrorIs(t, err, domain.ErrInvalidAPIKey, name)
	}

	keys.activeTenants[tn.ID] = false
	_, err = h.svc.VerifyAPIKey(context.Background(), secret)
	require.ErrorIs(t, err, domain.ErrInvalidAPIKey, "a suspended tenant's keys authenticate nothing")
}

func TestAPIKeyRevokeAndExpiry(t *testing.T) {
	h, _, cache, _, owner := keyHarness(t)
	ctx := ctxAs(principalOf(owner))

	k, secret, err := h.svc.CreateAPIKey(ctx, CreateAPIKeyCmd{Name: "revoke me"})
	require.NoError(t, err)
	require.NoError(t, h.svc.RevokeAPIKey(ctx, k.ID))
	require.NoError(t, h.svc.RevokeAPIKey(ctx, k.ID), "revoking twice is a no-op")
	require.Contains(t, cache.evicted, keyCacheKey(k.Prefix), "evicted after commit")
	_, err = h.svc.VerifyAPIKey(context.Background(), secret)
	require.ErrorIs(t, err, domain.ErrInvalidAPIKey)

	exp := h.clock.Now().Add(time.Hour)
	_, secret, err = h.svc.CreateAPIKey(ctx, CreateAPIKeyCmd{Name: "short", ExpiresAt: &exp})
	require.NoError(t, err)
	_, err = h.svc.VerifyAPIKey(context.Background(), secret)
	require.NoError(t, err)
	h.clock.Advance(2 * time.Hour)
	_, err = h.svc.VerifyAPIKey(context.Background(), secret)
	require.ErrorIs(t, err, domain.ErrInvalidAPIKey)

	past := h.clock.Now().Add(-time.Minute)
	_, _, err = h.svc.CreateAPIKey(ctx, CreateAPIKeyCmd{Name: "past", ExpiresAt: &past})
	require.ErrorIs(t, err, domain.ErrAPIKeyExpiryInPast)
}

func TestAPIKeyRolesAreBounded(t *testing.T) {
	h, _, _, tn, owner := keyHarness(t)

	_, _, err := h.svc.CreateAPIKey(ctxAs(principalOf(owner)), CreateAPIKeyCmd{Name: "owner key", RoleIDs: []int64{contracts.RoleOwner}})
	require.ErrorIs(t, err, domain.ErrAPIKeyRoleNotAllowed)
	_, _, err = h.svc.CreateAPIKey(ctxAs(principalOf(owner)), CreateAPIKeyCmd{Name: "platform", RoleIDs: []int64{contracts.RolePlatformAdmin}})
	require.ErrorIs(t, err, domain.ErrPlatformRoleForbidden)

	dev := h.seedUser(t, tn.ID, "dev@example.com", "dev-password", contracts.RoleDeveloper)
	_, _, err = h.svc.CreateAPIKey(ctxAs(principalOf(dev)), CreateAPIKeyCmd{Name: "escalate", RoleIDs: []int64{contracts.RoleAdmin}})
	require.ErrorIs(t, err, domain.ErrRoleEscalation, "a key never outranks its creator")
}

// A leaked admin key must not be able to mint people or more keys.
func TestAPIKeyPrincipalCannotAdminister(t *testing.T) {
	h, _, _, _, owner := keyHarness(t)
	k, secret, err := h.svc.CreateAPIKey(ctxAs(principalOf(owner)), CreateAPIKeyCmd{Name: "admin key", RoleIDs: []int64{contracts.RoleAdmin}})
	require.NoError(t, err)
	p, err := h.svc.VerifyAPIKey(context.Background(), secret)
	require.NoError(t, err)
	keyCtx := ctxAs(p)

	_, _, err = h.svc.CreateAPIKey(keyCtx, CreateAPIKeyCmd{Name: "child"})
	require.ErrorIs(t, err, domain.ErrHumanPrincipalRequired)
	require.ErrorIs(t, h.svc.RevokeAPIKey(keyCtx, k.ID), domain.ErrHumanPrincipalRequired)
	_, _, err = h.svc.ListAPIKeys(keyCtx, 10, "")
	require.ErrorIs(t, err, domain.ErrHumanPrincipalRequired)
	_, err = h.svc.CreateUser(keyCtx, CreateUserCmd{Name: "Mallory", Email: "m@example.com", Password: "mallory-pass"})
	require.ErrorIs(t, err, domain.ErrHumanPrincipalRequired)
	_, err = h.svc.AssignRoles(keyCtx, owner.ID, []int64{contracts.RoleAdmin})
	require.ErrorIs(t, err, domain.ErrHumanPrincipalRequired)
	require.ErrorIs(t, h.svc.DeleteCurrentTenant(keyCtx), domain.ErrHumanPrincipalRequired)
}

func TestAPIKeysAreTenantScoped(t *testing.T) {
	h, keys, _, _, owner := keyHarness(t)
	k, _, err := h.svc.CreateAPIKey(ctxAs(principalOf(owner)), CreateAPIKeyCmd{Name: "mine"})
	require.NoError(t, err)

	otherTenant, otherOwner := h.seedTenant(t, "Other Co")
	keys.activeTenants[otherTenant.ID] = true
	require.ErrorIs(t, h.svc.RevokeAPIKey(ctxAs(principalOf(otherOwner)), k.ID), domain.ErrAPIKeyNotFound)
	list, _, err := h.svc.ListAPIKeys(ctxAs(principalOf(otherOwner)), 10, "")
	require.NoError(t, err)
	require.Empty(t, list)
}

// The platform middleware turns a presented key into the principal.
func TestMiddlewareAuthenticatesAPIKeys(t *testing.T) {
	h, _, _, tn, owner := keyHarness(t)
	_, secret, err := h.svc.CreateAPIKey(ctxAs(principalOf(owner)), CreateAPIKeyCmd{Name: "mw"})
	require.NoError(t, err)

	mw := authn.Middleware(h.issuer, nil, zap.NewNop(), h.svc)
	for _, set := range []func(*http.Request){
		func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+secret) },
		func(r *http.Request) { r.Header.Set("X-API-Key", secret) },
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		set(req)
		var got authz.Principal
		var ok bool
		mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got, ok = authz.From(r.Context()) })).
			ServeHTTP(httptest.NewRecorder(), req)
		require.True(t, ok)
		require.Equal(t, tn.ID, got.TenantID)
		require.True(t, got.IsAPIKey())
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer lvl_live_bogus")
	var ok bool
	mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { _, ok = authz.From(r.Context()) })).
		ServeHTTP(httptest.NewRecorder(), req)
	require.False(t, ok, "an invalid key leaves the request anonymous; RequireAuth then answers 401")
	mustKind(t, domain.ErrInvalidAPIKey, errs.Unauthenticated)
}
