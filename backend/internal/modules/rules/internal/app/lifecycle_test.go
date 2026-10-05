package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/rules/contracts"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

const credit50 = `[{"type":"credit_points","amount":50}]`

func TestCreateRuleIsDraftWithVersion1(t *testing.T) {
	h := newHarness(t, allPerms)
	v, err := h.svc.CreateRule(asTenant(tenantA), CreateRuleInput{
		Name: "First Purchase Bonus", TriggerEvent: "purchase_completed", Actions: raw(credit50),
	})
	require.NoError(t, err)
	require.Equal(t, "first-purchase-bonus", v.Rule.Slug)
	require.Equal(t, domain.StatusDraft, v.Rule.Status)
	require.Equal(t, tenantA, v.Rule.TenantID, "tenant is stamped from the principal")
	require.Empty(t, v.Rule.CurrentVersionID)
	require.NotNil(t, v.Latest)
	require.Equal(t, 1, v.Latest.Version)
	require.False(t, v.Latest.Published())
	require.Equal(t, "0198d000-0000-7000-8000-00000000c0de", v.Latest.CreatedBy)
	require.Empty(t, h.ob.events, "a draft publishes nothing")
	require.Zero(t, h.repo.generations[tenantA], "a draft does not change the ruleset")
}

func TestCreateRuleRejectsBadGrammarAndStoresNothing(t *testing.T) {
	h := newHarness(t, allPerms)
	_, err := h.svc.CreateRule(asTenant(tenantA), CreateRuleInput{
		Name: "Bad", TriggerEvent: "purchase_completed",
		Conditions: raw(`[{"field":"amount","operator":"minimum","value":1}]`), Actions: raw(credit50),
	})
	isKind(t, err, errs.Invalid)
	require.Equal(t, eval.CodeInvalidDefinition, errs.CodeOf(err))
	require.Empty(t, h.repo.rules)
	require.Empty(t, h.repo.versions)
}

func TestCreateRuleValidatesFields(t *testing.T) {
	h := newHarness(t, allPerms)
	cases := map[string]CreateRuleInput{
		"invalid_trigger_event": {Name: "x", TriggerEvent: "Purchase Completed", Actions: raw(credit50)},
		"invalid_slug":          {Name: "x", Slug: "Not A Slug", TriggerEvent: "a", Actions: raw(credit50)},
		"name_required":         {Name: "  ", TriggerEvent: "a", Actions: raw(credit50)},
		"invalid_program_id":    {Name: "x", TriggerEvent: "a", ProgramID: "7", Actions: raw(credit50)},
	}
	for code, in := range cases {
		_, err := h.svc.CreateRule(asTenant(tenantA), in)
		isKind(t, err, errs.Invalid)
		require.Equal(t, code, errs.CodeOf(err))
	}
}

func TestCreateRuleSlugUniquePerTenant(t *testing.T) {
	h := newHarness(t, allPerms)
	in := CreateRuleInput{Name: "Bonus", TriggerEvent: "a", Actions: raw(credit50)}
	_, err := h.svc.CreateRule(asTenant(tenantA), in)
	require.NoError(t, err)
	_, err = h.svc.CreateRule(asTenant(tenantA), in)
	isKind(t, err, errs.AlreadyExists)
	require.Equal(t, "slug_taken", errs.CodeOf(err))
	_, err = h.svc.CreateRule(asTenant(tenantB), in)
	require.NoError(t, err, "slugs are unique per tenant only")
}

