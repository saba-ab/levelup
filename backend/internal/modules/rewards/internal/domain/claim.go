package domain

import (
	"time"

	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/shared/id"
)

// Claim is one player's claim on a reward (was player_rewards). Its status
// is the saga's state machine (00-target-architecture F6):
//
//	[*] → claimed                  free reward or rewards.grant
//	[*] → pending_payment          paid reward; job.points.debit issued
//	pending_payment → claimed      points.debited.v1 / sweep finds the debit
//	pending_payment → rejected     points.debit_rejected.v1 (stock released)
//	pending_payment → cancelled    hold expired with no debit, or admin (stock released)
//	cancelled → refund_pending     late points.debited.v1 (job.points.refund)
//	claimed → refund_pending       admin cancel of a paid claim (stock released)
//	claimed → cancelled            admin cancel of an unpaid claim (stock released)
//	refund_pending → refunded      points.refunded.v1
//	claimed → redeemed | expired
type Claim struct {
	ID              string
	TenantID        string
	PlayerID        string
	RewardID        string
	RewardSlug      string
	RewardType      string
	Status          string
	PointsCost      int64 // points charged (0 for free rewards and grants)
	DebitKey        string
	ClientRequestID *string // Idempotency-Key of the HTTP claim
	GrantKey        *string // GrantCmdV1.IdempotencyKey
	RejectReason    string
	HoldExpiresAt   *time.Time
	ClaimedAt       *time.Time
	RedeemedAt      *time.Time
	ExpiresAt       *time.Time
	CancelledAt     *time.Time
	// FulfilledAt is when the reward was delivered: the fulfilment
	// commands were issued (points, badge, level) or the voucher was
	// redeemed (discount, item, custom). Nil until then.
	FulfilledAt *time.Time
	Code        *string
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewClaim starts a claim on a reward whose stock the caller already held.
// A free reward (or a grant, free=true) settles immediately as claimed; a
// paid one waits in pending_payment until holdTTL.
func NewClaim(r Reward, playerID string, free bool, holdTTL time.Duration, code func() string, now time.Time) Claim {
	c := Claim{
		ID:         id.NewID(),
		TenantID:   r.TenantID,
		PlayerID:   playerID,
		RewardID:   r.ID,
		RewardSlug: r.Slug,
		RewardType: r.Type,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	c.DebitKey = contracts.DebitKey(c.ID)
	if free || r.PointsCost == 0 {
		c.Status = contracts.ClaimClaimed
		c.settle(r, code, now)
		return c
	}
	c.Status = contracts.ClaimPendingPayment
	c.PointsCost = r.PointsCost
	hold := now.Add(holdTTL)
	c.HoldExpiresAt = &hold
	return c
}

// NewRejectedGrant records a rewards.grant command that was refused, so a
// redelivery of the same command finds it and publishes nothing.
func NewRejectedGrant(tenantID, playerID, rewardID, slug, rewardType, reason string, now time.Time) Claim {
	c := Claim{
		ID:           id.NewID(),
		TenantID:     tenantID,
		PlayerID:     playerID,
		RewardID:     rewardID,
		RewardSlug:   slug,
		RewardType:   rewardType,
		Status:       contracts.ClaimRejected,
		RejectReason: reason,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	c.DebitKey = contracts.DebitKey(c.ID)
	return c
}

// Paid reports whether points were charged for this claim.
func (c *Claim) Paid() bool { return c.PointsCost > 0 }

// HoldsStock reports whether the claim currently holds a unit of stock.
func (c *Claim) HoldsStock() bool {
	switch c.Status {
	case contracts.ClaimPendingPayment, contracts.ClaimClaimed, contracts.ClaimRedeemed:
		return true
	}
	return false
}

// MarkPaid settles a pending claim after the debit was applied.
func (c *Claim) MarkPaid(r Reward, code func() string, now time.Time) error {
	if c.Status != contracts.ClaimPendingPayment {
		return ErrInvalidTransition
	}
	c.Status = contracts.ClaimClaimed
	c.settle(r, code, now)
	return nil
}

// MarkRejected ends a pending claim whose debit points refused.
func (c *Claim) MarkRejected(reason string, now time.Time) error {
	if c.Status != contracts.ClaimPendingPayment {
		return ErrInvalidTransition
	}
	c.Status = contracts.ClaimRejected
	c.RejectReason = reason
	c.HoldExpiresAt = nil
	c.UpdatedAt = now
	return nil
}

// Cancel ends a claim: a pending claim becomes cancelled; a claimed one
// becomes cancelled when nothing was paid, refund_pending when it was.
// Every cancel releases the claim's unit of stock.
func (c *Claim) Cancel(now time.Time) error {
	switch {
	case c.Status == contracts.ClaimPendingPayment:
		c.Status = contracts.ClaimCancelled
	case c.Status == contracts.ClaimClaimed && c.Paid():
		c.Status = contracts.ClaimRefundPending
	case c.Status == contracts.ClaimClaimed:
		c.Status = contracts.ClaimCancelled
	default:
		return ErrInvalidTransition
	}
	c.HoldExpiresAt = nil
	c.CancelledAt = &now
	c.UpdatedAt = now
	return nil
}

// LateDebit handles a debit that landed after the claim was cancelled: the
// points must go back.
func (c *Claim) LateDebit(now time.Time) error {
	if c.Status != contracts.ClaimCancelled {
		return ErrInvalidTransition
	}
	c.Status = contracts.ClaimRefundPending
	c.UpdatedAt = now
	return nil
}

// MarkRefunded closes a refund.
func (c *Claim) MarkRefunded(now time.Time) error {
	if c.Status != contracts.ClaimRefundPending {
		return ErrInvalidTransition
	}
	c.Status = contracts.ClaimRefunded
	c.UpdatedAt = now
	return nil
}

// Redeem consumes a claimed, unexpired claim.
func (c *Claim) Redeem(now time.Time) error {
	if c.Status != contracts.ClaimClaimed {
		return ErrInvalidTransition
	}
	if c.ExpiresAt != nil && !now.Before(*c.ExpiresAt) {
		return ErrClaimExpired
	}
	c.Status = contracts.ClaimRedeemed
	c.RedeemedAt = &now
	c.UpdatedAt = now
	return nil
}

// Expire ends a claimed claim past its expiry. Stock stays consumed (doc 05
// Q-R2 default).
func (c *Claim) Expire(now time.Time) error {
	if c.Status != contracts.ClaimClaimed || c.ExpiresAt == nil || now.Before(*c.ExpiresAt) {
		return ErrInvalidTransition
	}
	c.Status = contracts.ClaimExpired
	c.UpdatedAt = now
	return nil
}

// MarkFulfilled stamps the delivery time once; later calls keep the first.
func (c *Claim) MarkFulfilled(now time.Time) {
	if c.FulfilledAt != nil {
		return
	}
	c.FulfilledAt = &now
	c.UpdatedAt = now
}

// Fulfilled reports whether the reward was already delivered.
func (c *Claim) Fulfilled() bool { return c.FulfilledAt != nil }

func (c *Claim) settle(r Reward, code func() string, now time.Time) {
	c.ClaimedAt = &now
	c.HoldExpiresAt = nil
	c.ExpiresAt = r.ClaimExpiry(now)
	if r.NeedsCode() && c.Code == nil && code != nil {
		v := code()
		c.Code = &v
	}
	c.UpdatedAt = now
}
