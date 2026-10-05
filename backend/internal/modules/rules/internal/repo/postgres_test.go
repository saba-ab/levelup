package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/rules/internal/app"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/modules/rules/migrations"
	authzmigrations "levelup/internal/platform/authz/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

var t0 = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "authz", authzmigrations.FS))
	require.NoError(t, postgres.Apply(ctx, dsn, "rules", migrations.FS, migrations.Go()...))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 16)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "rules")
	return NewPostgres(moduleDB), moduleDB
}

func inTx(t *testing.T, db *gorm.DB, fn func(tx *gorm.DB) error) {
	t.Helper()
	require.NoError(t, postgres.InTx(context.Background(), db, fn))
}

// seedLiveRule inserts a rule with versions 1..n and publishes `live`.
func seedLiveRule(t *testing.T, r *Postgres, db *gorm.DB, tenantID, trigger string, n, live int) (domain.Rule, []domain.Version) {
	t.Helper()
	ctx := context.Background()
	rule, err := domain.NewRule(domain.NewRuleParams{TenantID: tenantID, Name: "r " + id.NewID()[24:], TriggerEvent: trigger}, t0)
	require.NoError(t, err)
	var vs []domain.Version
	inTx(t, db, func(tx *gorm.DB) error {
		require.NoError(t, r.CreateRule(ctx, tx, rule))
		for i := 1; i <= n; i++ {
			v, err := domain.NewVersion(rule, i, json.RawMessage(`[{"source":"trigger","field":"amount","operator":"gte","value":1}]`),
				json.RawMessage(fmt.Sprintf(`[{"type":"credit_points","amount":%d}]`, i)), json.RawMessage(`{"max_per_player":2}`),
				"u", eval.Options{}, t0.Add(time.Duration(i)*time.Second))
			require.NoError(t, err)
			require.NoError(t, r.CreateVersion(ctx, tx, v))
			vs = append(vs, v)
		}
		if live > 0 {
			v := vs[live-1]
			require.NoError(t, rule.Publish(v, t0))
			v.MarkPublished(t0)
			require.NoError(t, r.PublishVersion(ctx, tx, v))
			require.NoError(t, r.SaveRule(ctx, tx, rule))
		}
		return nil
	})
	return rule, vs
}

