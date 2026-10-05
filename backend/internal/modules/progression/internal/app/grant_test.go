package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	badgescontracts "levelup/internal/modules/badges/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

var (
	creditTopic = pointscontracts.Topic(pointscontracts.JobCredit)
	awardTopic  = badgescontracts.Topic(badgescontracts.JobAward)
)

func grantCmd(tenantID, playerID, key string, amount int64) contracts.GrantXPCmdV1 {
	return contracts.GrantXPCmdV1{
		IdempotencyKey: key,
		TenantID:       tenantID,
		PlayerID:       playerID,
		Amount:         amount,
		Source:         effect.Source{Kind: effect.SourceRule, ID: "exec-" + key, ActivityID: "act-1"},
		OccurredAt:     testNow,
	}
}

// standardLadder: L1 starts at 0 (never rewarded), L2 gives points and a
// badge, L3 points only, L4 badge only.
func standardLadder(h *harness, tenantID string) {
	h.seedLevel(tenantID, "l1", 1, 0, 10, "")
	h.seedLevel(tenantID, "l2", 2, 100, 20, "badge-2")
	h.seedLevel(tenantID, "l3", 3, 250, 30, "")
	h.seedLevel(tenantID, "l4", 4, 500, 0, "badge-4")
}

func TestHandleGrantXPHappyPathPublishesXPGained(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	standardLadder(h, "t1")
	h.players.add("t1", "p1", true)

	res, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k1", 50))
	require.NoError(t, err)
	require.False(t, res.Replayed)
	require.Empty(t, res.LevelsReached)
	require.Equal(t, int64(50), res.Progress.TotalXP)
	require.Equal(t, 1, res.Progress.LevelNumber)

	gained := h.outbox.byTopic(contracts.TopicXPGained)
	require.Len(t, gained, 1)
	ev := gained[0].(contracts.XPGainedV1)
	require.Equal(t, "k1", ev.IdempotencyKey)
	require.Equal(t, "t1", ev.TenantID)
	require.Equal(t, int64(50), ev.TotalXP)
	require.Equal(t, 1, ev.LevelNumber)
	require.Equal(t, "act-1", ev.Source.ActivityID)
	require.Empty(t, h.outbox.byTopic(contracts.TopicLevelReached))
	require.Empty(t, h.outbox.byTopic(creditTopic))
}

func TestHandleGrantXPRedeliveryIsNoOp(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	standardLadder(h, "t1")
	h.players.add("t1", "p1", true)
	cmd := grantCmd("t1", "p1", "k1", 120)

	first, err := h.svc.HandleGrantXP(context.Background(), cmd)
	require.NoError(t, err)
	published := len(h.outbox.published)

	again, err := h.svc.HandleGrantXP(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, again.Replayed)
	require.Equal(t, first.Grant.ID, again.Grant.ID, "replay reports the original grant")
	require.Equal(t, int64(120), again.Progress.TotalXP)
	require.Len(t, h.repo.grants, 1, "one ledger row")
	require.Len(t, h.outbox.published, published, "a redelivery publishes nothing")
	require.Len(t, h.outbox.byTopic(contracts.TopicXPGained), 1)
	require.Len(t, h.outbox.byTopic(contracts.TopicLevelReached), 1)
	require.Len(t, h.outbox.byTopic(creditTopic), 1)
}

func TestMultiLevelJumpEmitsOneRewardSetPerLevel(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	standardLadder(h, "t1")
	h.players.add("t1", "p1", true)

	res, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k1", 600))
	require.NoError(t, err)
	require.Equal(t, 4, res.Progress.LevelNumber)
	require.Len(t, res.LevelsReached, 3)

	reached := h.outbox.byTopic(contracts.TopicLevelReached)
	require.Len(t, reached, 3, "l2, l3, l4; the 0-threshold start level is never rewarded")
	var numbers []int
	for _, r := range reached {
		ev := r.(contracts.LevelReachedV1)
		numbers = append(numbers, ev.LevelNumber)
		require.NotEmpty(t, ev.LevelRewardID)
		require.Equal(t, int64(600), ev.TotalXP)
	}
	require.Equal(t, []int{2, 3, 4}, numbers)

	credits := h.outbox.byTopic(creditTopic)
	require.Len(t, credits, 2, "l2 and l3 carry points; l4 has none")
	c := credits[0].(pointscontracts.CreditCmdV1)
	require.Equal(t, id.Derive("level_reward", "p1", "l2", "points"), c.IdempotencyKey)
	require.Equal(t, int64(20), c.Amount)
	require.Equal(t, pointscontracts.KindBonus, c.Kind)
	require.Equal(t, effect.SourceLevel, c.Source.Kind)
	require.Equal(t, reached[0].(contracts.LevelReachedV1).LevelRewardID, c.Source.ID)
	require.Equal(t, "t1", c.TenantID)
	require.Equal(t, int64(30), credits[1].(pointscontracts.CreditCmdV1).Amount)

	awards := h.outbox.byTopic(awardTopic)
	require.Len(t, awards, 2, "badge_reward_id is awarded (Laravel never did, B13)")
	a := awards[0].(badgescontracts.AwardCmdV1)
	require.Equal(t, "badge-2", a.BadgeID)
	require.Equal(t, id.Derive("level_reward", "p1", "l2", "badge"), a.IdempotencyKey)
	require.Equal(t, effect.SourceLevel, a.Source.Kind)
	require.Equal(t, "badge-4", awards[1].(badgescontracts.AwardCmdV1).BadgeID)
}

