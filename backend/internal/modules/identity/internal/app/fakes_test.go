package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authn"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/mail"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// ---- repository ----

type fakeRepo struct {
	tenants map[string]domain.Tenant
	users   map[string]domain.User

	resets  map[string]domain.PasswordReset     // by id
	verifs  map[string]domain.EmailVerification // by id
	invites map[string]domain.Invitation        // by id

	slugCollisions int // fail this many CreateTenant calls with ErrSlugTaken
	rehashed       map[string]string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		tenants: map[string]domain.Tenant{}, users: map[string]domain.User{}, rehashed: map[string]string{},
		resets: map[string]domain.PasswordReset{}, verifs: map[string]domain.EmailVerification{},
		invites: map[string]domain.Invitation{},
	}
}

func (f *fakeRepo) snapshot() *fakeRepo {
	c := &fakeRepo{
		tenants: maps.Clone(f.tenants), users: maps.Clone(f.users), rehashed: maps.Clone(f.rehashed),
		resets: maps.Clone(f.resets), verifs: maps.Clone(f.verifs), invites: maps.Clone(f.invites),
	}
	c.slugCollisions = f.slugCollisions
	return c
}

func (f *fakeRepo) restore(s *fakeRepo) {
	f.tenants, f.users, f.rehashed = s.tenants, s.users, s.rehashed
	f.resets, f.verifs, f.invites = s.resets, s.verifs, s.invites
}

func (f *fakeRepo) CreateTenant(_ context.Context, _ *gorm.DB, t domain.Tenant) error {
	if f.slugCollisions > 0 {
		f.slugCollisions--
		return domain.ErrSlugTaken
	}
	for _, x := range f.tenants {
		if x.Slug == t.Slug && x.DeletedAt == nil {
			return domain.ErrSlugTaken
		}
	}
	f.tenants[t.ID] = t
	return nil
}

func (f *fakeRepo) TenantByID(_ context.Context, id string) (domain.Tenant, error) {
	t, ok := f.tenants[id]
	if !ok || t.DeletedAt != nil {
		return domain.Tenant{}, domain.ErrTenantNotFound
	}
	return t, nil
}

func (f *fakeRepo) TenantByIDForUpdate(ctx context.Context, _ *gorm.DB, id string) (domain.Tenant, error) {
	return f.TenantByID(ctx, id)
}

