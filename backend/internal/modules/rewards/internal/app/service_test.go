package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/modules/rewards/internal/ports"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
)

var (
	topicDebit  = pointscontracts.Topic(pointscontracts.JobDebit)
	topicRefund = pointscontracts.Topic(pointscontracts.JobRefund)
)

func debited(c domain.Claim) pointscontracts.LedgerMovedV1 {
	return pointscontracts.LedgerMovedV1{IdempotencyKey: c.DebitKey, TenantID: c.TenantID, PlayerID: c.PlayerID, Amount: c.PointsCost}
}

// ---- catalogue ----

func TestCreateRewardRequiresPermission(t *testing.T) {
	h := newHarness(t, allowKeys{contracts.PermView.Key(): true})
	_, err := h.svc.CreateReward(ctxFor(tenantA), domain.RewardPatch{Name: ptr("Mug")})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Empty(t, h.repo.rewards)
}

func TestCreateRewardStampsTenantAndRejectsDuplicateSlug(t *testing.T) {
	h := newHarness(t, allowAll())
	r, err := h.svc.CreateReward(ctxFor(tenantA), domain.RewardPatch{Name: ptr("Coffee Mug")})
	require.NoError(t, err)
	require.Equal(t, tenantA, r.TenantID)
	require.Equal(t, "coffee-mug", r.Slug)

	_, err = h.svc.CreateReward(ctxFor(tenantA), domain.RewardPatch{Name: ptr("Coffee Mug")})
	require.ErrorIs(t, err, domain.ErrSlugTaken)
	_, err = h.svc.CreateReward(ctxFor(tenantB), domain.RewardPatch{Name: ptr("Coffee Mug")})
	require.NoError(t, err, "slugs are unique per tenant")
}

func TestRequiresTenant(t *testing.T) {
	h := newHarness(t, allowAll())
	_, err := h.svc.GetReward(context.Background(), "x")
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
	_, err = h.svc.GetReward(ctxFor(""), "x")
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestCrossTenantRewardIsNotFound(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, nil)
	_, err := h.svc.GetReward(ctxFor(tenantB), r.ID)
	require.ErrorIs(t, err, domain.ErrRewardNotFound)
	_, err = h.svc.UpdateReward(ctxFor(tenantB), r.ID, domain.RewardPatch{Name: ptr("x")})
	require.ErrorIs(t, err, domain.ErrRewardNotFound)
	require.ErrorIs(t, h.svc.DeleteReward(ctxFor(tenantB), r.ID), domain.ErrRewardNotFound)
	_, _, err = h.svc.Claim(ctxFor(tenantB), r.ID, player1, "")
	require.Error(t, err)
}

func TestUpdateRewardIsPartial(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.Description = ptr("keep me") })
	got, err := h.svc.UpdateReward(ctxFor(tenantA), r.ID, domain.RewardPatch{PointsCost: ptr(int64(250))})
	require.NoError(t, err)
	require.EqualValues(t, 250, got.PointsCost)
	require.Equal(t, "keep me", got.Description)
}

func TestListRewardsPaginates(t *testing.T) {
	h := newHarness(t, allowAll())
	for range 3 {
		h.seedReward(t, tenantA, nil)
	}
	h.seedReward(t, tenantB, nil)
	page, next, err := h.svc.ListRewards(ctxFor(tenantA), RewardFilter{}, "", 2)
	require.NoError(t, err)
	require.Len(t, page, 2)
	require.NotEmpty(t, next)
	page, next, err = h.svc.ListRewards(ctxFor(tenantA), RewardFilter{}, next, 2)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Empty(t, next)
}

// ---- claim (tx1) ----

func TestFreeRewardIsClaimedImmediately(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) {
		p.PointsCost = ptr(int64(0))
		p.Type = ptr(contracts.TypeDiscount)
		p.ClaimTTLDays = ptr(30)
	})
	c, created, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, contracts.ClaimClaimed, c.Status)
	require.NotNil(t, c.Code, "discount rewards carry a voucher code")
	require.Equal(t, t0.AddDate(0, 0, 30), *c.ExpiresAt)
	require.Equal(t, []string{contracts.TopicClaimed}, h.ob.topics(), "no debit for a free reward")
	require.Equal(t, 1, h.reward(r.ID).StockUsed)
}