// R58: out-of-order (and duplicated) grants reach the same level and the
// same single reward per level.
func TestOutOfOrderGrantsConverge(t *testing.T) {
	amounts := []int64{50, 120, 30, 300}
	orders := permutations([]int{0, 1, 2, 3})
	require.Len(t, orders, 24)

	for _, order := range orders {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			h := newHarness(t, adminKeys, Options{})
			standardLadder(h, "t1")
			h.players.add("t1", "p1", true)
			for _, i := range order {
				cmd := grantCmd("t1", "p1", fmt.Sprintf("k%d", i), amounts[i])
				_, err := h.svc.HandleGrantXP(context.Background(), cmd)
				require.NoError(t, err)
				// Redeliver every command once, interleaved.
				_, err = h.svc.HandleGrantXP(context.Background(), cmd)
				require.NoError(t, err)
			}

			prog := h.repo.progress[pk("t1", "p1")]
			require.Equal(t, int64(500), prog.TotalXP)
			require.Equal(t, 4, prog.LevelNumber)
			require.Equal(t, "l4", prog.LevelID)

			reachedLevels := map[string]int{}
			for _, r := range h.outbox.byTopic(contracts.TopicLevelReached) {
				reachedLevels[r.(contracts.LevelReachedV1).LevelID]++
			}
			require.Equal(t, map[string]int{"l2": 1, "l3": 1, "l4": 1}, reachedLevels)

			creditKeys := map[string]int{}
			for _, c := range h.outbox.byTopic(creditTopic) {
				creditKeys[c.(pointscontracts.CreditCmdV1).IdempotencyKey]++
			}
			require.Equal(t, map[string]int{
				id.Derive("level_reward", "p1", "l2", "points"): 1,
				id.Derive("level_reward", "p1", "l3", "points"): 1,
			}, creditKeys)
			require.Len(t, h.outbox.byTopic(awardTopic), 2)
			require.Len(t, h.outbox.byTopic(contracts.TopicXPGained), 4)
			require.Len(t, h.repo.rewards, 3)
		})
	}
}

func TestLevelRewardNeverRepeatsAfterLadderEdit(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	standardLadder(h, "t1")
	h.players.add("t1", "p1", true)

	_, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k1", 150))
	require.NoError(t, err)
	require.Len(t, h.outbox.byTopic(contracts.TopicLevelReached), 1)

	// The admin raises l2 above the player's total; crossing it again must
	// not pay twice.
	l2 := h.repo.levels["l2"]
	l2.XPRequired = 200
	h.repo.levels["l2"] = l2
	_, err = h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k2", 60))
	require.NoError(t, err)
	require.Len(t, h.outbox.byTopic(contracts.TopicLevelReached), 1)
	require.Len(t, h.outbox.byTopic(creditTopic), 1)
}

func TestEmptyLadderGrantsWithoutError(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	h.players.add("t1", "p1", true)

	res, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k1", 75))
	require.NoError(t, err, "Laravel answered 500 here (B11)")
	require.Equal(t, int64(75), res.Progress.TotalXP)
	require.Equal(t, 0, res.Progress.LevelNumber)
	require.Equal(t, "", res.Progress.LevelID)
	require.Len(t, h.outbox.byTopic(contracts.TopicXPGained), 1)
	require.Equal(t, 0, h.outbox.byTopic(contracts.TopicXPGained)[0].(contracts.XPGainedV1).LevelNumber)
	require.Empty(t, h.outbox.byTopic(contracts.TopicLevelReached))
}

func TestInactiveLevelsAreSkipped(t *testing.T) {
	h := newHarness(t, adminKeys, Options{})
	standardLadder(h, "t1")
	l3 := h.repo.levels["l3"]
	l3.Active = false
	h.repo.levels["l3"] = l3
	h.players.add("t1", "p1", true)

	res, err := h.svc.HandleGrantXP(context.Background(), grantCmd("t1", "p1", "k1", 300))
	require.NoError(t, err)
	require.Equal(t, 2, res.Progress.LevelNumber)
	require.Len(t, h.outbox.byTopic(contracts.TopicLevelReached), 1)
}

