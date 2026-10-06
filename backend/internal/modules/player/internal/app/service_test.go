package app_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/player/contracts"
	"levelup/internal/modules/player/internal/app"
	"levelup/internal/modules/player/internal/app/apptest"
	"levelup/internal/modules/player/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

const (
	tenantA = "0198d000-0000-7000-8000-00000000000a"
	tenantB = "0198d000-0000-7000-8000-00000000000b"
	userA   = "0198d000-0000-7000-8000-0000000000a1"
)

var allPerms = apptest.AllowKeys{
	contracts.PermViewAny.Key(): true,
	contracts.PermView.Key():    true,
	contracts.PermCreate.Key():  true,
	contracts.PermUpdate.Key():  true,
	contracts.PermDelete.Key():  true,
}

// memberPerms is what MemberRoles hold: everything except delete.
var memberPerms = apptest.AllowKeys{
	contracts.PermViewAny.Key(): true,
	contracts.PermView.Key():    true,
	contracts.PermCreate.Key():  true,
	contracts.PermUpdate.Key():  true,
}

type fixture struct {
	svc   *app.Service
	repo  *apptest.Repo
	ob    *apptest.Outbox
	clock *clock.Fake
}

func newFixture(t *testing.T, enf authz.Enforcer) *fixture {
	t.Helper()
	state := &apptest.TxState{}
	repo := apptest.NewRepo(state)
	ob := &apptest.Outbox{Tx: state}
	c := clock.NewFake(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC))
	svc := app.NewService(repo, ob, enf, nil, c, 2).WithTxRunner(apptest.Runner(state))
	return &fixture{svc: svc, repo: repo, ob: ob, clock: c}
}

func asTenant(tenantID string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: userA, TenantID: tenantID, RoleIDs: []int64{4}})
}

func (f *fixture) create(t *testing.T, ctx context.Context, ext string) domain.Player {
	t.Helper()
	p, err := f.svc.Create(ctx, app.CreateCmd{ExternalID: ext, DisplayName: "Player " + ext, Email: ext + "@example.com",
		Attributes: map[string]any{"tier": "gold"}})
	require.NoError(t, err)
	f.clock.Advance(time.Second)
	return p
}

func TestCreatePublishesInSameTx(t *testing.T) {
	f := newFixture(t, allPerms)
	p, err := f.svc.Create(asTenant(tenantA), app.CreateCmd{ExternalID: " ext-1 ", DisplayName: "Neo", Email: "NEO@x.io"})
	require.NoError(t, err)

	require.Equal(t, tenantA, p.TenantID, "tenant comes from the principal")
	require.Equal(t, userA, p.CreatedBy, "created_by comes from the principal")
	require.Equal(t, "ext-1", p.ExternalID)
	require.Equal(t, "neo@x.io", p.Email)
	require.True(t, p.Active)
	require.Len(t, f.repo.Rows, 1)

	require.Len(t, f.ob.Published, 1)
	ev := f.ob.Published[0]
	require.Equal(t, contracts.TopicPlayerCreated, ev.Topic)
	require.True(t, ev.InTx, "player.created.v1 must ride the creating transaction")
	require.Equal(t, contracts.PlayerCreatedV1{
		PlayerID: p.ID, TenantID: tenantA, ExternalID: "ext-1", DisplayName: "Neo", At: p.CreatedAt,
	}, ev.Payload)

	require.Len(t, f.repo.Evictions, 1, "create evicts any tombstone left by an earlier miss")
	require.False(t, f.repo.Evictions[0].InTx, "eviction happens after commit (R44)")
}

func TestCreateDuplicateExternalIDIsConflictWithCode(t *testing.T) {
	f := newFixture(t, allPerms)
	f.create(t, asTenant(tenantA), "dup")

	_, err := f.svc.Create(asTenant(tenantA), app.CreateCmd{ExternalID: "dup"})
	require.Equal(t, errs.AlreadyExists, errs.KindOf(err))
	require.Equal(t, contracts.CodeExternalIDTaken, errs.CodeOf(err))
	require.Len(t, f.ob.Published, 1, "a rejected create publishes nothing")
}

