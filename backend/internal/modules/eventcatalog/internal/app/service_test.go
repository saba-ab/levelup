package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/eventcatalog/contracts"
	"levelup/internal/modules/eventcatalog/internal/domain"
	"levelup/internal/modules/eventcatalog/migrations"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

const (
	tenantA = "0199a000-0000-7000-8000-00000000000a"
	tenantB = "0199a000-0000-7000-8000-00000000000b"
)

var t0 = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// seededGrants mirrors the permission seed migration exactly, so these tests
// also prove the default grant matrix.
func seededGrants() allowKeys {
	a := allowKeys{}
	for _, p := range contracts.AllPermissions {
		for _, role := range migrations.GrantedRoles(p) {
			a[role] = append(a[role], p.Key())
		}
	}
	return a
}

type fixture struct {
	repo  *fakeRepo
	ob    *fakeOutbox
	clock *clock.Fake
	svc   *Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{repo: newFakeRepo(), ob: &fakeOutbox{}, clock: clock.NewFake(t0)}
	f.svc = NewService(f.repo, f.ob, seededGrants(), nil, f.clock)
	f.svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
	return f
}

func as(tenantID string, roles ...int64) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "u-" + tenantID, TenantID: tenantID, RoleIDs: roles})
}

func admin(tenantID string) context.Context { return as(tenantID, identitycontracts.RoleAdmin) }
func member(tenantID string) context.Context {
	return as(tenantID, identitycontracts.RoleProgramManager)
}
func platform() context.Context { return as("", identitycontracts.RolePlatformAdmin) }

// seedGlobal creates a global event type through the platform surface.
func (f *fixture) seedGlobal(t *testing.T, name, slug string) domain.EventType {
	t.Helper()
	et, err := f.svc.PlatformCreateType(platform(), CreateTypeCmd{Name: name, Slug: slug})
	require.NoError(t, err)
	f.clock.Advance(time.Second)
	return et
}

func (f *fixture) seedTenant(t *testing.T, tenantID, name, slug string) domain.EventType {
	t.Helper()
	et, err := f.svc.CreateType(member(tenantID), CreateTypeCmd{Name: name, Slug: slug})
	require.NoError(t, err)
	f.clock.Advance(time.Second)
	return et
}

func (f *fixture) topics() []string {
	out := make([]string, len(f.ob.published))
	for i, e := range f.ob.published {
		out[i] = e.topic
	}
	return out
}

func requireCode(t *testing.T, err error, kind errs.Kind, code string) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, kind, errs.KindOf(err), err.Error())
	require.Equal(t, code, errs.CodeOf(err))
}

// --- create ------------------------------------------------------------------

func TestCreateStampsTenantDerivesSlugAndPublishes(t *testing.T) {
	f := newFixture(t)
	et, err := f.svc.CreateType(member(tenantA), CreateTypeCmd{
		Name:           "My Custom Event",
		Description:    "Fired by our checkout",
		PropertySchema: map[string]any{"type": "object"},
	})
	require.NoError(t, err)
	require.Equal(t, tenantA, et.TenantID)
	require.Equal(t, "my_custom_event", et.Slug)
	require.True(t, et.Active, "is_active defaults to true")
	require.False(t, et.IsGlobal())

	require.Len(t, f.ob.published, 1)
	require.Equal(t, contracts.TopicTypeCreated, f.ob.published[0].topic)
	require.Equal(t, contracts.EventTypeChangedV1{
		EventTypeID: et.ID, TenantID: tenantA, Slug: "my_custom_event", Active: true, At: t0,
	}, f.ob.published[0].payload)
}

func TestCreateRequiresTenantAndPermission(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.CreateType(context.Background(), CreateTypeCmd{Name: "x"})
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))

	// Platform admin has no tenant: tenant routes refuse, never read unscoped.
	_, err = f.svc.CreateType(platform(), CreateTypeCmd{Name: "x"})
	require.ErrorIs(t, err, authz.ErrNoTenant)

	// A role-less tenant user holds no grant.
	_, err = f.svc.CreateType(as(tenantA), CreateTypeCmd{Name: "x"})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Empty(t, f.ob.published)
}