func TestHandleGrantXPRejectsUnknownAndInactivePlayers(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(*fakePlayers)
		reason string
	}{
		{"not found", func(*fakePlayers) {}, effect.ReasonPlayerNotFound},
		{"inactive", func(p *fakePlayers) { p.add("t1", "p1", false) }, effect.ReasonPlayerInactive},
		{"other tenant's player", func(p *fakePlayers) { p.add("t2", "p1", true) }, effect.ReasonPlayerNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, adminKeys, Options{})
			standardLadder(h, "t1")
			tc.setup(h.players)
			cmd := grantCmd("t1", "p1", "k1", 500)

			res, err := h.svc.HandleGrantXP(context.Background(), cmd)
			require.NoError(t, err, "a rejection is a result, never an error")
			require.Equal(t, tc.reason, res.Rejected)
			_, err = h.svc.HandleGrantXP(context.Background(), cmd)
			require.NoError(t, err)

			rejected := h.outbox.byTopic(contracts.TopicGrantRejected)
			require.Len(t, rejected, 1, "a redelivered rejection publishes once")
			ev := rejected[0].(contracts.GrantRejectedV1)
			require.Equal(t, tc.reason, ev.Reason)
			require.Equal(t, "k1", ev.IdempotencyKey)
			require.Empty(t, h.repo.grants)
			require.Empty(t, h.outbox.byTopic(contracts.TopicXPGained))
		})
	}
}

func TestHandleGrantXPMalformedCommandIsInvalid(t *testing.T) {
	cases := map[string]contracts.GrantXPCmdV1{
		"zero amount": grantCmd("t1", "p1", "k1", 0),
		"no key":      grantCmd("t1", "p1", "", 10),
		"no tenant":   grantCmd("", "p1", "k1", 10),
		"no player":   grantCmd("t1", "", "k1", 10),
		"negative xp": grantCmd("t1", "p1", "k1", -10),
	}
	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, adminKeys, Options{})
			h.players.add("t1", "p1", true)
			_, err := h.svc.HandleGrantXP(context.Background(), cmd)
			require.Equal(t, errs.Invalid, errs.KindOf(err), "malformed commands go straight to the DLQ")
			require.Empty(t, h.outbox.published)
		})
	}
}

func TestManualGrantXP(t *testing.T) {
	h := newHarness(t, memberKeys, Options{})
	standardLadder(h, "t1")
	h.players.add("t1", "p1", true)
	ctx := asUser("t1")

	res, err := h.svc.GrantXP(ctx, ManualGrant{PlayerID: "p1", Amount: 120, Description: "bonus", IdempotencyKey: "abc"})
	require.NoError(t, err)
	require.Equal(t, "manual:abc", res.Grant.IdempotencyKey)
	require.Equal(t, effect.SourceManual, res.Grant.SourceKind)
	require.Equal(t, "user-1", res.Grant.SourceID)
	require.Equal(t, "user-1", res.Grant.CreatedBy)
	require.Len(t, res.LevelsReached, 1)

	again, err := h.svc.GrantXP(ctx, ManualGrant{PlayerID: "p1", Amount: 120, IdempotencyKey: "abc"})
	require.NoError(t, err)
	require.True(t, again.Replayed)
	require.Len(t, h.repo.grants, 1)

	// Without a key, each request is a distinct grant.
	_, err = h.svc.GrantXP(ctx, ManualGrant{PlayerID: "p1", Amount: 1})
	require.NoError(t, err)
	_, err = h.svc.GrantXP(ctx, ManualGrant{PlayerID: "p1", Amount: 1})
	require.NoError(t, err)
	require.Len(t, h.repo.grants, 3)
}

func TestManualGrantXPErrors(t *testing.T) {
	h := newHarness(t, allowKeys{"progression:view": true}, Options{})
	h.players.add("t1", "p1", true)

	_, err := h.svc.GrantXP(asUser("t1"), ManualGrant{PlayerID: "p1", Amount: 5})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	_, err = h.svc.GrantXP(context.Background(), ManualGrant{PlayerID: "p1", Amount: 5})
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))

	_, err = h.svc.GrantXP(asUser(""), ManualGrant{PlayerID: "p1", Amount: 5})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err), "no tenant claim → 403, never an unscoped write")

	h = newHarness(t, memberKeys, Options{})
	h.players.add("t1", "inactive", false)
	h.players.add("t2", "foreign", true)

	_, err = h.svc.GrantXP(asUser("t1"), ManualGrant{PlayerID: "foreign", Amount: 5})
	require.ErrorIs(t, err, domain.ErrPlayerNotFound, "another tenant's player is 404")
	require.Equal(t, "player_not_found", errs.CodeOf(err))

	_, err = h.svc.GrantXP(asUser("t1"), ManualGrant{PlayerID: "inactive", Amount: 5})
	require.ErrorIs(t, err, domain.ErrPlayerInactive)

	h.players.add("t1", "p1", true)
	_, err = h.svc.GrantXP(asUser("t1"), ManualGrant{PlayerID: "p1", Amount: 0})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	require.Empty(t, h.outbox.published)
}

func permutations(xs []int) [][]int {
	if len(xs) <= 1 {
		return [][]int{append([]int(nil), xs...)}
	}
	var out [][]int
	for i := range xs {
		rest := make([]int, 0, len(xs)-1)
		rest = append(rest, xs[:i]...)
		rest = append(rest, xs[i+1:]...)
		for _, p := range permutations(rest) {
			out = append(out, append([]int{xs[i]}, p...))
		}
	}
	return out
}