func TestSameExternalIDInAnotherTenantIsFine(t *testing.T) {
	f := newFixture(t, allPerms)
	f.create(t, asTenant(tenantA), "shared")
	f.create(t, asTenant(tenantB), "shared")
	require.Len(t, f.repo.Rows, 2)
}

func TestRecreateAfterSoftDelete(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	first := f.create(t, ctx, "phoenix")
	require.NoError(t, f.svc.Delete(ctx, first.ID))

	again, err := f.svc.Create(ctx, app.CreateCmd{ExternalID: "phoenix"})
	require.NoError(t, err, "a deleted player's external id is reusable (Laravel returned 500)")
	require.NotEqual(t, first.ID, again.ID)

	got, err := f.svc.GetByExternalID(ctx, "phoenix")
	require.NoError(t, err)
	require.Equal(t, again.ID, got.ID, "lookups see only the live player")
}

func TestCreateValidationAndAuthz(t *testing.T) {
	t.Run("invalid email", func(t *testing.T) {
		f := newFixture(t, allPerms)
		_, err := f.svc.Create(asTenant(tenantA), app.CreateCmd{ExternalID: "e", Email: "nope"})
		require.Equal(t, errs.Invalid, errs.KindOf(err))
		require.Empty(t, f.ob.Published)
	})
	t.Run("missing permission", func(t *testing.T) {
		f := newFixture(t, apptest.AllowKeys{})
		_, err := f.svc.Create(asTenant(tenantA), app.CreateCmd{ExternalID: "e"})
		require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
		require.Empty(t, f.repo.Rows)
	})
	t.Run("principal without tenant", func(t *testing.T) {
		f := newFixture(t, allPerms)
		ctx := authz.Into(context.Background(), authz.Principal{UserID: userA, RoleIDs: []int64{1}})
		_, err := f.svc.Create(ctx, app.CreateCmd{ExternalID: "e"})
		require.ErrorIs(t, err, authz.ErrNoTenant)
	})
	t.Run("anonymous", func(t *testing.T) {
		f := newFixture(t, allPerms)
		_, err := f.svc.Create(context.Background(), app.CreateCmd{ExternalID: "e"})
		require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
	})
}

func TestCrossTenantAccessIsNotFound(t *testing.T) {
	f := newFixture(t, allPerms)
	theirs := f.create(t, asTenant(tenantB), "theirs")
	ctx := asTenant(tenantA)
	published := len(f.ob.Published)

	assertNotFound := func(t *testing.T, err error) {
		t.Helper()
		require.Equal(t, errs.NotFound, errs.KindOf(err))
		require.Equal(t, contracts.CodePlayerNotFound, errs.CodeOf(err))
	}

	_, err := f.svc.Get(ctx, theirs.ID)
	assertNotFound(t, err)
	_, err = f.svc.GetByExternalID(ctx, "theirs")
	assertNotFound(t, err)
	_, err = f.svc.Update(ctx, theirs.ID, domain.Patch{DisplayName: new("hijack")})
	assertNotFound(t, err)
	_, err = f.svc.Deactivate(ctx, theirs.ID)
	assertNotFound(t, err)
	assertNotFound(t, f.svc.Delete(ctx, theirs.ID))

	page, err := f.svc.List(ctx, app.ListQuery{})
	require.NoError(t, err)
	require.Empty(t, page.Players)

	snaps, err := f.svc.PlayersByIDs(context.Background(), tenantA, []string{theirs.ID})
	require.NoError(t, err)
	require.Empty(t, snaps, "the Reader filters by the tenant it is given")

	require.Len(t, f.ob.Published, published, "nothing published for foreign rows")
	require.Equal(t, "Player theirs", f.repo.Rows[theirs.ID].DisplayName)
}

func TestMalformedIDIsNotFound(t *testing.T) {
	f := newFixture(t, allPerms)
	_, err := f.svc.Get(asTenant(tenantA), "42")
	require.Equal(t, errs.NotFound, errs.KindOf(err), "a non-uuid id never reaches Postgres as a cast error")
}

