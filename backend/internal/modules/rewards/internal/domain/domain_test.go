package domain

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/rewards/contracts"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func activeReward(t *testing.T, mut func(*RewardPatch)) Reward {
	t.Helper()
	p := RewardPatch{Name: ptr("Coffee Mug"), Status: ptr(contracts.RewardActive), PointsCost: ptr(int64(100))}
	if mut != nil {
		mut(&p)
	}
	r, err := NewReward("tenant-1", p, t0)
	require.NoError(t, err)
	return r
}

func TestNewRewardValidation(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*RewardPatch)
		want error
	}{
		{"ok", nil, nil},
		{"name required", func(p *RewardPatch) { p.Name = ptr("  ") }, ErrNameRequired},
		{"bad type", func(p *RewardPatch) { p.Type = ptr("cash") }, ErrBadType},
		{"bad status", func(p *RewardPatch) { p.Status = ptr("live") }, ErrBadStatus},
		{"negative cost", func(p *RewardPatch) { p.PointsCost = ptr(int64(-1)) }, ErrBadPointsCost},
		{"bad value", func(p *RewardPatch) { p.Value = ptr("ten") }, ErrBadValue},
		{"too many decimals", func(p *RewardPatch) { p.Value = ptr("1.234") }, ErrBadValue},
		{"good value", func(p *RewardPatch) { p.Value = ptr("10.50") }, nil},
		{"bad value type", func(p *RewardPatch) { p.ValueType = ptr("ratio") }, ErrBadValueType},
		{"negative limit", func(p *RewardPatch) { p.MaxRedemptions = ptr(-2) }, ErrBadLimit},
		{"zero limit clears", func(p *RewardPatch) { p.MaxRedemptions = ptr(0) }, nil},
		{"bad slug", func(p *RewardPatch) { p.Slug = ptr("Not A Slug") }, ErrBadSlug},
		{"window inverted", func(p *RewardPatch) {
			p.StartAt = ptr(t0.Add(time.Hour))
			p.EndAt = ptr(t0)
		}, ErrBadWindow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := RewardPatch{Name: ptr("Coffee Mug")}
			if tc.mut != nil {
				tc.mut(&p)
			}
			r, err := NewReward("tenant-1", p, t0)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "coffee-mug", r.Slug)
			require.Equal(t, contracts.TypePoints, r.Type)
		})
	}
}

func TestNewRewardNeedsTenant(t *testing.T) {
	_, err := NewReward("", RewardPatch{Name: ptr("x")}, t0)
	require.ErrorIs(t, err, ErrNoTenant)
}

func TestSlugify(t *testing.T) {
	require.Equal(t, "10-off-coupon", Slugify("10% Off  Coupon!"))
	require.Equal(t, "reward", Slugify("ბონუსი"))
}

func TestCheckClaimable(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Reward)
		want error
	}{
		{"active", func(*Reward) {}, nil},
		{"inactive flag", func(r *Reward) { r.IsActive = false }, ErrRewardNotAvailable},
		{"draft", func(r *Reward) { r.Status = contracts.RewardDraft }, ErrRewardNotAvailable},
		{"paused", func(r *Reward) { r.Status = contracts.RewardPaused }, ErrRewardNotAvailable},
		{"not started", func(r *Reward) { r.StartAt = ptr(t0.Add(time.Minute)) }, ErrRewardNotAvailable},
		{"ended", func(r *Reward) { r.EndAt = ptr(t0) }, ErrRewardNotAvailable},
		{"depleted status", func(r *Reward) { r.Status = contracts.RewardDepleted }, ErrRewardDepleted},
		{"sold out", func(r *Reward) { r.MaxRedemptions = ptr(1); r.StockUsed = 1 }, ErrRewardDepleted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := activeReward(t, nil)
			tc.mut(&r)
			err := r.CheckClaimable(t0)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.want)
		})
	}
}

// R1: stock counts every held claim; the last unit flips status to
// depleted, a release flips it back.
func TestHoldAndReleaseStock(t *testing.T) {
	r := activeReward(t, func(p *RewardPatch) { p.MaxRedemptions = ptr(2) })
	require.NoError(t, r.HoldStock(t0))
	require.Equal(t, contracts.RewardActive, r.Status)
	require.NoError(t, r.HoldStock(t0))
	require.Equal(t, 2, r.StockUsed)
	require.Equal(t, contracts.RewardDepleted, r.Status)
	require.ErrorIs(t, r.HoldStock(t0), ErrRewardDepleted)

	r.ReleaseStock(t0)
	require.Equal(t, 1, r.StockUsed)
	require.Equal(t, contracts.RewardActive, r.Status)
	require.NoError(t, r.CheckClaimable(t0))

	unlimited := activeReward(t, nil)
	for range 5 {
		require.NoError(t, unlimited.HoldStock(t0))
	}
	unlimited.ReleaseStock(t0)
	require.Equal(t, 4, unlimited.StockUsed)
}

func TestMaxRedemptionsCannotDropBelowUsed(t *testing.T) {
	r := activeReward(t, func(p *RewardPatch) { p.MaxRedemptions = ptr(5) })
	r.StockUsed = 3
	require.ErrorIs(t, r.Apply(RewardPatch{MaxRedemptions: ptr(2)}, t0), ErrStockBelowUsed)
}

func TestPlayerLimitAndLevel(t *testing.T) {
	r := activeReward(t, func(p *RewardPatch) {
		p.MaxPerPlayer = ptr(2)
		p.LevelRequirement = ptr(3)
	})
	require.NoError(t, r.CheckPlayerLimit(1))
	require.ErrorIs(t, r.CheckPlayerLimit(2), ErrPlayerLimitReached)
	require.ErrorIs(t, r.CheckLevel(2), ErrLevelTooLow)
	require.NoError(t, r.CheckLevel(3))
}

