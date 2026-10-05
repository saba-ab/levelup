package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/modules/progression/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "progression", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 4)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "progression")
	return NewPostgres(moduleDB), moduleDB
}

var now = time.Now().UTC().Truncate(time.Microsecond)

func inTx(t *testing.T, db *gorm.DB, fn func(tx *gorm.DB) error) {
	t.Helper()
	require.NoError(t, postgres.InTx(context.Background(), db, fn))
}

func mustLevel(t *testing.T, tenantID string, number int, xp int64) domain.Level {
	t.Helper()
	l, err := domain.NewLevel(tenantID, domain.LevelSpec{
		Number: number, XPRequired: xp, PointsReward: 5, Active: true,
		Perks: map[string]any{"discount": 0.1}, BadgeRewardID: id.NewID(),
	}, now)
	require.NoError(t, err)
	return l
}

func TestLevelRoundTripAndPartialUniqueNumber(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()

	l := mustLevel(t, tenant, 1, 0)
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateLevel(ctx, tx, l) })

	got, err := r.LevelByID(ctx, tenant, l.ID)
	require.NoError(t, err)
	require.Equal(t, l.BadgeRewardID, got.BadgeRewardID)
	require.Equal(t, map[string]any{"discount": 0.1}, got.Perks)

	_, err = r.LevelByID(ctx, id.NewID(), l.ID)
	require.ErrorIs(t, err, domain.ErrLevelNotFound, "another tenant's level is not found")

	dup := mustLevel(t, tenant, 1, 10)
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateLevel(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrLevelNumberTaken)

	inTx(t, db, func(tx *gorm.DB) error { return r.SoftDeleteLevel(ctx, tx, tenant, l.ID, now) })
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateLevel(ctx, tx, dup) })

	ladder, err := r.Ladder(ctx, nil, tenant, true)
	require.NoError(t, err)
	require.Len(t, ladder, 1)
	require.Equal(t, dup.ID, ladder[0].ID)
}

func TestInsertGrantIsIdempotentPerTenant(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()

	mk := func(tenantID string) domain.XPGrant {
		g, err := domain.NewXPGrant(domain.GrantSpec{
			TenantID: tenantID, PlayerID: player, IdempotencyKey: "same-key", Amount: 10, SourceKind: "rule",
		}, now)
		require.NoError(t, err)
		return g
	}
	var first, second, otherTenant bool
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		first, err = r.InsertGrant(ctx, tx, mk(tenant))
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		second, err = r.InsertGrant(ctx, tx, mk(tenant))
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		otherTenant, err = r.InsertGrant(ctx, tx, mk(id.NewID()))
		return err
	})
	require.True(t, first)
	require.False(t, second, "same (tenant, key) inserts nothing")
	require.True(t, otherTenant, "keys are unique per tenant")

	g, err := r.GrantByKey(ctx, nil, tenant, "same-key")
	require.NoError(t, err)
	require.Equal(t, int64(10), g.Amount)
}

func TestProgressLockAndVersion(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()

	inTx(t, db, func(tx *gorm.DB) error {
		require.NoError(t, r.EnsureProgress(ctx, tx, domain.NewProgress(tenant, player, now)))
		return r.EnsureProgress(ctx, tx, domain.NewProgress(tenant, player, now)) // no-op
	})

	var stale domain.Progress
	inTx(t, db, func(tx *gorm.DB) error {
		p, err := r.ProgressForUpdate(ctx, tx, tenant, player)
		if err != nil {
			return err
		}
		stale = p
		p.TotalXP = 40
		return r.SaveProgress(ctx, tx, p)
	})
	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveProgress(ctx, tx, stale) })
	require.ErrorIs(t, err, domain.ErrVersionConflict)

	got, err := r.ProgressByPlayers(ctx, tenant, []string{player, id.NewID()})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int64(40), got[player].TotalXP)
	require.Equal(t, 1, got[player].Version)
}