func (f *fakeRepo) TenantsByIDs(_ context.Context, ids []string) ([]domain.Tenant, error) {
	var out []domain.Tenant
	for _, id := range ids {
		if t, ok := f.tenants[id]; ok && t.DeletedAt == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeRepo) ListTenants(_ context.Context, page Page) ([]domain.Tenant, error) {
	var all []domain.Tenant
	for _, t := range f.tenants {
		if t.DeletedAt == nil {
			all = append(all, t)
		}
	}
	return pageOf(all, page, func(t domain.Tenant) (time.Time, string) { return t.CreatedAt, t.ID }), nil
}

func (f *fakeRepo) SaveTenant(_ context.Context, _ *gorm.DB, t domain.Tenant) error {
	cur, ok := f.tenants[t.ID]
	if !ok {
		return domain.ErrTenantNotFound
	}
	if cur.Version != t.Version {
		return domain.ErrVersionConflict
	}
	t.Version++
	f.tenants[t.ID] = t
	return nil
}

func (f *fakeRepo) CreateUser(_ context.Context, _ *gorm.DB, u domain.User) error {
	for _, x := range f.users {
		if strings.EqualFold(x.Email, u.Email) {
			return domain.ErrEmailTaken
		}
	}
	u.RoleIDs = slices.Clone(u.RoleIDs)
	f.users[u.ID] = u
	return nil
}

func (f *fakeRepo) UserInTenant(_ context.Context, tenantID, id string) (domain.User, error) {
	u, ok := f.users[id]
	if !ok || u.TenantID != tenantID {
		return domain.User{}, domain.ErrUserNotFound
	}
	u.RoleIDs = slices.Clone(u.RoleIDs)
	return u, nil
}

func (f *fakeRepo) UserInTenantForUpdate(ctx context.Context, _ *gorm.DB, tenantID, id string) (domain.User, error) {
	return f.UserInTenant(ctx, tenantID, id)
}

func (f *fakeRepo) UserByID(_ context.Context, id string) (domain.User, error) {
	u, ok := f.users[id]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (f *fakeRepo) UserByEmail(_ context.Context, email string) (domain.User, error) {
	for _, u := range f.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (f *fakeRepo) ListUsers(_ context.Context, tenantID string, page Page) ([]domain.User, error) {
	var all []domain.User
	for _, u := range f.users {
		if u.TenantID == tenantID {
			all = append(all, u)
		}
	}
	return pageOf(all, page, func(u domain.User) (time.Time, string) { return u.CreatedAt, u.ID }), nil
}

func (f *fakeRepo) UserIDsInTenant(_ context.Context, tenantID string) ([]string, error) {
	var ids []string
	for _, u := range f.users {
		if u.TenantID == tenantID {
			ids = append(ids, u.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (f *fakeRepo) SaveUser(_ context.Context, _ *gorm.DB, u domain.User) error {
	cur, ok := f.users[u.ID]
	if !ok {
		return domain.ErrUserNotFound
	}
	if cur.Version != u.Version {
		return domain.ErrVersionConflict
	}
	for _, x := range f.users {
		if x.ID != u.ID && strings.EqualFold(x.Email, u.Email) {
			return domain.ErrEmailTaken
		}
	}
	u.Version++
	u.RoleIDs = cur.RoleIDs // roles are written by SetUserRoles only
	f.users[u.ID] = u
	return nil
}

func (f *fakeRepo) SetUserRoles(_ context.Context, _ *gorm.DB, userID string, roleIDs []int64, _ time.Time) error {
	u, ok := f.users[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.RoleIDs = slices.Clone(roleIDs)
	f.users[userID] = u
	return nil
}

func (f *fakeRepo) UpdatePasswordHash(_ context.Context, _ *gorm.DB, userID, hash string) error {
	u := f.users[userID]
	u.PasswordHash = hash
	f.users[userID] = u
	f.rehashed[userID] = hash
	return nil
}

func (f *fakeRepo) DeleteUser(_ context.Context, _ *gorm.DB, tenantID, id string) error {
	u, ok := f.users[id]
	if !ok || u.TenantID != tenantID {
		return domain.ErrUserNotFound
	}
	delete(f.users, id)
	return nil
}

func pageOf[T any](all []T, page Page, key func(T) (time.Time, string)) []T {
	sort.Slice(all, func(i, j int) bool {
		ti, ii := key(all[i])
		tj, ij := key(all[j])
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return ii > ij
	})
	var out []T
	for _, x := range all {
		ts, xid := key(x)
		after := ts.Before(page.AfterCreated) || (ts.Equal(page.AfterCreated) && xid < page.AfterID)
		if !page.AfterCreated.IsZero() && !after {
			continue
		}
		out = append(out, x)
		if len(out) == page.Limit+1 {
			break
		}
	}
	return out
}

// ---- outbox ----

type recordedEvent struct {
	topic   string
	payload any
}

type fakeOutbox struct{ published []recordedEvent }

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.published = append(f.published, recordedEvent{topic, payload})
	return nil
}

func (f *fakeOutbox) topics() []string {
	out := make([]string, len(f.published))
	for i, e := range f.published {
		out[i] = e.topic
	}
	return out
}

// ---- refresh store ----

type fakeRefresh struct {
	mu      sync.Mutex
	live    map[string]string // token → userID
	revoked []string
	denied  map[string]time.Duration
}

func newFakeRefresh() *fakeRefresh {
	return &fakeRefresh{live: map[string]string{}, denied: map[string]time.Duration{}}
}

func (f *fakeRefresh) Issue(_ context.Context, userID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tok := base64.RawURLEncoding.EncodeToString([]byte(userID + "." + id.NewID()))
	f.live[tok] = userID
	return tok, nil
}

func (f *fakeRefresh) Rotate(ctx context.Context, raw string) (string, error) {
	userID, _, err := authn.DecodeUserID(raw)
	if err != nil {
		return "", err
	}
	f.mu.Lock()
	_, ok := f.live[raw]
	delete(f.live, raw)
	f.mu.Unlock()
	if !ok {
		_ = f.RevokeAll(ctx, userID)
		return "", authn.ErrTokenReused
	}
	return f.Issue(ctx, userID)
}

func (f *fakeRefresh) RevokeAll(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for tok, uid := range f.live {
		if uid == userID {
			delete(f.live, tok)
		}
	}
	f.revoked = append(f.revoked, userID)
	return nil
}

func (f *fakeRefresh) Deny(_ context.Context, jti string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.denied[jti] = ttl
	return nil
}

// ---- authz ----

// allowKeys grants only the listed permission keys.
type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

// ---- harness ----

const testSecret = "0123456789abcdef0123456789abcdef"

// t0 is "now": access tokens minted by the real issuer are verified against
// the wall clock.
var t0 = time.Now().UTC().Truncate(time.Second)

type harness struct {
	svc      *Service
	repo     *fakeRepo
	outbox   *fakeOutbox
	refresh  *fakeRefresh
	issuer   *authn.Issuer
	clock    *clock.Fake
	mail     *mail.Recorder
	throttle *fakeThrottle
}

func newHarness(t *testing.T, enf authz.Enforcer) *harness {
	t.Helper()
	h := &harness{
		repo: newFakeRepo(), outbox: &fakeOutbox{}, refresh: newFakeRefresh(),
		clock: clock.NewFake(t0),
	}
	h.issuer = authn.NewIssuer(testSecret, 15*time.Minute, h.clock)
	h.mail = &mail.Recorder{}
	h.throttle = newFakeThrottle()
	h.svc = NewService(h.repo, h.outbox, enf, h.issuer, h.refresh, nil, h.clock, zap.NewNop(),
		Settings{BcryptCost: 4, AllowSelfSignup: true, PortalURL: "https://portal.test/"})
	h.svc.WithAccountTokens(h.repo, h.mail, h.throttle)
	h.svc.background = func(fn func()) { fn() }
	// The fake transaction rolls back repo AND staged events on error, so
	// atomicity is observable without a database.
	h.svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error {
		snap := h.repo.snapshot()
		n := len(h.outbox.published)
		if err := fn(nil); err != nil {
			h.repo.restore(snap)
			h.outbox.published = h.outbox.published[:n]
			return err
		}
		return nil
	}
	return h
}

// seedTenant creates a tenant with an owner and returns both.
func (h *harness) seedTenant(t *testing.T, name string) (domain.Tenant, domain.User) {
	t.Helper()
	tn, err := domain.NewTenant(name, "UTC", randomSuffix(), h.clock.Now())
	require.NoError(t, err)
	owner := h.seedUser(t, tn.ID, "owner-"+tn.Slug+"@example.com", "owner-password", 2)
	tn.OwnerUserID = owner.ID
	h.repo.tenants[tn.ID] = tn
	return tn, owner
}

func (h *harness) seedUser(t *testing.T, tenantID, email, password string, roles ...int64) domain.User {
	t.Helper()
	hash, err := h.svc.pw.Hash(password)
	require.NoError(t, err)
	u, err := domain.NewUser(tenantID, domain.NameFromEmail(email), email, hash, roles, h.clock.Now())
	require.NoError(t, err)
	h.repo.users[u.ID] = u
	h.clock.Advance(time.Second) // distinct created_at for paging
	return u
}

func principalOf(u domain.User) authz.Principal {
	return authz.Principal{UserID: u.ID, TenantID: u.TenantID, RoleIDs: slices.Clone(u.RoleIDs)}
}

func ctxAs(p authz.Principal) context.Context {
	return authz.Into(context.Background(), p)
}

// ctxWithToken runs a real access token through the platform middleware so
// the context carries both the principal and the jti, exactly as in a
// request.
func (h *harness) ctxWithToken(t *testing.T, token string) context.Context {
	t.Helper()
	var got context.Context
	mw := authn.Middleware(h.issuer, nil, zap.NewNop(), nil)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.Context() })).
		ServeHTTP(httptest.NewRecorder(), req)
	require.NotNil(t, got)
	_, ok := authz.From(got)
	require.True(t, ok, "token did not verify")
	return got
}

func mustKind(t *testing.T, err error, k errs.Kind) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, k, errs.KindOf(err), fmt.Sprintf("got %v", err))
}
