package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
)

const maxKeyLen = 255

var errKeyReused = errs.WithCode(
	errs.New(errs.Invalid, "Idempotency-Key was already used for a different claim"), "idempotency_key_reused")

// Claim starts a claim (tx1 of the saga). A free reward settles as claimed
// in the same transaction; a paid one is held as pending_payment while
// job.points.debit runs. clientRequestID (the Idempotency-Key header) makes
// the call replayable: the same key returns the same claim. created=false
// means the claim already existed for that key.
func (s *Service) Claim(ctx context.Context, rewardID, playerID, clientRequestID string) (domain.Claim, bool, error) {
	p, err := s.authorize(ctx, contracts.PermClaim)
	if err != nil {
		return domain.Claim{}, false, err
	}
	if len(clientRequestID) > maxKeyLen {
		return domain.Claim{}, false, domain.ErrIdempotencyKeyLong
	}
	if clientRequestID != "" {
		if c, found, err := s.replay(ctx, p.TenantID, clientRequestID, rewardID, playerID); err != nil || found {
			return c, false, err
		}
	}

	if err := s.checkPlayer(ctx, p.TenantID, playerID); err != nil {
		return domain.Claim{}, false, err
	}
	pre, err := s.repo.RewardByID(ctx, p.TenantID, rewardID, false)
	if err != nil {
		return domain.Claim{}, false, err
	}
	level, err := s.levelFor(ctx, pre, playerID)
	if err != nil {
		return domain.Claim{}, false, err
	}

	var out domain.Claim
	err = s.withCodeRetry(func() error {
		return s.tx(ctx, func(tx *gorm.DB) error {
			r, err := s.repo.RewardForUpdate(ctx, tx, p.TenantID, rewardID, false)
			if err != nil {
				return err
			}
			if err := s.validateHold(ctx, tx, &r, playerID, level); err != nil {
				return err
			}
			now := s.clock.Now()
			if err := r.HoldStock(now); err != nil {
				return err
			}
			if err := s.repo.SaveReward(ctx, tx, r); err != nil {
				return err
			}
			c := domain.NewClaim(r, playerID, false, s.cfg.HoldTTL, s.code, now)
			if clientRequestID != "" {
				c.ClientRequestID = &clientRequestID
			}
			if err := s.repo.InsertClaim(ctx, tx, c); err != nil {
				return err
			}
			out = c
			if c.Status == contracts.ClaimClaimed {
				return s.outbox.Publish(ctx, tx, contracts.TopicClaimed, claimEvent(c, &r, "", nil, now))
			}
			if err := s.outbox.Publish(ctx, tx, pointscontracts.Topic(pointscontracts.JobDebit), pointscontracts.DebitCmdV1{
				IdempotencyKey: c.DebitKey,
				TenantID:       c.TenantID,
				PlayerID:       c.PlayerID,
				Amount:         c.PointsCost,
				Kind:           pointscontracts.KindRedeem,
				Description:    fmt.Sprintf("Claimed reward: %s", r.Name),
				Source:         effect.Source{Kind: effect.SourceReward, ID: c.ID},
				OccurredAt:     now,
			}); err != nil {
				return err
			}
			return s.outbox.Publish(ctx, tx, contracts.TopicClaimRequested, claimEvent(c, &r, "", nil, now))
		})
	})
	if errors.Is(err, domain.ErrDuplicateRequest) && clientRequestID != "" {
		// A concurrent request with the same key won the insert.
		c, found, rerr := s.replay(ctx, p.TenantID, clientRequestID, rewardID, playerID)
		if rerr != nil || !found {
			return domain.Claim{}, false, errors.Join(err, rerr)
		}
		return c, false, nil
	}
	if err != nil {
		return domain.Claim{}, false, err
	}
	return out, true, nil
}

