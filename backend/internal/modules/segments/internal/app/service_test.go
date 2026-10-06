package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/segments/contracts"
	"levelup/internal/modules/segments/internal/domain"
	"levelup/internal/shared/errs"
)

var (
	admin  = allowKeys{contracts.PermView.Key(): true, contracts.PermManage.Key(): true}
	member = allowKeys{contracts.PermView.Key(): true}
)

func cond(field, op string, value any) map[string]any {
	return map[string]any{"all": []any{map[string]any{"field": field, "op": op, "value": value}}}
}

func create(t *testing.T, h *harness, tenant string, c map[string]any) domain.Segment {
	t.Helper()
	s, err := h.svc.Create(asTenant(tenant), domain.NewSegmentInput{Name: "seg " + time.Now().String(), Conditions: c})
	require.NoError(t, err)
	return s
}

func changes(ob *fakeOutbox) map[string][]string {
	out := map[string][]string{}
	for _, p := range ob.published {
		if ev, ok := p.payload.(contracts.MembershipChangedV1); ok {
			out[ev.Change] = append(out[ev.Change], ev.PlayerID)
		}
	}
	return out
}

func TestCreateQueuesRefreshInTheSameTransaction(t *testing.T) {
	h := newHarness(admin, 500)
	s := create(t, h, tenantA, cond("is_active", "eq", true))
	require.Equal(t, tenantA, s.TenantID)
	require.Equal(t, "u1", s.CreatedBy)
	require.Equal(t, []string{contracts.Topic(contracts.JobRefreshSegment)}, h.ob.topics())
	cmd := h.ob.published[0].payload.(contracts.RefreshSegmentCmdV1)
	require.Equal(t, contracts.RefreshSegmentCmdV1{TenantID: tenantA, SegmentID: s.ID, RequestedAt: t0}, cmd)
}

func TestCreateValidatesConditions(t *testing.T) {
	h := newHarness(admin, 500)
	_, err := h.svc.Create(asTenant(tenantA), domain.NewSegmentInput{Name: "x", Conditions: map[string]any{"all": []any{}}})
	require.Equal(t, domain.CodeInvalidConditions, errs.CodeOf(err))
	require.Empty(t, h.ob.published)
}

func TestMembersCannotManage(t *testing.T) {
	h := newHarness(member, 500)
	_, err := h.svc.Create(asTenant(tenantA), domain.NewSegmentInput{Name: "x", Conditions: cond("is_active", "eq", true)})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	seg, err := domain.NewSegment(tenantA, "u", domain.NewSegmentInput{Name: "x", Conditions: cond("is_active", "eq", true)}, t0)
	require.NoError(t, err)
	h.repo.segs[seg.ID] = seg
	_, err = h.svc.RequestRefresh(asTenant(tenantA), seg.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(h.svc.Delete(asTenant(tenantA), seg.ID)))
	name := "y"
	_, err = h.svc.Update(asTenant(tenantA), seg.ID, domain.SegmentPatch{Name: &name})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Empty(t, h.ob.published)

	got, err := h.svc.Get(asTenant(tenantA), seg.ID)
	require.NoError(t, err, "view is a member permission")
	require.Equal(t, seg.ID, got.ID)
}

func TestOtherTenantsSegmentIs404(t *testing.T) {
	h := newHarness(admin, 500)
	s := create(t, h, tenantA, cond("is_active", "eq", true))
	_, err := h.svc.Get(asTenant(tenantB), s.ID)
	require.ErrorIs(t, err, domain.ErrSegmentNotFound)
	_, _, err = h.svc.Members(asTenant(tenantB), s.ID, "", 10)
	require.ErrorIs(t, err, domain.ErrSegmentNotFound)
	require.ErrorIs(t, h.svc.Delete(asTenant(tenantB), s.ID), domain.ErrSegmentNotFound)
}

func TestUpdateQueuesRefreshOnlyWhenConditionsChange(t *testing.T) {
	h := newHarness(admin, 500)
	s := create(t, h, tenantA, cond("is_active", "eq", true))
	h.ob.reset()

	name := "renamed"
	got, err := h.svc.Update(asTenant(tenantA), s.ID, domain.SegmentPatch{Name: &name})
	require.NoError(t, err)
	require.Equal(t, "renamed", got.Name)
	require.Empty(t, h.ob.published)

	_, err = h.svc.Update(asTenant(tenantA), s.ID, domain.SegmentPatch{Conditions: cond("level", "gte", 3.0)})
	require.NoError(t, err)
	require.Equal(t, []string{contracts.Topic(contracts.JobRefreshSegment)}, h.ob.topics())
}

