package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/modules/leaderboards/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

const (
	tenantA = "0198d000-0000-7000-8000-00000000000a"
	tenantB = "0198d000-0000-7000-8000-00000000000b"
	p1      = "0198d000-0000-7000-8000-000000000001"
	p2      = "0198d000-0000-7000-8000-000000000002"
	p3      = "0198d000-0000-7000-8000-000000000003"
	prog    = "0198d000-0000-7000-8000-0000000000f0"
)

var t0 = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) // Wednesday

var allPerms = allowKeys{
	"leaderboards:view_any": true, "leaderboards:view": true, "leaderboards:create": true,
	"leaderboards:update": true, "leaderboards:delete": true, "leaderboards:rebuild": true,
}

var memberPerms = allowKeys{"leaderboards:view_any": true, "leaderboards:view": true}

type harness struct {
	svc   *Service
	repo  *fakeRepo
	ranks *fakeRanks
	ob    *fakeOutbox
	clock *clock.Fake
}

func newHarness(t *testing.T, enf authz.Enforcer) *harness {
	t.Helper()
	h := &harness{repo: newFakeRepo(), ranks: newFakeRanks(), ob: &fakeOutbox{}, clock: clock.NewFake(t0)}
	players := fakePlayers{players: map[string]ports.PlayerSnapshot{
		p1: {ID: p1, ExternalID: "ext-1", DisplayName: "Ana", Active: true},
		p2: {ID: p2, ExternalID: "ext-2", DisplayName: "Ben", Active: true},
		p3: {ID: p3, ExternalID: "ext-3", DisplayName: "Cy", Active: true},
	}}
	h.svc = NewService(h.repo, h.ranks, players, h.ob, enf, nil, h.clock, nil, Settings{
		ClosedRetention: 48 * time.Hour, AllTimeTTL: 72 * time.Hour, CloseGrace: 10 * time.Minute,
		SnapshotLimit: 1000, AppliedRetention: 720 * time.Hour,
	})
	h.svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
	h.svc.async = func(fn func()) { fn() }
	return h
}

func ctxFor(tenant string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "u1", TenantID: tenant, RoleIDs: []int64{2}})
}

func (h *harness) board(t *testing.T, in CreateInput) domain.Leaderboard {
	t.Helper()
	if in.Name == "" {
		in.Name = "Board " + in.Type + in.Metric + in.ResetFrequency + in.ProgramID
	}
	in.Active = true
	lb, err := h.svc.Create(ctxFor(tenantA), in)
	require.NoError(t, err)
	return lb
}

func credit(eventID, player string, amount, balance int64, at time.Time) domain.Fact {
	return domain.Fact{EventID: eventID, TenantID: tenantA, PlayerID: player, Kind: domain.FactPointsCredited, Amount: amount, Absolute: balance, At: at}
}

// ---- definitions ----

func TestCreateStampsTenantFromPrincipal(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Type: contracts.TypePoints})
	require.Equal(t, tenantA, lb.TenantID)
	require.Equal(t, contracts.MetricEarned, lb.Metric)
}

func TestCreateRequiresTenantAndAdminPermission(t *testing.T) {
	h := newHarness(t, memberPerms)
	_, err := h.svc.Create(ctxFor(tenantA), CreateInput{Name: "x", Type: contracts.TypePoints})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	h = newHarness(t, allPerms)
	_, err = h.svc.Create(ctxFor(""), CreateInput{Name: "x", Type: contracts.TypePoints})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err), "a principal without tenant gets no unscoped access")

	_, err = h.svc.Create(context.Background(), CreateInput{Name: "x", Type: contracts.TypePoints})
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
}

func TestCreateDuplicateSlugConflicts(t *testing.T) {
	h := newHarness(t, allPerms)
	h.board(t, CreateInput{Name: "Race", Type: contracts.TypePoints})
	_, err := h.svc.Create(ctxFor(tenantA), CreateInput{Name: "Race", Type: contracts.TypeBadges})
	require.Equal(t, domain.CodeSlugTaken, errs.CodeOf(err))

	// Slugs are unique per tenant only.
	_, err = h.svc.Create(ctxFor(tenantB), CreateInput{Name: "Race", Type: contracts.TypePoints})
	require.NoError(t, err)
}

