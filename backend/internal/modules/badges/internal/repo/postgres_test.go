package repo

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/badges/internal/app"
	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/modules/badges/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "badges", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 8)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "badges")
	return NewPostgres(moduleDB), moduleDB
}

func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func seedBadge(t *testing.T, r *Postgres, db *gorm.DB, tenantID string, mut func(*domain.NewBadgeParams)) domain.Badge {
	t.Helper()
	p := domain.NewBadgeParams{TenantID: tenantID, Name: "Badge " + id.NewID(), Tier: "gold", Category: "skill", Active: true,
		Requirements: map[string]any{"missions": 3.0}}
	if mut != nil {
		mut(&p)
	}
	b, err := domain.NewBadge(p, now())
	require.NoError(t, err)
	require.NoError(t, postgres.InTx(context.Background(), db, func(tx *gorm.DB) error { return r.CreateBadge(context.Background(), tx, b) }))
	return b
}

func TestBadgeRoundTripSlugAndVersion(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	b := seedBadge(t, r, db, tenant, nil)

	got, err := r.BadgeByID(ctx, tenant, b.ID)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"missions": 3.0}, got.Requirements, "jsonb round-trips as a string param")
	_, err = r.BadgeByID(ctx, id.NewID(), b.ID)
	require.ErrorIs(t, err, domain.ErrBadgeNotFound, "another tenant's badge is not found")

	dup := b
	dup.ID = id.NewID()
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateBadge(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrSlugTaken)

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveBadge(ctx, tx, got) }))
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveBadge(ctx, tx, got) })
	require.ErrorIs(t, err, domain.ErrVersionConflict, "a stale write is refused")

	// Soft-delete frees the slug.
	fresh, err := r.BadgeByID(ctx, tenant, b.ID)
	require.NoError(t, err)
	deleted := now()
	fresh.DeletedAt = &deleted
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveBadge(ctx, tx, fresh) }))
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateBadge(ctx, tx, dup) }))
}

func TestListBadgesKeyset(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	for range 3 {
		seedBadge(t, r, db, tenant, nil)
	}
	seedBadge(t, r, db, tenant, func(p *domain.NewBadgeParams) { p.Tier = "bronze" })

	page, err := r.ListBadges(ctx, tenant, app.BadgeFilter{Tier: "gold", Limit: 2})
	require.NoError(t, err)
	require.Len(t, page, 2)
	rest, err := r.ListBadges(ctx, tenant, app.BadgeFilter{Tier: "gold", Limit: 2, Before: page[1].CreatedAt, BeforeID: page[1].ID})
	require.NoError(t, err)
	require.Len(t, rest, 1)
}

func TestInsertAwardIsIdempotentPerTenant(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	a := domain.Award{ID: id.NewID(), TenantID: tenant, PlayerID: id.NewID(), BadgeID: id.NewID(),
		IdempotencyKey: "k", Source: effect.Source{Kind: effect.SourceRule, ID: "e1"}, OccurredAt: now(),
		Status: domain.AwardRejected, Reason: effect.ReasonTargetNotFound, CreatedAt: now()}

	insert := func(a domain.Award) bool {
		var ok bool
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			var err error
			ok, err = r.InsertAward(ctx, tx, a)
			return err
		}))
		return ok
	}
	require.True(t, insert(a))
	again := a
	again.ID = id.NewID()
	require.False(t, insert(again), "same (tenant, key) inserts nothing")
	other := a
	other.ID, other.TenantID = id.NewID(), id.NewID()
	require.True(t, insert(other), "keys are scoped per tenant")

	got, found, err := r.AwardByKey(ctx, tenant, "k")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, a.ID, got.ID)
	require.Equal(t, a.Source, got.Source)
}

// Concurrent awards of one stackable badge serialize on the holding row:
// the count never exceeds max_awards and every applied stack has a ledger
// row (B16).
func TestConcurrentAwardsNeverExceedMaxAwards(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	maxAwards := 3
	b := seedBadge(t, r, db, tenant, func(p *domain.NewBadgeParams) { p.Stackable = true; p.MaxAwards = &maxAwards })

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for range 10 {
		wg.Go(func() {
			err := postgres.InTx(ctx, db, func(tx *gorm.DB) error {
				pb, err := r.LockOrCreatePlayerBadge(ctx, tx, domain.NewPlayerBadge(tenant, player, b.ID, now()))
				if err != nil {
					return err
				}
				if b.CanAward(pb.EarnedCount) != "" {
					return nil
				}
				pb.Award(now())
				if _, err := r.InsertAward(ctx, tx, domain.Award{ID: id.NewID(), TenantID: tenant, PlayerID: player,
					BadgeID: b.ID, PlayerBadgeID: pb.ID, IdempotencyKey: id.NewID(), OccurredAt: now(),
					Status: domain.AwardApplied, EarnedCount: pb.EarnedCount, CreatedAt: now()}); err != nil {
					return err
				}
				return r.SavePlayerBadge(ctx, tx, pb)
			})
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	require.Empty(t, errs)

	pb, found, err := r.PlayerBadge(ctx, tenant, player, b.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 3, pb.EarnedCount)

	drift, err := r.DriftSince(ctx, time.Time{})
	require.NoError(t, err)
	for _, d := range drift {
		require.NotEqual(t, pb.ID, d.PlayerBadgeID, "the holding is consistent with its ledger")
	}
}

func TestDriftSinceFindsMismatches(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	b := seedBadge(t, r, db, tenant, nil)
	start := now()

	// A holding with count 2 but no ledger rows on a non-stackable badge.
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		pb, err := r.LockOrCreatePlayerBadge(ctx, tx, domain.NewPlayerBadge(tenant, player, b.ID, now()))
		if err != nil {
			return err
		}
		pb.Award(now())
		pb.Award(now())
		return r.SavePlayerBadge(ctx, tx, pb)
	}))

	drift, err := r.DriftSince(ctx, start)
	require.NoError(t, err)
	var mine *domain.Drift
	for i := range drift {
		if drift[i].TenantID == tenant {
			mine = &drift[i]
		}
	}
	require.NotNil(t, mine)
	require.ElementsMatch(t, []string{domain.DriftCountMismatch, domain.DriftOverMax}, mine.Checks())

	require.NoError(t, r.MarkRun(ctx, start))
	require.NoError(t, r.MarkRun(ctx, start.Add(time.Second)))
	last, err := r.LastRun(ctx)
	require.NoError(t, err)
	require.True(t, start.Add(time.Second).Equal(last))
}

func TestPurgeTenantIsIdempotentAndScoped(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	gone, kept, player := id.NewID(), id.NewID(), id.NewID()
	b := seedBadge(t, r, db, gone, nil)
	k := seedBadge(t, r, db, kept, nil)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		_, err := r.LockOrCreatePlayerBadge(ctx, tx, domain.NewPlayerBadge(gone, player, b.ID, now()))
		return err
	}))

	for range 2 {
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, gone) }))
	}
	_, err := r.BadgeByID(ctx, gone, b.ID)
	require.ErrorIs(t, err, domain.ErrBadgeNotFound)
	_, found, err := r.PlayerBadge(ctx, gone, player, b.ID)
	require.NoError(t, err)
	require.False(t, found)
	_, err = r.BadgeByID(ctx, kept, k.ID)
	require.NoError(t, err)
}