func TestNewClaim(t *testing.T) {
	code := func() string { return "CODE-1" }

	free := activeReward(t, func(p *RewardPatch) {
		p.PointsCost = ptr(int64(0))
		p.Type = ptr(contracts.TypeDiscount)
		p.ClaimTTLDays = ptr(30)
	})
	c := NewClaim(free, "p1", false, 10*time.Minute, code, t0)
	require.Equal(t, contracts.ClaimClaimed, c.Status)
	require.Equal(t, contracts.DebitKey(c.ID), c.DebitKey)
	require.Equal(t, "CODE-1", *c.Code)
	require.Equal(t, t0.AddDate(0, 0, 30), *c.ExpiresAt)
	require.Nil(t, c.HoldExpiresAt)

	paid := activeReward(t, nil)
	c = NewClaim(paid, "p1", false, 10*time.Minute, code, t0)
	require.Equal(t, contracts.ClaimPendingPayment, c.Status)
	require.EqualValues(t, 100, c.PointsCost)
	require.Equal(t, t0.Add(10*time.Minute), *c.HoldExpiresAt)
	require.Nil(t, c.Code, "points-type rewards carry no voucher code")

	granted := NewClaim(paid, "p1", true, 10*time.Minute, code, t0)
	require.Equal(t, contracts.ClaimClaimed, granted.Status)
	require.Zero(t, granted.PointsCost, "a grant charges nothing")
}

func TestClaimTransitions(t *testing.T) {
	r := activeReward(t, nil)
	mk := func(status string, paid bool) Claim {
		c := NewClaim(r, "p1", false, time.Minute, nil, t0)
		c.Status = status
		if !paid {
			c.PointsCost = 0
		}
		return c
	}
	type step func(*Claim) error
	paidStep := func(c *Claim) error { return c.MarkPaid(r, nil, t0) }
	rejectStep := func(c *Claim) error { return c.MarkRejected("insufficient_balance", t0) }
	cancelStep := func(c *Claim) error { return c.Cancel(t0) }
	lateStep := func(c *Claim) error { return c.LateDebit(t0) }
	refundedStep := func(c *Claim) error { return c.MarkRefunded(t0) }
	redeemStep := func(c *Claim) error { return c.Redeem(t0) }

	cases := []struct {
		name  string
		from  string
		paid  bool
		do    step
		want  string
		errIs error
	}{
		{"pending paid", contracts.ClaimPendingPayment, true, paidStep, contracts.ClaimClaimed, nil},
		{"claimed paid again", contracts.ClaimClaimed, true, paidStep, "", ErrInvalidTransition},
		{"pending rejected", contracts.ClaimPendingPayment, true, rejectStep, contracts.ClaimRejected, nil},
		{"claimed rejected", contracts.ClaimClaimed, true, rejectStep, "", ErrInvalidTransition},
		{"cancel pending", contracts.ClaimPendingPayment, true, cancelStep, contracts.ClaimCancelled, nil},
		{"cancel claimed paid", contracts.ClaimClaimed, true, cancelStep, contracts.ClaimRefundPending, nil},
		{"cancel claimed free", contracts.ClaimClaimed, false, cancelStep, contracts.ClaimCancelled, nil},
		{"cancel redeemed", contracts.ClaimRedeemed, true, cancelStep, "", ErrInvalidTransition},
		{"late debit", contracts.ClaimCancelled, true, lateStep, contracts.ClaimRefundPending, nil},
		{"late debit on claimed", contracts.ClaimClaimed, true, lateStep, "", ErrInvalidTransition},
		{"refunded", contracts.ClaimRefundPending, true, refundedStep, contracts.ClaimRefunded, nil},
		{"refunded twice", contracts.ClaimRefunded, true, refundedStep, "", ErrInvalidTransition},
		{"redeem", contracts.ClaimClaimed, true, redeemStep, contracts.ClaimRedeemed, nil},
		{"redeem pending", contracts.ClaimPendingPayment, true, redeemStep, "", ErrInvalidTransition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := mk(tc.from, tc.paid)
			err := tc.do(&c)
			if tc.errIs != nil {
				require.ErrorIs(t, err, tc.errIs)
				require.Equal(t, tc.from, c.Status)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, c.Status)
		})
	}
}

func TestRedeemAndExpireRespectExpiry(t *testing.T) {
	r := activeReward(t, func(p *RewardPatch) {
		p.PointsCost = ptr(int64(0))
		p.ClaimTTLDays = ptr(1)
	})
	c := NewClaim(r, "p1", false, time.Minute, nil, t0)
	later := t0.AddDate(0, 0, 1)
	require.ErrorIs(t, c.Redeem(later), ErrClaimExpired)
	require.ErrorIs(t, c.Expire(t0), ErrInvalidTransition, "not yet due")
	require.NoError(t, c.Expire(later))
	require.Equal(t, contracts.ClaimExpired, c.Status)
}

func TestVoucherCodeShape(t *testing.T) {
	re := regexp.MustCompile(`^[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}-[2-9A-HJKMNP-Z]{4}$`)
	seen := map[string]bool{}
	for range 200 {
		c := NewVoucherCode()
		require.Regexp(t, re, c)
		require.False(t, seen[c])
		seen[c] = true
	}
}

func TestExpireIfEnded(t *testing.T) {
	r := activeReward(t, func(p *RewardPatch) { p.EndAt = ptr(t0) })
	require.True(t, r.ExpireIfEnded(t0))
	require.Equal(t, contracts.RewardExpired, r.Status)
	require.False(t, r.ExpireIfEnded(t0))
}
