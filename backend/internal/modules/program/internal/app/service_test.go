package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/program/contracts"
	"levelup/internal/modules/program/internal/domain"
	"levelup/internal/modules/program/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

const (
	tenantA = "0198d000-0000-7000-8000-00000000000a"
	tenantB = "0198d000-0000-7000-8000-00000000000b"
	player1 = "0198d000-0000-7000-8000-000000000001"
	player2 = "0198d000-0000-7000-8000-000000000002"
	player3 = "0198d000-0000-7000-8000-000000000003"
)

var start = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func allPerms() allowKeys {
	out := allowKeys{}
	for _, p := range contracts.AllPermissions {
		out[p.Key()] = true
	}
	return out
}

type harness struct {
	svc     *Service
	repo    *fakeRepo
	ob      *fakeOutbox
	players *fakePlayers
	clock   *clock.Fake
}

func newHarness(t *testing.T, enf authz.Enforcer) *harness {
	t.Helper()
	h := &harness{
		repo: newFakeRepo(),
		ob:   &fakeOutbox{},
		players: &fakePlayers{players: map[string]ports.PlayerSnapshot{
			player1: {ID: player1, TenantID: tenantA, ExternalID: "ext-1", DisplayName: "One", Active: true},
			player2: {ID: player2, TenantID: tenantA, ExternalID: "ext-2", DisplayName: "Two", Active: false},
			player3: {ID: player3, TenantID: tenantB, ExternalID: "ext-3", DisplayName: "Three", Active: true},
		}},
		clock: clock.NewFake(start),
	}
	h.svc = NewService(h.repo, h.players, h.ob, enf, nil, h.clock, 2)
	h.svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
	return h
}

func ctxFor(tenantID string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "u-" + tenantID, TenantID: tenantID, RoleIDs: []int64{4}})
}

func (h *harness) create(t *testing.T, ctx context.Context, name string) domain.Program {
	t.Helper()
	p, err := h.svc.Create(ctx, CreateCmd{Name: name})
	require.NoError(t, err)
	return p
}

func (h *harness) active(t *testing.T, ctx context.Context, name string) domain.Program {
	t.Helper()
	p := h.create(t, ctx, name)
	p, err := h.svc.Activate(ctx, p.ID)
	require.NoError(t, err)
	return p
}

func requireCode(t *testing.T, err error, kind errs.Kind, code string) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, kind, errs.KindOf(err), "kind of %v", err)
	require.Equal(t, code, errs.CodeOf(err), "code of %v", err)
}

// ---- create / read -----------------------------------------------------------

func TestCreatePublishesAndStampsTenant(t *testing.T) {
	h := newHarness(t, allPerms())
	ends := start.Add(24 * time.Hour)
	p, err := h.svc.Create(ctxFor(tenantA), CreateCmd{
		Name: "Loyalty Rewards", EndsAt: &ends, Mechanics: map[string]any{"points_enabled": true},
	})
	require.NoError(t, err)
	require.Equal(t, tenantA, p.TenantID, "tenant comes from the principal")
	require.Equal(t, domain.StatusDraft, p.Status)
	require.Equal(t, "loyalty-rewards", p.Slug)

	require.Equal(t, []string{contracts.TopicProgramCreated}, h.ob.topics())
	ev := h.ob.published[0].payload.(contracts.ProgramChangedV1)
	require.Equal(t, p.ID, ev.ProgramID)
	require.Equal(t, tenantA, ev.TenantID)
	require.Equal(t, "draft", ev.Status)
	require.Equal(t, ends, *ev.EndsAt)
}

func TestCreateDeniedWithoutPermission(t *testing.T) {
	h := newHarness(t, allowKeys{})
	_, err := h.svc.Create(ctxFor(tenantA), CreateCmd{Name: "x"})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Empty(t, h.ob.published)
}

func TestTenantlessPrincipalIsRefused(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := authz.Into(context.Background(), authz.Principal{UserID: "platform", RoleIDs: []int64{1}})
	_, err := h.svc.Create(ctx, CreateCmd{Name: "x"})
	require.ErrorIs(t, err, authz.ErrNoTenant)

	_, err = h.svc.Get(context.Background(), "whatever")
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
}