// Grant runs job rewards.grant: a free claim decided by the rules engine.
// Same validations as Claim, no payment, keyed by the command's
// idempotency key. A refusal is recorded and published as
// rewards.claim_rejected.v1; it is a result, never an error.
func (s *Service) Grant(ctx context.Context, cmd contracts.GrantCmdV1) error {
	if err := validateGrant(cmd); err != nil {
		return err
	}
	if _, err := s.repo.ClaimByGrantKey(ctx, cmd.TenantID, cmd.IdempotencyKey); err == nil {
		return nil // redelivery: already applied or already rejected
	} else if !isNotFound(err) {
		return err
	}

	reject := func(slug, rewardType, reason string) error {
		return s.rejectGrant(ctx, cmd, slug, rewardType, reason)
	}
	if reason, err := s.playerRejection(ctx, cmd.TenantID, cmd.PlayerID); err != nil {
		return err
	} else if reason != "" {
		return reject("", "", reason)
	}
	pre, err := s.repo.RewardByID(ctx, cmd.TenantID, cmd.RewardID, false)
	if isNotFound(err) {
		return reject("", "", contracts.ReasonRewardNotFound)
	}
	if err != nil {
		return err
	}
	level, err := s.levelFor(ctx, pre, cmd.PlayerID)
	if err != nil {
		return err
	}

	err = s.withCodeRetry(func() error {
		return s.tx(ctx, func(tx *gorm.DB) error {
			now := s.clock.Now()
			r, err := s.repo.RewardForUpdate(ctx, tx, cmd.TenantID, cmd.RewardID, false)
			if isNotFound(err) {
				return s.insertRejectedGrant(ctx, tx, cmd, "", "", contracts.ReasonRewardNotFound)
			}
			if err != nil {
				return err
			}
			if err := s.validateHold(ctx, tx, &r, cmd.PlayerID, level); err != nil {
				if reason := rejectionReason(err); reason != "" {
					return s.insertRejectedGrant(ctx, tx, cmd, r.Slug, r.Type, reason)
				}
				return err
			}
			if err := r.HoldStock(now); err != nil {
				return s.insertRejectedGrant(ctx, tx, cmd, r.Slug, r.Type, contracts.ReasonRewardDepleted)
			}
			if err := s.repo.SaveReward(ctx, tx, r); err != nil {
				return err
			}
			c := domain.NewClaim(r, cmd.PlayerID, true, s.cfg.HoldTTL, s.code, now)
			key := cmd.IdempotencyKey
			c.GrantKey = &key
			if err := s.repo.InsertClaim(ctx, tx, c); err != nil {
				return err
			}
			src := cmd.Source
			return s.outbox.Publish(ctx, tx, contracts.TopicClaimed, claimEvent(c, &r, "", &src, now))
		})
	})
	if errors.Is(err, domain.ErrDuplicateRequest) {
		return nil // a concurrent delivery of the same command won
	}
	return err
}

func (s *Service) GetClaim(ctx context.Context, claimID string) (domain.Claim, error) {
	p, err := s.authorize(ctx, contracts.PermViewClaims)
	if err != nil {
		return domain.Claim{}, err
	}
	return s.repo.ClaimByID(ctx, p.TenantID, claimID)
}