// Laravel bug 03 §10.1 #2: any tenant could create a global event. A tenant
// principal, even an owner, can never reach the global scope.
func TestTenantCannotCreateGlobalType(t *testing.T) {
	f := newFixture(t)
	owner := as(tenantA, identitycontracts.RoleOwner, identitycontracts.RoleSuperAdmin)

	et, err := f.svc.CreateType(owner, CreateTypeCmd{Name: "Sneaky"})
	require.NoError(t, err)
	require.Equal(t, tenantA, et.TenantID, "the tenant is stamped from the principal")

	_, err = f.svc.PlatformCreateType(owner, CreateTypeCmd{Name: "Sneaky Global"})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	// Even a platform-admin role inside a tenant token is refused.
	_, err = f.svc.PlatformCreateType(as(tenantA, identitycontracts.RolePlatformAdmin), CreateTypeCmd{Name: "x"})
	requireCode(t, err, errs.PermissionDenied, "platform_principal_required")

	page, err := f.svc.PlatformListTypes(platform(), ListTypesQuery{})
	require.NoError(t, err)
	require.Empty(t, page.Items, "no global row was created")
}

// Laravel bug 03 §10.1 #3: slug unique globally across tenants → 500.
func TestSlugUniquenessIsPerScope(t *testing.T) {
	f := newFixture(t)
	f.seedTenant(t, tenantA, "Checkout", "checkout")

	_, err := f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Checkout again", Slug: "checkout"})
	requireCode(t, err, errs.AlreadyExists, "event_type_slug_taken")

	_, err = f.svc.CreateType(member(tenantB), CreateTypeCmd{Name: "Checkout", Slug: "checkout"})
	require.NoError(t, err, "another tenant may reuse the slug")

	f.seedGlobal(t, "Level Up", "level_up")
	_, err = f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Our level up", Slug: "level_up"})
	require.NoError(t, err, "a tenant row may shadow a global slug")

	_, err = f.svc.PlatformCreateType(platform(), CreateTypeCmd{Name: "Level Up", Slug: "level_up"})
	requireCode(t, err, errs.AlreadyExists, "event_type_slug_taken")
}

func TestCreateRejectsInvariantViolations(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "x", Slug: "Not A Slug"})
	requireCode(t, err, errs.Invalid, "invalid_slug")
	_, err = f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "x", PropertySchema: map[string]any{"type": "array"}})
	requireCode(t, err, errs.Invalid, "invalid_property_schema")
	require.Empty(t, f.ob.published)
}

func TestCreateChecksCategoryVisibility(t *testing.T) {
	f := newFixture(t)
	global, err := f.svc.PlatformCreateCategory(platform(), CreateCategoryCmd{Name: "Commerce"})
	require.NoError(t, err)
	foreign := domain.Category{ID: "0199a000-0000-7000-8000-0000000000cf", TenantID: tenantB, Slug: "theirs", Name: "Theirs"}
	f.repo.categories[foreign.ID] = foreign

	et, err := f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Buy", CategoryID: global.ID})
	require.NoError(t, err)
	require.Equal(t, global.ID, et.CategoryID)

	_, err = f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Buy 2", CategoryID: foreign.ID})
	requireCode(t, err, errs.Invalid, "unknown_event_category")

	_, err = f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Buy 3", CategoryID: "0199a000-0000-7000-8000-0000000000ff"})
	requireCode(t, err, errs.Invalid, "unknown_event_category")

	// A global type may not reference a tenant category.
	own := domain.Category{ID: "0199a000-0000-7000-8000-0000000000ca", TenantID: tenantA, Slug: "mine", Name: "Mine"}
	f.repo.categories[own.ID] = own
	_, err = f.svc.PlatformCreateType(platform(), CreateTypeCmd{Name: "G", CategoryID: own.ID})
	requireCode(t, err, errs.Invalid, "unknown_event_category")
}

// --- get / list ------------------------------------------------------------------