func TestSlugUniquePerTenant(t *testing.T) {
	h := newHarness(t, allPerms())
	h.create(t, ctxFor(tenantA), "Welcome")

	_, err := h.svc.Create(ctxFor(tenantA), CreateCmd{Name: "Welcome"})
	requireCode(t, err, errs.AlreadyExists, "slug_taken")

	_, err = h.svc.Create(ctxFor(tenantB), CreateCmd{Name: "Welcome"})
	require.NoError(t, err, "another tenant may reuse the slug")
}

func TestGetCrossTenantIsNotFound(t *testing.T) {
	h := newHarness(t, allPerms())
	p := h.create(t, ctxFor(tenantA), "A")

	_, err := h.svc.Get(ctxFor(tenantB), p.ID)
	requireCode(t, err, errs.NotFound, "program_not_found")

	got, err := h.svc.Get(ctxFor(tenantA), p.ID)
	require.NoError(t, err)
	require.Equal(t, p.ID, got.ID)
}

func TestListFiltersAndPages(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	var ids []string
	for _, n := range []string{"p1", "p2", "p3"} {
		ids = append(ids, h.create(t, ctx, n).ID)
		h.clock.Advance(time.Second)
	}
	_, err := h.svc.Activate(ctx, ids[1])
	require.NoError(t, err)
	h.create(t, ctxFor(tenantB), "foreign")

	page1, next, err := h.svc.List(ctx, "", "", "", 2)
	require.NoError(t, err)
	require.Equal(t, []string{ids[2], ids[1]}, programIDs(page1), "newest first")
	require.NotEmpty(t, next)

	page2, next, err := h.svc.List(ctx, "", "", next, 2)
	require.NoError(t, err)
	require.Equal(t, []string{ids[0]}, programIDs(page2))
	require.Empty(t, next, "empty cursor when done")

	active, _, err := h.svc.List(ctx, "active", "", "", 0)
	require.NoError(t, err)
	require.Equal(t, []string{ids[1]}, programIDs(active))

	_, _, err = h.svc.List(ctx, "archived", "", "", 0)
	requireCode(t, err, errs.Invalid, "invalid_status")

	_, _, err = h.svc.List(ctx, "", "", "%%%", 0)
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	_, _, err = newHarness(t, allowKeys{}).svc.List(ctx, "", "", "", 0)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func programIDs(ps []domain.Program) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

// ---- update ------------------------------------------------------------------

func TestUpdateIsPartialAndPublishesChangedFields(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	desc := "keep me"
	p, err := h.svc.Create(ctx, CreateCmd{Name: "Old", Description: &desc, Settings: map[string]any{"welcome_points": float64(10)}})
	require.NoError(t, err)
	p, err = h.svc.Activate(ctx, p.ID)
	require.NoError(t, err)
	h.ob.published = nil

	name := "New"
	got, err := h.svc.Update(ctx, p.ID, domain.Patch{Name: &name})
	require.NoError(t, err)
	require.Equal(t, "New", got.Name)
	require.Equal(t, "keep me", *got.Description, "omitted fields are not nulled (Laravel PUT bug)")
	require.Equal(t, float64(10), got.Settings["welcome_points"])
	require.Equal(t, domain.StatusActive, got.Status, "update cannot move status")
	require.Equal(t, p.Version+1, got.Version)

	require.Equal(t, []string{contracts.TopicProgramUpdated}, h.ob.topics())
	ev := h.ob.published[0].payload.(contracts.ProgramChangedV1)
	require.Equal(t, []string{"name"}, ev.ChangedFields)

	stored, err := h.svc.Get(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, got, stored)
}

func TestUpdateWithoutChangesPublishesNothing(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	p := h.create(t, ctx, "Same")
	h.ob.published = nil

	got, err := h.svc.Update(ctx, p.ID, domain.Patch{Name: &p.Name})
	require.NoError(t, err)
	require.Equal(t, p, got)
	require.Empty(t, h.ob.published)
}

func TestUpdateRejectsCrossTenantDenialAndInvalid(t *testing.T) {
	h := newHarness(t, allPerms())
	p := h.create(t, ctxFor(tenantA), "A")
	h.ob.published = nil
	name := "B"

	_, err := h.svc.Update(ctxFor(tenantB), p.ID, domain.Patch{Name: &name})
	requireCode(t, err, errs.NotFound, "program_not_found")

	deny := newHarness(t, allowKeys{contracts.PermCreate.Key(): true})
	q := deny.create(t, ctxFor(tenantA), "A")
	_, err = deny.svc.Update(ctxFor(tenantA), q.ID, domain.Patch{Name: &name})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	before := start.Add(-time.Hour)
	_, err = h.svc.Update(ctxFor(tenantA), p.ID, domain.Patch{
		StartsAt: domain.Nullable[time.Time]{Set: true, Value: &start},
		EndsAt:   domain.Nullable[time.Time]{Set: true, Value: &before},
	})
	requireCode(t, err, errs.Invalid, "invalid_program_window")
	require.Empty(t, h.ob.published)
}

func TestUpdateSlugCollision(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	h.create(t, ctx, "Taken")
	p := h.create(t, ctx, "Other")
	slug := "taken"
	_, err := h.svc.Update(ctx, p.ID, domain.Patch{Slug: &slug})
	requireCode(t, err, errs.AlreadyExists, "slug_taken")
}

// ---- transitions -------------------------------------------------------------

func TestTransitionsPublishCanonicalTopics(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	p := h.create(t, ctx, "Lifecycle")
	h.ob.published = nil

	steps := []struct {
		do    func(context.Context, string) (domain.Program, error)
		topic string
		from  string
		to    domain.Status
	}{
		{h.svc.Activate, contracts.TopicProgramActivated, "draft", domain.StatusActive},
		{h.svc.Pause, contracts.TopicProgramPaused, "active", domain.StatusPaused},
		{h.svc.Activate, contracts.TopicProgramActivated, "paused", domain.StatusActive},
		{h.svc.End, contracts.TopicProgramEnded, "active", domain.StatusEnded},
	}
	for i, s := range steps {
		got, err := s.do(ctx, p.ID)
		require.NoError(t, err)
		require.Equal(t, s.to, got.Status)
		ev := h.ob.published[i].payload.(contracts.ProgramChangedV1)
		require.Equal(t, s.topic, h.ob.published[i].topic)
		require.Equal(t, s.from, ev.PreviousStatus)
		require.Equal(t, string(s.to), ev.Status)
	}
}

func TestInvalidTransitionsAreConflicts(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)

	draft := h.create(t, ctx, "draft")
	ended := h.active(t, ctx, "ended")
	_, err := h.svc.End(ctx, ended.ID)
	require.NoError(t, err)
	h.ob.published = nil

	cases := []struct {
		name string
		do   func(context.Context, string) (domain.Program, error)
		id   string
	}{
		{"pause draft", h.svc.Pause, draft.ID},
		{"end draft", h.svc.End, draft.ID},
		{"activate ended", h.svc.Activate, ended.ID},
		{"pause ended", h.svc.Pause, ended.ID},
		{"end ended (terminal)", h.svc.End, ended.ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.do(ctx, tc.id)
			requireCode(t, err, errs.Conflict, "invalid_status_transition")
		})
	}
	require.Empty(t, h.ob.published, "a refused transition publishes nothing")
}