func TestGetCrossTenantIsNotFound(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Type: contracts.TypePoints})
	_, err := h.svc.Get(ctxFor(tenantB), lb.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	_, err = h.svc.Update(ctxFor(tenantB), lb.ID, domain.Patch{})
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	require.Equal(t, errs.NotFound, errs.KindOf(h.svc.Delete(ctxFor(tenantB), lb.ID)))
}

func TestListRequiresViewAnyAndPaginates(t *testing.T) {
	h := newHarness(t, allPerms)
	for i := range 3 {
		h.clock.Advance(time.Second)
		h.board(t, CreateInput{Name: fmt.Sprintf("b%d", i), Type: contracts.TypePoints})
	}
	page, next, err := h.svc.List(ctxFor(tenantA), ListFilter{}, "", 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	require.Equal(t, "b2", page[0].Name)
	require.NotEmpty(t, next)
	rest, next, err := h.svc.List(ctxFor(tenantA), ListFilter{}, next, 2)
	require.NoError(t, err)
	require.Len(t, rest, 1)
	require.Empty(t, next)

	denied := newHarness(t, allowKeys{})
	_, _, err = denied.svc.List(ctxFor(tenantA), ListFilter{}, "", 10)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err), "fixes L4: index is authorized")
}

func TestUpdateIsValidatedAndPartial(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Name: "Race", Description: "keep", Type: contracts.TypePoints})
	bad := 0
	_, err := h.svc.Update(ctxFor(tenantA), lb.ID, domain.Patch{MaxEntries: &bad})
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	name := "Renamed"
	got, err := h.svc.Update(ctxFor(tenantA), lb.ID, domain.Patch{Name: &name})
	require.NoError(t, err)
	require.Equal(t, "Renamed", got.Name)
	require.Equal(t, "keep", got.Description)
	require.Equal(t, tenantA, got.TenantID, "fixes L3: tenant cannot be moved by update")

	member := newHarness(t, memberPerms)
	member.repo = h.repo
	member.svc.repo = h.repo
	_, err = member.svc.Update(ctxFor(tenantA), lb.ID, domain.Patch{Name: &name})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestDeleteDropsReadModel(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Type: contracts.TypePoints})
	require.NoError(t, h.svc.ApplyFact(context.Background(), credit("e1", p1, 10, 10, t0)))
	require.NoError(t, h.svc.Delete(ctxFor(tenantA), lb.ID))
	require.NotEmpty(t, h.ranks.deleted)
	_, err := h.svc.Get(ctxFor(tenantA), lb.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
}

// ---- scoring ----

func TestDuplicateEventDoesNotDoubleCount(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Type: contracts.TypePoints})
	period := domain.PeriodOf(lb, t0)
	require.NoError(t, h.ranks.Replace(context.Background(), period, nil, t0.Add(time.Hour)))

	f := credit("evt-1", p1, 50, 50, t0)
	require.NoError(t, h.svc.ApplyFact(context.Background(), f))
	require.NoError(t, h.svc.ApplyFact(context.Background(), f))

	score, _ := h.repo.score(lb.ID, period.Start, p1)
	require.Equal(t, int64(50), score)
	require.Equal(t, []domain.Standing{{PlayerID: p1, Score: 50}}, h.ranks.snapshot(period), "redelivery must not ZINCRBY twice")
}

func TestOutOfOrderCreditsConverge(t *testing.T) {
	facts := []domain.Fact{
		credit("e1", p1, 10, 10, t0),
		credit("e2", p1, 20, 30, t0.Add(time.Minute)),
		credit("e3", p1, 5, 35, t0.Add(2*time.Minute)),
	}
	run := func(order []int) (earned, balance int64) {
		h := newHarness(t, allPerms)
		e := h.board(t, CreateInput{Type: contracts.TypePoints, Metric: contracts.MetricEarned})
		b := h.board(t, CreateInput{Type: contracts.TypePoints, Metric: contracts.MetricBalance})
		for _, i := range order {
			require.NoError(t, h.svc.ApplyFact(context.Background(), facts[i]))
		}
		earned, _ = h.repo.score(e.ID, domain.PeriodOf(e, t0).Start, p1)
		balance, _ = h.repo.score(b.ID, domain.AllTimeStart, p1)
		return earned, balance
	}
	e1, b1 := run([]int{0, 1, 2})
	e2, b2 := run([]int{2, 0, 1})
	require.Equal(t, int64(35), e1)
	require.Equal(t, e1, e2)
	require.Equal(t, int64(35), b1, "balance follows the newest balance_after")
	require.Equal(t, b1, b2, "an older balance delivered late must not win")
}