func TestPaidClaimHoldsStockAndIssuesDebit(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, nil)
	c, created, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "req-1")
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, contracts.ClaimPendingPayment, c.Status)
	require.Equal(t, t0.Add(10*time.Minute), *c.HoldExpiresAt)
	require.Equal(t, []string{topicDebit, contracts.TopicClaimRequested}, h.ob.topics())

	cmd := h.ob.last(topicDebit).(pointscontracts.DebitCmdV1)
	require.Equal(t, contracts.DebitKey(c.ID), cmd.IdempotencyKey)
	require.Equal(t, pointscontracts.KindRedeem, cmd.Kind)
	require.EqualValues(t, 100, cmd.Amount)
	require.Equal(t, tenantA, cmd.TenantID)
	require.Equal(t, effect.Source{Kind: effect.SourceReward, ID: c.ID}, cmd.Source)
	require.Equal(t, 1, h.reward(r.ID).StockUsed)
}

// R3: retrying a POST with the same Idempotency-Key must not double-charge.
func TestIdempotencyKeyReplayReturnsSameClaim(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, nil)
	first, created, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "req-1")
	require.NoError(t, err)
	require.True(t, created)

	again, created, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "req-1")
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, first.ID, again.ID)
	require.Equal(t, 1, h.ob.count(topicDebit), "one debit command only")
	require.Equal(t, 1, h.reward(r.ID).StockUsed)
	require.Len(t, h.repo.claims, 1)

	_, _, err = h.svc.Claim(ctxFor(tenantA), r.ID, player2, "req-1")
	require.Equal(t, "idempotency_key_reused", errs.CodeOf(err))
}

// The concurrent-replay path: the duplicate insert rolls back the stock
// increment and the existing claim is returned.
func TestDuplicateInsertRollsBackStock(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, nil)
	first, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "req-1")
	require.NoError(t, err)
	h.ob.reset()

	// Simulate the race: the pre-check misses, the insert hits the index.
	h.repo.missClientRequestOnce = true
	got, created, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "req-1")
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, first.ID, got.ID)
	require.Empty(t, h.ob.topics())
	require.Equal(t, 1, h.reward(r.ID).StockUsed)
}

// R1: outstanding (pending) claims hold stock, so the last unit cannot be
// sold twice.
func TestLastUnitCannotBeOversold(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.MaxRedemptions = ptr(1) })
	_, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	_, _, err = h.svc.Claim(ctxFor(tenantA), r.ID, player2, "")
	require.ErrorIs(t, err, domain.ErrRewardDepleted)
	require.Equal(t, contracts.RewardDepleted, h.reward(r.ID).Status)
}

func TestPerPlayerLimitCountsPendingClaims(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.MaxPerPlayer = ptr(1) })
	_, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	_, _, err = h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.ErrorIs(t, err, domain.ErrPlayerLimitReached)
	_, _, err = h.svc.Claim(ctxFor(tenantA), r.ID, player2, "")
	require.NoError(t, err)
	require.Equal(t, 2, h.reward(r.ID).StockUsed, "the refused claim held no stock")
}

func TestLevelRequirementIsEnforced(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.LevelRequirement = ptr(5) })
	_, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player2, "") // no progression → level 0
	require.ErrorIs(t, err, domain.ErrLevelTooLow)
	_, _, err = h.svc.Claim(ctxFor(tenantA), r.ID, player1, "") // level 5
	require.NoError(t, err)
}

func TestClaimRejections(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, nil)
	draft := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.Status = ptr(contracts.RewardDraft) })

	_, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, "0198d000-0000-7000-8000-000000000099", "")
	require.ErrorIs(t, err, domain.ErrPlayerNotFound)
	_, _, err = h.svc.Claim(ctxFor(tenantA), r.ID, "0198d000-0000-7000-8000-0000000000ff", "")
	require.ErrorIs(t, err, domain.ErrPlayerInactive)
	_, _, err = h.svc.Claim(ctxFor(tenantA), draft.ID, player1, "")
	require.ErrorIs(t, err, domain.ErrRewardNotAvailable)
	_, _, err = h.svc.Claim(ctxFor(tenantA), "0198d000-0000-7000-8000-000000000123", player1, "")
	require.ErrorIs(t, err, domain.ErrRewardNotFound)
	require.Empty(t, h.ob.topics())
}

