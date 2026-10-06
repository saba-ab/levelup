package repo

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/ai/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "ai", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 8)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "ai")
	return NewPostgres(moduleDB), moduleDB
}

func reserve(t *testing.T, r *Postgres, db *gorm.DB, tenant string, day time.Time, limit int) (int64, bool) {
	t.Helper()
	var used int64
	var ok bool
	require.NoError(t, postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		var err error
		used, ok, err = r.Reserve(context.Background(), tx, tenant, day, limit, time.Now().UTC())
		return err
	}))
	return used, ok
}

func TestReserveCapsRequestsPerTenantPerDay(t *testing.T) {
	r, db := setupRepo(t)
	tenant, other := id.NewID(), id.NewID()
	day := time.Date(2026, 10, 6, 15, 30, 0, 0, time.UTC)

	for want := int64(1); want <= 3; want++ {
		used, ok := reserve(t, r, db, tenant, day, 3)
		require.True(t, ok)
		require.Equal(t, want, used)
	}
	_, ok := reserve(t, r, db, tenant, day, 3)
	require.False(t, ok, "fourth request over a limit of 3")

	used, ok := reserve(t, r, db, other, day, 3)
	require.True(t, ok, "other tenants are independent")
	require.Equal(t, int64(1), used)
	used, ok = reserve(t, r, db, tenant, day.AddDate(0, 0, 1), 3)
	require.True(t, ok, "a new UTC day resets the count")
	require.Equal(t, int64(1), used)

	for range 5 {
		_, ok = reserve(t, r, db, tenant, day, 0)
		require.True(t, ok, "limit 0 is unlimited")
	}
}

func TestReserveIsAtomicUnderConcurrency(t *testing.T) {
	r, db := setupRepo(t)
	tenant := id.NewID()
	day := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)

	const workers, limit = 20, 7
	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0
	for range workers {
		wg.Go(func() {
			_ = postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
				_, ok, err := r.Reserve(context.Background(), tx, tenant, day, limit, time.Now().UTC())
				if ok {
					mu.Lock()
					granted++
					mu.Unlock()
				}
				return err
			})
		})
	}
	wg.Wait()
	require.Equal(t, limit, granted)

	rows, err := r.UsageBetween(context.Background(), tenant, day, day)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(limit), rows[0].Requests)
}

func TestAddTokensUsageAndPurge(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, other := id.NewID(), id.NewID()
	d1 := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 10, 6, 22, 0, 0, 0, time.UTC)

	reserve(t, r, db, tenant, d1, 10)
	reserve(t, r, db, tenant, d2, 10)
	reserve(t, r, db, other, d2, 10)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		if err := r.AddTokens(ctx, tx, tenant, d2, 100, 40, time.Now().UTC()); err != nil {
			return err
		}
		if err := r.AddTokens(ctx, tx, tenant, d2, 50, 10, time.Now().UTC()); err != nil {
			return err
		}
		// Tokens for a day without a reservation still land (upsert).
		return r.AddTokens(ctx, tx, tenant, d1.AddDate(0, 0, -10), 5, 5, time.Now().UTC())
	}))

	rows, err := r.UsageBetween(ctx, tenant, d1, d2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), rows[0].Day, "newest first, truncated to the day")
	require.Equal(t, int64(1), rows[0].Requests)
	require.Equal(t, int64(150), rows[0].InputTokens)
	require.Equal(t, int64(50), rows[0].OutputTokens)
	require.Equal(t, int64(0), rows[1].InputTokens)

	rows, err = r.UsageBetween(ctx, "not-a-uuid", d1, d2)
	require.NoError(t, err)
	require.Empty(t, rows)

	for range 2 { // idempotent under redelivery
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) }))
	}
	rows, err = r.UsageBetween(ctx, tenant, d1.AddDate(0, 0, -30), d2)
	require.NoError(t, err)
	require.Empty(t, rows)
	rows, err = r.UsageBetween(ctx, other, d1, d2)
	require.NoError(t, err)
	require.Len(t, rows, 1, "other tenants are untouched")
}