func TestNetMetricSubtractsDebits(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Type: contracts.TypePoints, Metric: contracts.MetricNet})
	require.NoError(t, h.svc.ApplyFact(context.Background(), credit("e1", p1, 100, 100, t0)))
	require.NoError(t, h.svc.ApplyFact(context.Background(), domain.Fact{
		EventID: "e2", TenantID: tenantA, PlayerID: p1, Kind: domain.FactPointsDebited, Amount: 30, Absolute: 70, At: t0,
	}))
	score, _ := h.repo.score(lb.ID, domain.AllTimeStart, p1)
	require.Equal(t, int64(70), score)
}

func TestPeriodBucketingFromFactTime(t *testing.T) {
	h := newHarness(t, allPerms)
	weekly := h.board(t, CreateInput{Type: contracts.TypeBadges, ResetFrequency: contracts.ResetWeekly})
	monday := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	lastWeek := monday.Add(-time.Minute) // Sunday 23:59 UTC
	award := func(id string, at time.Time) domain.Fact {
		return domain.Fact{EventID: id, TenantID: tenantA, PlayerID: p1, Kind: domain.FactBadgeAwarded, Amount: 1, At: at}
	}
	require.NoError(t, h.svc.ApplyFact(context.Background(), award("a1", lastWeek)))
	require.NoError(t, h.svc.ApplyFact(context.Background(), award("a2", monday)))
	require.NoError(t, h.svc.ApplyFact(context.Background(), award("a3", t0)))

	prev, _ := h.repo.score(weekly.ID, monday.AddDate(0, 0, -7), p1)
	cur, _ := h.repo.score(weekly.ID, monday, p1)
	require.Equal(t, int64(1), prev)
	require.Equal(t, int64(2), cur)
}

func TestOnlyMatchingActiveBoardsOfTheTenantScore(t *testing.T) {
	h := newHarness(t, allPerms)
	points := h.board(t, CreateInput{Type: contracts.TypePoints})
	badges := h.board(t, CreateInput{Type: contracts.TypeBadges})
	inactive, err := h.svc.Create(ctxFor(tenantA), CreateInput{Name: "off", Type: contracts.TypePoints, Active: false})
	require.NoError(t, err)

	f := credit("e1", p1, 10, 10, t0)
	f.TenantID = tenantB
	require.NoError(t, h.svc.ApplyFact(context.Background(), f))
	require.NoError(t, h.svc.ApplyFact(context.Background(), credit("e2", p1, 10, 10, t0)))

	s, ok := h.repo.score(points.ID, domain.AllTimeStart, p1)
	require.True(t, ok)
	require.Equal(t, int64(10), s, "the other tenant's credit is not counted")
	_, ok = h.repo.score(badges.ID, domain.AllTimeStart, p1)
	require.False(t, ok)
	_, ok = h.repo.score(inactive.ID, domain.AllTimeStart, p1)
	require.False(t, ok)
}