func TestPartialUpdateLeavesOmittedFieldsUntouched(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	p := f.create(t, ctx, "ext")

	got, err := f.svc.Update(ctx, p.ID, domain.Patch{Email: new("New@Example.com")})
	require.NoError(t, err)
	require.Equal(t, "new@example.com", got.Email)
	require.Equal(t, "Player ext", got.DisplayName, "omitted display_name untouched (Laravel PUT nulled it)")
	require.Equal(t, map[string]any{"tier": "gold"}, got.Attributes, "omitted attributes untouched")
	require.True(t, got.Active)
	require.Equal(t, p.Version+1, got.Version)

	stored := f.repo.Rows[p.ID]
	require.Equal(t, "Player ext", stored.DisplayName)
	require.Equal(t, "new@example.com", stored.Email)

	require.Equal(t, []string{contracts.TopicPlayerCreated, contracts.TopicPlayerUpdated}, f.ob.Topics())
	ev := f.ob.Published[1]
	require.True(t, ev.InTx)
	payload := ev.Payload.(contracts.PlayerUpdatedV1)
	require.Equal(t, []string{domain.FieldEmail}, payload.ChangedFields)
	require.True(t, payload.Active)
}

func TestUpdateWithoutChangesWritesAndPublishesNothing(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	p := f.create(t, ctx, "ext")
	evictions := len(f.repo.Evictions)

	got, err := f.svc.Update(ctx, p.ID, domain.Patch{DisplayName: new("Player ext")})
	require.NoError(t, err)
	require.Equal(t, p.Version, got.Version)
	require.Equal(t, []string{contracts.TopicPlayerCreated}, f.ob.Topics(), "no-op update fires no event (Laravel always fired)")
	require.Len(t, f.repo.Evictions, evictions)
}

func TestUpdateAuthzDenied(t *testing.T) {
	f := newFixture(t, apptest.AllowKeys{contracts.PermCreate.Key(): true})
	ctx := asTenant(tenantA)
	p := f.create(t, ctx, "ext")
	_, err := f.svc.Update(ctx, p.ID, domain.Patch{DisplayName: new("x")})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Equal(t, "Player ext", f.repo.Rows[p.ID].DisplayName)
}

func TestUpdateInvalidPatchRollsBack(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	p := f.create(t, ctx, "ext")
	_, err := f.svc.Update(ctx, p.ID, domain.Patch{Email: new("broken")})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	require.Equal(t, []string{contracts.TopicPlayerCreated}, f.ob.Topics())
}

func TestPatchIsActivePublishesStatusTopic(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	p := f.create(t, ctx, "ext")

	_, err := f.svc.Update(ctx, p.ID, domain.Patch{Active: new(false), DisplayName: new("Renamed")})
	require.NoError(t, err)
	require.Equal(t, []string{
		contracts.TopicPlayerCreated, contracts.TopicPlayerUpdated, contracts.TopicPlayerDeactivated,
	}, f.ob.Topics())
	for _, ev := range f.ob.Published {
		require.True(t, ev.InTx)
	}
}

func TestActivateDeactivate(t *testing.T) {
	f := newFixture(t, memberPerms)
	ctx := asTenant(tenantA)
	p := f.create(t, ctx, "ext")

	got, err := f.svc.Deactivate(ctx, p.ID)
	require.NoError(t, err)
	require.False(t, got.Active)
	require.False(t, f.repo.Rows[p.ID].Active)

	_, err = f.svc.Deactivate(ctx, p.ID)
	require.NoError(t, err, "deactivating an inactive player is a no-op, not an error")

	got, err = f.svc.Activate(ctx, p.ID)
	require.NoError(t, err)
	require.True(t, got.Active)

	require.Equal(t, []string{
		contracts.TopicPlayerCreated, contracts.TopicPlayerDeactivated, contracts.TopicPlayerActivated,
	}, f.ob.Topics(), "status-only changes publish only the status topic, once per real flip")
	deactivated := f.ob.Published[1]
	require.True(t, deactivated.InTx)
	require.Equal(t, contracts.PlayerStatusV1{PlayerID: p.ID, TenantID: tenantA, Active: false, At: f.clock.Now()},
		deactivated.Payload)
	for _, e := range f.repo.Evictions {
		require.False(t, e.InTx)
	}
}

