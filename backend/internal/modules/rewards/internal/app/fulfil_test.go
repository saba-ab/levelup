package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	badgescontracts "levelup/internal/modules/badges/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/id"
)

const badgeTarget = "0198d000-0000-7000-8000-00000000bad9"

var (
	topicCredit = pointscontracts.Topic(pointscontracts.JobCredit)
	topicAward  = badgescontracts.Topic(badgescontracts.JobAward)
	topicXP     = progressioncontracts.Topic(progressioncontracts.JobGrantXP)
)

func freeOf(typ string, mut func(*domain.RewardPatch)) func(*domain.RewardPatch) {
	return func(p *domain.RewardPatch) {
		p.PointsCost = ptr(int64(0))
		p.Type = ptr(typ)
		if mut != nil {
			mut(p)
		}
	}
}

func TestRedeemFulfilsByType(t *testing.T) {
	cases := []struct {
		name  string
		typ   string
		mut   func(*domain.RewardPatch)
		topic string
		check func(t *testing.T, c domain.Claim, payload any)
	}{
		{
			name: "points credits the value", typ: contracts.TypePoints,
			mut:   func(p *domain.RewardPatch) { p.Value = ptr("250") },
			topic: topicCredit,
			check: func(t *testing.T, c domain.Claim, payload any) {
				cmd := payload.(pointscontracts.CreditCmdV1)
				require.Equal(t, id.Derive("reward_fulfil", c.ID, "points"), cmd.IdempotencyKey)
				require.EqualValues(t, 250, cmd.Amount)
				require.Equal(t, pointscontracts.KindReward, cmd.Kind)
				require.Equal(t, effect.Source{Kind: effect.SourceReward, ID: c.ID}, cmd.Source)
				require.Equal(t, tenantA, cmd.TenantID)
				require.Equal(t, player1, cmd.PlayerID)
			},
		},
		{
			name: "badge awards the badge", typ: contracts.TypeBadge,
			mut:   func(p *domain.RewardPatch) { p.BadgeRewardID = ptr(badgeTarget) },
			topic: topicAward,
			check: func(t *testing.T, c domain.Claim, payload any) {
				cmd := payload.(badgescontracts.AwardCmdV1)
				require.Equal(t, id.Derive("reward_fulfil", c.ID, "badge"), cmd.IdempotencyKey)
				require.Equal(t, badgeTarget, cmd.BadgeID)
				require.Equal(t, player1, cmd.PlayerID)
			},
		},
		{
			name: "level grants the value as XP", typ: contracts.TypeLevel,
			mut:   func(p *domain.RewardPatch) { p.Value = ptr("500.00") },
			topic: topicXP,
			check: func(t *testing.T, c domain.Claim, payload any) {
				cmd := payload.(progressioncontracts.GrantXPCmdV1)
				require.Equal(t, contracts.FulfilmentKey(c.ID, contracts.FulfilLevel), cmd.IdempotencyKey)
				require.EqualValues(t, 500, cmd.Amount)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, allowAll())
			r := h.seedReward(t, tenantA, freeOf(tc.typ, tc.mut))
			c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
			require.NoError(t, err)
			require.Nil(t, c.FulfilledAt, "an HTTP claim is delivered on redeem")
			require.Zero(t, h.ob.count(tc.topic))

			h.clock.Advance(time.Minute)
			got, err := h.svc.Redeem(ctxFor(tenantA), c.ID)
			require.NoError(t, err)
			require.NotNil(t, got.FulfilledAt)
			require.Equal(t, h.clock.Now(), *got.FulfilledAt)
			require.Equal(t, got.FulfilledAt, h.claim(c.ID).FulfilledAt, "fulfilled_at is persisted")
			require.Equal(t, 1, h.ob.count(tc.topic))
			tc.check(t, got, h.ob.last(tc.topic))
			ev := h.ob.last(contracts.TopicRedeemed).(contracts.ClaimV1)
			require.Equal(t, got.FulfilledAt, ev.FulfilledAt)

			// A second redeem is refused and issues nothing more.
			_, err = h.svc.Redeem(ctxFor(tenantA), c.ID)
			require.ErrorIs(t, err, domain.ErrInvalidTransition)
			require.Equal(t, 1, h.ob.count(tc.topic))
		})
	}
}