func TestProgramBoardsCountMembersOnly(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Type: contracts.TypeMissions, ProgramID: prog})
	ctx := context.Background()
	require.NoError(t, h.svc.SetMembership(ctx, tenantA, prog, p1, true, t0))
	complete := func(id, player string) domain.Fact {
		return domain.Fact{EventID: id, TenantID: tenantA, PlayerID: player, Kind: domain.FactMissionCompleted, Amount: 1, At: t0}
	}
	require.NoError(t, h.svc.ApplyFact(ctx, complete("m1", p1)))
	require.NoError(t, h.svc.ApplyFact(ctx, complete("m2", p2)))
	_, ok := h.repo.score(lb.ID, domain.AllTimeStart, p1)
	require.True(t, ok)
	_, ok = h.repo.score(lb.ID, domain.AllTimeStart, p2)
	require.False(t, ok)

	// Unenrolment hides the member; a stale enrol delivered late does not win.
	require.NoError(t, h.svc.SetMembership(ctx, tenantA, prog, p1, false, t0.Add(time.Minute)))
	require.NoError(t, h.svc.SetMembership(ctx, tenantA, prog, p1, true, t0))
	page, err := h.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 10)
	require.NoError(t, err)
	require.Empty(t, page.Entries)
}

func TestInvalidFactIsInvalid(t *testing.T) {
	h := newHarness(t, allPerms)
	err := h.svc.ApplyFact(context.Background(), domain.Fact{TenantID: tenantA, PlayerID: p1, Kind: domain.FactPointsCredited, At: t0})
	require.Equal(t, errs.Invalid, errs.KindOf(err), "malformed facts dead-letter instead of retrying")
}

func TestRedisFailureDoesNotFailTheFact(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Type: contracts.TypePoints})
	h.ranks.fail = true
	require.NoError(t, h.svc.ApplyFact(context.Background(), credit("e1", p1, 10, 10, t0)))
	s, _ := h.repo.score(lb.ID, domain.AllTimeStart, p1)
	require.Equal(t, int64(10), s)

	page, err := h.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 10)
	require.NoError(t, err, "reads fall back to postgres")
	require.Len(t, page.Entries, 1)
}

// ---- reads ----

func seedTie(t *testing.T, h *harness) domain.Leaderboard {
	t.Helper()
	lb := h.board(t, CreateInput{Type: contracts.TypePoints})
	ctx := context.Background()
	// p3 and p2 tie on 20 (delivered p3 first), p1 has 30.
	require.NoError(t, h.svc.ApplyFact(ctx, credit("a", p3, 20, 20, t0)))
	require.NoError(t, h.svc.ApplyFact(ctx, credit("b", p2, 20, 20, t0)))
	require.NoError(t, h.svc.ApplyFact(ctx, credit("c", p1, 30, 30, t0)))
	return lb
}

func ranksOf(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = fmt.Sprintf("%d:%s:%d", e.Rank, e.DisplayName, e.Score)
	}
	return out
}

func TestTieOrderingIsDeterministicFromPostgresAndRedis(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := seedTie(t, h)
	want := []string{"1:Ana:30", "2:Ben:20", "2:Cy:20"}

	// First read: cache miss → postgres, which warms redis synchronously here.
	page, err := h.svc.Entries(ctxFor(tenantA), lb.ID, "current", "", 10)
	require.NoError(t, err)
	require.Equal(t, want, ranksOf(page.Entries))
	ready, _ := h.ranks.Ready(context.Background(), page.Period)
	require.True(t, ready, "the miss warmed the read model")

	page, err = h.svc.Entries(ctxFor(tenantA), lb.ID, "current", "", 10)
	require.NoError(t, err)
	require.Equal(t, want, ranksOf(page.Entries), "redis serves the identical order")
	require.Equal(t, "ext-1", page.Entries[0].ExternalID)
}

func TestEntriesPaginateAndRespectMaxEntries(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := seedTie(t, h)
	page, err := h.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 2)
	require.NoError(t, err)
	require.Equal(t, []string{"1:Ana:30", "2:Ben:20"}, ranksOf(page.Entries))
	require.NotEmpty(t, page.NextCursor)
	page, err = h.svc.Entries(ctxFor(tenantA), lb.ID, "", page.NextCursor, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"2:Cy:20"}, ranksOf(page.Entries), "a page starting mid-tie keeps the shared rank")
	require.Empty(t, page.NextCursor)

	two := 2
	_, err = h.svc.Update(ctxFor(tenantA), lb.ID, domain.Patch{MaxEntries: &two})
	require.NoError(t, err)
	page, err = h.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 10)
	require.NoError(t, err)
	require.Len(t, page.Entries, 2)
	require.Empty(t, page.NextCursor)

	_, err = h.svc.Entries(ctxFor(tenantA), lb.ID, "", "garbage!", 10)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	_, err = h.svc.Entries(ctxFor(tenantA), lb.ID, "yesterday", "", 10)
	require.Equal(t, domain.CodeInvalidPeriod, errs.CodeOf(err))
}