func (s *Service) ListPlayerClaims(ctx context.Context, playerID, cursor string, limit int) ([]domain.Claim, string, error) {
	p, err := s.authorize(ctx, contracts.PermViewClaims)
	if err != nil {
		return nil, "", err
	}
	if err := s.playerExists(ctx, p.TenantID, playerID); err != nil {
		return nil, "", err
	}
	page, err := pageOf(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	want := page.Limit
	page.Limit++
	rows, err := s.repo.ListPlayerClaims(ctx, p.TenantID, playerID, page)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > want {
		rows = rows[:want]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

// Redeem consumes a claimed claim (claimed → redeemed) under its row lock
// and publishes rewards.redeemed.v1 for the fulfilment consumers (badges,
// points). A badge already earned no longer blocks the redeem (doc 05 R8).
func (s *Service) Redeem(ctx context.Context, claimID string) (domain.Claim, error) {
	p, err := s.authorize(ctx, contracts.PermRedeem)
	if err != nil {
		return domain.Claim{}, err
	}
	var out domain.Claim
	err = s.tx(ctx, func(tx *gorm.DB) error {
		c, err := s.repo.ClaimForUpdate(ctx, tx, p.TenantID, claimID)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if err := c.Redeem(now); err != nil {
			return err
		}
		if err := s.repo.SaveClaim(ctx, tx, c); err != nil {
			return err
		}
		c.Version++
		out = c
		r, err := s.repo.RewardByID(ctx, c.TenantID, c.RewardID, true)
		if err != nil && !isNotFound(err) {
			return err
		}
		var rp *domain.Reward
		if err == nil {
			rp = &r
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicRedeemed, claimEvent(c, rp, "", nil, now))
	})
	return out, err
}

// Cancel is the admin cancel. pending_payment → cancelled (a late debit is
// refunded by the settle handler); claimed → cancelled, or refund_pending
// with job.points.refund when points were paid. Stock is released.
func (s *Service) Cancel(ctx context.Context, claimID string) (domain.Claim, error) {
	p, err := s.authorize(ctx, contracts.PermCancel)
	if err != nil {
		return domain.Claim{}, err
	}
	var out domain.Claim
	err = s.tx(ctx, func(tx *gorm.DB) error {
		c, err := s.repo.ClaimForUpdate(ctx, tx, p.TenantID, claimID)
		if err != nil {
			return err
		}
		if err := s.cancelLocked(ctx, tx, &c, contracts.CancelByAdmin); err != nil {
			return err
		}
		out = c
		return nil
	})
	return out, err
}

// cancelLocked cancels a claim the caller holds the lock on, releases its
// stock, issues the refund when needed and publishes the cancellation.
func (s *Service) cancelLocked(ctx context.Context, tx *gorm.DB, c *domain.Claim, reason string) error {
	now := s.clock.Now()
	held := c.HoldsStock()
	if err := c.Cancel(now); err != nil {
		return err
	}
	if held {
		if err := s.releaseStock(ctx, tx, c.TenantID, c.RewardID); err != nil {
			return err
		}
	}
	if err := s.repo.SaveClaim(ctx, tx, *c); err != nil {
		return err
	}
	c.Version++
	if c.Status == contracts.ClaimRefundPending {
		if err := s.publishRefund(ctx, tx, *c, reason); err != nil {
			return err
		}
	}
	return s.outbox.Publish(ctx, tx, contracts.TopicClaimCancelled, claimEvent(*c, nil, reason, nil, now))
}

func (s *Service) releaseStock(ctx context.Context, tx *gorm.DB, tenantID, rewardID string) error {
	r, err := s.repo.RewardForUpdate(ctx, tx, tenantID, rewardID, true)
	if isNotFound(err) {
		return nil // purged reward: nothing to give back
	}
	if err != nil {
		return err
	}
	r.ReleaseStock(s.clock.Now())
	return s.repo.SaveReward(ctx, tx, r)
}

func (s *Service) publishRefund(ctx context.Context, tx *gorm.DB, c domain.Claim, reason string) error {
	return s.outbox.Publish(ctx, tx, pointscontracts.Topic(pointscontracts.JobRefund), pointscontracts.RefundCmdV1{
		IdempotencyKey:      contracts.RefundKey(c.ID),
		TenantID:            c.TenantID,
		DebitIdempotencyKey: c.DebitKey,
		Reason:              reason,
		Source:              effect.Source{Kind: effect.SourceReward, ID: c.ID},
		OccurredAt:          s.clock.Now(),
	})
}

// validateHold runs the checks that must see the locked reward row.
func (s *Service) validateHold(ctx context.Context, tx *gorm.DB, r *domain.Reward, playerID string, level int) error {
	if err := r.CheckClaimable(s.clock.Now()); err != nil {
		return err
	}
	if err := r.CheckLevel(level); err != nil {
		return err
	}
	if r.MaxPerPlayer != nil {
		n, err := s.repo.CountHeldClaims(ctx, tx, r.TenantID, r.ID, playerID)
		if err != nil {
			return err
		}
		if err := r.CheckPlayerLimit(n); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) replay(ctx context.Context, tenantID, key, rewardID, playerID string) (domain.Claim, bool, error) {
	c, err := s.repo.ClaimByClientRequest(ctx, tenantID, key)
	if isNotFound(err) {
		return domain.Claim{}, false, nil
	}
	if err != nil {
		return domain.Claim{}, false, err
	}
	if c.RewardID != rewardID || c.PlayerID != playerID {
		return domain.Claim{}, false, errKeyReused
	}
	return c, true, nil
}

func (s *Service) rejectGrant(ctx context.Context, cmd contracts.GrantCmdV1, slug, rewardType, reason string) error {
	err := s.tx(ctx, func(tx *gorm.DB) error {
		return s.insertRejectedGrant(ctx, tx, cmd, slug, rewardType, reason)
	})
	if errors.Is(err, domain.ErrDuplicateRequest) {
		return nil
	}
	return err
}

func (s *Service) insertRejectedGrant(ctx context.Context, tx *gorm.DB, cmd contracts.GrantCmdV1, slug, rewardType, reason string) error {
	now := s.clock.Now()
	c := domain.NewRejectedGrant(cmd.TenantID, cmd.PlayerID, cmd.RewardID, slug, rewardType, reason, now)
	key := cmd.IdempotencyKey
	c.GrantKey = &key
	if err := s.repo.InsertClaim(ctx, tx, c); err != nil {
		return err
	}
	src := cmd.Source
	return s.outbox.Publish(ctx, tx, contracts.TopicClaimRejected, claimEvent(c, nil, reason, &src, now))
}

// checkPlayer is the HTTP-side player validation: 404 / 422.
func (s *Service) checkPlayer(ctx context.Context, tenantID, playerID string) error {
	reason, err := s.playerRejection(ctx, tenantID, playerID)
	switch {
	case err != nil:
		return err
	case reason == effect.ReasonPlayerNotFound:
		return domain.ErrPlayerNotFound
	case reason == effect.ReasonPlayerInactive:
		return domain.ErrPlayerInactive
	}
	return nil
}

func (s *Service) playerExists(ctx context.Context, tenantID, playerID string) error {
	reason, err := s.playerRejection(ctx, tenantID, playerID)
	if err != nil {
		return err
	}
	if reason == effect.ReasonPlayerNotFound {
		return domain.ErrPlayerNotFound
	}
	return nil
}

func (s *Service) playerRejection(ctx context.Context, tenantID, playerID string) (string, error) {
	got, err := s.players.PlayersByIDs(ctx, tenantID, []string{playerID})
	if err != nil {
		return "", err
	}
	pl, ok := got[playerID]
	if !ok || pl.TenantID != tenantID {
		return effect.ReasonPlayerNotFound, nil
	}
	if !pl.Active {
		return effect.ReasonPlayerInactive, nil
	}
	return "", nil
}

// levelFor reads the player's level only when the reward requires one.
func (s *Service) levelFor(ctx context.Context, r domain.Reward, playerID string) (int, error) {
	if r.LevelRequirement == nil {
		return 0, nil
	}
	levels, err := s.progress.LevelsByPlayerIDs(ctx, r.TenantID, []string{playerID})
	if err != nil {
		return 0, err
	}
	return levels[playerID], nil
}

// rejectionReason maps a hold validation error to its published reason.
func rejectionReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrRewardNotAvailable):
		return contracts.ReasonRewardNotAvailable
	case errors.Is(err, domain.ErrRewardDepleted):
		return contracts.ReasonRewardDepleted
	case errors.Is(err, domain.ErrPlayerLimitReached):
		return contracts.ReasonPlayerLimitReached
	case errors.Is(err, domain.ErrLevelTooLow):
		return contracts.ReasonLevelRequirementNotMet
	}
	return ""
}

func validateGrant(cmd contracts.GrantCmdV1) error {
	switch {
	case cmd.IdempotencyKey == "" || len(cmd.IdempotencyKey) > maxKeyLen:
		return errs.New(errs.Invalid, "rewards.grant: idempotency_key required (max 255)")
	case !isUUID(cmd.TenantID), !isUUID(cmd.PlayerID), !isUUID(cmd.RewardID):
		return errs.New(errs.Invalid, "rewards.grant: tenant_id, player_id and reward_id must be uuids")
	}
	return nil
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func claimEvent(c domain.Claim, r *domain.Reward, reason string, src *effect.Source, now time.Time) contracts.ClaimV1 {
	ev := contracts.ClaimV1{
		ClaimID:    c.ID,
		TenantID:   c.TenantID,
		PlayerID:   c.PlayerID,
		RewardID:   c.RewardID,
		RewardSlug: c.RewardSlug,
		Status:     c.Status,
		PointsCost: c.PointsCost,
		Reason:     reason,
		At:         now,
		RewardType: c.RewardType,
		ExpiresAt:  c.ExpiresAt,
		Source:     src,
	}
	if c.Paid() {
		ev.DebitKey = c.DebitKey
	}
	if c.GrantKey != nil {
		ev.IdempotencyKey = *c.GrantKey
	}
	if c.Code != nil {
		ev.Code = *c.Code
	}
	if r != nil {
		ev.BadgeRewardID = deref(r.BadgeRewardID)
		ev.LevelRewardID = deref(r.LevelRewardID)
		ev.Value = deref(r.Value)
		ev.ValueType = deref(r.ValueType)
	}
	return ev
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
