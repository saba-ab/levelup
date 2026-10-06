package app

import (
	"context"
	"time"

	"github.com/google/uuid"

	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
)

// ClaimFilter narrows the tenant-wide claim listing (the portal's
// Redemption History). Empty fields do not filter. From is inclusive, To
// exclusive, both on the claim's created_at.
type ClaimFilter struct {
	Status   string
	RewardID string
	PlayerID string
	From     *time.Time
	To       *time.Time
}

// RewardStats is one reward's claim counters.
//
//	Claimed     claims that settled (claimed_at set), whatever happened next
//	Redeemed    status redeemed
//	Expired     status expired
//	Cancelled   status cancelled, refund_pending or refunded
//	PointsSpent points kept by rewards: points_cost of claims now claimed, redeemed or expired
type RewardStats struct {
	RewardID    string
	Slug        string
	Name        string
	Type        string
	Deleted     bool
	Claimed     int64
	Redeemed    int64
	Expired     int64
	Cancelled   int64
	PointsSpent int64
}

// StatsReport is GET /rewards/stats: per reward and the tenant totals.
type StatsReport struct {
	Rewards []RewardStats
	Totals  RewardStats
}

var errBadClaimFilter = errs.New(errs.Invalid, "validation failed")

// ListClaims is the tenant-wide claim history. Unlike the per-player
// listing it does not resolve the player: player_id is a plain filter.
func (s *Service) ListClaims(ctx context.Context, f ClaimFilter, cursor string, limit int) ([]domain.Claim, string, error) {
	p, err := s.authorize(ctx, contracts.PermViewClaims)
	if err != nil {
		return nil, "", err
	}
	if err := validateClaimFilter(f); err != nil {
		return nil, "", err
	}
	page, err := pageOf(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	want := page.Limit
	page.Limit++
	rows, err := s.repo.ListClaims(ctx, p.TenantID, f, page)
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

// Stats aggregates claims per reward for the portal dashboards.
func (s *Service) Stats(ctx context.Context) (StatsReport, error) {
	p, err := s.authorize(ctx, contracts.PermViewClaims)
	if err != nil {
		return StatsReport{}, err
	}
	rows, err := s.repo.RewardStats(ctx, p.TenantID)
	if err != nil {
		return StatsReport{}, err
	}
	out := StatsReport{Rewards: rows}
	if out.Rewards == nil {
		out.Rewards = []RewardStats{}
	}
	for _, r := range rows {
		out.Totals.Claimed += r.Claimed
		out.Totals.Redeemed += r.Redeemed
		out.Totals.Expired += r.Expired
		out.Totals.Cancelled += r.Cancelled
		out.Totals.PointsSpent += r.PointsSpent
	}
	return out, nil
}

func validateClaimFilter(f ClaimFilter) error {
	fields := map[string]string{}
	if f.Status != "" && !validClaimStatus(f.Status) {
		fields["status"] = "unknown claim status"
	}
	if f.RewardID != "" {
		if _, err := uuid.Parse(f.RewardID); err != nil {
			fields["reward_id"] = "must be a uuid"
		}
	}
	if f.PlayerID != "" {
		if _, err := uuid.Parse(f.PlayerID); err != nil {
			fields["player_id"] = "must be a uuid"
		}
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		fields["to"] = "must be after from"
	}
	if len(fields) > 0 {
		return errs.WithFields(errBadClaimFilter, fields)
	}
	return nil
}

func validClaimStatus(st string) bool {
	switch st {
	case contracts.ClaimPendingPayment, contracts.ClaimClaimed, contracts.ClaimRejected, contracts.ClaimRedeemed,
		contracts.ClaimExpired, contracts.ClaimCancelled, contracts.ClaimRefundPending, contracts.ClaimRefunded:
		return true
	}
	return false
}