func TestClaimRequiresPermission(t *testing.T) {
	h := newHarness(t, allowKeys{})
	r := h.seedReward(t, tenantA, nil)
	_, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

// ---- settlement (tx2) ----

func TestDebitedSettlesClaimedOnceDespiteRedelivery(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) {
		p.Type = ptr(contracts.TypeItem)
		p.ClaimTTLDays = ptr(7)
	})
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	h.ob.reset()

	h.clock.Advance(time.Minute)
	require.NoError(t, h.svc.OnDebited(context.Background(), debited(c)))
	require.NoError(t, h.svc.OnDebited(context.Background(), debited(c)))

	got := h.claim(c.ID)
	require.Equal(t, contracts.ClaimClaimed, got.Status)
	require.NotNil(t, got.Code)
	require.Equal(t, t0.Add(time.Minute).AddDate(0, 0, 7), *got.ExpiresAt)
	require.Equal(t, []string{contracts.TopicClaimed}, h.ob.topics(), "duplicate settle publishes nothing")
	ev := h.ob.last(contracts.TopicClaimed).(contracts.ClaimV1)
	require.Equal(t, c.DebitKey, ev.DebitKey)
	require.Equal(t, *got.Code, ev.Code)

	// A rejection arriving after the debit is ignored.
	require.NoError(t, h.svc.OnDebitRejected(context.Background(), pointscontracts.MoveRejectedV1{
		IdempotencyKey: c.DebitKey, TenantID: tenantA, Reason: effect.ReasonInsufficientBalance}))
	require.Equal(t, contracts.ClaimClaimed, h.claim(c.ID).Status)
}

func TestDebitRejectedReleasesStockOnce(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.MaxRedemptions = ptr(1) })
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	require.Equal(t, contracts.RewardDepleted, h.reward(r.ID).Status)
	h.ob.reset()

	ev := pointscontracts.MoveRejectedV1{IdempotencyKey: c.DebitKey, TenantID: tenantA, Reason: effect.ReasonInsufficientBalance}
	require.NoError(t, h.svc.OnDebitRejected(context.Background(), ev))
	require.NoError(t, h.svc.OnDebitRejected(context.Background(), ev))

	got := h.claim(c.ID)
	require.Equal(t, contracts.ClaimRejected, got.Status)
	require.Equal(t, effect.ReasonInsufficientBalance, got.RejectReason)
	require.Equal(t, 0, h.reward(r.ID).StockUsed)
	require.Equal(t, contracts.RewardActive, h.reward(r.ID).Status)
	require.Equal(t, []string{contracts.TopicClaimRejected}, h.ob.topics())
	require.Equal(t, effect.ReasonInsufficientBalance, h.ob.last(contracts.TopicClaimRejected).(contracts.ClaimV1).Reason)
}

func TestSettleIgnoresForeignKeysAndUnknownClaims(t *testing.T) {
	h := newHarness(t, allowAll())
	require.NoError(t, h.svc.OnDebited(context.Background(), pointscontracts.LedgerMovedV1{IdempotencyKey: "mission:1", TenantID: tenantA}))
	require.NoError(t, h.svc.OnDebited(context.Background(), pointscontracts.LedgerMovedV1{
		IdempotencyKey: contracts.DebitKey("0198d000-0000-7000-8000-000000000777"), TenantID: tenantA}))
	err := h.svc.OnDebited(context.Background(), pointscontracts.LedgerMovedV1{IdempotencyKey: contracts.DebitKey("x")})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	require.Empty(t, h.ob.topics())
}