func TestActivateRefusesElapsedWindow(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	ends := start.Add(time.Hour)
	p, err := h.svc.Create(ctx, CreateCmd{Name: "x", EndsAt: &ends})
	require.NoError(t, err)
	h.clock.Advance(2 * time.Hour)

	_, err = h.svc.Activate(ctx, p.ID)
	requireCode(t, err, errs.Conflict, "program_window_elapsed")
}

func TestTransitionsNeedAdminPermissions(t *testing.T) {
	members := allPerms()
	for _, p := range []authz.Permission{contracts.PermActivate, contracts.PermPause, contracts.PermEnd, contracts.PermDelete} {
		delete(members, p.Key())
	}
	h := newHarness(t, members)
	ctx := ctxFor(tenantA)
	p := h.create(t, ctx, "x")

	for _, do := range []func(context.Context, string) (domain.Program, error){h.svc.Activate, h.svc.Pause, h.svc.End} {
		_, err := do(ctx, p.ID)
		require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	}
	require.Equal(t, errs.PermissionDenied, errs.KindOf(h.svc.Delete(ctx, p.ID)))
}

func TestTransitionCrossTenantIsNotFound(t *testing.T) {
	h := newHarness(t, allPerms())
	p := h.create(t, ctxFor(tenantA), "x")
	_, err := h.svc.Activate(ctxFor(tenantB), p.ID)
	requireCode(t, err, errs.NotFound, "program_not_found")
}

// ---- delete ------------------------------------------------------------------