func TestRedeemVoucherTypesIssueNoCommand(t *testing.T) {
	for _, typ := range []string{contracts.TypeDiscount, contracts.TypeItem, contracts.TypeCustom} {
		t.Run(typ, func(t *testing.T) {
			h := newHarness(t, allowAll())
			r := h.seedReward(t, tenantA, freeOf(typ, func(p *domain.RewardPatch) { p.Value = ptr("15.50") }))
			c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
			require.NoError(t, err)
			got, err := h.svc.Redeem(ctxFor(tenantA), c.ID)
			require.NoError(t, err)
			require.NotNil(t, got.FulfilledAt, "the redeemed voucher is the fulfilment")
			require.Zero(t, h.ob.count(topicCredit)+h.ob.count(topicAward)+h.ob.count(topicXP))
		})
	}
}

func TestRedeemWithoutTargetStaysUnfulfilled(t *testing.T) {
	h := newHarness(t, allowAll())
	points := h.seedReward(t, tenantA, freeOf(contracts.TypePoints, nil))
	badge := h.seedReward(t, tenantA, freeOf(contracts.TypeBadge, nil))
	for _, r := range []domain.Reward{points, badge} {
		c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
		require.NoError(t, err)
		got, err := h.svc.Redeem(ctxFor(tenantA), c.ID)
		require.NoError(t, err)
		require.Equal(t, contracts.ClaimRedeemed, got.Status)
		require.Nil(t, got.FulfilledAt)
	}
	require.Zero(t, h.ob.count(topicCredit)+h.ob.count(topicAward))
}

func TestRedeemOfPurgedRewardStillRedeems(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, freeOf(contracts.TypePoints, func(p *domain.RewardPatch) { p.Value = ptr("10") }))
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	delete(h.repo.rewards, r.ID)
	got, err := h.svc.Redeem(ctxFor(tenantA), c.ID)
	require.NoError(t, err)
	require.Nil(t, got.FulfilledAt)
	require.Zero(t, h.ob.count(topicCredit))
}

func TestPaidPointsRewardFulfilsOnRedeemAfterDebit(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.Value = ptr("1000") })
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	require.NoError(t, h.svc.OnDebited(context.Background(), debited(c)))
	require.Zero(t, h.ob.count(topicCredit), "settling the payment delivers nothing yet")
	_, err = h.svc.Redeem(ctxFor(tenantA), c.ID)
	require.NoError(t, err)
	cmd := h.ob.last(topicCredit).(pointscontracts.CreditCmdV1)
	require.EqualValues(t, 1000, cmd.Amount)
	require.NotEqual(t, c.DebitKey, cmd.IdempotencyKey)
}

func TestGrantFulfilsCommandTypesImmediately(t *testing.T) {
	cases := []struct {
		typ   string
		mut   func(*domain.RewardPatch)
		topic string
		part  string
	}{
		{contracts.TypePoints, func(p *domain.RewardPatch) { p.Value = ptr("75") }, topicCredit, contracts.FulfilPoints},
		{contracts.TypeBadge, func(p *domain.RewardPatch) { p.BadgeRewardID = ptr(badgeTarget) }, topicAward, contracts.FulfilBadge},
		{contracts.TypeLevel, func(p *domain.RewardPatch) { p.Value = ptr("40") }, topicXP, contracts.FulfilLevel},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			h := newHarness(t, allowAll())
			r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) {
				p.Type = ptr(tc.typ)
				tc.mut(p)
			})
			cmd := grantCmd(r.ID, player1, "rule:"+tc.typ)
			cmd.Source.ActivityID = "act-1"
			require.NoError(t, h.svc.Grant(context.Background(), cmd))
			require.NoError(t, h.svc.Grant(context.Background(), cmd), "redelivery")

			require.Len(t, h.repo.claims, 1)
			var c domain.Claim
			for _, x := range h.repo.claims {
				c = x
			}
			require.Equal(t, contracts.ClaimClaimed, c.Status)
			require.NotNil(t, c.FulfilledAt)
			require.Equal(t, []string{tc.topic, contracts.TopicClaimed}, h.ob.topics(), "one command, once")
			ev := h.ob.last(contracts.TopicClaimed).(contracts.ClaimV1)
			require.NotNil(t, ev.FulfilledAt)

			switch p := h.ob.last(tc.topic).(type) {
			case pointscontracts.CreditCmdV1:
				require.Equal(t, contracts.FulfilmentKey(c.ID, tc.part), p.IdempotencyKey)
				require.Equal(t, "act-1", p.Source.ActivityID)
			case badgescontracts.AwardCmdV1:
				require.Equal(t, contracts.FulfilmentKey(c.ID, tc.part), p.IdempotencyKey)
			case progressioncontracts.GrantXPCmdV1:
				require.Equal(t, contracts.FulfilmentKey(c.ID, tc.part), p.IdempotencyKey)
			default:
				t.Fatalf("unexpected payload %T", p)
			}

			// Redeeming the granted claim later delivers nothing again.
			h.ob.reset()
			_, err := h.svc.Redeem(ctxFor(tenantA), c.ID)
			require.NoError(t, err)
			require.Equal(t, []string{contracts.TopicRedeemed}, h.ob.topics())
		})
	}
}

