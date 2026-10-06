package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/shared/errs"
)

func TestGetProgressZeroStateWritesNothing(t *testing.T) {
	h := newHarness(t, memberKeys, Options{})
	standardLadder(h, "t1")
	h.players.add("t1", "p1", true)

	v, err := h.svc.GetProgress(asUser("t1"), "p1")
	require.NoError(t, err)
	require.Equal(t, int64(0), v.TotalXP)
	require.Equal(t, "l1", v.Current.ID)
	require.Equal(t, "l2", v.Next.ID)
	require.Equal(t, int64(100), *v.XPToNext)
	require.Empty(t, h.repo.progress, "a read never creates a row (B17)")
}

func TestGetProgressAfterGrants(t *testing.T) {
	h := newHarness(t, memberKeys, Options{})
	standardLadder(h, "t1")
	h.players.add("t1", "p1", true)
	_, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k1", 175))
	require.NoError(t, err)

	v, err := h.svc.GetProgress(asUser("t1"), "p1")
	require.NoError(t, err)
	require.Equal(t, int64(175), v.TotalXP)
	require.Equal(t, 2, v.Current.Number)
	require.Equal(t, 3, v.Next.Number)
	require.Equal(t, int64(75), *v.XPToNext)
	require.InDelta(t, 50.0, v.ProgressPercent, 0.001)
}

