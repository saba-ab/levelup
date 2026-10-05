package repo

import (
	"context"
	"fmt"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/modules/leaderboards/internal/app"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/modules/leaderboards/migrations"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/platform/redis/redistest"
	"levelup/internal/shared/id"
)

var t0 = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "leaderboards", migrations.FS))
	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "leaderboards")
	return NewPostgres(moduleDB), moduleDB
}

func setupRedis(t *testing.T) *Redis {
	t.Helper()
	rdb := goredis.NewClient(&goredis.Options{Addr: redistest.Addr(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewRedis(rdb)
}

func mustBoard(t *testing.T, r *Postgres, db *gorm.DB, tenant string, in domain.NewLeaderboardInput) domain.Leaderboard {
	t.Helper()
	in.TenantID = tenant
	if in.Name == "" {
		in.Name = "board " + id.NewID()
	}
	in.Active = true
	lb, err := domain.NewLeaderboard(in, t0)
	require.NoError(t, err)
	require.NoError(t, postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		return r.Create(context.Background(), tx, lb)
	}))
	return lb
}

func apply(t *testing.T, r *Postgres, db *gorm.DB, c app.ScoreChange) app.ScoreResult {
	t.Helper()
	var res app.ScoreResult
	require.NoError(t, postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		var err error
		res, err = r.ApplyScore(context.Background(), tx, c)
		return err
	}))
	return res
}

func TestApplyScoreIsIdempotentPerEventAndBoard(t *testing.T) {
	r, db := setupRepo(t)
	tenant, player := id.NewID(), id.NewID()
	lb := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypePoints})
	p := domain.PeriodOf(lb, t0)
	c := app.ScoreChange{EventID: "evt-1", Period: p, PlayerID: player, Op: domain.Op{Kind: domain.OpIncrement, Value: 5}, At: t0, Now: t0}

	first := apply(t, r, db, c)
	require.Equal(t, app.ScoreResult{Applied: true, Changed: true, Score: 5}, first)
	again := apply(t, r, db, c)
	require.False(t, again.Applied, "redelivery inserts nothing")

	require.Equal(t, int64(12), apply(t, r, db, app.ScoreChange{EventID: "evt-2", Period: p, PlayerID: player,
		Op: domain.Op{Kind: domain.OpIncrement, Value: 7}, At: t0.Add(-time.Hour), Now: t0}).Score)
}

func TestApplyScoreSetIsLastWriterWins(t *testing.T) {
	r, db := setupRepo(t)
	tenant, player := id.NewID(), id.NewID()
	lb := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypePoints, Metric: contracts.MetricBalance})
	p := domain.PeriodOf(lb, t0)
	set := func(ev string, v int64, at time.Time) app.ScoreResult {
		return apply(t, r, db, app.ScoreChange{EventID: ev, Period: p, PlayerID: player, Op: domain.Op{Kind: domain.OpSet, Value: v}, At: at, Now: t0})
	}
	require.True(t, set("b", 300, t0.Add(time.Minute)).Changed)
	older := set("a", 100, t0)
	require.True(t, older.Applied)
	require.False(t, older.Changed, "an older balance must not overwrite a newer one")
	st, ok, err := r.PositionOf(context.Background(), lb, p.Start, player)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(300), st.Score)
}