func TestLateDebitAfterCancelTriggersRefund(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.MaxRedemptions = ptr(1) })
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	h.ob.reset()

	// Hold expires, points has no outcome: the sweep cancels and releases.
	h.clock.Advance(11 * time.Minute)
	require.NoError(t, h.svc.ReconcileClaims(context.Background()))
	require.Equal(t, contracts.ClaimCancelled, h.claim(c.ID).Status)
	require.Equal(t, 0, h.reward(r.ID).StockUsed)
	require.Equal(t, []string{contracts.TopicClaimCancelled}, h.ob.topics())
	require.Equal(t, contracts.CancelHoldExpired, h.ob.last(contracts.TopicClaimCancelled).(contracts.ClaimV1).Reason)
	h.ob.reset()

	// The debit lands late: refund it, twice delivered → one refund.
	require.NoError(t, h.svc.OnDebited(context.Background(), debited(c)))
	require.NoError(t, h.svc.OnDebited(context.Background(), debited(c)))
	require.Equal(t, contracts.ClaimRefundPending, h.claim(c.ID).Status)
	require.Equal(t, []string{topicRefund}, h.ob.topics())
	cmd := h.ob.last(topicRefund).(pointscontracts.RefundCmdV1)
	require.Equal(t, contracts.RefundKey(c.ID), cmd.IdempotencyKey)
	require.Equal(t, c.DebitKey, cmd.DebitIdempotencyKey)
	require.Equal(t, tenantA, cmd.TenantID)
	require.Equal(t, 0, h.reward(r.ID).StockUsed, "the late debit does not re-take stock")

	refunded := pointscontracts.LedgerMovedV1{IdempotencyKey: contracts.RefundKey(c.ID), TenantID: tenantA}
	require.NoError(t, h.svc.OnRefunded(context.Background(), refunded))
	require.NoError(t, h.svc.OnRefunded(context.Background(), refunded))
	require.Equal(t, contracts.ClaimRefunded, h.claim(c.ID).Status)
}

func TestReconcileSettlesFromPointsOutcome(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, nil)
	paid, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	refused, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player2, "")
	require.NoError(t, err)
	fresh := h.seedReward(t, tenantA, nil)
	h.clock.Advance(11 * time.Minute)
	notDue, _, err := h.svc.Claim(ctxFor(tenantA), fresh.ID, player1, "")
	require.NoError(t, err)

	h.points.set(paid.DebitKey, ports.PaymentOutcome{Status: ports.PaymentApplied})
	h.points.set(refused.DebitKey, ports.PaymentOutcome{Status: ports.PaymentRejected, Reason: effect.ReasonWalletInactive})
	require.NoError(t, h.svc.ReconcileClaims(context.Background()))

	require.Equal(t, contracts.ClaimClaimed, h.claim(paid.ID).Status)
	require.Equal(t, contracts.ClaimRejected, h.claim(refused.ID).Status)
	require.Equal(t, effect.ReasonWalletInactive, h.claim(refused.ID).RejectReason)
	require.Equal(t, contracts.ClaimPendingPayment, h.claim(notDue.ID).Status, "hold not expired yet")
	require.Equal(t, 1, h.reward(r.ID).StockUsed)
	require.Equal(t, h.clock.Now(), h.repo.markers[JobClaimsReconcile])

	// Running again changes nothing.
	n := len(h.ob.topics())
	require.NoError(t, h.svc.ReconcileClaims(context.Background()))
	require.Len(t, h.ob.topics(), n)
}

func TestReconcileRefundsCancelledClaimWhoseDebitEventWasLost(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, nil)
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	_, err = h.svc.Cancel(ctxFor(tenantA), c.ID)
	require.NoError(t, err)
	h.ob.reset()

	h.points.set(c.DebitKey, ports.PaymentOutcome{Status: ports.PaymentApplied})
	require.NoError(t, h.svc.ReconcileClaims(context.Background()))
	require.Equal(t, contracts.ClaimRefundPending, h.claim(c.ID).Status)
	require.Equal(t, 1, h.ob.count(topicRefund))

	h.points.set(contracts.RefundKey(c.ID), ports.PaymentOutcome{Status: ports.PaymentApplied})
	require.NoError(t, h.svc.ReconcileClaims(context.Background()))
	require.Equal(t, contracts.ClaimRefunded, h.claim(c.ID).Status)
	require.Equal(t, 1, h.ob.count(topicRefund))
}

// ---- redeem, cancel, expire ----

func TestRedeem(t *testing.T) {
	badge := "0198d000-0000-7000-8000-00000000bad9"
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) {
		p.PointsCost = ptr(int64(0))
		p.BadgeRewardID = &badge
	})
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)

	got, err := h.svc.Redeem(ctxFor(tenantA), c.ID)
	require.NoError(t, err)
	require.Equal(t, contracts.ClaimRedeemed, got.Status)
	ev := h.ob.last(contracts.TopicRedeemed).(contracts.ClaimV1)
	require.Equal(t, badge, ev.BadgeRewardID)

	_, err = h.svc.Redeem(ctxFor(tenantA), c.ID)
	require.ErrorIs(t, err, domain.ErrInvalidTransition)
	_, err = h.svc.Redeem(ctxFor(tenantB), c.ID)
	require.ErrorIs(t, err, domain.ErrClaimNotFound)
	require.Equal(t, 1, h.ob.count(contracts.TopicRedeemed))
	require.Equal(t, 1, h.reward(r.ID).StockUsed, "redeemed claims keep their unit")
}

