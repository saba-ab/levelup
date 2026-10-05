package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/streaks/internal/app"
	"levelup/internal/modules/streaks/internal/domain"
	"levelup/internal/modules/streaks/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "streaks", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "streaks")
	return NewPostgres(moduleDB), moduleDB
}

func inTx(t *testing.T, db *gorm.DB, fn func(tx *gorm.DB) error) {
	t.Helper()
	require.NoError(t, postgres.InTx(context.Background(), db, fn))
}

func seed(t *testing.T, r *Postgres, db *gorm.DB, tenantID, key string) domain.Streak {
	t.Helper()
	st, err := domain.NewStreak(tenantID, domain.NewStreakInput{
		Name: "Streak " + key, ActivityKey: key, Period: "daily", Active: true,
		Milestones: []domain.Milestone{{Count: 7, BonusPoints: 100}},
	}, time.Now().UTC().Truncate(time.Microsecond))
	require.NoError(t, err)
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateStreak(context.Background(), tx, st) })
	return st
}

func TestStreakRoundTripAndUniqueness(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	st := seed(t, r, db, tenant, "daily_login")

	got, err := r.StreakByActivityKey(ctx, tenant, "daily_login")
	require.NoError(t, err)
	require.Equal(t, st.ID, got.ID)
	require.Equal(t, []domain.Milestone{{Count: 7, BonusPoints: 100}}, got.Milestones, "JSONB round trip")

	dup := st
	dup.ID = id.NewID()
	dup.Slug = "other-slug"
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateStreak(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrActivityKeyTaken)

	_, err = r.StreakByID(ctx, id.NewID(), st.ID)
	require.ErrorIs(t, err, domain.ErrStreakNotFound, "another tenant's streak is not found")
	_, err = r.StreakByID(ctx, tenant, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrStreakNotFound)

	// Soft delete frees the key.
	inTx(t, db, func(tx *gorm.DB) error { return r.SoftDeleteStreak(ctx, tx, tenant, st.ID, time.Now()) })
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateStreak(ctx, tx, dup) })

	// Version guard.
	dup.Name = "Renamed"
	inTx(t, db, func(tx *gorm.DB) error { return r.SaveStreak(ctx, tx, dup) })
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveStreak(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrVersionConflict)

	rows, err := r.ListStreaks(ctx, tenant, app.ListFilter{}, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestBucketsRequestsAndAwardsAreIdempotent(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	st := seed(t, r, db, tenant, "k")
	now := time.Now().UTC()
	day := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)

	ps, err := domain.NewPlayerStreak(tenant, player, st.ID, now)
	require.NoError(t, err)
	var locked domain.PlayerStreak
	inTx(t, db, func(tx *gorm.DB) error {
		if err := r.EnsurePlayerStreak(ctx, tx, ps); err != nil {
			return err
		}
		other, _ := domain.NewPlayerStreak(tenant, player, st.ID, now)
		if err := r.EnsurePlayerStreak(ctx, tx, other); err != nil { // conflict: no-op
			return err
		}
		locked, err = r.PlayerStreakForUpdate(ctx, tx, tenant, player, st.ID)
		return err
	})
	require.Equal(t, ps.ID, locked.ID)

	insert := func(key string, at time.Time) bool {
		var ok bool
		inTx(t, db, func(tx *gorm.DB) error {
			var err error
			ok, err = r.InsertPeriod(ctx, tx, domain.PeriodBucket{ID: id.NewID(), TenantID: tenant, PlayerStreakID: ps.ID,
				PeriodStart: at, IdempotencyKey: key, RecordedAt: now})
			return err
		})
		return ok
	}
	require.True(t, insert("a", day))
	require.False(t, insert("b", day), "same period twice is a no-op")
	require.True(t, insert("c", day.AddDate(0, 0, -1)))

	var starts []time.Time
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		starts, err = r.PeriodStarts(ctx, tx, ps.ID)
		return err
	})
	require.Equal(t, []time.Time{day.AddDate(0, 0, -1), day}, starts)

	req := domain.RecordRequest{TenantID: tenant, IdempotencyKey: "k1", PlayerID: player, Status: domain.RequestApplied, CreatedAt: now.Add(-40 * 24 * time.Hour)}
	for i, want := range []bool{true, false} {
		inTx(t, db, func(tx *gorm.DB) error {
			ok, err := r.InsertRequest(ctx, tx, req)
			require.Equal(t, want, ok, "attempt %d", i)
			return err
		})
	}
	exists, err := r.RequestExists(ctx, tenant, "k1")
	require.NoError(t, err)
	require.True(t, exists)
	n, err := r.PruneRequests(ctx, now.Add(-30*24*time.Hour))
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	award := domain.MilestoneAward{ID: domain.MilestoneAwardID(ps.ID, 7, day), TenantID: tenant, PlayerStreakID: ps.ID,
		Milestone: 7, BonusPoints: 100, RunStartedAt: day, AwardedAt: now}
	for _, want := range []bool{true, false} {
		inTx(t, db, func(tx *gorm.DB) error {
			ok, err := r.InsertMilestoneAward(ctx, tx, award)
			require.Equal(t, want, ok)
			return err
		})
	}
	inTx(t, db, func(tx *gorm.DB) error {
		paid, err := r.AwardedMilestonesBetween(ctx, tx, ps.ID, day.AddDate(0, 0, -3), day)
		require.True(t, paid[7])
		return err
	})
}

func TestPlayerStreakSaveSweepAndPurge(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	st := seed(t, r, db, tenant, "sweep")
	now := time.Now().UTC().Truncate(time.Microsecond)
	last := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	ps, _ := domain.NewPlayerStreak(tenant, player, st.ID, now)
	inTx(t, db, func(tx *gorm.DB) error {
		if err := r.EnsurePlayerStreak(ctx, tx, ps); err != nil {
			return err
		}
		ps.CurrentCount, ps.LongestCount, ps.LastPeriodStart, ps.RunStartedAt = 3, 3, &last, &last
		return r.SavePlayerStreak(ctx, tx, ps)
	})
	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SavePlayerStreak(ctx, tx, ps) })
	require.ErrorIs(t, err, domain.ErrVersionConflict, "stale version is refused")

	refs, err := r.LiveStreakRefs(ctx)
	require.NoError(t, err)
	require.Contains(t, refs, app.StreakRef{TenantID: tenant, StreakID: st.ID})

	inTx(t, db, func(tx *gorm.DB) error {
		rows, err := r.LapsedForUpdate(ctx, tx, tenant, st.ID, last, 10)
		require.Empty(t, rows, "a bucket ON the threshold is not lapsed")
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error {
		rows, err := r.LapsedForUpdate(ctx, tx, tenant, st.ID, last.AddDate(0, 0, 1), 10)
		require.Len(t, rows, 1)
		require.Equal(t, last, *rows[0].LastPeriodStart)
		return err
	})

	got, err := r.PlayerStreaksByPlayers(ctx, tenant, []string{player, "junk"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 3, got[0].CurrentCount)

	require.NoError(t, r.MarkRun(ctx, "j", now))
	at, err := r.LastRun(ctx, "j")
	require.NoError(t, err)
	require.True(t, at.Equal(now))

	inTx(t, db, func(tx *gorm.DB) error { return r.DeletePlayer(ctx, tx, tenant, player) })
	got, err = r.PlayerStreaksByPlayers(ctx, tenant, []string{player})
	require.NoError(t, err)
	require.Empty(t, got)

	inTx(t, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) })
	inTx(t, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) })
	_, err = r.StreakByID(ctx, tenant, st.ID)
	require.ErrorIs(t, err, domain.ErrStreakNotFound)
}