func TestGetProgressNoLadderAndUnknownPlayer(t *testing.T) {
	h := newHarness(t, memberKeys, Options{})
	h.players.add("t1", "p1", true)
	h.players.add("t2", "foreign", true)

	v, err := h.svc.GetProgress(asUser("t1"), "p1")
	require.NoError(t, err)
	require.Nil(t, v.Current)
	require.Nil(t, v.Next)

	_, err = h.svc.GetProgress(asUser("t1"), "foreign")
	require.ErrorIs(t, err, domain.ErrPlayerNotFound)

	denied := newHarness(t, allowKeys{}, Options{})
	denied.players.add("t1", "p1", true)
	_, err = denied.svc.GetProgress(asUser("t1"), "p1")
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestListGrantsPaginates(t *testing.T) {
	h := newHarness(t, memberKeys, Options{})
	h.players.add("t1", "p1", true)
	for i := range 5 {
		h.clock.Advance(time.Second)
		_, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", fmt.Sprintf("k%d", i), int64(i+1)))
		require.NoError(t, err)
	}
	ctx := asUser("t1")

	page, err := h.svc.ListGrants(ctx, "p1", "", 2)
	require.NoError(t, err)
	require.Len(t, page.Grants, 2)
	require.Equal(t, int64(5), page.Grants[0].Amount, "newest first")
	require.NotEmpty(t, page.NextCursor)

	var amounts []int64
	cursor := ""
	for {
		page, err := h.svc.ListGrants(ctx, "p1", cursor, 2)
		require.NoError(t, err)
		for _, g := range page.Grants {
			amounts = append(amounts, g.Amount)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	require.Equal(t, []int64{5, 4, 3, 2, 1}, amounts)

	_, err = h.svc.ListGrants(ctx, "p1", "garbage!", 2)
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	_, err = h.svc.ListGrants(ctx, "nobody", "", 2)
	require.ErrorIs(t, err, domain.ErrPlayerNotFound)
}

func TestReaderReturnsEverySnapshot(t *testing.T) {
	h := newHarness(t, memberKeys, Options{})
	standardLadder(h, "t1")
	h.players.add("t1", "p1", true)
	_, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k1", 260))
	require.NoError(t, err)

	got, err := h.svc.ProgressByPlayerIDs(context.Background(), "t1", []string{"p1", "p2", "p1"})
	require.NoError(t, err)
	require.Equal(t, []contracts.ProgressSnapshot{
		{PlayerID: "p1", TotalXP: 260, LevelID: "l3", LevelNumber: 3},
		{PlayerID: "p2", TotalXP: 0, LevelID: "l1", LevelNumber: 1},
	}, got)

	other, err := h.svc.ProgressByPlayerIDs(context.Background(), "t2", []string{"p1"})
	require.NoError(t, err)
	require.Equal(t, []contracts.ProgressSnapshot{{PlayerID: "p1"}}, other, "tenant t2 sees nothing of t1")
}

func TestPurgeTenantIsIdempotentAndScoped(t *testing.T) {
	h := newHarness(t, memberKeys, Options{})
	standardLadder(h, "t1")
	h.seedLevel("t2", "t2-l1", 1, 0, 0, "")
	h.players.add("t1", "p1", true)
	h.players.add("t2", "p9", true)
	_, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k1", 300))
	require.NoError(t, err)
	_, err = h.svc.HandleGrantXP(context.Background(), grantCmd("t2", "p9", "k1", 5))
	require.NoError(t, err)

	require.NoError(t, h.svc.PurgeTenant(context.Background(), "t1"))
	require.NoError(t, h.svc.PurgeTenant(context.Background(), "t1"), "redelivery is harmless")

	for _, l := range h.repo.levels {
		require.Equal(t, "t2", l.TenantID)
	}
	require.Len(t, h.repo.grants, 1)
	require.Equal(t, "t2", h.repo.grants[0].TenantID)
	require.Len(t, h.repo.progress, 1)
	require.Empty(t, h.repo.rewards)
}

func TestReconcileReportsDriftAndOptionallyReplaces(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprintf("replace=%v", replace), func(t *testing.T) {
			h := newHarness(t, memberKeys, Options{ReplaceLevels: replace, ReconcileBatchSize: 1})
			standardLadder(h, "t1")
			h.players.add("t1", "p1", true)
			h.players.add("t1", "p2", true)
			_, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k1", 120))
			require.NoError(t, err)
			_, err = h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p2", "k2", 50))
			require.NoError(t, err)

			// Ladder edit lowers l3 below p1's total: stored level 2, ladder says 3.
			l3 := h.repo.levels["l3"]
			l3.XPRequired = 110
			h.repo.levels["l3"] = l3
			// And p2's counter is corrupted behind the ledger's back.
			p2 := h.repo.progress[pk("t1", "p2")]
			p2.TotalXP = 999
			h.repo.progress[pk("t1", "p2")] = p2
			published := len(h.outbox.published)

			h.clock.Advance(time.Hour)
			report, err := h.svc.Reconcile(context.Background(), h.repo)
			require.NoError(t, err)
			require.Equal(t, 1, report.XPDrift)
			require.Equal(t, 2, report.LevelDrift, "p1 (ladder edit) and p2 (corrupted total)")
			require.Equal(t, 1, h.drift["total_xp"])
			require.Equal(t, 2, h.drift["level"])
			require.Equal(t, testNow.Add(time.Hour), h.repo.lastRun, "marker advances")
			require.Len(t, h.outbox.published, published, "reconcile never publishes or rewards")
			require.Len(t, h.repo.rewards, 1)

			if replace {
				require.Equal(t, 2, report.Replaced)
				require.Equal(t, 3, h.repo.progress[pk("t1", "p1")].LevelNumber)
			} else {
				require.Zero(t, report.Replaced)
				require.Equal(t, 2, h.repo.progress[pk("t1", "p1")].LevelNumber)
			}
		})
	}
}

func TestBatchProgressMatchesSingleViewInRequestOrder(t *testing.T) {
	h := newHarness(t, memberKeys, Options{})
	standardLadder(h, "t1")
	h.players.add("t1", "p1", true)
	h.players.add("t1", "p2", true)
	h.players.add("t2", "foreign", true)
	_, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k1", 175))
	require.NoError(t, err)

	views, err := h.svc.BatchProgress(asUser("t1"), []string{"p2", "ghost", "p1", "foreign", "p1"})
	require.NoError(t, err)
	require.Len(t, views, 2, "unknown and foreign players are omitted, duplicates collapsed")
	require.Equal(t, "p2", views[0].PlayerID)
	require.Equal(t, int64(0), views[0].TotalXP)
	require.Equal(t, "p1", views[1].PlayerID)

	single, err := h.svc.GetProgress(asUser("t1"), "p1")
	require.NoError(t, err)
	require.Equal(t, single, views[1])
	require.Len(t, h.repo.progress, 1, "a batch read never creates rows")
}

func TestBatchProgressLimitsAndAuthz(t *testing.T) {
	h := newHarness(t, memberKeys, Options{})
	ids := make([]string, MaxBatchPlayers+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("p%d", i)
	}
	_, err := h.svc.BatchProgress(asUser("t1"), ids)
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	views, err := h.svc.BatchProgress(asUser("t1"), nil)
	require.NoError(t, err)
	require.Empty(t, views)

	denied := newHarness(t, allowKeys{}, Options{})
	_, err = denied.svc.BatchProgress(asUser("t1"), []string{"p1"})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}