func TestRedeemRequiresPermission(t *testing.T) {
	h := newHarness(t, allowKeys{contracts.PermClaim.Key(): true})
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.PointsCost = ptr(int64(0)) })
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	_, err = h.svc.Redeem(ctxFor(tenantA), c.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestAdminCancelPaidClaimRefundsAndReleasesStock(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, nil)
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	require.NoError(t, h.svc.OnDebited(context.Background(), debited(c)))
	h.ob.reset()

	got, err := h.svc.Cancel(ctxFor(tenantA), c.ID)
	require.NoError(t, err)
	require.Equal(t, contracts.ClaimRefundPending, got.Status)
	require.Equal(t, []string{topicRefund, contracts.TopicClaimCancelled}, h.ob.topics())
	require.Equal(t, 0, h.reward(r.ID).StockUsed)

	_, err = h.svc.Cancel(ctxFor(tenantA), c.ID)
	require.ErrorIs(t, err, domain.ErrInvalidTransition)
}

func TestAdminCancelFreeAndPendingClaims(t *testing.T) {
	h := newHarness(t, allowAll())
	free := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.PointsCost = ptr(int64(0)) })
	c, _, err := h.svc.Claim(ctxFor(tenantA), free.ID, player1, "")
	require.NoError(t, err)
	got, err := h.svc.Cancel(ctxFor(tenantA), c.ID)
	require.NoError(t, err)
	require.Equal(t, contracts.ClaimCancelled, got.Status)
	require.Zero(t, h.ob.count(topicRefund))
	require.Equal(t, 0, h.reward(free.ID).StockUsed)

	paid := h.seedReward(t, tenantA, nil)
	p, _, err := h.svc.Claim(ctxFor(tenantA), paid.ID, player1, "")
	require.NoError(t, err)
	got, err = h.svc.Cancel(ctxFor(tenantA), p.ID)
	require.NoError(t, err)
	require.Equal(t, contracts.ClaimCancelled, got.Status)
	require.Zero(t, h.ob.count(topicRefund), "nothing was debited yet")
	require.Equal(t, 0, h.reward(paid.ID).StockUsed)
}

func TestCancelRequiresAdminPermission(t *testing.T) {
	h := newHarness(t, allowKeys{contracts.PermClaim.Key(): true, contracts.PermRedeem.Key(): true})
	r := h.seedReward(t, tenantA, nil)
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	_, err = h.svc.Cancel(ctxFor(tenantA), c.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestExpireSweep(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) {
		p.PointsCost = ptr(int64(0))
		p.ClaimTTLDays = ptr(1)
		p.EndAt = ptr(t0.Add(48 * time.Hour))
	})
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)
	h.ob.reset()

	require.NoError(t, h.svc.ExpireClaims(context.Background()))
	require.Equal(t, contracts.ClaimClaimed, h.claim(c.ID).Status)

	h.clock.Advance(49 * time.Hour)
	require.NoError(t, h.svc.ExpireClaims(context.Background()))
	require.NoError(t, h.svc.ExpireClaims(context.Background()))
	require.Equal(t, contracts.ClaimExpired, h.claim(c.ID).Status)
	require.Equal(t, contracts.RewardExpired, h.reward(r.ID).Status)
	require.Equal(t, []string{contracts.TopicClaimExpired}, h.ob.topics())
	require.Equal(t, 1, h.reward(r.ID).StockUsed, "expired claims keep their unit (Q-R2)")
}

// ---- grant job ----

func grantCmd(rewardID, playerID, key string) contracts.GrantCmdV1 {
	return contracts.GrantCmdV1{
		IdempotencyKey: key, TenantID: tenantA, PlayerID: playerID, RewardID: rewardID,
		Source: effect.Source{Kind: effect.SourceRule, ID: "eff-1"}, OccurredAt: t0,
	}
}

