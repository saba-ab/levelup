package app

import (
	"context"
	"strconv"

	"levelup/internal/modules/rules/internal/domain/eval"
)

// ruleset returns the compiled live program for (tenant, trigger) and the
// generation it was built at. Read path: generation (PK read) → in-process
// compiled program → redis-cache rule sources → Postgres.
func (s *Service) ruleset(ctx context.Context, tenantID, trigger string) (*eval.Program, int64, error) {
	gen, err := s.repo.Generation(ctx, tenantID)
	if err != nil {
		return nil, 0, err
	}
	key := tenantID + "|" + strconv.FormatInt(gen, 10) + "|" + trigger
	if p, ok := s.compiled.get(key); ok {
		return p, gen, nil
	}
	load := func(ctx context.Context) ([]eval.RuleSource, error) {
		return s.repo.LiveRuleSources(ctx, tenantID, trigger)
	}
	var srcs []eval.RuleSource
	if s.cache != nil {
		srcs, err = s.cache.Load(ctx, tenantID, gen, trigger, load)
	} else {
		srcs, err = load(ctx)
	}
	if err != nil {
		return nil, 0, err
	}
	p := eval.Compile(srcs, s.cfg.Compile)
	s.compiled.put(key, p)
	return p, gen, nil
}

// evictRuleset drops the superseded generation's cache entries. Called only
// after the transaction that bumped the generation committed (R44).
func (s *Service) evictRuleset(ctx context.Context, tenantID string, newGeneration int64, triggers ...string) {
	if s.cache == nil || newGeneration <= 0 {
		return
	}
	s.cache.Evict(ctx, tenantID, newGeneration-1, triggers...)
}