func TestDeleteSoftDeletesAndFreesSlug(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	p := h.active(t, ctx, "Gone")
	h.ob.published = nil

	require.Equal(t, errs.NotFound, errs.KindOf(h.svc.Delete(ctxFor(tenantB), p.ID)))
	require.NoError(t, h.svc.Delete(ctx, p.ID))
	require.Equal(t, []string{contracts.TopicProgramDeleted}, h.ob.topics())

	_, err := h.svc.Get(ctx, p.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	require.Equal(t, errs.NotFound, errs.KindOf(h.svc.Delete(ctx, p.ID)), "second delete is 404")

	_, err = h.svc.Create(ctx, CreateCmd{Name: "Gone"})
	require.NoError(t, err, "a deleted program's slug is reusable")
}

// ---- enrolment ---------------------------------------------------------------

func TestEnrollIsIdempotent(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	p := h.active(t, ctx, "x")
	h.ob.published = nil

	first, created, err := h.svc.Enroll(ctx, p.ID, player1)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, start, first.EnrolledAt)

	h.clock.Advance(time.Minute)
	again, created, err := h.svc.Enroll(ctx, p.ID, player1)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, first, again, "re-enrolling returns the existing enrolment")

	require.Len(t, h.repo.enrollments, 1)
	require.Equal(t, []string{contracts.TopicPlayerEnrolled}, h.ob.topics(), "one row, one event")
	ev := h.ob.published[0].payload.(contracts.EnrollmentChangedV1)
	require.Equal(t, contracts.EnrollmentChangedV1{ProgramID: p.ID, TenantID: tenantA, PlayerID: player1, At: start}, ev)
}

func TestEnrollRequiresActiveProgramInsideWindow(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)

	draft := h.create(t, ctx, "draft")
	_, _, err := h.svc.Enroll(ctx, draft.ID, player1)
	requireCode(t, err, errs.Conflict, "program_not_accepting_players")

	paused := h.active(t, ctx, "paused")
	_, err = h.svc.Pause(ctx, paused.ID)
	require.NoError(t, err)
	_, _, err = h.svc.Enroll(ctx, paused.ID, player1)
	requireCode(t, err, errs.Conflict, "program_not_accepting_players")

	later := start.Add(time.Hour)
	future, err := h.svc.Create(ctx, CreateCmd{Name: "future", StartsAt: &later})
	require.NoError(t, err)
	_, err = h.svc.Activate(ctx, future.ID)
	require.NoError(t, err)
	_, _, err = h.svc.Enroll(ctx, future.ID, player1)
	requireCode(t, err, errs.Conflict, "program_not_accepting_players")

	h.clock.Advance(time.Hour)
	_, created, err := h.svc.Enroll(ctx, future.ID, player1)
	require.NoError(t, err)
	require.True(t, created, "accepted once the window opens")
}

func TestEnrollPlayerChecks(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	p := h.active(t, ctx, "x")
	h.ob.published = nil

	_, _, err := h.svc.Enroll(ctx, p.ID, "0198d000-0000-7000-8000-0000000000ff")
	requireCode(t, err, errs.NotFound, "player_not_found")

	_, _, err = h.svc.Enroll(ctx, p.ID, player3)
	requireCode(t, err, errs.NotFound, "player_not_found") // another tenant's player

	_, _, err = h.svc.Enroll(ctx, p.ID, player2)
	requireCode(t, err, errs.Invalid, "player_inactive")

	h.players.err = errs.New(errs.Unavailable, "player service down")
	_, _, err = h.svc.Enroll(ctx, p.ID, player1)
	require.Equal(t, errs.Unavailable, errs.KindOf(err), "transient port failures are not masked")

	require.Empty(t, h.repo.enrollments)
	require.Empty(t, h.ob.published)
}

func TestEnrollCrossTenantAndDenied(t *testing.T) {
	h := newHarness(t, allPerms())
	p := h.active(t, ctxFor(tenantA), "x")

	_, _, err := h.svc.Enroll(ctxFor(tenantB), p.ID, player3)
	requireCode(t, err, errs.NotFound, "program_not_found")

	deny := newHarness(t, allowKeys{contracts.PermCreate.Key(): true, contracts.PermActivate.Key(): true})
	q := deny.active(t, ctxFor(tenantA), "x")
	_, _, err = deny.svc.Enroll(ctxFor(tenantA), q.ID, player1)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(deny.svc.Unenroll(ctxFor(tenantA), q.ID, player1)))
}