func TestRankingTiesHiddenAndProgramFilter(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, program := id.NewID(), id.NewID()
	a, b, c := "00000000-0000-7000-8000-00000000000a", "00000000-0000-7000-8000-00000000000b", "00000000-0000-7000-8000-00000000000c"
	lb := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypePoints})
	pb := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypePoints, ProgramID: program})
	for _, board := range []domain.Leaderboard{lb, pb} {
		p := domain.PeriodOf(board, t0)
		for i, s := range []struct {
			player string
			score  int64
		}{{c, 20}, {b, 20}, {a, 30}} {
			apply(t, r, db, app.ScoreChange{EventID: fmt.Sprint("e", i), Period: p, PlayerID: s.player,
				Op: domain.Op{Kind: domain.OpIncrement, Value: s.score}, At: t0, Now: t0})
		}
	}
	rows, err := r.RangeByPosition(ctx, lb, domain.AllTimeStart, 0, 10)
	require.NoError(t, err)
	require.Equal(t, []domain.Standing{
		{PlayerID: a, Score: 30, Rank: 1, Position: 0},
		{PlayerID: b, Score: 20, Rank: 2, Position: 1},
		{PlayerID: c, Score: 20, Rank: 2, Position: 2},
	}, rows)
	page, err := r.RangeByPosition(ctx, lb, domain.AllTimeStart, 2, 10)
	require.NoError(t, err)
	require.Equal(t, int64(2), page[0].Rank)

	all, err := r.VisibleScores(ctx, lb, domain.AllTimeStart)
	require.NoError(t, err)
	require.Equal(t, rows, all)

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		if err := r.SetPlayerStatus(ctx, tx, tenant, a, false, t0.Add(time.Minute)); err != nil {
			return err
		}
		return r.SetPlayerStatus(ctx, tx, tenant, a, true, t0) // stale: ignored
	}))
	rows, err = r.RangeByPosition(ctx, lb, domain.AllTimeStart, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(1), rows[0].Rank)

	// Program board: nobody enrolled yet → empty; enrol b.
	rows, err = r.RangeByPosition(ctx, pb, domain.AllTimeStart, 0, 10)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		return r.SetMembership(ctx, tx, tenant, program, b, true, t0)
	}))
	rows, err = r.RangeByPosition(ctx, pb, domain.AllTimeStart, 0, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, b, rows[0].PlayerID)

	scores, err := r.PlayerScores(ctx, tenant, a, t0)
	require.NoError(t, err)
	require.Len(t, scores, 2)
	for _, s := range scores {
		require.False(t, s.Visible)
	}
}

func TestClosePeriodSnapshotsExactlyOnce(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	lb := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypeBadges, ResetFrequency: contracts.ResetDaily})
	p := domain.PeriodOf(lb, t0)
	for i := range 3 {
		apply(t, r, db, app.ScoreChange{EventID: fmt.Sprint("e", i), Period: p, PlayerID: id.NewID(),
			Op: domain.Op{Kind: domain.OpIncrement, Value: int64(i + 1)}, At: t0, Now: t0})
	}
	due, err := r.DuePeriods(ctx, p.End, 100)
	require.NoError(t, err)
	found := false
	for _, d := range due {
		if d.LeaderboardID == lb.ID {
			found = true
			require.True(t, d.Start.Equal(p.Start))
			require.True(t, d.End.Equal(p.End))
		}
	}
	require.True(t, found)

	closeOnce := func() (bool, []domain.Standing) {
		var closed bool
		var top []domain.Standing
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			var err error
			closed, top, err = r.ClosePeriod(ctx, tx, lb, p, t0.Add(24*time.Hour), 1000, 2)
			return err
		}))
		return closed, top
	}
	closed, top := closeOnce()
	require.True(t, closed)
	require.Len(t, top, 2)
	require.Equal(t, int64(3), top[0].Score)
	closed, _ = closeOnce()
	require.False(t, closed, "second close is a no-op")

	open, err := r.Periods(ctx, tenant, lb.ID, true)
	require.NoError(t, err)
	require.Empty(t, open)
}

func TestPurgeTenantRemovesEverything(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, other := id.NewID(), id.NewID()
	lb := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypePoints})
	keep := mustBoard(t, r, db, other, domain.NewLeaderboardInput{Type: contracts.TypePoints})
	for _, b := range []domain.Leaderboard{lb, keep} {
		apply(t, r, db, app.ScoreChange{EventID: "e", Period: domain.PeriodOf(b, t0), PlayerID: id.NewID(),
			Op: domain.Op{Kind: domain.OpIncrement, Value: 1}, At: t0, Now: t0})
	}
	var boards []domain.Leaderboard
	var periods []domain.Period
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		var err error
		boards, periods, err = r.PurgeTenant(ctx, tx, tenant)
		return err
	}))
	require.Len(t, boards, 1)
	require.Len(t, periods, 1)
	_, err := r.ByID(ctx, tenant, lb.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = r.ByID(ctx, other, keep.ID)
	require.NoError(t, err)
}