// Laravel bug: GET of a global event answered 403.
func TestGetGlobalIsViewableAndForeignIsNotFound(t *testing.T) {
	f := newFixture(t)
	g := f.seedGlobal(t, "User Login", "user_login")
	theirs := f.seedTenant(t, tenantB, "Theirs", "theirs")
	mine := f.seedTenant(t, tenantA, "Mine", "mine")

	got, err := f.svc.GetType(member(tenantA), g.ID)
	require.NoError(t, err)
	require.True(t, got.IsGlobal())

	got, err = f.svc.GetType(member(tenantA), mine.ID)
	require.NoError(t, err)
	require.Equal(t, mine.ID, got.ID)

	_, err = f.svc.GetType(member(tenantA), theirs.ID)
	requireCode(t, err, errs.NotFound, "event_type_not_found")

	_, err = f.svc.GetType(as(tenantA), mine.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestListIncludesGlobalsShadowsAndFilters(t *testing.T) {
	f := newFixture(t)
	f.seedGlobal(t, "User Login", "user_login")
	gLevel := f.seedGlobal(t, "Level Up", "level_up")
	f.seedTenant(t, tenantB, "Foreign", "foreign")
	ownLogin := f.seedTenant(t, tenantA, "Our Login", "user_login")
	inactive := false
	ownOff, err := f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Off", Active: &inactive})
	require.NoError(t, err)

	ids := func(p Page[domain.EventType]) []string {
		var out []string
		for _, et := range p.Items {
			out = append(out, et.ID)
		}
		return out
	}

	page, err := f.svc.ListTypes(member(tenantA), ListTypesQuery{IncludeGlobal: true})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{gLevel.ID, ownLogin.ID, ownOff.ID}, ids(page),
		"own rows plus globals, the shadowed global user_login hidden, no foreign rows")

	page, err = f.svc.ListTypes(member(tenantA), ListTypesQuery{IncludeGlobal: false})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{ownLogin.ID, ownOff.ID}, ids(page))

	active := true
	page, err = f.svc.ListTypes(member(tenantA), ListTypesQuery{IncludeGlobal: true, Active: &active})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{gLevel.ID, ownLogin.ID}, ids(page))

	page, err = f.svc.ListTypes(member(tenantA), ListTypesQuery{IncludeGlobal: true, Search: "LEVEL"})
	require.NoError(t, err)
	require.Equal(t, []string{gLevel.ID}, ids(page))
}

func TestListFiltersByCategoryIDOrSlug(t *testing.T) {
	f := newFixture(t)
	cat, err := f.svc.PlatformCreateCategory(platform(), CreateCategoryCmd{Name: "Commerce"})
	require.NoError(t, err)
	inCat, err := f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Buy", CategoryID: cat.ID})
	require.NoError(t, err)
	f.seedTenant(t, tenantA, "Other", "other")

	for _, filter := range []string{cat.ID, "commerce"} {
		page, err := f.svc.ListTypes(member(tenantA), ListTypesQuery{IncludeGlobal: true, Category: filter})
		require.NoError(t, err)
		require.Len(t, page.Items, 1, filter)
		require.Equal(t, inCat.ID, page.Items[0].ID)
	}

	page, err := f.svc.ListTypes(member(tenantA), ListTypesQuery{IncludeGlobal: true, Category: "nope"})
	require.NoError(t, err)
	require.Empty(t, page.Items, "an unknown category slug filters everything out")
}