func TestUnenrollIsIdempotent(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	p := h.active(t, ctx, "x")
	_, _, err := h.svc.Enroll(ctx, p.ID, player1)
	require.NoError(t, err)
	h.ob.published = nil

	require.NoError(t, h.svc.Unenroll(ctx, p.ID, player1))
	require.NoError(t, h.svc.Unenroll(ctx, p.ID, player1))
	require.Empty(t, h.repo.enrollments)
	require.Equal(t, []string{contracts.TopicPlayerUnenrolled}, h.ob.topics(), "only the removing call publishes")

	require.Equal(t, errs.NotFound, errs.KindOf(h.svc.Unenroll(ctxFor(tenantB), p.ID, player1)))
}

func TestListMembersHydratesWithOneBatchRead(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	p := h.active(t, ctx, "x")
	_, _, err := h.svc.Enroll(ctx, p.ID, player1)
	require.NoError(t, err)
	h.clock.Advance(time.Second)
	// An enrolment whose player the player module no longer knows.
	ghost := "0198d000-0000-7000-8000-0000000000ee"
	h.repo.enrollments[[2]string{p.ID, ghost}] = domain.Enrollment{ProgramID: p.ID, TenantID: tenantA, PlayerID: ghost, EnrolledAt: h.clock.Now()}

	h.players.calls = 0
	page, next, err := h.svc.ListMembers(ctx, p.ID, "", 1)
	require.NoError(t, err)
	require.Equal(t, 1, h.players.calls)
	require.Len(t, page, 1)
	require.Equal(t, ghost, page[0].Enrollment.PlayerID)
	require.Nil(t, page[0].Player)
	require.NotEmpty(t, next)

	page, next, err = h.svc.ListMembers(ctx, p.ID, next, 1)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, "ext-1", page[0].Player.ExternalID)
	require.Empty(t, next)

	_, _, err = h.svc.ListMembers(ctxFor(tenantB), p.ID, "", 0)
	requireCode(t, err, errs.NotFound, "program_not_found")
}

// ---- subscriptions -------------------------------------------------------------

func TestRemovePlayerRedeliveryPublishesOnce(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	a := h.active(t, ctx, "a")
	b := h.active(t, ctx, "b")
	for _, id := range []string{a.ID, b.ID} {
		_, _, err := h.svc.Enroll(ctx, id, player1)
		require.NoError(t, err)
	}
	h.ob.published = nil

	require.NoError(t, h.svc.RemovePlayer(context.Background(), tenantA, player1))
	require.NoError(t, h.svc.RemovePlayer(context.Background(), tenantA, player1))

	require.Empty(t, h.repo.enrollments)
	require.Equal(t, []string{contracts.TopicPlayerUnenrolled, contracts.TopicPlayerUnenrolled}, h.ob.topics(),
		"one event per removed row, nothing on redelivery")
	got := map[string]bool{}
	for _, e := range h.ob.published {
		ev := e.payload.(contracts.EnrollmentChangedV1)
		require.Equal(t, player1, ev.PlayerID)
		require.Equal(t, tenantA, ev.TenantID)
		got[ev.ProgramID] = true
	}
	require.Equal(t, map[string]bool{a.ID: true, b.ID: true}, got)

	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.RemovePlayer(context.Background(), "", player1)))
}

func TestRemovePlayerIgnoresOtherTenants(t *testing.T) {
	h := newHarness(t, allPerms())
	p := h.active(t, ctxFor(tenantA), "a")
	_, _, err := h.svc.Enroll(ctxFor(tenantA), p.ID, player1)
	require.NoError(t, err)
	h.ob.published = nil

	require.NoError(t, h.svc.RemovePlayer(context.Background(), tenantB, player1))
	require.Len(t, h.repo.enrollments, 1)
	require.Empty(t, h.ob.published)
}

func TestPurgeTenantIsIdempotentAndScoped(t *testing.T) {
	h := newHarness(t, allPerms())
	pa := h.active(t, ctxFor(tenantA), "a")
	pb := h.active(t, ctxFor(tenantB), "b")
	_, _, err := h.svc.Enroll(ctxFor(tenantA), pa.ID, player1)
	require.NoError(t, err)
	_, _, err = h.svc.Enroll(ctxFor(tenantB), pb.ID, player3)
	require.NoError(t, err)
	h.ob.published = nil

	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))
	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))

	require.Len(t, h.repo.programs, 1)
	require.Contains(t, h.repo.programs, pb.ID)
	require.Len(t, h.repo.enrollments, 1)
	require.Empty(t, h.ob.published)
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.PurgeTenant(context.Background(), "")))
}

