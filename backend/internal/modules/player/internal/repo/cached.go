package repo

import (
	"context"

	"levelup/internal/modules/player/internal/app"
	"levelup/internal/modules/player/internal/domain"
	"levelup/internal/platform/redis"
)

// keyVersion is the invalidation escape hatch: changing the cached shape is
// a version bump, not a flush.
const keyVersion = "player:v1:"

// Cached is a read-through decorator for the batch Reader paths only — the
// ones every mechanic hits per activity. HTTP reads and every write go to
// Postgres. Eviction is called by the service AFTER commit (R44).
type Cached struct {
	*Postgres
	cache *redis.ModuleCache
}

func NewCached(inner *Postgres, cache *redis.ModuleCache) *Cached {
	return &Cached{Postgres: inner, cache: cache}
}

func idKey(tenantID, id string) string   { return keyVersion + "id:" + tenantID + ":" + id }
func extKey(tenantID, ext string) string { return keyVersion + "ext:" + tenantID + ":" + ext }

func (c *Cached) ByIDs(ctx context.Context, tenantID string, ids []string) ([]domain.Player, error) {
	return c.batch(ctx, tenantID, ids, idKey, c.Postgres.ByIDs, func(p domain.Player) string { return p.ID })
}

func (c *Cached) ByExternalIDs(ctx context.Context, tenantID string, externalIDs []string) ([]domain.Player, error) {
	return c.batch(ctx, tenantID, externalIDs, extKey, c.Postgres.ByExternalIDs,
		func(p domain.Player) string { return p.ExternalID })
}

func (c *Cached) batch(ctx context.Context, tenantID string, raw []string,
	key func(tenantID, v string) string,
	load func(ctx context.Context, tenantID string, vals []string) ([]domain.Player, error),
	natural func(domain.Player) string) ([]domain.Player, error) {

	if len(raw) == 0 {
		return []domain.Player{}, nil
	}
	keys := make([]string, len(raw))
	back := make(map[string]string, len(raw))
	for i, v := range raw {
		keys[i] = key(tenantID, v)
		back[keys[i]] = v
	}
	got, err := redis.MGetOrLoad(ctx, c.cache, keys,
		func(ctx context.Context, missing []string) (map[string]domain.Player, error) {
			vals := make([]string, len(missing))
			for i, k := range missing {
				vals[i] = back[k]
			}
			rows, err := load(ctx, tenantID, vals)
			if err != nil {
				return nil, err
			}
			out := make(map[string]domain.Player, len(rows))
			for _, p := range rows {
				out[key(tenantID, natural(p))] = p
			}
			return out, nil
		})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Player, 0, len(got))
	for _, k := range keys {
		if p, ok := got[k]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// Evict drops both lookup keys of each player, including the tombstones a
// read may have left before the player existed. Call after commit only.
func (c *Cached) Evict(ctx context.Context, tenantID string, refs ...app.Ref) {
	if len(refs) == 0 {
		return
	}
	keys := make([]string, 0, 2*len(refs))
	for _, r := range refs {
		if r.ID != "" {
			keys = append(keys, idKey(tenantID, r.ID))
		}
		if r.ExternalID != "" {
			keys = append(keys, extKey(tenantID, r.ExternalID))
		}
	}
	_ = c.cache.Del(ctx, keys...)
}
