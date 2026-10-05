package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/player/internal/app"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/redis"
	"levelup/internal/platform/redis/redistest"
	"levelup/internal/shared/id"
)

// A Reader miss tombstones the external id; the post-commit eviction on
// create is what makes the new player visible to ingestion at once.
func TestCachedReaderTombstoneEvictedAfterCreate(t *testing.T) {
	pg, db := setupRepo(t)
	cache := redis.NewModuleCache(redis.NewCache(redistest.Addr(t)), "player", time.Minute, zap.NewNop())
	c := NewCached(pg, cache)
	ctx := context.Background()
	tenant := id.NewID()

	got, err := c.ByExternalIDs(ctx, tenant, []string{"late"})
	require.NoError(t, err)
	require.Empty(t, got)

	p := newPlayer(t, tenant, "late", base)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return c.Create(ctx, tx, p) }))

	got, err = c.ByExternalIDs(ctx, tenant, []string{"late"})
	require.NoError(t, err)
	require.Empty(t, got, "tombstone still served before eviction")

	c.Evict(ctx, tenant, app.Ref{ID: p.ID, ExternalID: p.ExternalID})
	got, err = c.ByExternalIDs(ctx, tenant, []string{"late"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, p.Attributes, got[0].Attributes, "cached JSON round trip")

	byID, err := c.ByIDs(ctx, id.NewID(), []string{p.ID})
	require.NoError(t, err)
	require.Empty(t, byID, "cache keys carry the tenant")
}