func TestGrantClaimsForFreeAndIsIdempotent(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.PointsCost = ptr(int64(500)) })
	cmd := grantCmd(r.ID, player1, "rule:eff-1")
	require.NoError(t, h.svc.Grant(context.Background(), cmd))
	require.NoError(t, h.svc.Grant(context.Background(), cmd))

	require.Len(t, h.repo.claims, 1)
	require.Equal(t, []string{contracts.TopicClaimed}, h.ob.topics())
	ev := h.ob.last(contracts.TopicClaimed).(contracts.ClaimV1)
	require.Equal(t, "rule:eff-1", ev.IdempotencyKey)
	require.Zero(t, ev.PointsCost)
	require.Equal(t, &cmd.Source, ev.Source)
	require.Zero(t, h.ob.count(topicDebit))
	require.Equal(t, 1, h.reward(r.ID).StockUsed)
}

func TestGrantRejectionIsRecordedOnce(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.MaxRedemptions = ptr(1) })
	require.NoError(t, h.svc.Grant(context.Background(), grantCmd(r.ID, player1, "k1")))

	cmd := grantCmd(r.ID, player2, "k2")
	require.NoError(t, h.svc.Grant(context.Background(), cmd))
	require.NoError(t, h.svc.Grant(context.Background(), cmd))
	require.Equal(t, 1, h.ob.count(contracts.TopicClaimRejected))
	ev := h.ob.last(contracts.TopicClaimRejected).(contracts.ClaimV1)
	require.Equal(t, contracts.ReasonRewardDepleted, ev.Reason)
	require.Equal(t, "k2", ev.IdempotencyKey)
	require.Equal(t, 1, h.reward(r.ID).StockUsed)
}

func TestGrantRejectionReasons(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.LevelRequirement = ptr(9) })
	cases := map[string]contracts.GrantCmdV1{
		effect.ReasonPlayerInactive:            grantCmd(r.ID, "0198d000-0000-7000-8000-0000000000ff", "a"),
		effect.ReasonPlayerNotFound:            grantCmd(r.ID, "0198d000-0000-7000-8000-000000000099", "b"),
		contracts.ReasonRewardNotFound:         grantCmd("0198d000-0000-7000-8000-000000000123", player1, "c"),
		contracts.ReasonLevelRequirementNotMet: grantCmd(r.ID, player1, "d"),
	}
	for reason, cmd := range cases {
		h.ob.reset()
		require.NoError(t, h.svc.Grant(context.Background(), cmd))
		require.Equal(t, reason, h.ob.last(contracts.TopicClaimRejected).(contracts.ClaimV1).Reason, reason)
	}
}

func TestGrantMalformedIsInvalid(t *testing.T) {
	h := newHarness(t, allowAll())
	err := h.svc.Grant(context.Background(), contracts.GrantCmdV1{TenantID: tenantA, PlayerID: player1, RewardID: "x"})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	err = h.svc.Grant(context.Background(), grantCmd("not-a-uuid", player1, "k"))
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

// ---- reads, tenant purge ----

func TestGetAndListClaims(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, nil)
	c, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)

	got, err := h.svc.GetClaim(ctxFor(tenantA), c.ID)
	require.NoError(t, err)
	require.Equal(t, c.ID, got.ID)
	_, err = h.svc.GetClaim(ctxFor(tenantB), c.ID)
	require.ErrorIs(t, err, domain.ErrClaimNotFound)

	list, next, err := h.svc.ListPlayerClaims(ctxFor(tenantA), player1, "", 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Empty(t, next)
	_, _, err = h.svc.ListPlayerClaims(ctxFor(tenantB), player1, "", 0)
	require.ErrorIs(t, err, domain.ErrPlayerNotFound)
}

func TestPurgeTenantIsIdempotent(t *testing.T) {
	h := newHarness(t, allowAll())
	r := h.seedReward(t, tenantA, func(p *domain.RewardPatch) { p.PointsCost = ptr(int64(0)) })
	other := h.seedReward(t, tenantB, nil)
	_, _, err := h.svc.Claim(ctxFor(tenantA), r.ID, player1, "")
	require.NoError(t, err)

	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))
	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))
	require.Empty(t, h.repo.claims)
	require.Len(t, h.repo.rewards, 1)
	require.Contains(t, h.repo.rewards, other.ID)
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.PurgeTenant(context.Background(), "")))
}