func TestEntriesForPastPeriod(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Type: contracts.TypePoints, ResetFrequency: contracts.ResetDaily})
	yesterday := t0.Add(-24 * time.Hour)
	require.NoError(t, h.svc.ApplyFact(context.Background(), credit("e1", p1, 10, 10, yesterday)))
	page, err := h.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 10)
	require.NoError(t, err)
	require.Empty(t, page.Entries, "a new day starts empty")
	page, err = h.svc.Entries(ctxFor(tenantA), lb.ID, yesterday.Format(time.RFC3339), "", 10)
	require.NoError(t, err)
	require.Len(t, page.Entries, 1)
}

func TestHiddenPlayersAreExcluded(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := seedTie(t, h)
	ctx := context.Background()
	_, err := h.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 10) // warm redis
	require.NoError(t, err)

	require.NoError(t, h.svc.SetPlayerActive(ctx, tenantA, p1, false, t0.Add(time.Minute)))
	page, err := h.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 10)
	require.NoError(t, err)
	require.Equal(t, []string{"1:Ben:20", "1:Cy:20"}, ranksOf(page.Entries))

	// Reordered stale activation is ignored; a newer one restores the player.
	require.NoError(t, h.svc.SetPlayerActive(ctx, tenantA, p1, true, t0))
	page, _ = h.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 10)
	require.Len(t, page.Entries, 2)
	require.NoError(t, h.svc.SetPlayerActive(ctx, tenantA, p1, true, t0.Add(2*time.Minute)))
	page, _ = h.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 10)
	require.Len(t, page.Entries, 3)

	// Deletion is terminal.
	require.NoError(t, h.svc.PlayerDeleted(ctx, tenantA, p2, t0))
	require.NoError(t, h.svc.SetPlayerActive(ctx, tenantA, p2, true, t0.Add(time.Hour)))
	page, _ = h.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 10)
	require.Equal(t, []string{"1:Ana:30", "2:Cy:20"}, ranksOf(page.Entries))
}

func TestPlayerStandingWithNeighbours(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := seedTie(t, h)
	for _, warm := range []bool{false, true} {
		res, err := h.svc.PlayerStanding(ctxFor(tenantA), lb.ID, p2, "", 1)
		require.NoError(t, err, "warm=%v", warm)
		require.Equal(t, int64(2), res.Entry.Rank)
		require.Equal(t, int64(20), res.Entry.Score)
		require.Equal(t, []string{"1:Ana:30", "2:Cy:20"}, ranksOf(res.Neighbours))
	}

	_, err := h.svc.PlayerStanding(ctxFor(tenantA), lb.ID, "0198d000-0000-7000-8000-0000000000ff", "", 1)
	require.Equal(t, domain.CodePlayerNotFound, errs.CodeOf(err))

	other := h.board(t, CreateInput{Type: contracts.TypeXP})
	_, err = h.svc.PlayerStanding(ctxFor(tenantA), other.ID, p1, "", 1)
	require.Equal(t, domain.CodePlayerNotRanked, errs.CodeOf(err))
}