// ---- auto-end -----------------------------------------------------------------

func TestAutoEndSweepsDueProgramsOnce(t *testing.T) {
	h := newHarness(t, allPerms()) // batch size 2
	ctx := ctxFor(tenantA)
	soon := start.Add(time.Hour)
	later := start.Add(48 * time.Hour)

	mk := func(name string, ends *time.Time, pause bool) domain.Program {
		p, err := h.svc.Create(ctx, CreateCmd{Name: name, EndsAt: ends})
		require.NoError(t, err)
		if _, err := h.svc.Activate(ctx, p.ID); err != nil {
			require.NoError(t, err)
		}
		if pause {
			_, err = h.svc.Pause(ctx, p.ID)
			require.NoError(t, err)
		}
		return p
	}
	due1 := mk("due-active", &soon, false)
	due2 := mk("due-paused", &soon, true)
	due3 := mk("due-third", &soon, false)
	notDue := mk("not-due", &later, false)
	open := mk("open", nil, false)
	draft, err := h.svc.Create(ctx, CreateCmd{Name: "draft", EndsAt: &soon})
	require.NoError(t, err)
	h.ob.published = nil

	h.clock.Advance(2 * time.Hour)
	n, err := h.svc.AutoEnd(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, n, "a sweep is capped by the batch size")
	n, err = h.svc.AutoEnd(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n, "the next tick picks up the rest")
	n, err = h.svc.AutoEnd(context.Background())
	require.NoError(t, err)
	require.Zero(t, n, "reconciled: nothing left to do")

	for _, p := range []domain.Program{due1, due2, due3} {
		require.Equal(t, domain.StatusEnded, h.repo.programs[p.ID].Status)
	}
	require.Equal(t, domain.StatusActive, h.repo.programs[notDue.ID].Status)
	require.Equal(t, domain.StatusActive, h.repo.programs[open.ID].Status)
	require.Equal(t, domain.StatusDraft, h.repo.programs[draft.ID].Status, "drafts are never auto-ended")

	require.Len(t, h.ob.published, 3)
	for _, e := range h.ob.published {
		require.Equal(t, contracts.TopicProgramEnded, e.topic)
		ev := e.payload.(contracts.ProgramChangedV1)
		require.True(t, ev.AutoEnded)
		require.Equal(t, tenantA, ev.TenantID)
		require.Equal(t, "ended", ev.Status)
	}
}

func TestAutoEndContinuesPastFailures(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	soon := start.Add(time.Minute)
	p, err := h.svc.Create(ctx, CreateCmd{Name: "x", EndsAt: &soon})
	require.NoError(t, err)
	_, err = h.svc.Activate(ctx, p.ID)
	require.NoError(t, err)
	h.clock.Advance(time.Hour)

	h.repo.saveErr = errs.New(errs.Internal, "boom")
	n, err := h.svc.AutoEnd(context.Background())
	require.Zero(t, n)
	require.Equal(t, errs.Unavailable, errs.KindOf(err), "a failed sweep is retried by the job ladder")
}

// ---- reader ---------------------------------------------------------------------

func TestReaderIsTenantScoped(t *testing.T) {
	h := newHarness(t, allPerms())
	ctx := ctxFor(tenantA)
	active := h.active(t, ctx, "active")
	paused := h.active(t, ctx, "paused")
	draft := h.create(t, ctx, "draft")
	foreign := h.create(t, ctxFor(tenantB), "foreign")
	for _, id := range []string{active.ID, paused.ID} {
		_, _, err := h.svc.Enroll(ctx, id, player1)
		require.NoError(t, err)
	}
	_, err := h.svc.Pause(ctx, paused.ID)
	require.NoError(t, err)

	snaps, err := h.svc.ProgramsByIDs(context.Background(), tenantA, []string{active.ID, draft.ID, foreign.ID, "missing"})
	require.NoError(t, err)
	require.Len(t, snaps, 2, "foreign and unknown ids are absent, not errors")

	ids, err := h.svc.EnrolledProgramIDs(context.Background(), tenantA, player1)
	require.NoError(t, err)
	require.Equal(t, []string{active.ID}, ids, "only active programs")

	none, err := h.svc.ProgramsByIDs(context.Background(), tenantA, nil)
	require.NoError(t, err)
	require.Empty(t, none)
}