func TestDeletePublishesSegmentDeleted(t *testing.T) {
	h := newHarness(admin, 500)
	s := create(t, h, tenantA, cond("is_active", "eq", true))
	h.ob.reset()
	require.NoError(t, h.svc.Delete(asTenant(tenantA), s.ID))
	require.Equal(t, []string{contracts.TopicSegmentDeleted}, h.ob.topics())
	_, err := h.svc.Get(asTenant(tenantA), s.ID)
	require.ErrorIs(t, err, domain.ErrSegmentNotFound)
}

func TestRefreshMaterializesAndPublishesOnlyChanges(t *testing.T) {
	h := newHarness(admin, 3) // several pages
	ge := h.players.add(tenantA, 4, map[string]any{"country": "GE"})
	am := h.players.add(tenantA, 3, map[string]any{"country": "AM"})
	h.players.add(tenantB, 2, map[string]any{"country": "GE"}) // other tenant: never scanned
	s := create(t, h, tenantA, cond("attributes.country", "eq", "GE"))
	h.ob.reset()

	out, err := h.svc.RefreshSegment(context.Background(), tenantA, s.ID)
	require.NoError(t, err)
	require.Equal(t, RefreshOutcome{Ran: true, Scanned: 7, Added: 4}, out)
	require.ElementsMatch(t, ge, changes(h.ob)[contracts.ChangeAdded])
	require.Empty(t, changes(h.ob)[contracts.ChangeRemoved])
	got, err := h.svc.Get(asTenant(tenantA), s.ID)
	require.NoError(t, err)
	require.Equal(t, 4, got.MemberCount)
	require.Equal(t, &t0, got.LastRefreshedAt)
	require.False(t, got.Refreshing(t0))

	// A second run with nothing changed publishes nothing.
	h.ob.reset()
	out, err = h.svc.RefreshSegment(context.Background(), tenantA, s.ID)
	require.NoError(t, err)
	require.Zero(t, out.Added+out.Removed)
	require.Empty(t, h.ob.published, "an unchanged membership must not re-publish")

	// One player moves out, one in, one disappears from the player module.
	p := h.players.players[ge[0]]
	p.Attributes = map[string]any{"country": "AM"}
	h.players.players[ge[0]] = p
	p = h.players.players[am[0]]
	p.Attributes = map[string]any{"country": "GE"}
	h.players.players[am[0]] = p
	delete(h.players.players, ge[3])

	h.ob.reset()
	out, err = h.svc.RefreshSegment(context.Background(), tenantA, s.ID)
	require.NoError(t, err)
	require.Equal(t, 1, out.Added)
	require.Equal(t, 2, out.Removed)
	require.Equal(t, []string{am[0]}, changes(h.ob)[contracts.ChangeAdded])
	require.ElementsMatch(t, []string{ge[0], ge[3]}, changes(h.ob)[contracts.ChangeRemoved])
	for _, pub := range h.ob.published {
		ev := pub.payload.(contracts.MembershipChangedV1)
		require.Equal(t, tenantA, ev.TenantID)
		require.Equal(t, s.ID, ev.SegmentID)
		require.NotEmpty(t, ev.RunID)
	}
	got, err = h.svc.Get(asTenant(tenantA), s.ID)
	require.NoError(t, err)
	require.Equal(t, 3, got.MemberCount)
}

func TestRefreshCallsOnlyTheReadersTheConditionsNeed(t *testing.T) {
	h := newHarness(admin, 500)
	ids := h.players.add(tenantA, 3, nil)
	h.levels.levels[ids[0]] = 5
	h.activity.seen[ids[1]] = t0.Add(-time.Hour)
	s := create(t, h, tenantA, map[string]any{"any": []any{
		map[string]any{"field": "level", "op": "gte", "value": 5.0},
		map[string]any{"field": "last_seen_days", "op": "lte", "value": 1.0},
	}})
	out, err := h.svc.RefreshSegment(context.Background(), tenantA, s.ID)
	require.NoError(t, err)
	require.Equal(t, 2, out.Added)
	require.Equal(t, 1, h.levels.calls)
	require.Equal(t, 1, h.activity.calls)
	require.Zero(t, h.wallets.calls)
	require.Zero(t, h.badges.calls)
}