func TestMigrationsApplyAndSeedsAreIdempotent(t *testing.T) {
	_, db := setupRepo(t)
	dsn := pgtest.DSN(t)
	require.NoError(t, postgres.Apply(context.Background(), dsn, "rules", migrations.FS, migrations.Go()...))
	var perms, grants int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM authz_svc.permissions WHERE module = 'rules'`).Scan(&perms).Error)
	require.EqualValues(t, 8, perms)
	require.NoError(t, db.Raw(`SELECT count(*) FROM authz_svc.casbin_rule WHERE v1 LIKE 'rules:%'`).Scan(&grants).Error)
	require.EqualValues(t, 4*3+4*5, grants, "admin perms → 3 admin roles, member perms → 5 member roles")
}

func TestRuleSlugUniquePerTenantAndVersionsUnique(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	rule, _ := seedLiveRule(t, r, db, tenant, "slug_test", 1, 0)

	dup := rule
	dup.ID = id.NewID()
	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateRule(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrSlugTaken)

	other := dup
	other.TenantID = id.NewID()
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateRule(ctx, tx, other) })

	v, err := domain.NewVersion(rule, 1, nil, json.RawMessage(`[{"type":"grant_xp","amount":1}]`), nil, "u", eval.Options{}, t0)
	require.NoError(t, err)
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateVersion(ctx, tx, v) })
	require.ErrorIs(t, err, domain.ErrVersionConflict)
}

// G23/G24 + R63: only the current version of live rules is loaded.
func TestLiveRuleSourcesReturnsOnlyCurrentVersionOfLiveRules(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	live, vs := seedLiveRule(t, r, db, tenant, "purchase", 3, 2)
	seedLiveRule(t, r, db, tenant, "purchase", 1, 0) // draft: not live
	inactive, _ := seedLiveRule(t, r, db, tenant, "purchase", 1, 1)
	deleted, _ := seedLiveRule(t, r, db, tenant, "purchase", 1, 1)
	seedLiveRule(t, r, db, id.NewID(), "purchase", 1, 1) // other tenant
	inTx(t, db, func(tx *gorm.DB) error {
		require.NoError(t, inactive.SetStatus(domain.StatusInactive, t0))
		require.NoError(t, r.SaveRule(ctx, tx, inactive))
		deleted.Delete(t0)
		return r.SaveRule(ctx, tx, deleted)
	})

	srcs, err := r.LiveRuleSources(ctx, tenant, "purchase")
	require.NoError(t, err)
	require.Len(t, srcs, 1)
	require.Equal(t, live.ID, srcs[0].RuleID)
	require.Equal(t, vs[1].ID, srcs[0].RuleVersionID)
	require.JSONEq(t, `[{"type":"credit_points","amount":2}]`, string(srcs[0].Actions))
	require.JSONEq(t, `{"max_per_player":2}`, string(srcs[0].Limits))
	// the stored definition compiles back
	p := eval.Compile(srcs, eval.Options{})
	require.Equal(t, 1, p.Len())
}

func TestPublishedVersionIsImmutable(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	_, vs := seedLiveRule(t, r, db, id.NewID(), "imm", 2, 1)
	edited := vs[0]
	edited.Actions = json.RawMessage(`[{"type":"grant_xp","amount":9}]`)
	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveDraftVersion(ctx, tx, edited) })
	require.ErrorIs(t, err, domain.ErrVersionPublished)

	draft := vs[1]
	draft.Actions = json.RawMessage(`[{"type":"grant_xp","amount":9}]`)
	inTx(t, db, func(tx *gorm.DB) error { return r.SaveDraftVersion(ctx, tx, draft) })
	got, err := r.LatestVersion(ctx, draft.TenantID, draft.RuleID)
	require.NoError(t, err)
	require.JSONEq(t, `[{"type":"grant_xp","amount":9}]`, string(got.Actions))

	// published_at is stamped once
	later := vs[0]
	ts := t0.Add(time.Hour)
	later.PublishedAt = &ts
	inTx(t, db, func(tx *gorm.DB) error { return r.PublishVersion(ctx, tx, later) })
	vsByID, err := r.VersionsByIDs(ctx, vs[0].TenantID, []string{vs[0].ID})
	require.NoError(t, err)
	require.True(t, vsByID[vs[0].ID].PublishedAt.Equal(t0))
}

func TestGenerationBumps(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	g, err := r.Generation(ctx, tenant)
	require.NoError(t, err)
	require.Zero(t, g)
	for want := int64(1); want <= 3; want++ {
		var got int64
		inTx(t, db, func(tx *gorm.DB) error {
			var err error
			got, err = r.BumpGeneration(ctx, tx, tenant)
			return err
		})
		require.Equal(t, want, got)
	}
	g, _ = r.Generation(ctx, tenant)
	require.EqualValues(t, 3, g)
}

func newDecision(tenant, activityID string) domain.Decision {
	return domain.Decision{ID: domain.DecisionID(activityID), TenantID: tenant, ActivityID: activityID,
		PlayerID: id.NewID(), EventType: "purchase", Outcome: "no_match", OccurredAt: t0, EvaluatedAt: t0}
}

func TestDecisionInsertIsIdempotentAndStoresJSONB(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, activityID := id.NewID(), id.NewID()
	d := newDecision(tenant, activityID)

	var first, second bool
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		first, err = r.InsertDecision(ctx, tx, d)
		require.NoError(t, err)
		ex := domain.Execution{ID: domain.ExecutionID(d.ID, "v"), TenantID: tenant, DecisionID: d.ID, RuleID: id.NewID(),
			RuleVersionID: id.NewID(), PlayerID: d.PlayerID, Status: domain.ExecFired, Matched: true, EffectsCount: 1, CreatedAt: t0,
			ConditionResults: []eval.CondTrace{{Path: "conditions[0]", Source: "trigger", Field: "amount", Operator: "gte",
				Expected: json.Number("1"), Actual: int64(5), Present: true, Result: true}}}
		require.NoError(t, r.InsertExecutions(ctx, tx, []domain.Execution{ex}))
		e := domain.Effect{ID: id.NewID(), TenantID: tenant, DecisionID: d.ID, ExecutionID: ex.ID, RuleID: ex.RuleID,
			RuleVersionID: ex.RuleVersionID, PlayerID: d.PlayerID, IdempotencyKey: "k1", Type: "credit_points",
			Params: map[string]any{"amount": int64(5)}, Target: "job.points.credit", Command: []byte(`{"amount":5}`),
			Status: domain.EffectRequested, RequestedAt: t0}
		require.NoError(t, r.InsertEffects(ctx, tx, []domain.Effect{e}))
		d.Outcome = "matched"
		return r.SetDecisionOutcome(ctx, tx, d)
	})
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		second, err = r.InsertDecision(ctx, tx, d)
		return err
	})
	require.True(t, first)
	require.False(t, second, "ON CONFLICT (activity_id) DO NOTHING")

	exists, err := r.DecisionExists(ctx, activityID)
	require.NoError(t, err)
	require.True(t, exists)

	got, err := r.DecisionByID(ctx, tenant, d.ID)
	require.NoError(t, err)
	require.Equal(t, "matched", got.Outcome)
	_, err = r.DecisionByID(ctx, id.NewID(), d.ID)
	require.ErrorIs(t, err, domain.ErrDecisionNotFound)

	execs, err := r.ExecutionsByDecision(ctx, tenant, d.ID)
	require.NoError(t, err)
	require.Len(t, execs, 1)
	require.Equal(t, "amount", execs[0].ConditionResults[0].Field)
	effects, err := r.EffectsByDecision(ctx, tenant, d.ID)
	require.NoError(t, err)
	require.JSONEq(t, `{"amount":5}`, string(effects[0].Command))
	require.EqualValues(t, 5, effects[0].Params["amount"])

	rows, err := r.ListDecisions(ctx, tenant, app.DecisionFilter{ActivityID: activityID}, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestSettleEffectOnlyMovesRequested(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	d := newDecision(tenant, id.NewID())
	ex := domain.Execution{ID: id.NewID(), TenantID: tenant, DecisionID: d.ID, RuleID: id.NewID(), RuleVersionID: id.NewID(),
		Status: domain.ExecFired, CreatedAt: t0}
	e := domain.Effect{ID: id.NewID(), TenantID: tenant, DecisionID: d.ID, ExecutionID: ex.ID, RuleID: ex.RuleID,
		RuleVersionID: ex.RuleVersionID, IdempotencyKey: "key-1", Type: "award_badge", Params: map[string]any{},
		Target: "job.badges.award", Command: []byte(`{}`), Status: domain.EffectRequested, RequestedAt: t0}
	inTx(t, db, func(tx *gorm.DB) error {
		_, err := r.InsertDecision(ctx, tx, d)
		require.NoError(t, err)
		require.NoError(t, r.InsertExecutions(ctx, tx, []domain.Execution{ex}))
		return r.InsertEffects(ctx, tx, []domain.Effect{e})
	})

	// The sweep is global by design; other tests in this package share the
	// database, so only this tenant's rows are asserted.
	pending, err := r.PendingEffects(ctx, t0.Add(time.Minute), 10, 100)
	require.NoError(t, err)
	require.Len(t, ofTenant(pending, tenant), 1)

	settle := func(status string) bool {
		var ok bool
		inTx(t, db, func(tx *gorm.DB) error {
			var err error
			ok, err = r.SettleEffect(ctx, tx, tenant, "key-1", status, "already_earned", t0)
			return err
		})
		return ok
	}
	require.True(t, settle(domain.EffectRejected))
	require.False(t, settle(domain.EffectApplied), "settled effects never move again")
	pending, err = r.PendingEffects(ctx, t0.Add(time.Minute), 10, 100)
	require.NoError(t, err)
	require.Empty(t, ofTenant(pending, tenant))
}

// Limits must hold under concurrency without any ordering (ADR-0012):
// 20 concurrent decisions for one player against max_per_player=3 fire
// exactly 3 times.
func TestApplyLimitsUnderConcurrency(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	check := app.LimitCheck{TenantID: id.NewID(), RuleID: id.NewID(), PlayerID: id.NewID(),
		Limits: eval.Limits{MaxPerPlayer: 3}, At: t0}

	var fired atomic.Int64
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			err := postgres.InTx(ctx, db, func(tx *gorm.DB) error {
				ok, err := r.ApplyLimits(ctx, tx, check)
				if ok {
					fired.Add(1)
				}
				return err
			})
			require.NoError(t, err)
		})
	}
	wg.Wait()
	require.EqualValues(t, 3, fired.Load())
}

func TestApplyLimitsIsAllOrNothingPerRule(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	c := app.LimitCheck{TenantID: id.NewID(), RuleID: id.NewID(), PlayerID: id.NewID(),
		Limits: eval.Limits{MaxPerPlayer: 10, MaxPerPlayerPerDay: 1}, At: t0}
	apply := func(c app.LimitCheck) bool {
		var ok bool
		inTx(t, db, func(tx *gorm.DB) error {
			var err error
			ok, err = r.ApplyLimits(ctx, tx, c)
			return err
		})
		return ok
	}
	require.True(t, apply(c))
	require.False(t, apply(c), "daily cap reached")

	var lifetime int64
	require.NoError(t, db.Raw(`SELECT count FROM rules_svc.rule_player_counters WHERE rule_id = ? AND window_key = 'lifetime'`, c.RuleID).Scan(&lifetime).Error)
	require.EqualValues(t, 1, lifetime, "the refused firing rolled back the lifetime increment (savepoint)")

	next := c
	next.At = t0.Add(24 * time.Hour)
	require.True(t, apply(next), "a new day opens a new window")

	cool := app.LimitCheck{TenantID: c.TenantID, RuleID: id.NewID(), PlayerID: c.PlayerID,
		Limits: eval.Limits{CooldownSeconds: 3600}, At: t0}
	require.True(t, apply(cool))
	cool.At = t0.Add(30 * time.Minute)
	require.False(t, apply(cool), "inside cooldown")
	cool.At = t0.Add(-30 * time.Minute)
	require.False(t, apply(cool), "a late older activity inside the cooldown is refused too")
	cool.At = t0.Add(2 * time.Hour)
	require.True(t, apply(cool))
}

func TestPurgeTenant(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, keep := id.NewID(), id.NewID()
	seedLiveRule(t, r, db, tenant, "p", 2, 1)
	seedLiveRule(t, r, db, keep, "p", 1, 1)
	inTx(t, db, func(tx *gorm.DB) error {
		_, err := r.InsertDecision(ctx, tx, newDecision(tenant, id.NewID()))
		require.NoError(t, err)
		_, err = r.BumpGeneration(ctx, tx, tenant)
		return err
	})
	for range 2 {
		inTx(t, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) })
	}
	rules, err := r.ListRules(ctx, tenant, app.RuleFilter{}, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, rules)
	srcs, err := r.LiveRuleSources(ctx, keep, "p")
	require.NoError(t, err)
	require.Len(t, srcs, 1, "other tenants untouched")
}

func TestRuleByIDRejectsMalformedID(t *testing.T) {
	r, _ := setupRepo(t)
	_, err := r.RuleByID(context.Background(), id.NewID(), "not-a-uuid")
	require.Equal(t, errs.NotFound, errs.KindOf(err))
}

func ofTenant(effects []domain.Effect, tenant string) []domain.Effect {
	var out []domain.Effect
	for _, e := range effects {
		if e.TenantID == tenant {
			out = append(out, e)
		}
	}
	return out
}
