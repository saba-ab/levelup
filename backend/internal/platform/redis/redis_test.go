package redis_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"levelup/internal/platform/redis"
	"levelup/internal/platform/redis/redistest"
	"levelup/internal/shared/errs"
)

type payload struct {
	Name string `json:"name"`
}

func cacheFor(t *testing.T, module string) *redis.ModuleCache {
	t.Helper()
	client := redis.NewCache(redistest.Addr(t))
	return redis.NewModuleCache(client, module, 5*time.Minute, zap.NewNop())
}

func TestGetOrLoadCachesAfterFirstMiss(t *testing.T) {
	c := cacheFor(t, "alpha")
	var calls atomic.Int64
	load := func(context.Context) (any, error) {
		calls.Add(1)
		return payload{Name: "cached"}, nil
	}

	var got payload
	require.NoError(t, c.GetOrLoad(context.Background(), "k1", &got, load))
	require.NoError(t, c.GetOrLoad(context.Background(), "k1", &got, load))
	require.Equal(t, "cached", got.Name)
	require.EqualValues(t, 1, calls.Load(), "second read must come from cache")
}

func TestGetOrLoadNegativeCache(t *testing.T) {
	c := cacheFor(t, "beta")
	var calls atomic.Int64
	load := func(context.Context) (any, error) {
		calls.Add(1)
		return nil, errs.New(errs.NotFound, "no such row")
	}

	var got payload
	err := c.GetOrLoad(context.Background(), "missing", &got, load)
	require.Equal(t, errs.NotFound, errs.KindOf(err))

	err = c.GetOrLoad(context.Background(), "missing", &got, load)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	require.EqualValues(t, 1, calls.Load(),
		"tombstone must stop repeat lookups from reaching the loader (PRD §7.8.1)")
}

// R9: 5k concurrent misses on one key produce exactly 1 load.
func TestSingleflightCollapsesConcurrentMisses(t *testing.T) {
	c := cacheFor(t, "gamma")
	var calls atomic.Int64
	load := func(context.Context) (any, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond) // widen the window
		return payload{Name: "hot"}, nil
	}

	const n = 5000
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			var got payload
			_ = c.GetOrLoad(context.Background(), "hotkey", &got, load)
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, calls.Load(),
		"R9: 5000 concurrent misses on one key must produce exactly one load")
}

func TestModulePrefixIsolation(t *testing.T) {
	a, b := cacheFor(t, "walletiso"), cacheFor(t, "useriso")
	ctx := context.Background()

	var loads atomic.Int64
	loadA := func(context.Context) (any, error) { loads.Add(1); return payload{Name: "A"}, nil }
	loadB := func(context.Context) (any, error) { loads.Add(1); return payload{Name: "B"}, nil }

	var got payload
	require.NoError(t, a.GetOrLoad(ctx, "shared-id", &got, loadA))
	require.NoError(t, b.GetOrLoad(ctx, "shared-id", &got, loadB))
	require.EqualValues(t, 2, loads.Load(), "same logical key, different modules → different entries (R42)")
	require.Equal(t, "B", got.Name)

	// B evicting its key must not touch A's.
	require.NoError(t, b.Del(ctx, "shared-id"))
	require.NoError(t, a.GetOrLoad(ctx, "shared-id", &got, loadA))
	require.EqualValues(t, 2, loads.Load(), "A still cached after B's eviction (R42)")
}

func TestCacheReadFailureDegradesToLoader(t *testing.T) {
	addr, stop := redistest.Fresh(t)
	client := redis.NewCache(addr)
	c := redis.NewModuleCache(client, "degraded", time.Minute, zap.NewNop())

	stop() // Redis is now gone

	var got payload
	err := c.GetOrLoad(context.Background(), "k", &got, func(context.Context) (any, error) {
		return payload{Name: "from-db"}, nil
	})
	require.NoError(t, err, "a cache outage is a latency event, not an availability event (R42)")
	require.Equal(t, "from-db", got.Name)
}

func TestMGetOrLoadBatches(t *testing.T) {
	c := cacheFor(t, "batch")
	ctx := context.Background()
	var loadedKeys []string

	load := func(_ context.Context, missing []string) (map[string]payload, error) {
		loadedKeys = append(loadedKeys, missing...)
		out := map[string]payload{}
		for _, k := range missing {
			if k == "ghost" {
				continue // simulates a nonexistent row
			}
			out[k] = payload{Name: k}
		}
		return out, nil
	}

	got, err := redis.MGetOrLoad(ctx, c, []string{"x", "y", "ghost"}, load)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.ElementsMatch(t, []string{"x", "y", "ghost"}, loadedKeys)

	// Second call: x and y from cache, ghost tombstoned → loader untouched.
	loadedKeys = nil
	got, err = redis.MGetOrLoad(ctx, c, []string{"x", "y", "ghost"}, load)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Empty(t, loadedKeys, "fully cached batch must not touch the loader")
}

func TestLockAcquireIsExclusive(t *testing.T) {
	locker := redis.NewLocker(redis.NewCore(redistest.Addr(t)), zap.NewNop())
	ctx := context.Background()

	l1, err := locker.Acquire(ctx, "lock:cron:test", 30*time.Second)
	require.NoError(t, err)

	_, err = locker.Acquire(ctx, "lock:cron:test", 30*time.Second)
	require.Error(t, err, "second acquire while held must fail (fail closed)")

	require.NoError(t, l1.Release(ctx))
	l3, err := locker.Acquire(ctx, "lock:cron:test", 30*time.Second)
	require.NoError(t, err)
	_ = l3.Release(ctx)
}

func TestLockReleaseIsCompareAndDelete(t *testing.T) {
	locker := redis.NewLocker(redis.NewCore(redistest.Addr(t)), zap.NewNop())
	ctx := context.Background()

	stale, err := locker.Acquire(ctx, "lock:cron:cad", 300*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(400 * time.Millisecond) // stale's TTL expires

	fresh, err := locker.Acquire(ctx, "lock:cron:cad", 30*time.Second)
	require.NoError(t, err, "expired lock must be acquirable")

	// The stale holder releasing must NOT free the fresh holder's lock —
	// that is the double-run the lock exists to prevent (PRD §7.8.3).
	_ = stale.Release(ctx)
	_, err = locker.Acquire(ctx, "lock:cron:cad", 30*time.Second)
	require.Error(t, err, "fresh lock must survive a stale release")
	_ = fresh.Release(ctx)
}

func TestLockAcquireFailsClosedWhenRedisDown(t *testing.T) {
	addr, stop := redistest.Fresh(t)
	locker := redis.NewLocker(redis.NewCore(addr), zap.NewNop())
	stop()

	_, err := locker.Acquire(context.Background(), "lock:cron:down", time.Minute)
	require.Error(t, err, "lock acquisition error means the caller must NOT run (R43)")
}