func TestRefreshCoalescesWhileLeaseHeld(t *testing.T) {
	h := newHarness(admin, 500)
	h.players.add(tenantA, 2, nil)
	s := create(t, h, tenantA, cond("is_active", "eq", true))
	until := t0.Add(5 * time.Minute)
	held := h.repo.segs[s.ID]
	held.RefreshLeaseUntil, held.RefreshRunID = &until, "other-run"
	h.repo.segs[s.ID] = held
	h.ob.reset()

	out, err := h.svc.RefreshSegment(context.Background(), tenantA, s.ID)
	require.NoError(t, err)
	require.False(t, out.Ran)
	require.Empty(t, h.ob.published)

	h.clock.Advance(6 * time.Minute) // the crashed run's lease expired
	out, err = h.svc.RefreshSegment(context.Background(), tenantA, s.ID)
	require.NoError(t, err)
	require.True(t, out.Ran)
	require.Equal(t, 2, out.Added)
}

func TestRefreshOfDeletedSegmentIsANoop(t *testing.T) {
	h := newHarness(admin, 500)
	out, err := h.svc.RefreshSegment(context.Background(), tenantA, "missing")
	require.NoError(t, err)
	require.False(t, out.Ran)
	_, err = h.svc.RefreshSegment(context.Background(), "", "x")
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestRefreshPropagatesReaderErrors(t *testing.T) {
	h := newHarness(admin, 500)
	h.players.add(tenantA, 1, nil)
	h.wallets.err = errs.New(errs.Unavailable, "points down")
	s := create(t, h, tenantA, cond("balance", "gte", 1.0))
	_, err := h.svc.RefreshSegment(context.Background(), tenantA, s.ID)
	require.Equal(t, errs.Unavailable, errs.KindOf(err), "transient errors climb the retry ladder")
}

func TestRefreshAllRefreshesEveryTenantAndMarksRun(t *testing.T) {
	h := newHarness(admin, 500)
	h.players.add(tenantA, 2, nil)
	h.players.add(tenantB, 3, nil)
	a := create(t, h, tenantA, cond("is_active", "eq", true))
	b := create(t, h, tenantB, cond("is_active", "eq", true))

	n, err := h.svc.RefreshAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Len(t, h.repo.members[a.ID], 2)
	require.Len(t, h.repo.members[b.ID], 3)
	require.Equal(t, []string{contracts.JobRefresh}, h.repo.marked)
}

func TestRefreshAllContinuesPastAFailingSegment(t *testing.T) {
	h := newHarness(admin, 500)
	h.players.add(tenantA, 1, nil)
	h.wallets.err = errors.New("boom")
	create(t, h, tenantA, cond("balance", "gte", 1.0))
	ok := create(t, h, tenantA, cond("is_active", "eq", true))

	n, err := h.svc.RefreshAll(context.Background())
	require.Error(t, err)
	require.Equal(t, 1, n)
	require.Len(t, h.repo.members[ok.ID], 1)
	require.Empty(t, h.repo.marked, "a failed sweep does not advance the marker")
}

func TestPreviewScansAtMostTheLimitAndSamples(t *testing.T) {
	h := newHarness(member, 400)
	h.svc.set.PreviewLimit = 1000
	h.players.add(tenantA, 1200, map[string]any{"vip": true})

	p, err := h.svc.Preview(asTenant(tenantA), cond("attributes.vip", "eq", true))
	require.NoError(t, err)
	require.Equal(t, 1000, p.Scanned)
	require.Equal(t, 1000, p.Matched)
	require.False(t, p.Complete)
	require.Len(t, p.Sample, 10)
	require.Empty(t, h.ob.published, "preview never changes membership")
	require.Empty(t, h.repo.members)

	small := newHarness(member, 400)
	small.players.add(tenantA, 5, map[string]any{"vip": true})
	small.players.add(tenantA, 5, map[string]any{"vip": false})
	p, err = small.svc.Preview(asTenant(tenantA), cond("attributes.vip", "eq", true))
	require.NoError(t, err)
	require.Equal(t, Preview{Matched: 5, Scanned: 10, Complete: true, Sample: p.Sample}, p)
	require.Len(t, p.Sample, 5)

	_, err = small.svc.Preview(asTenant(tenantA), map[string]any{"oops": 1})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	_, err = newHarness(allowKeys{}, 10).svc.Preview(asTenant(tenantA), cond("is_active", "eq", true))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestMembersPageWithPlayerData(t *testing.T) {
	h := newHarness(admin, 500)
	h.players.add(tenantA, 3, nil)
	s := create(t, h, tenantA, cond("is_active", "eq", true))
	_, err := h.svc.RefreshSegment(context.Background(), tenantA, s.ID)
	require.NoError(t, err)

	rows, next, err := h.svc.Members(asTenant(tenantA), s.ID, "", 2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NotEmpty(t, next)
	require.Equal(t, "ext-"+rows[0].Member.PlayerID, rows[0].Player.ExternalID)

	rest, next, err := h.svc.Members(asTenant(tenantA), s.ID, next, 2)
	require.NoError(t, err)
	require.Len(t, rest, 1)
	require.Empty(t, next)
}

func TestPlayerDeletedLeavesEverySegmentOnce(t *testing.T) {
	h := newHarness(admin, 500)
	ids := h.players.add(tenantA, 2, nil)
	s1 := create(t, h, tenantA, cond("is_active", "eq", true))
	s2 := create(t, h, tenantA, cond("created_at", "before", "2030-01-01"))
	for _, s := range []domain.Segment{s1, s2} {
		_, err := h.svc.RefreshSegment(context.Background(), tenantA, s.ID)
		require.NoError(t, err)
	}
	h.ob.reset()

	require.NoError(t, h.svc.PlayerDeleted(context.Background(), tenantA, ids[0]))
	require.Equal(t, []string{ids[0], ids[0]}, changes(h.ob)[contracts.ChangeRemoved])
	h.ob.reset()
	require.NoError(t, h.svc.PlayerDeleted(context.Background(), tenantA, ids[0]))
	require.Empty(t, h.ob.published, "redelivery publishes nothing")
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.PlayerDeleted(context.Background(), tenantA, "")))

	got, err := h.svc.SegmentsOfPlayers(context.Background(), tenantA, ids)
	require.NoError(t, err)
	require.Empty(t, got[ids[0]])
	require.Len(t, got[ids[1]], 2)
}

func TestPurgeTenant(t *testing.T) {
	h := newHarness(admin, 500)
	a := create(t, h, tenantA, cond("is_active", "eq", true))
	b := create(t, h, tenantB, cond("is_active", "eq", true))
	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))
	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))
	_, err := h.svc.Get(asTenant(tenantA), a.ID)
	require.ErrorIs(t, err, domain.ErrSegmentNotFound)
	_, err = h.svc.Get(asTenant(tenantB), b.ID)
	require.NoError(t, err)
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.PurgeTenant(context.Background(), "")))
}