func TestSlugUniquePerTenantAndCrossTenantNotFound(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	lb := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Name: "Race", Type: contracts.TypePoints})
	dup, err := domain.NewLeaderboard(domain.NewLeaderboardInput{TenantID: tenant, Name: "Race", Type: contracts.TypeXP}, t0)
	require.NoError(t, err)
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.Create(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrSlugTaken)
	mustBoard(t, r, db, id.NewID(), domain.NewLeaderboardInput{Name: "Race", Type: contracts.TypePoints})

	_, err = r.ByID(ctx, id.NewID(), lb.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = r.ByID(ctx, tenant, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

type nopOutbox struct{}

func (nopOutbox) Publish(context.Context, *gorm.DB, string, any) error { return nil }

type allowAll struct{}

func (allowAll) Authorize(context.Context, authz.Principal, authz.Permission, any) error { return nil }

// R64: incremental redis updates and a rebuild from Postgres give identical
// rankings, ties included.
func TestRebuildEqualsIncrementalAgainstRealRedis(t *testing.T) {
	r, db := setupRepo(t)
	ranks := setupRedis(t)
	ctx := context.Background()
	tenant := id.NewID()
	lb := mustBoard(t, r, db, tenant, domain.NewLeaderboardInput{Type: contracts.TypePoints, Metric: contracts.MetricNet, ResetFrequency: contracts.ResetWeekly})
	clk := clock.NewFake(t0)
	svc := app.NewService(r, ranks, nil, nopOutbox{}, allowAll{}, db, clk, nil, app.Settings{
		ClosedRetention: 48 * time.Hour, AllTimeTTL: 72 * time.Hour, CloseGrace: time.Minute, SnapshotLimit: 100, AppliedRetention: time.Hour,
	})
	p := domain.PeriodOf(lb, t0)
	require.NoError(t, svc.RebuildAll(ctx)) // marks the (empty) period ready

	players := []string{id.NewID(), id.NewID(), id.NewID(), id.NewID()}
	for i := range 40 {
		kind := domain.FactPointsCredited
		if i%5 == 0 {
			kind = domain.FactPointsDebited
		}
		f := domain.Fact{EventID: fmt.Sprint("evt-", i), TenantID: tenant, PlayerID: players[i%4], Kind: kind, Amount: int64(i%7 + 1), At: t0}
		require.NoError(t, svc.ApplyFact(ctx, f))
		require.NoError(t, svc.ApplyFact(ctx, f), "redelivery")
	}
	incremental, err := ranks.Range(ctx, p, 0, -1)
	require.NoError(t, err)
	require.Len(t, incremental, 4)

	require.NoError(t, ranks.Delete(ctx, p))
	ready, err := ranks.Ready(ctx, p)
	require.NoError(t, err)
	require.False(t, ready)
	require.NoError(t, ranks.Increment(ctx, p, players[0], 1), "writes to an unloaded period are no-ops")

	require.NoError(t, svc.RebuildAll(ctx))
	rebuilt, err := ranks.Range(ctx, p, 0, -1)
	require.NoError(t, err)
	require.Equal(t, incremental, rebuilt)

	pg, err := r.VisibleScores(ctx, lb, p.Start)
	require.NoError(t, err)
	for i := range pg {
		require.Equal(t, pg[i].PlayerID, rebuilt[i].PlayerID)
		require.Equal(t, pg[i].Score, rebuilt[i].Score)
	}
}

func TestRedisRankStoreOrderingAndCounts(t *testing.T) {
	ranks := setupRedis(t)
	ctx := context.Background()
	p := domain.Period{LeaderboardID: id.NewID(), Start: domain.AllTimeStart}
	rows := []domain.Standing{{PlayerID: "a", Score: 30}, {PlayerID: "b", Score: 20}, {PlayerID: "c", Score: 20}, {PlayerID: "d", Score: -5}}
	require.NoError(t, ranks.Replace(ctx, p, rows, time.Now().Add(time.Hour)))

	got, err := ranks.Range(ctx, p, 0, -1)
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b", "c", "d"}, []string{got[0].PlayerID, got[1].PlayerID, got[2].PlayerID, got[3].PlayerID})
	require.Equal(t, int64(-5), got[3].Score)

	above, err := ranks.CountAbove(ctx, p, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), above)

	pos, score, found, err := ranks.Position(ctx, p, "c")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(2), pos)
	require.Equal(t, int64(20), score)
	_, _, found, err = ranks.Position(ctx, p, "zz")
	require.NoError(t, err)
	require.False(t, found)

	require.NoError(t, ranks.Increment(ctx, p, "c", 15))
	pos, _, _, _ = ranks.Position(ctx, p, "c")
	require.Equal(t, int64(0), pos)

	ttl, err := ranks.rdb.PTTL(ctx, setKey(p)).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, time.Duration(0), "every redis-core key carries a TTL")

	locked, err := ranks.TryLock(ctx, p, time.Second)
	require.NoError(t, err)
	require.True(t, locked)
	locked, _ = ranks.TryLock(ctx, p, time.Second)
	require.False(t, locked)
}