func TestAuthzDenials(t *testing.T) {
	member := allowKeys{"rules:view_any": true, "rules:view": true, "rules:simulate": true, "rules:view_decisions": true}
	h := newHarness(t, member)
	ctx := asTenant(tenantA)
	_, err := h.svc.CreateRule(ctx, CreateRuleInput{Name: "x", TriggerEvent: "a", Actions: raw(credit50)})
	isKind(t, err, errs.PermissionDenied)

	admin := newHarness(t, allPerms)
	v := admin.liveRule(t, "r", "a", 0, ``, credit50, ``)
	h.repo = admin.repo
	h.svc.repo = admin.repo
	_, err = h.svc.Publish(ctx, v.Rule.ID, 1)
	isKind(t, err, errs.PermissionDenied)
	_, err = h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{Name: ptrTo("y")})
	isKind(t, err, errs.PermissionDenied)
	isKind(t, h.svc.DeleteRule(ctx, v.Rule.ID), errs.PermissionDenied)
	_, err = h.svc.CreateVersion(ctx, v.Rule.ID, CreateVersionInput{})
	isKind(t, err, errs.PermissionDenied)
	_, err = h.svc.GetRule(ctx, v.Rule.ID)
	require.NoError(t, err, "members may read")

	// no tenant in the principal → denied, never an unscoped read
	noTenant := authz.Into(context.Background(), authz.Principal{UserID: "u"})
	_, _, err = h.svc.ListRules(noTenant, RuleFilter{}, "", 0)
	isKind(t, err, errs.PermissionDenied)
	_, err = h.svc.GetRule(context.Background(), v.Rule.ID)
	isKind(t, err, errs.Unauthenticated)
}