func TestListPagesWithKeysetCursor(t *testing.T) {
	f := newFixture(t)
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		f.seedTenant(t, tenantA, "Event "+n, "event_"+n)
	}

	var seen []string
	cursor := ""
	for range 3 {
		page, err := f.svc.ListTypes(member(tenantA), ListTypesQuery{Limit: 2, Cursor: cursor})
		require.NoError(t, err)
		for _, et := range page.Items {
			seen = append(seen, et.Slug)
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	require.Equal(t, []string{"event_e", "event_d", "event_c", "event_b", "event_a"}, seen, "newest first, no gaps or repeats")
	require.Empty(t, cursor)

	_, err := f.svc.ListTypes(member(tenantA), ListTypesQuery{Cursor: "garbage!"})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

// --- update ----------------------------------------------------------------------

func TestUpdateIsPartialAndPublishes(t *testing.T) {
	f := newFixture(t)
	et, err := f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Checkout", Description: "keep me"})
	require.NoError(t, err)
	f.clock.Advance(time.Minute)

	off := false
	got, err := f.svc.UpdateType(member(tenantA), et.ID, domain.EventTypePatch{Active: &off})
	require.NoError(t, err)
	require.False(t, got.Active)
	require.Equal(t, "Checkout", got.Name, "omitted fields stay untouched")
	require.Equal(t, "keep me", got.Description)
	require.Equal(t, t0.Add(time.Minute), got.UpdatedAt)

	require.Equal(t, []string{contracts.TopicTypeCreated, contracts.TopicTypeUpdated}, f.topics())
	ev := f.ob.published[1].payload.(contracts.EventTypeChangedV1)
	require.False(t, ev.Active)
	require.Equal(t, tenantA, ev.TenantID)

	empty := ""
	got, err = f.svc.UpdateType(member(tenantA), et.ID, domain.EventTypePatch{Description: &empty})
	require.NoError(t, err)
	require.Empty(t, got.Description, "description can be cleared (Laravel could not)")
}

func TestUpdateRefusesSlugChange(t *testing.T) {
	f := newFixture(t)
	et := f.seedTenant(t, tenantA, "Checkout", "checkout")
	slug := "checkout_v2"
	_, err := f.svc.UpdateType(member(tenantA), et.ID, domain.EventTypePatch{Slug: &slug})
	requireCode(t, err, errs.Invalid, "event_type_slug_immutable")
	require.Len(t, f.ob.published, 1, "only the create")
}

func TestTenantCannotModifyOrDeleteGlobal(t *testing.T) {
	f := newFixture(t)
	g := f.seedGlobal(t, "User Login", "user_login")
	before := len(f.ob.published)
	name := "Hijacked"
	owner := as(tenantA, identitycontracts.RoleOwner)

	_, err := f.svc.UpdateType(owner, g.ID, domain.EventTypePatch{Name: &name})
	requireCode(t, err, errs.PermissionDenied, "global_event_type_read_only")

	err = f.svc.DeleteType(owner, g.ID)
	requireCode(t, err, errs.PermissionDenied, "global_event_type_read_only")

	got, err := f.svc.GetType(owner, g.ID)
	require.NoError(t, err)
	require.Equal(t, "User Login", got.Name)
	require.Len(t, f.ob.published, before, "refused writes publish nothing")
}

func TestUpdateForeignIsNotFound(t *testing.T) {
	f := newFixture(t)
	theirs := f.seedTenant(t, tenantB, "Theirs", "theirs")
	name := "x"
	_, err := f.svc.UpdateType(admin(tenantA), theirs.ID, domain.EventTypePatch{Name: &name})
	requireCode(t, err, errs.NotFound, "event_type_not_found")
	err = f.svc.DeleteType(admin(tenantA), theirs.ID)
	requireCode(t, err, errs.NotFound, "event_type_not_found")
}

func TestUpdateCategoryAssignAndClear(t *testing.T) {
	f := newFixture(t)
	cat, err := f.svc.PlatformCreateCategory(platform(), CreateCategoryCmd{Name: "Commerce"})
	require.NoError(t, err)
	et := f.seedTenant(t, tenantA, "Buy", "buy")

	got, err := f.svc.UpdateType(member(tenantA), et.ID, domain.EventTypePatch{CategoryID: &cat.ID})
	require.NoError(t, err)
	require.Equal(t, cat.ID, got.CategoryID)

	missing := "0199a000-0000-7000-8000-0000000000ff"
	_, err = f.svc.UpdateType(member(tenantA), et.ID, domain.EventTypePatch{CategoryID: &missing})
	requireCode(t, err, errs.Invalid, "unknown_event_category")

	clear := ""
	got, err = f.svc.UpdateType(member(tenantA), et.ID, domain.EventTypePatch{CategoryID: &clear})
	require.NoError(t, err)
	require.Empty(t, got.CategoryID)
}

// --- delete ----------------------------------------------------------------------

func TestDeleteIsAdminOnlySoftAndPublishes(t *testing.T) {
	f := newFixture(t)
	et := f.seedTenant(t, tenantA, "Checkout", "checkout")

	err := f.svc.DeleteType(member(tenantA), et.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err), "program_manager holds no delete grant")

	require.NoError(t, f.svc.DeleteType(admin(tenantA), et.ID))
	require.Equal(t, []string{contracts.TopicTypeCreated, contracts.TopicTypeDeleted}, f.topics())
	require.Contains(t, f.repo.deleted, et.ID, "soft delete")

	_, err = f.svc.GetType(admin(tenantA), et.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err))

	err = f.svc.DeleteType(admin(tenantA), et.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	require.Len(t, f.ob.published, 2, "a second delete publishes nothing")

	// A soft-deleted slug is free again.
	_, err = f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Checkout", Slug: "checkout"})
	require.NoError(t, err)
}

// --- platform surface --------------------------------------------------------------

func TestPlatformManagesGlobalCatalogue(t *testing.T) {
	f := newFixture(t)
	g := f.seedGlobal(t, "Badge Earned", "badge_earned")
	require.True(t, g.IsGlobal())
	require.Equal(t, contracts.EventTypeChangedV1{
		EventTypeID: g.ID, Slug: "badge_earned", Active: true, At: t0,
	}, f.ob.published[0].payload, "global payload carries no tenant")

	name := "Badge Awarded"
	got, err := f.svc.PlatformUpdateType(platform(), g.ID, domain.EventTypePatch{Name: &name})
	require.NoError(t, err)
	require.Equal(t, "Badge Awarded", got.Name)

	// Platform scope never sees tenant rows.
	mine := f.seedTenant(t, tenantA, "Mine", "mine")
	_, err = f.svc.PlatformGetType(platform(), mine.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	err = f.svc.PlatformDeleteType(platform(), mine.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err))

	require.NoError(t, f.svc.PlatformDeleteType(platform(), g.ID))
	require.Equal(t, contracts.TopicTypeDeleted, f.ob.published[len(f.ob.published)-1].topic)
}

func TestPlatformSurfaceDeniedToTenantRoles(t *testing.T) {
	f := newFixture(t)
	g := f.seedGlobal(t, "User Login", "user_login")
	for _, ctx := range []context.Context{admin(tenantA), member(tenantA), as("", identitycontracts.RoleOwner)} {
		_, err := f.svc.PlatformListTypes(ctx, ListTypesQuery{})
		require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
		_, err = f.svc.PlatformGetType(ctx, g.ID)
		require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
		err = f.svc.PlatformDeleteType(ctx, g.ID)
		require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
		_, err = f.svc.PlatformCreateCategory(ctx, CreateCategoryCmd{Name: "x"})
		require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	}
	_, err := f.svc.PlatformListTypes(context.Background(), ListTypesQuery{})
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
}

func TestCategoriesListAndPlatformCRUD(t *testing.T) {
	f := newFixture(t)
	gamification, err := f.svc.PlatformCreateCategory(platform(), CreateCategoryCmd{Name: "Gamification", SortOrder: 50})
	require.NoError(t, err)
	_, err = f.svc.PlatformCreateCategory(platform(), CreateCategoryCmd{Name: "Account", SortOrder: 10})
	require.NoError(t, err)
	_, err = f.svc.PlatformCreateCategory(platform(), CreateCategoryCmd{Name: "Account again", Slug: "account"})
	requireCode(t, err, errs.AlreadyExists, "event_category_slug_taken")
	f.repo.categories["0199a000-0000-7000-8000-0000000000cb"] = domain.Category{
		ID: "0199a000-0000-7000-8000-0000000000cb", TenantID: tenantB, Slug: "foreign", Name: "Foreign",
	}

	cs, err := f.svc.ListCategories(member(tenantA))
	require.NoError(t, err)
	require.Len(t, cs, 2, "globals only; the other tenant's category is invisible")
	require.Equal(t, "account", cs[0].Slug, "ordered by sort_order")

	_, err = f.svc.ListCategories(as(tenantA))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	et, err := f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Lvl", CategoryID: gamification.ID})
	require.NoError(t, err)

	order := 5
	got, err := f.svc.PlatformUpdateCategory(platform(), gamification.ID, domain.CategoryPatch{SortOrder: &order})
	require.NoError(t, err)
	require.Equal(t, 5, got.SortOrder)
	require.Equal(t, "Gamification", got.Name)

	require.NoError(t, f.svc.PlatformDeleteCategory(platform(), gamification.ID))
	after, err := f.svc.GetType(member(tenantA), et.ID)
	require.NoError(t, err)
	require.Empty(t, after.CategoryID, "deleting a category uncategorises its types")

	err = f.svc.PlatformDeleteCategory(platform(), gamification.ID)
	requireCode(t, err, errs.NotFound, "event_category_not_found")
}

// --- reader ------------------------------------------------------------------------

func TestReaderResolvesShadowingInInputOrder(t *testing.T) {
	f := newFixture(t)
	gLogin := f.seedGlobal(t, "User Login", "user_login")
	gLevel := f.seedGlobal(t, "Level Up", "level_up")
	ownLevel, err := f.svc.CreateType(member(tenantA), CreateTypeCmd{
		Name: "Our Level Up", Slug: "level_up", PropertySchema: map[string]any{"type": "object"},
	})
	require.NoError(t, err)
	f.seedTenant(t, tenantB, "Foreign", "foreign")

	got, err := f.svc.EventTypesBySlugs(context.Background(), tenantA,
		[]string{"level_up", "foreign", "user_login", "level_up", "unknown"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, ownLevel.ID, got[0].ID, "tenant row shadows the global one")
	require.Equal(t, tenantA, got[0].TenantID)
	require.Equal(t, map[string]any{"type": "object"}, got[0].PropertySchema)
	require.Equal(t, gLogin.ID, got[1].ID)
	require.Empty(t, got[1].TenantID)

	got, err = f.svc.EventTypesBySlugs(context.Background(), tenantB, []string{"level_up"})
	require.NoError(t, err)
	require.Equal(t, gLevel.ID, got[0].ID, "tenant B still sees the global row")

	got, err = f.svc.EventTypesBySlugs(context.Background(), "", []string{"level_up", "foreign"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, gLevel.ID, got[0].ID)

	got, err = f.svc.EventTypesBySlugs(context.Background(), tenantA, nil)
	require.NoError(t, err)
	require.Empty(t, got)

	_, err = f.svc.EventTypesBySlugs(context.Background(), "not-a-uuid", []string{"x"})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestReaderReturnsInactiveTypesFlagged(t *testing.T) {
	f := newFixture(t)
	off := false
	_, err := f.svc.CreateType(member(tenantA), CreateTypeCmd{Name: "Off", Active: &off})
	require.NoError(t, err)
	got, err := f.svc.EventTypesBySlugs(context.Background(), tenantA, []string{"off"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.False(t, got[0].Active)
}

// --- tenant.deleted.v1 ---------------------------------------------------------------

func TestPurgeTenantIsIdempotentAndSparesOthers(t *testing.T) {
	f := newFixture(t)
	g := f.seedGlobal(t, "User Login", "user_login")
	f.seedTenant(t, tenantA, "Mine", "mine")
	gone := f.seedTenant(t, tenantA, "Gone", "gone")
	require.NoError(t, f.svc.DeleteType(admin(tenantA), gone.ID))
	f.repo.categories["0199a000-0000-7000-8000-0000000000ca"] = domain.Category{
		ID: "0199a000-0000-7000-8000-0000000000ca", TenantID: tenantA, Slug: "mine", Name: "Mine",
	}
	theirs := f.seedTenant(t, tenantB, "Theirs", "theirs")
	published := len(f.ob.published)

	require.NoError(t, f.svc.PurgeTenant(context.Background(), tenantA))
	require.Zero(t, f.repo.countTenantRows(tenantA), "live and soft-deleted rows and categories purged")

	// Redelivery: nothing left to delete, no error, still no publishes.
	require.NoError(t, f.svc.PurgeTenant(context.Background(), tenantA))
	require.Len(t, f.ob.published, published, "a purge publishes nothing")

	_, err := f.svc.GetType(member(tenantB), theirs.ID)
	require.NoError(t, err)
	_, err = f.svc.PlatformGetType(platform(), g.ID)
	require.NoError(t, err, "global rows survive")

	err = f.svc.PurgeTenant(context.Background(), "")
	require.Equal(t, errs.Invalid, errs.KindOf(err), "malformed payload parks immediately")
}