func TestDeleteIsAdminOnly(t *testing.T) {
	f := newFixture(t, memberPerms)
	ctx := asTenant(tenantA)
	p := f.create(t, ctx, "ext")

	err := f.svc.Delete(ctx, p.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Nil(t, f.repo.Rows[p.ID].DeletedAt)
	require.Equal(t, []string{contracts.TopicPlayerCreated}, f.ob.Topics())
}

func TestDeleteSoftDeletesAndPublishesInTx(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	p := f.create(t, ctx, "ext")

	require.NoError(t, f.svc.Delete(ctx, p.ID))
	require.NotNil(t, f.repo.Rows[p.ID].DeletedAt, "soft delete keeps the row")

	ev := f.ob.Published[len(f.ob.Published)-1]
	require.Equal(t, contracts.TopicPlayerDeleted, ev.Topic)
	require.True(t, ev.InTx)
	require.Equal(t, contracts.PlayerDeletedV1{PlayerID: p.ID, TenantID: tenantA, ExternalID: "ext", At: f.clock.Now()}, ev.Payload)

	_, err := f.svc.Get(ctx, p.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	require.Equal(t, errs.NotFound, errs.KindOf(f.svc.Delete(ctx, p.ID)), "deleting twice is 404")

	last := f.repo.Evictions[len(f.repo.Evictions)-1]
	require.False(t, last.InTx)
	require.Equal(t, []app.Ref{{ID: p.ID, ExternalID: "ext"}}, last.Refs)
}

func TestPublishFailureFailsTheWrite(t *testing.T) {
	f := newFixture(t, allPerms)
	f.ob.Fail = errs.New(errs.Unavailable, "outbox down")
	_, err := f.svc.Create(asTenant(tenantA), app.CreateCmd{ExternalID: "e"})
	require.Equal(t, errs.Unavailable, errs.KindOf(err), "the tx runner sees the publish error and rolls back")
	require.Empty(t, f.repo.Evictions)
}

func TestListPaginatesAndFilters(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	var created []domain.Player
	for i := range 5 {
		created = append(created, f.create(t, ctx, fmt.Sprintf("p-%d", i)))
	}
	_, err := f.svc.Deactivate(ctx, created[1].ID)
	require.NoError(t, err)

	page1, err := f.svc.List(ctx, app.ListQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1.Players, 2)
	require.Equal(t, created[4].ID, page1.Players[0].ID, "newest first")
	require.NotEmpty(t, page1.NextCursor)

	page2, err := f.svc.List(ctx, app.ListQuery{Limit: 2, Cursor: page1.NextCursor})
	require.NoError(t, err)
	require.Equal(t, []string{created[2].ID, created[1].ID}, ids(page2.Players))

	page3, err := f.svc.List(ctx, app.ListQuery{Limit: 2, Cursor: page2.NextCursor})
	require.NoError(t, err)
	require.Equal(t, []string{created[0].ID}, ids(page3.Players))
	require.Empty(t, page3.NextCursor, "last page carries no cursor")

	inactive, err := f.svc.List(ctx, app.ListQuery{Active: new(false)})
	require.NoError(t, err)
	require.Equal(t, []string{created[1].ID}, ids(inactive.Players))

	search, err := f.svc.List(ctx, app.ListQuery{Search: "P-3"})
	require.NoError(t, err)
	require.Equal(t, []string{created[3].ID}, ids(search.Players))

	_, err = f.svc.List(ctx, app.ListQuery{Cursor: "garbage"})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestListClampsLimitAndNeedsViewAny(t *testing.T) {
	f := newFixture(t, apptest.AllowKeys{contracts.PermView.Key(): true})
	_, err := f.svc.List(asTenant(tenantA), app.ListQuery{Limit: 1000})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestPurgeTenantIsIdempotentAndScoped(t *testing.T) {
	f := newFixture(t, allPerms) // purge batch size 2 → several batches
	for i := range 5 {
		f.create(t, asTenant(tenantA), fmt.Sprintf("a-%d", i))
	}
	deleted := f.create(t, asTenant(tenantA), "gone")
	require.NoError(t, f.svc.Delete(asTenant(tenantA), deleted.ID))
	survivor := f.create(t, asTenant(tenantB), "b-0")
	published := len(f.ob.Published)

	n, err := f.svc.PurgeTenant(context.Background(), tenantA)
	require.NoError(t, err)
	require.Equal(t, 6, n, "live and soft-deleted rows alike")
	require.Len(t, f.repo.Rows, 1)
	require.Contains(t, f.repo.Rows, survivor.ID, "other tenants untouched")

	n, err = f.svc.PurgeTenant(context.Background(), tenantA)
	require.NoError(t, err)
	require.Zero(t, n, "redelivery finds nothing to do")
	require.Len(t, f.ob.Published, published, "a purge publishes nothing; peers purge on the same event")
	for _, e := range f.repo.Evictions {
		require.False(t, e.InTx)
	}
}

func TestPurgeTenantRejectsBadTenantID(t *testing.T) {
	f := newFixture(t, allPerms)
	_, err := f.svc.PurgeTenant(context.Background(), "")
	require.Equal(t, errs.Invalid, errs.KindOf(err), "malformed payloads dead-letter at once")
}

func TestReaderBatches(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := asTenant(tenantA)
	a := f.create(t, ctx, "a")
	b := f.create(t, ctx, "b")
	gone := f.create(t, ctx, "gone")
	require.NoError(t, f.svc.Delete(ctx, gone.ID))

	snaps, err := f.svc.PlayersByIDs(context.Background(), tenantA,
		[]string{a.ID, b.ID, b.ID, gone.ID, "0198d000-0000-7000-8000-0000000000ff", "not-a-uuid"})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{a.ID, b.ID}, snapIDs(snaps), "unknown, deleted and malformed ids are simply absent")

	byExt, err := f.svc.PlayersByExternalIDs(context.Background(), tenantA, []string{"a", " b ", "gone", "nobody", ""})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{a.ID, b.ID}, snapIDs(byExt))
	for _, s := range byExt {
		require.Equal(t, tenantA, s.TenantID)
		require.Equal(t, map[string]any{"tier": "gold"}, s.Attributes)
		require.True(t, s.Active)
	}

	empty, err := f.svc.PlayersByIDs(context.Background(), tenantA, nil)
	require.NoError(t, err)
	require.Empty(t, empty)

	_, err = f.svc.PlayersByIDs(context.Background(), "", []string{a.ID})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestReaderSatisfiesContract(t *testing.T) {
	var _ contracts.Reader = (*app.Service)(nil)
}

func ids(ps []domain.Player) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func snapIDs(ps []contracts.PlayerSnapshot) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func TestListPlayerIDsPagesAndValidates(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := context.Background()
	var ids []string
	for _, ext := range []string{"a", "b", "c"} {
		p, err := f.svc.Create(asTenant(tenantA), app.CreateCmd{ExternalID: ext})
		require.NoError(t, err)
		ids = append(ids, p.ID)
	}
	slices.Sort(ids)

	page, err := f.svc.ListPlayerIDs(ctx, tenantA, "", 2)
	require.NoError(t, err)
	require.Equal(t, ids[:2], page)
	page, err = f.svc.ListPlayerIDs(ctx, tenantA, page[1], 2)
	require.NoError(t, err)
	require.Equal(t, ids[2:], page)

	_, err = f.svc.ListPlayerIDs(ctx, "", "", 10)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	_, err = f.svc.ListPlayerIDs(ctx, tenantA, "not-a-uuid", 10)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}