func TestGrantOfVoucherTypeIsNotFulfilledUntilRedeem(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.Type = ptr(contracts.TypeDiscount) })
	require.NoError(t, h.svc.Grant(context.Background(), grantCmd(r.ID, player1, "rule:disc")))
	var c domain.Claim
	for _, x := range h.repo.claims {
		c = x
	}
	require.Nil(t, c.FulfilledAt)
	require.NotNil(t, c.Code)
	got, err := h.svc.Redeem(ctxFor(tenantA), c.ID)
	require.NoError(t, err)
	require.NotNil(t, got.FulfilledAt)
}

func TestRewardValueValidatedOnCreateAndUpdate(t *testing.T) {
	h := newHarness(t, allowAll())
	_, err := h.svc.CreateReward(ctxFor(tenantA), domain.RewardPatch{Name: ptr("Bonus"), Value: ptr("12.5")})
	require.ErrorIs(t, err, domain.ErrBadRewardValue)

	rw, err := h.svc.CreateReward(ctxFor(tenantA), domain.RewardPatch{Name: ptr("Bonus"), Value: ptr("12")})
	require.NoError(t, err)
	h.repo.rewards[rw.ID] = rw
	_, err = h.svc.UpdateReward(ctxFor(tenantA), rw.ID, domain.RewardPatch{Value: ptr("-1")})
	require.Error(t, err)
	_, err = h.svc.UpdateReward(ctxFor(tenantA), rw.ID, domain.RewardPatch{Value: ptr("0")})
	require.ErrorIs(t, err, domain.ErrBadRewardValue)
}

// ---- tenant-wide history and stats ----

func TestListClaimsFiltersAndPaginates(t *testing.T) {
	h := newHarness(t, allowAll())
	free := h.seedReward(t, tenantA, freeOf(contracts.TypeCustom, nil))
	paid := h.seedReward(t, tenantA, nil)
	var ids []string
	for _, pl := range []string{player1, player2, player1} {
		c, _, err := h.svc.Claim(ctxFor(tenantA), free.ID, pl, "")
		require.NoError(t, err)
		ids = append(ids, c.ID)
		h.clock.Advance(time.Minute)
	}
	pc, _, err := h.svc.Claim(ctxFor(tenantA), paid.ID, player2, "")
	require.NoError(t, err)
	_, err = h.svc.Redeem(ctxFor(tenantA), ids[0])
	require.NoError(t, err)

	all, next, err := h.svc.ListClaims(ctxFor(tenantA), ClaimFilter{}, "", 0)
	require.NoError(t, err)
	require.Len(t, all, 4)
	require.Empty(t, next)

	byPlayer, _, err := h.svc.ListClaims(ctxFor(tenantA), ClaimFilter{PlayerID: player1}, "", 0)
	require.NoError(t, err)
	require.Len(t, byPlayer, 2)

	redeemed, _, err := h.svc.ListClaims(ctxFor(tenantA), ClaimFilter{Status: contracts.ClaimRedeemed}, "", 0)
	require.NoError(t, err)
	require.Len(t, redeemed, 1)
	require.Equal(t, ids[0], redeemed[0].ID)

	byReward, _, err := h.svc.ListClaims(ctxFor(tenantA), ClaimFilter{RewardID: paid.ID}, "", 0)
	require.NoError(t, err)
	require.Len(t, byReward, 1)
	require.Equal(t, pc.ID, byReward[0].ID)

	from, to := t0.Add(time.Minute), t0.Add(2*time.Minute)
	window, _, err := h.svc.ListClaims(ctxFor(tenantA), ClaimFilter{From: &from, To: &to}, "", 0)
	require.NoError(t, err)
	require.Len(t, window, 1)
	require.Equal(t, ids[1], window[0].ID)

	page1, next, err := h.svc.ListClaims(ctxFor(tenantA), ClaimFilter{}, "", 3)
	require.NoError(t, err)
	require.Len(t, page1, 3)
	require.NotEmpty(t, next)
	page2, next, err := h.svc.ListClaims(ctxFor(tenantA), ClaimFilter{}, next, 3)
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.Empty(t, next)

	other, _, err := h.svc.ListClaims(ctxFor(tenantB), ClaimFilter{}, "", 0)
	require.NoError(t, err)
	require.Empty(t, other, "another tenant sees nothing")
}