func TestCrossTenantRuleIs404(t *testing.T) {
	h := newHarness(t, allPerms)
	v := h.liveRule(t, "r", "a", 0, ``, credit50, ``)
	other := asTenant(tenantB)
	_, err := h.svc.GetRule(other, v.Rule.ID)
	isKind(t, err, errs.NotFound)
	require.Equal(t, "rule_not_found", errs.CodeOf(err))
	_, err = h.svc.Publish(other, v.Rule.ID, 1)
	isKind(t, err, errs.NotFound)
	_, err = h.svc.UpdateRule(other, v.Rule.ID, UpdateRuleInput{Name: ptrTo("x")})
	isKind(t, err, errs.NotFound)
	isKind(t, h.svc.DeleteRule(other, v.Rule.ID), errs.NotFound)
	_, _, err = h.svc.ListVersions(other, v.Rule.ID, "", 0)
	isKind(t, err, errs.NotFound)
	rows, _, err := h.svc.ListRules(other, RuleFilter{}, "", 0)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestPublishMakesOneLiveVersionAtomically(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := asTenant(tenantA)
	v := h.liveRule(t, "r", "purchase", 0, ``, credit50, ``)
	require.Equal(t, domain.StatusActive, v.Rule.Status)
	require.NotNil(t, v.Current)
	require.True(t, v.Current.Published())
	require.Equal(t, int64(1), h.repo.generations[tenantA])
	pubs := h.ob.byTopic(contracts.TopicVersionPublished)
	require.Len(t, pubs, 1)
	require.Equal(t, contracts.VersionPublishedV1{RuleID: v.Rule.ID, RuleVersionID: v.Current.ID, TenantID: tenantA,
		Version: 1, TriggerEvent: "purchase", At: t0.Add(1e6)}, pubs[0])

	// publishing the live version again is a no-op: no event, no bump
	_, err := h.svc.Publish(ctx, v.Rule.ID, 1)
	require.NoError(t, err)
	require.Len(t, h.ob.byTopic(contracts.TopicVersionPublished), 1)
	require.Equal(t, int64(1), h.repo.generations[tenantA])

	// G23: v2 published → only v2 is live; v1 never fires again
	v2, err := h.svc.CreateVersion(ctx, v.Rule.ID, CreateVersionInput{Actions: raw(`[{"type":"credit_points","amount":70}]`)})
	require.NoError(t, err)
	require.Equal(t, 2, v2.Version)
	srcs, err := h.repo.LiveRuleSources(ctx, tenantA, "purchase")
	require.NoError(t, err)
	require.Len(t, srcs, 1)
	require.Equal(t, v.Current.ID, srcs[0].RuleVersionID, "a draft is not live")

	pub, err := h.svc.Publish(ctx, v.Rule.ID, 2)
	require.NoError(t, err)
	require.Equal(t, v2.ID, pub.Rule.CurrentVersionID)
	srcs, _ = h.repo.LiveRuleSources(ctx, tenantA, "purchase")
	require.Len(t, srcs, 1)
	require.Equal(t, v2.ID, srcs[0].RuleVersionID)
	require.Equal(t, int64(2), h.repo.generations[tenantA])

	// rollback: re-publishing v1 keeps its original published_at
	first := *v.Current.PublishedAt
	h.clock.Advance(1e9)
	back, err := h.svc.Publish(ctx, v.Rule.ID, 1)
	require.NoError(t, err)
	require.Equal(t, first, *back.Current.PublishedAt)

	_, err = h.svc.Publish(ctx, v.Rule.ID, 9)
	isKind(t, err, errs.NotFound)
}

func TestPublishEvictsCacheAfterCommitAndDecideSeesNewVersion(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := asTenant(tenantA)
	v := h.liveRule(t, "r", "purchase", 0, ``, credit50, ``)

	require.NoError(t, h.svc.Decide(context.Background(), activity("0198d000-0000-7000-8000-00000000a001", playerP, "purchase", `{}`)))
	require.Contains(t, h.cache.entries, "0198d000-0000-7000-8000-0000000000a0:1:purchase")

	_, err := h.svc.CreateVersion(ctx, v.Rule.ID, CreateVersionInput{Actions: raw(`[{"type":"credit_points","amount":70}]`)})
	require.NoError(t, err)
	_, err = h.svc.Publish(ctx, v.Rule.ID, 2) // fakeCache fails the test if Evict runs inside tx
	require.NoError(t, err)
	require.Equal(t, []string{
		tenantA + ":0:purchase", // first publish evicted generation 0
		tenantA + ":1:purchase",
	}, h.cache.evicted)
	require.NotContains(t, h.cache.entries, tenantA+":1:purchase")

	require.NoError(t, h.svc.Decide(context.Background(), activity("0198d000-0000-7000-8000-00000000a002", playerP, "purchase", `{}`)))
	credits := h.ob.byTopic("job.points.credit")
	require.Len(t, credits, 2)
	require.EqualValues(t, 70, amountOf(credits[1]), "the next decision uses the new generation's version")
}

func TestPatchCannotTouchPublishedVersion(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := asTenant(tenantA)
	v := h.liveRule(t, "r", "purchase", 0, ``, credit50, ``)
	before := string(h.repo.versions[v.Current.ID].Actions)

	_, err := h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{Actions: raw(`[{"type":"grant_xp","amount":1}]`)})
	isKind(t, err, errs.Conflict)
	require.Equal(t, "no_draft_version", errs.CodeOf(err))
	require.Equal(t, before, string(h.repo.versions[v.Current.ID].Actions))

	draft, err := h.svc.CreateVersion(ctx, v.Rule.ID, CreateVersionInput{})
	require.NoError(t, err)
	require.JSONEq(t, before, string(draft.Actions), "omitted parts are copied")
	out, err := h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{Actions: raw(`[{"type":"grant_xp","amount":1}]`)})
	require.NoError(t, err)
	require.JSONEq(t, `[{"type":"grant_xp","amount":1}]`, string(out.Latest.Actions))
	require.Equal(t, before, string(out.Current.Actions), "live version untouched")

	_, err = h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{Conditions: raw(`[{"source":"x"}]`)})
	isKind(t, err, errs.Invalid)
}