func TestReadsRequireViewAndTenant(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := seedTie(t, h)
	_, err := h.svc.Entries(ctxFor(tenantB), lb.ID, "", "", 10)
	require.Equal(t, errs.NotFound, errs.KindOf(err))

	denied := newHarness(t, allowKeys{})
	denied.svc.repo = h.repo
	_, err = denied.svc.Entries(ctxFor(tenantA), lb.ID, "", "", 10)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

// ---- rebuild, rollover, purge, prune ----

func TestRebuildEqualsIncremental(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Type: contracts.TypePoints, Metric: contracts.MetricNet})
	period := domain.PeriodOf(lb, t0)
	require.NoError(t, h.ranks.Replace(context.Background(), period, nil, t0.Add(time.Hour)))
	ctx := context.Background()
	for i := range 30 {
		player := []string{p1, p2, p3}[i%3]
		kind := domain.FactPointsCredited
		if i%4 == 0 {
			kind = domain.FactPointsDebited
		}
		require.NoError(t, h.svc.ApplyFact(ctx, domain.Fact{
			EventID: fmt.Sprintf("e%d", i), TenantID: tenantA, PlayerID: player, Kind: kind, Amount: int64(i + 1), At: t0,
		}))
	}
	incremental := h.ranks.snapshot(period)

	h.ranks.Delete(ctx, period) //nolint:errcheck // simulate a flush
	res, err := h.svc.Rebuild(ctxFor(tenantA), lb.ID)
	require.NoError(t, err)
	require.Equal(t, 1, res.Periods)
	require.Equal(t, incremental, h.ranks.snapshot(period))

	member := newHarness(t, memberPerms)
	member.svc.repo = h.repo
	_, err = member.svc.Rebuild(ctxFor(tenantA), lb.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestRolloverClosesOncePublishesTopTen(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := h.board(t, CreateInput{Type: contracts.TypePoints, ResetFrequency: contracts.ResetDaily})
	h.board(t, CreateInput{Type: contracts.TypePoints}) // never-resetting: never closes
	ctx := context.Background()
	for i := range 12 {
		player := fmt.Sprintf("0198d000-0000-7000-8000-%012d", 100+i)
		require.NoError(t, h.svc.ApplyFact(ctx, credit(fmt.Sprintf("e%d", i), player, int64(i+1), 0, t0)))
	}
	require.NoError(t, h.svc.Rollover(ctx))
	require.Empty(t, h.ob.published, "the period is still open")

	h.clock.Advance(12*time.Hour + 5*time.Minute) // past midnight, inside grace
	require.NoError(t, h.svc.Rollover(ctx))
	require.Empty(t, h.ob.published, "grace window not over")

	h.clock.Advance(10 * time.Minute)
	require.NoError(t, h.svc.Rollover(ctx))
	require.NoError(t, h.svc.Rollover(ctx))
	require.Len(t, h.ob.published, 1, "exactly one period_closed per period")
	ev := h.ob.published[0].payload.(contracts.PeriodClosedV1)
	require.Equal(t, contracts.TopicPeriodClosed, h.ob.published[0].topic)
	require.Equal(t, lb.ID, ev.LeaderboardID)
	require.Equal(t, tenantA, ev.TenantID)
	require.Len(t, ev.Top, TopN)
	require.Equal(t, int64(12), ev.Top[0].Score)
	require.Equal(t, 1, ev.Top[0].Rank)
	require.Equal(t, time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC), ev.PeriodEnd)
	require.Len(t, h.repo.snapshots[periodKey{lb.ID, ev.PeriodStart.Unix()}], 12)
	_, ok := h.repo.marks[JobRollover]
	require.True(t, ok)
}

func TestTenantDeletedPurgesRowsAndKeys(t *testing.T) {
	h := newHarness(t, allPerms)
	lb := seedTie(t, h)
	ctx := context.Background()
	require.NoError(t, h.svc.PurgeTenant(ctx, tenantA))
	require.NoError(t, h.svc.PurgeTenant(ctx, tenantA), "idempotent")
	_, ok := h.repo.score(lb.ID, domain.AllTimeStart, p1)
	require.False(t, ok)
	require.Contains(t, h.ranks.deleted, pkey(domain.PeriodOf(lb, t0)))
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.PurgeTenant(ctx, "")))
}

func TestPruneAppliedEvents(t *testing.T) {
	h := newHarness(t, allPerms)
	h.board(t, CreateInput{Type: contracts.TypePoints})
	require.NoError(t, h.svc.ApplyFact(context.Background(), credit("old", p1, 1, 1, t0)))
	h.clock.Advance(31 * 24 * time.Hour)
	require.NoError(t, h.svc.ApplyFact(context.Background(), credit("new", p1, 1, 2, h.clock.Now())))
	require.NoError(t, h.svc.PruneAppliedEvents(context.Background()))
	require.Len(t, h.repo.applied, 1)
}