func TestLevelRewardOncePerPlayerLevel(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	l := mustLevel(t, tenant, 1, 0)
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateLevel(ctx, tx, l) })

	var a, b bool
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		a, err = r.InsertLevelReward(ctx, tx, domain.NewLevelReward(tenant, player, l.ID, now))
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		b, err = r.InsertLevelReward(ctx, tx, domain.NewLevelReward(tenant, player, l.ID, now))
		return err
	})
	require.True(t, a)
	require.False(t, b)
}

func TestRejectionOncePerKey(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	rj := domain.GrantRejection{TenantID: id.NewID(), IdempotencyKey: "k", PlayerID: id.NewID(), Reason: "player_inactive", CreatedAt: now}
	var a, b bool
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		a, err = r.InsertRejection(ctx, tx, rj)
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		b, err = r.InsertRejection(ctx, tx, rj)
		return err
	})
	require.True(t, a)
	require.False(t, b)
}

func TestListGrantsKeyset(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	for i := range 3 {
		g, err := domain.NewXPGrant(domain.GrantSpec{
			TenantID: tenant, PlayerID: player, IdempotencyKey: id.NewID(), Amount: int64(i + 1),
		}, now.Add(time.Duration(i)*time.Second))
		require.NoError(t, err)
		inTx(t, db, func(tx *gorm.DB) error { _, err := r.InsertGrant(ctx, tx, g); return err })
	}
	page, err := r.ListGrants(ctx, tenant, player, time.Time{}, "", 2)
	require.NoError(t, err)
	require.Equal(t, []int64{3, 2}, []int64{page[0].Amount, page[1].Amount})
	rest, err := r.ListGrants(ctx, tenant, player, page[1].CreatedAt, page[1].ID, 2)
	require.NoError(t, err)
	require.Len(t, rest, 1)
	require.Equal(t, int64(1), rest[0].Amount)
}

func TestReconcileQueriesAndPurge(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	since := now.Add(-time.Minute)

	g, err := domain.NewXPGrant(domain.GrantSpec{TenantID: tenant, PlayerID: player, IdempotencyKey: "k", Amount: 10}, now)
	require.NoError(t, err)
	inTx(t, db, func(tx *gorm.DB) error {
		if _, err := r.InsertGrant(ctx, tx, g); err != nil {
			return err
		}
		p := domain.NewProgress(tenant, player, now)
		if err := r.EnsureProgress(ctx, tx, p); err != nil {
			return err
		}
		p.TotalXP = 25 // drifts from the ledger (10)
		return r.SaveProgress(ctx, tx, p)
	})

	drifts, err := r.XPDrift(ctx, since)
	require.NoError(t, err)
	var found bool
	for _, d := range drifts {
		if d.PlayerID == player {
			found = true
			require.Equal(t, int64(25), d.TotalXP)
			require.Equal(t, int64(10), d.LedgerXP)
		}
	}
	require.True(t, found)

	cands, err := r.ProgressCandidates(ctx, since, "", "", 1000)
	require.NoError(t, err)
	require.NotEmpty(t, cands)

	last, err := r.LastRun(ctx)
	require.NoError(t, err)
	require.True(t, last.IsZero())
	require.NoError(t, r.MarkRun(ctx, now))
	require.NoError(t, r.MarkRun(ctx, now.Add(time.Hour)))
	last, err = r.LastRun(ctx)
	require.NoError(t, err)
	require.True(t, now.Add(time.Hour).Equal(last))

	l := mustLevel(t, tenant, 1, 0)
	inTx(t, db, func(tx *gorm.DB) error {
		if err := r.CreateLevel(ctx, tx, l); err != nil {
			return err
		}
		_, err := r.InsertLevelReward(ctx, tx, domain.NewLevelReward(tenant, player, l.ID, now))
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) })
	inTx(t, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) })

	got, err := r.ProgressByPlayers(ctx, tenant, []string{player})
	require.NoError(t, err)
	require.Empty(t, got)
	ladder, err := r.Ladder(ctx, nil, tenant, true)
	require.NoError(t, err)
	require.Empty(t, ladder)
	_, err = r.GrantByKey(ctx, nil, tenant, "k")
	require.Error(t, err)
}