func TestPatchStatusTransitions(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := asTenant(tenantA)
	draft, err := h.svc.CreateRule(ctx, CreateRuleInput{Name: "d", TriggerEvent: "a", Actions: raw(credit50)})
	require.NoError(t, err)
	_, err = h.svc.UpdateRule(ctx, draft.Rule.ID, UpdateRuleInput{Status: ptrTo(domain.StatusActive)})
	require.Equal(t, "publish_required", errs.CodeOf(err))

	v := h.liveRule(t, "r", "a", 0, ``, credit50, ``)
	gen := h.repo.generations[tenantA]
	out, err := h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{Status: ptrTo(domain.StatusInactive)})
	require.NoError(t, err)
	require.Equal(t, domain.StatusInactive, out.Rule.Status)
	require.Equal(t, gen+1, h.repo.generations[tenantA], "deactivation changes the ruleset")
	srcs, _ := h.repo.LiveRuleSources(ctx, tenantA, "a")
	require.Empty(t, srcs, "G24: inactive rules are not evaluated")

	out, err = h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{Status: ptrTo(domain.StatusActive)})
	require.NoError(t, err)
	require.True(t, out.Rule.Live())

	_, err = h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{Status: ptrTo(domain.StatusArchived)})
	require.NoError(t, err)
	_, err = h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{Name: ptrTo("again")})
	require.Equal(t, "rule_archived", errs.CodeOf(err))
	_, err = h.svc.Publish(ctx, v.Rule.ID, 1)
	require.Equal(t, "rule_archived", errs.CodeOf(err))
}

func TestPatchMetadataBumpsGenerationOnlyWhenLive(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := asTenant(tenantA)
	draft, err := h.svc.CreateRule(ctx, CreateRuleInput{Name: "d", TriggerEvent: "a", Actions: raw(credit50), Description: "x"})
	require.NoError(t, err)
	_, err = h.svc.UpdateRule(ctx, draft.Rule.ID, UpdateRuleInput{Priority: ptrTo(5)})
	require.NoError(t, err)
	require.Zero(t, h.repo.generations[tenantA])

	v := h.liveRule(t, "r", "a", 0, ``, credit50, ``)
	out, err := h.svc.UpdateRule(ctx, v.Rule.ID, UpdateRuleInput{TriggerEvent: ptrTo("b"), Description: OptString{Set: true, Null: true}})
	require.NoError(t, err)
	require.Equal(t, "b", out.Rule.TriggerEvent)
	require.Equal(t, int64(2), h.repo.generations[tenantA])
	require.Equal(t, []string{tenantA + ":0:a", tenantA + ":1:a", tenantA + ":1:b"}, h.cache.evicted,
		"both the old and the new trigger are evicted after commit")
}

func TestDeleteRuleRemovesFromRuleset(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := asTenant(tenantA)
	v := h.liveRule(t, "r", "a", 0, ``, credit50, ``)
	require.NoError(t, h.svc.DeleteRule(ctx, v.Rule.ID))
	srcs, _ := h.repo.LiveRuleSources(ctx, tenantA, "a")
	require.Empty(t, srcs)
	require.NotNil(t, h.repo.rules[v.Rule.ID].DeletedAt, "soft delete keeps the row")
	_, err := h.svc.GetRule(ctx, v.Rule.ID)
	isKind(t, err, errs.NotFound)
	isKind(t, h.svc.DeleteRule(ctx, v.Rule.ID), errs.NotFound)
}

func TestListRulesPaginates(t *testing.T) {
	h := newHarness(t, allPerms)
	ctx := asTenant(tenantA)
	for _, n := range []string{"a", "b", "c"} {
		_, err := h.svc.CreateRule(ctx, CreateRuleInput{Name: n, TriggerEvent: "t", Actions: raw(credit50)})
		require.NoError(t, err)
	}
	page1, next, err := h.svc.ListRules(ctx, RuleFilter{}, "", 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, next)
	page2, next2, err := h.svc.ListRules(ctx, RuleFilter{}, next, 2)
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.Empty(t, next2)
	_, _, err = h.svc.ListRules(ctx, RuleFilter{}, "%%%", 2)
	isKind(t, err, errs.Invalid)
}

func ptrTo[T any](v T) *T { return &v }
