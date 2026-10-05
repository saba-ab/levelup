package redis

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"myapp/internal/shared/errs"
)

// tombstone marks "the source says this does not exist" (negative cache):
// without it, every lookup of a nonexistent ID reaches Postgres, and a
// scraper enumerating IDs becomes a database load test (PRD §7.8.1).
const tombstone = "\x00nil"

const tombstoneTTL = 30 * time.Second

// GetOrLoad implements the three mandatory read-path behaviours (PRD §7.8.1):
// singleflight (5k concurrent misses → 1 load), TTL jitter (no synchronized
// expiry stampedes), and negative caching. A cache READ failure degrades to
// the loader and never fails the request — Redis being down is a latency
// event, not an availability event.
func (c *ModuleCache) GetOrLoad(ctx context.Context, key string, dst any,
	load func(context.Context) (any, error)) error {

	full := c.key(key)

	// The whole read-through sits inside singleflight, not just the load:
	// concurrent callers join one flight before touching Redis, so a burst
	// of misses is exactly one GET + one load no matter how the connection
	// pool behaves under that burst. Concurrent hits share one GET, which
	// only lowers Redis load further.
	raw, err, _ := c.sf.Do(full, func() (any, error) {
		switch b, err := c.rdb.Get(ctx, full).Bytes(); {
		case err == nil && string(b) == tombstone:
			return nil, errs.New(errs.NotFound, "not found (cached)")
		case err == nil && json.Valid(b):
			return b, nil
		case err != nil && !errors.Is(err, redis.Nil):
			c.log.Warn("cache read failed, falling through", zap.Error(err))
		}

		val, err := load(ctx)
		if errs.KindOf(err) == errs.NotFound {
			c.set(ctx, full, []byte(tombstone), tombstoneTTL)
			return nil, err
		}
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(val)
		if err != nil {
			return nil, errs.Wrap(errs.Internal, "marshal cache value", err)
		}
		c.set(ctx, full, b, jitter(c.ttl))
		return b, nil
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(raw.([]byte), dst)
}

// Del evicts keys. Callers invoke it AFTER commit, never inside the
// transaction (R44): delete-inside-tx lets a concurrent reader repopulate
// from the pre-commit row and serve stale data for a full TTL.
func (c *ModuleCache) Del(ctx context.Context, keys ...string) error {
	full := make([]string, len(keys))
	for i, k := range keys {
		full[i] = c.key(k)
	}
	if err := c.rdb.Del(ctx, full...).Err(); err != nil {
		c.log.Warn("cache eviction failed — stale until TTL", zap.Error(err))
		return err
	}
	return nil
}

// set is a best-effort write; a failed cache write only costs a future miss.
func (c *ModuleCache) set(ctx context.Context, fullKey string, b []byte, ttl time.Duration) {
	if err := c.rdb.Set(ctx, fullKey, b, ttl).Err(); err != nil {
		c.log.Warn("cache write failed", zap.Error(err))
	}
}

// jitter spreads expiry ±10% so keys written together do not expire together
// and stampede the database (PRD §7.8.1).
func jitter(d time.Duration) time.Duration {
	return d + time.Duration(rand.Int64N(int64(d/5))) - d/10
}

// MGetOrLoad is the batch read path (PRD §7.8.1): one MGET, one batched load
// for the misses, one pipelined write-back. Missing entries are tombstoned.
// The ByIDs port methods exist for exactly this (PRD §6).
func MGetOrLoad[T any](ctx context.Context, c *ModuleCache, keys []string,
	load func(ctx context.Context, missing []string) (map[string]T, error)) (map[string]T, error) {

	out := make(map[string]T, len(keys))
	if len(keys) == 0 {
		return out, nil
	}

	full := make([]string, len(keys))
	for i, k := range keys {
		full[i] = c.key(k)
	}

	var missing []string
	vals, err := c.rdb.MGet(ctx, full...).Result()
	if err != nil {
		c.log.Warn("cache mget failed, loading all", zap.Error(err))
		missing = keys
	} else {
		for i, v := range vals {
			s, ok := v.(string)
			if !ok {
				missing = append(missing, keys[i])
				continue
			}
			if s == tombstone {
				continue // known-missing: skip silently, do not reload
			}
			var t T
			if json.Unmarshal([]byte(s), &t) != nil {
				missing = append(missing, keys[i])
				continue
			}
			out[keys[i]] = t
		}
	}

	if len(missing) == 0 {
		return out, nil
	}

	loaded, err := load(ctx, missing)
	if err != nil {
		return nil, err
	}

	pipe := c.rdb.Pipeline()
	for _, k := range missing {
		t, ok := loaded[k]
		if !ok {
			pipe.Set(ctx, c.key(k), tombstone, tombstoneTTL)
			continue
		}
		out[k] = t
		if b, err := json.Marshal(t); err == nil {
			pipe.Set(ctx, c.key(k), b, jitter(c.ttl))
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.Warn("cache batch write failed", zap.Error(err))
	}
	return out, nil
}
