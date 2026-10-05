package repo

import (
	"context"
	"encoding/json"
	"strconv"

	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/platform/redis"
)

// RulesetCache is the read-through cache of live rule sources on
// redis-cache (docs/examples.md §13). Keys embed the ruleset generation, so
// a publish (which bumps it in its tx) is visible to the next read without
// any eviction; Evict is post-commit hygiene for the superseded generation.
type RulesetCache struct{ cache *redis.ModuleCache }

func NewRulesetCache(c *redis.ModuleCache) *RulesetCache { return &RulesetCache{cache: c} }

// keyVersion is the shape escape hatch: changing cachedSource bumps it.
const keyVersion = "rs:v1:"

func rulesetKey(tenantID string, generation int64, trigger string) string {
	return keyVersion + tenantID + ":" + strconv.FormatInt(generation, 10) + ":" + trigger
}

type cachedSource struct {
	RuleID        string          `json:"rule_id"`
	RuleVersionID string          `json:"rule_version_id"`
	Name          string          `json:"name"`
	ProgramID     string          `json:"program_id,omitempty"`
	Priority      int             `json:"priority"`
	Conditions    json.RawMessage `json:"conditions,omitempty"`
	Actions       json.RawMessage `json:"actions"`
	Limits        json.RawMessage `json:"limits,omitempty"`
}

func (c *RulesetCache) Load(ctx context.Context, tenantID string, generation int64, trigger string,
	load func(ctx context.Context) ([]eval.RuleSource, error)) ([]eval.RuleSource, error) {

	var got []cachedSource
	err := c.cache.GetOrLoad(ctx, rulesetKey(tenantID, generation, trigger), &got, func(ctx context.Context) (any, error) {
		srcs, err := load(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]cachedSource, len(srcs))
		for i, s := range srcs {
			out[i] = cachedSource(s)
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]eval.RuleSource, len(got))
	for i, s := range got {
		out[i] = eval.RuleSource(s)
	}
	return out, nil
}

// Evict drops cached rulesets of one generation. Call after commit only (R44).
func (c *RulesetCache) Evict(ctx context.Context, tenantID string, generation int64, triggers ...string) {
	if len(triggers) == 0 {
		return
	}
	keys := make([]string, len(triggers))
	for i, t := range triggers {
		keys[i] = rulesetKey(tenantID, generation, t)
	}
	_ = c.cache.Del(ctx, keys...)
}