func TestListPagesNewestFirst(t *testing.T) {
	h := newHarness(member, 500)
	for range 3 {
		seg, err := domain.NewSegment(tenantA, "u", domain.NewSegmentInput{Name: "n" + time.Now().String(), Conditions: cond("is_active", "eq", true)}, t0)
		require.NoError(t, err)
		h.repo.segs[seg.ID] = seg
	}
	rows, next, err := h.svc.List(asTenant(tenantA), "", 2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NotEmpty(t, next)
	_, _, err = h.svc.List(asTenant(tenantA), "%%%", 2)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestRefreshRerunsWhenTheDefinitionChangedMidRun(t *testing.T) {
	h := newHarness(admin, 500)
	ids := h.players.add(tenantA, 2, nil)
	h.levels.levels[ids[0]] = 9
	s := create(t, h, tenantA, cond("is_active", "eq", true))

	edited := false
	h.players.onList = func() {
		if edited {
			return
		}
		edited = true
		// An admin narrows the segment while the first run is scanning.
		cur := h.repo.segs[s.ID]
		_, err := cur.Apply(domain.SegmentPatch{Conditions: cond("level", "gte", 5.0)}, t0)
		require.NoError(t, err)
		require.NoError(t, h.repo.SaveSegment(context.Background(), nil, cur))
	}
	out, err := h.svc.RefreshSegment(context.Background(), tenantA, s.ID)
	require.NoError(t, err)
	require.Equal(t, 2, out.Added)
	require.Equal(t, 1, out.Removed, "the second run applies the new definition")
	require.Len(t, h.repo.members[s.ID], 1)
	require.Contains(t, h.repo.members[s.ID], ids[0])
}