func TestListClaimsValidatesFilterAndPermission(t *testing.T) {
	h := newHarness(t, allowAll())
	for _, f := range []ClaimFilter{
		{Status: "nope"},
		{RewardID: "x"},
		{PlayerID: "x"},
		{From: ptr(t0), To: ptr(t0)},
	} {
		_, _, err := h.svc.ListClaims(ctxFor(tenantA), f, "", 0)
		require.Error(t, err)
	}
	denied := newHarness(t, allowKeys{contracts.PermViewAny.Key(): true})
	_, _, err := denied.svc.ListClaims(ctxFor(tenantA), ClaimFilter{}, "", 0)
	require.Error(t, err)
	_, err = denied.svc.Stats(ctxFor(tenantA))
	require.Error(t, err)
}

func TestStatsCountsPerRewardAndTotals(t *testing.T) {
	h := newHarness(t, allowAll())
	paid := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.PointsCost = ptr(int64(100)) })
	free := h.seedReward(t, tenantA, freeOf(contracts.TypeCustom, func(p *domain.RewardPatch) { p.ClaimTTLDays = ptr(1) }))

	// paid: one redeemed (100 kept), one cancelled after payment (refund),
	// one rejected by points.
	for range 2 {
		c, _, err := h.svc.Claim(ctxFor(tenantA), paid.ID, player1, "")
		require.NoError(t, err)
		require.NoError(t, h.svc.OnDebited(context.Background(), debited(c)))
	}
	var paidIDs []string
	for _, c := range h.repo.claims {
		paidIDs = append(paidIDs, c.ID)
	}
	_, err := h.svc.Redeem(ctxFor(tenantA), paidIDs[0])
	require.NoError(t, err)
	_, err = h.svc.Cancel(ctxFor(tenantA), paidIDs[1])
	require.NoError(t, err)
	rej, _, err := h.svc.Claim(ctxFor(tenantA), paid.ID, player2, "")
	require.NoError(t, err)
	require.NoError(t, h.svc.OnDebitRejected(context.Background(), pointscontracts.MoveRejectedV1{
		IdempotencyKey: rej.DebitKey, TenantID: tenantA, Reason: effect.ReasonInsufficientBalance}))

	// free: one expires.
	_, _, err = h.svc.Claim(ctxFor(tenantA), free.ID, player1, "")
	require.NoError(t, err)
	h.clock.Advance(48 * time.Hour)
	require.NoError(t, h.svc.ExpireClaims(context.Background()))

	rep, err := h.svc.Stats(ctxFor(tenantA))
	require.NoError(t, err)
	byID := map[string]RewardStats{}
	for _, s := range rep.Rewards {
		byID[s.RewardID] = s
	}
	require.Equal(t, RewardStats{RewardID: paid.ID, Slug: paid.Slug, Name: paid.Name, Type: paid.Type,
		Claimed: 2, Redeemed: 1, Cancelled: 1, PointsSpent: 100}, byID[paid.ID])
	require.Equal(t, RewardStats{RewardID: free.ID, Slug: free.Slug, Name: free.Name, Type: free.Type,
		Claimed: 1, Expired: 1}, byID[free.ID])
	require.Equal(t, RewardStats{Claimed: 3, Redeemed: 1, Expired: 1, Cancelled: 1, PointsSpent: 100}, rep.Totals)

	empty, err := h.svc.Stats(ctxFor(tenantB))
	require.NoError(t, err)
	require.Empty(t, empty.Rewards)
}
