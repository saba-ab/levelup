// Package repo implements rewards' persistence. Models stay unexported and
// derive their table names (rewards_svc.rewards, rewards_svc.reward_claims,
// rewards_svc.reconcile_markers); no TableName() overrides.
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/app"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/shared/errs"
)

type reward struct {
	ID               string `gorm:"primaryKey;type:uuid"`
	TenantID         string `gorm:"type:uuid"`
	LegacyID         *int64
	Name             string
	Slug             string
	Description      *string
	Type             string
	Status           string
	PointsCost       int64
	Value            *string `gorm:"type:numeric(10,2)"`
	ValueType        *string
	BadgeRewardID    *string `gorm:"type:uuid"`
	LevelRewardID    *string `gorm:"type:uuid"`
	MaxRedemptions   *int
	MaxPerPlayer     *int
	StockUsed        int
	ClaimTTLDays     *int `gorm:"column:claim_ttl_days"`
	StartAt          *time.Time
	EndAt            *time.Time
	LevelRequirement *int
	IsActive         bool
	Metadata         *string `gorm:"type:jsonb"`
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

type rewardClaim struct {
	ID              string `gorm:"primaryKey;type:uuid"`
	TenantID        string `gorm:"type:uuid"`
	LegacyID        *int64
	PlayerID        string `gorm:"type:uuid"`
	RewardID        string `gorm:"type:uuid"`
	RewardSlug      string
	RewardType      string
	Status          string
	PointsCost      int64
	DebitKey        string
	ClientRequestID *string
	GrantKey        *string
	RejectReason    *string
	HoldExpiresAt   *time.Time
	ClaimedAt       *time.Time
	RedeemedAt      *time.Time
	ExpiresAt       *time.Time
	CancelledAt     *time.Time
	FulfilledAt     *time.Time
	Code            *string
	Metadata        *string `gorm:"type:jsonb"`
	Version         int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type reconcileMarker struct {
	Job     string `gorm:"primaryKey"`
	LastRun time.Time
}

var heldStatuses = []string{contracts.ClaimPendingPayment, contracts.ClaimClaimed, contracts.ClaimRedeemed}

// schema qualifies raw SQL: Deps.DB pins the prefix only for model-derived
// table names, and search_path is not usable behind PgBouncer.
const schema = "rewards_svc."

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

// ---- rewards ----

func (r *Postgres) CreateReward(ctx context.Context, tx *gorm.DB, d domain.Reward) error {
	m, err := rewardFromDomain(d)
	if err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if constraintOf(err) == "ux_rewards_tenant_slug" {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "insert reward", err)
	}
	return nil
}

func (r *Postgres) RewardByID(ctx context.Context, tenantID, id string, includeDeleted bool) (domain.Reward, error) {
	return r.loadReward(r.db.WithContext(ctx), tenantID, id, includeDeleted)
}

func (r *Postgres) RewardForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string, includeDeleted bool) (domain.Reward, error) {
	return r.loadReward(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}), tenantID, id, includeDeleted)
}

func (r *Postgres) loadReward(q *gorm.DB, tenantID, id string, includeDeleted bool) (domain.Reward, error) {
	if !isUUID(tenantID) || !isUUID(id) {
		return domain.Reward{}, domain.ErrRewardNotFound
	}
	q = q.Where("tenant_id = ? AND id = ?", tenantID, id)
	if !includeDeleted {
		q = q.Where("deleted_at IS NULL")
	}
	var m reward
	err := q.First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Reward{}, domain.ErrRewardNotFound
	case err != nil:
		return domain.Reward{}, errs.Wrap(errs.Internal, "load reward", err)
	}
	return m.toDomain(), nil
}

// SaveReward writes back guarded by the optimistic version.
func (r *Postgres) SaveReward(ctx context.Context, tx *gorm.DB, d domain.Reward) error {
	m, err := rewardFromDomain(d)
	if err != nil {
		return err
	}
	res := tx.WithContext(ctx).Model(&reward{}).
		Where("tenant_id = ? AND id = ? AND version = ?", m.TenantID, m.ID, m.Version).
		Updates(map[string]any{
			"name":              m.Name,
			"slug":              m.Slug,
			"description":       m.Description,
			"type":              m.Type,
			"status":            m.Status,
			"points_cost":       m.PointsCost,
			"value":             m.Value,
			"value_type":        m.ValueType,
			"badge_reward_id":   m.BadgeRewardID,
			"level_reward_id":   m.LevelRewardID,
			"max_redemptions":   m.MaxRedemptions,
			"max_per_player":    m.MaxPerPlayer,
			"stock_used":        m.StockUsed,
			"claim_ttl_days":    m.ClaimTTLDays,
			"start_at":          m.StartAt,
			"end_at":            m.EndAt,
			"level_requirement": m.LevelRequirement,
			"is_active":         m.IsActive,
			"metadata":          m.Metadata,
			"updated_at":        m.UpdatedAt,
			"version":           gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		if constraintOf(res.Error) == "ux_rewards_tenant_slug" {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "save reward", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) SoftDeleteReward(ctx context.Context, tx *gorm.DB, tenantID, id string, at time.Time) error {
	if !isUUID(tenantID) || !isUUID(id) {
		return domain.ErrRewardNotFound
	}
	res := tx.WithContext(ctx).Model(&reward{}).
		Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantID, id).
		Updates(map[string]any{"deleted_at": at, "updated_at": at, "version": gorm.Expr("version + 1")})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "delete reward", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrRewardNotFound
	}
	return nil
}

func (r *Postgres) ListRewards(ctx context.Context, tenantID string, f app.RewardFilter, p app.Page) ([]domain.Reward, error) {
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.IsActive != nil {
		q = q.Where("is_active = ?", *f.IsActive)
	}
	if !p.AfterTime.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", p.AfterTime, p.AfterID)
	}
	var ms []reward
	if err := q.Order("created_at DESC, id DESC").Limit(p.Limit).Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list rewards", err)
	}
	out := make([]domain.Reward, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) EndedRewards(ctx context.Context, now time.Time, limit int) ([]domain.Reward, error) {
	var ms []reward
	err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL AND status IN ? AND end_at <= ?",
			[]string{contracts.RewardActive, contracts.RewardPaused, contracts.RewardDepleted}, now).
		Order("end_at").Limit(limit).Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "list ended rewards", err)
	}
	out := make([]domain.Reward, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// ---- claims ----

func (r *Postgres) InsertClaim(ctx context.Context, tx *gorm.DB, c domain.Claim) error {
	m := claimFromDomain(c)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		switch constraintOf(err) {
		case "ux_claims_client_req", "ux_claims_grant_key":
			return domain.ErrDuplicateRequest
		case "ux_claims_code":
			return domain.ErrCodeCollision
		}
		return errs.Wrap(errs.Internal, "insert reward claim", err)
	}
	return nil
}

func (r *Postgres) ClaimByID(ctx context.Context, tenantID, id string) (domain.Claim, error) {
	if !isUUID(tenantID) || !isUUID(id) {
		return domain.Claim{}, domain.ErrClaimNotFound
	}
	return r.loadClaim(r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (r *Postgres) ClaimForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Claim, error) {
	if !isUUID(tenantID) || !isUUID(id) {
		return domain.Claim{}, domain.ErrClaimNotFound
	}
	return r.loadClaim(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND id = ?", tenantID, id))
}

func (r *Postgres) ClaimByClientRequest(ctx context.Context, tenantID, key string) (domain.Claim, error) {
	if !isUUID(tenantID) {
		return domain.Claim{}, domain.ErrClaimNotFound
	}
	return r.loadClaim(r.db.WithContext(ctx).Where("tenant_id = ? AND client_request_id = ?", tenantID, key))
}

func (r *Postgres) ClaimByGrantKey(ctx context.Context, tenantID, key string) (domain.Claim, error) {
	if !isUUID(tenantID) {
		return domain.Claim{}, domain.ErrClaimNotFound
	}
	return r.loadClaim(r.db.WithContext(ctx).Where("tenant_id = ? AND grant_key = ?", tenantID, key))
}

func (r *Postgres) loadClaim(q *gorm.DB) (domain.Claim, error) {
	var m rewardClaim
	err := q.First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Claim{}, domain.ErrClaimNotFound
	case err != nil:
		return domain.Claim{}, errs.Wrap(errs.Internal, "load reward claim", err)
	}
	return m.toDomain(), nil
}

// SaveClaim writes the mutable columns back guarded by the version.
func (r *Postgres) SaveClaim(ctx context.Context, tx *gorm.DB, c domain.Claim) error {
	m := claimFromDomain(c)
	res := tx.WithContext(ctx).Model(&rewardClaim{}).
		Where("tenant_id = ? AND id = ? AND version = ?", m.TenantID, m.ID, m.Version).
		Updates(map[string]any{
			"status":          m.Status,
			"reject_reason":   m.RejectReason,
			"hold_expires_at": m.HoldExpiresAt,
			"claimed_at":      m.ClaimedAt,
			"redeemed_at":     m.RedeemedAt,
			"expires_at":      m.ExpiresAt,
			"cancelled_at":    m.CancelledAt,
			"fulfilled_at":    m.FulfilledAt,
			"code":            m.Code,
			"updated_at":      m.UpdatedAt,
			"version":         gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		if constraintOf(res.Error) == "ux_claims_code" {
			return domain.ErrCodeCollision
		}
		return errs.Wrap(errs.Internal, "save reward claim", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) CountHeldClaims(ctx context.Context, tx *gorm.DB, tenantID, rewardID, playerID string) (int, error) {
	var n int64
	err := tx.WithContext(ctx).Model(&rewardClaim{}).
		Where("tenant_id = ? AND reward_id = ? AND player_id = ? AND status IN ?", tenantID, rewardID, playerID, heldStatuses).
		Count(&n).Error
	if err != nil {
		return 0, errs.Wrap(errs.Internal, "count held claims", err)
	}
	return int(n), nil
}

func (r *Postgres) ListPlayerClaims(ctx context.Context, tenantID, playerID string, p app.Page) ([]domain.Claim, error) {
	if !isUUID(tenantID) || !isUUID(playerID) {
		return nil, nil
	}
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND player_id = ?", tenantID, playerID)
	if !p.AfterTime.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", p.AfterTime, p.AfterID)
	}
	return r.findClaims(q.Order("created_at DESC, id DESC").Limit(p.Limit), "list player claims")
}

// ListClaims is the tenant-wide claim history, keyset over (created_at, id).
func (r *Postgres) ListClaims(ctx context.Context, tenantID string, f app.ClaimFilter, p app.Page) ([]domain.Claim, error) {
	if !isUUID(tenantID) {
		return nil, nil
	}
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.RewardID != "" {
		q = q.Where("reward_id = ?", f.RewardID)
	}
	if f.PlayerID != "" {
		q = q.Where("player_id = ?", f.PlayerID)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at < ?", *f.To)
	}
	if !p.AfterTime.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", p.AfterTime, p.AfterID)
	}
	return r.findClaims(q.Order("created_at DESC, id DESC").Limit(p.Limit), "list claims")
}

const rewardStatsSQL = `
SELECT r.id AS reward_id, r.slug, r.name, r.type, r.deleted_at IS NOT NULL AS deleted,
       COALESCE(c.claimed, 0) AS claimed, COALESCE(c.redeemed, 0) AS redeemed,
       COALESCE(c.expired, 0) AS expired, COALESCE(c.cancelled, 0) AS cancelled,
       COALESCE(c.points_spent, 0) AS points_spent
FROM ` + schema + `rewards r
LEFT JOIN (
    SELECT reward_id,
           COUNT(*) FILTER (WHERE claimed_at IS NOT NULL) AS claimed,
           COUNT(*) FILTER (WHERE status = 'redeemed') AS redeemed,
           COUNT(*) FILTER (WHERE status = 'expired') AS expired,
           COUNT(*) FILTER (WHERE status IN ('cancelled','refund_pending','refunded')) AS cancelled,
           COALESCE(SUM(points_cost) FILTER (WHERE status IN ('claimed','redeemed','expired')), 0) AS points_spent
    FROM ` + schema + `reward_claims
    WHERE tenant_id = ?
    GROUP BY reward_id
) c ON c.reward_id = r.id
WHERE r.tenant_id = ? AND (r.deleted_at IS NULL OR c.reward_id IS NOT NULL)
ORDER BY r.created_at DESC, r.id DESC`

type rewardStatsRow struct {
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

func (r *Postgres) RewardStats(ctx context.Context, tenantID string) ([]app.RewardStats, error) {
	if !isUUID(tenantID) {
		return nil, nil
	}
	var rows []rewardStatsRow
	if err := r.db.WithContext(ctx).Raw(rewardStatsSQL, tenantID, tenantID).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "reward stats", err)
	}
	out := make([]app.RewardStats, len(rows))
	for i, row := range rows {
		out[i] = app.RewardStats(row)
	}
	return out, nil
}

func (r *Postgres) DuePendingClaims(ctx context.Context, now time.Time, limit int) ([]domain.Claim, error) {
	q := r.db.WithContext(ctx).
		Where("status = ? AND hold_expires_at <= ?", contracts.ClaimPendingPayment, now).
		Order("hold_expires_at").Limit(limit)
	return r.findClaims(q, "list due pending claims")
}

func (r *Postgres) PaidCancelledSince(ctx context.Context, since time.Time, limit int) ([]domain.Claim, error) {
	q := r.db.WithContext(ctx).
		Where("status = ? AND points_cost > 0 AND updated_at >= ?", contracts.ClaimCancelled, since).
		Order("updated_at").Limit(limit)
	return r.findClaims(q, "list cancelled paid claims")
}

func (r *Postgres) RefundPendingClaims(ctx context.Context, limit int) ([]domain.Claim, error) {
	q := r.db.WithContext(ctx).
		Where("status = ?", contracts.ClaimRefundPending).
		Order("updated_at").Limit(limit)
	return r.findClaims(q, "list refund pending claims")
}

func (r *Postgres) DueExpiringClaims(ctx context.Context, now time.Time, limit int) ([]domain.Claim, error) {
	q := r.db.WithContext(ctx).
		Where("status = ? AND expires_at <= ?", contracts.ClaimClaimed, now).
		Order("expires_at").Limit(limit)
	return r.findClaims(q, "list expiring claims")
}

func (r *Postgres) findClaims(q *gorm.DB, what string) ([]domain.Claim, error) {
	var ms []rewardClaim
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, what, err)
	}
	out := make([]domain.Claim, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// ---- markers, tenant purge ----

func (r *Postgres) LastRun(ctx context.Context, job string) (time.Time, error) {
	var m reconcileMarker
	err := r.db.WithContext(ctx).First(&m, "job = ?", job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil // never ran: sweep everything
	}
	if err != nil {
		return time.Time{}, errs.Wrap(errs.Internal, "load reconcile marker", err)
	}
	return m.LastRun, nil
}

func (r *Postgres) MarkRun(ctx context.Context, job string, at time.Time) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "job"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_run"}),
		}).
		Create(&reconcileMarker{Job: job, LastRun: at}).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "mark reconcile run", err)
	}
	return nil
}

func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	if !isUUID(tenantID) {
		return errs.New(errs.Invalid, "tenant id must be a uuid")
	}
	if err := tx.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(&rewardClaim{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge reward claims", err)
	}
	if err := tx.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(&reward{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge rewards", err)
	}
	return nil
}

// ---- mapping ----

func (m reward) toDomain() domain.Reward {
	d := domain.Reward{
		ID: m.ID, TenantID: m.TenantID, Name: m.Name, Slug: m.Slug,
		Type: m.Type, Status: m.Status, PointsCost: m.PointsCost,
		Value: m.Value, ValueType: m.ValueType,
		BadgeRewardID: m.BadgeRewardID, LevelRewardID: m.LevelRewardID,
		MaxRedemptions: m.MaxRedemptions, MaxPerPlayer: m.MaxPerPlayer, StockUsed: m.StockUsed,
		ClaimTTLDays: m.ClaimTTLDays, StartAt: utcPtr(m.StartAt), EndAt: utcPtr(m.EndAt),
		LevelRequirement: m.LevelRequirement, IsActive: m.IsActive, Version: m.Version,
		CreatedAt: m.CreatedAt.UTC(), UpdatedAt: m.UpdatedAt.UTC(),
	}
	if m.Description != nil {
		d.Description = *m.Description
	}
	if m.Metadata != nil {
		_ = json.Unmarshal([]byte(*m.Metadata), &d.Metadata)
	}
	return d
}

func rewardFromDomain(d domain.Reward) (reward, error) {
	m := reward{
		ID: d.ID, TenantID: d.TenantID, Name: d.Name, Slug: d.Slug,
		Type: d.Type, Status: d.Status, PointsCost: d.PointsCost,
		Value: d.Value, ValueType: d.ValueType,
		BadgeRewardID: d.BadgeRewardID, LevelRewardID: d.LevelRewardID,
		MaxRedemptions: d.MaxRedemptions, MaxPerPlayer: d.MaxPerPlayer, StockUsed: d.StockUsed,
		ClaimTTLDays: d.ClaimTTLDays, StartAt: d.StartAt, EndAt: d.EndAt,
		LevelRequirement: d.LevelRequirement, IsActive: d.IsActive, Version: d.Version,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if d.Description != "" {
		desc := d.Description
		m.Description = &desc
	}
	if d.Metadata != nil {
		b, err := json.Marshal(d.Metadata)
		if err != nil {
			return reward{}, errs.Wrap(errs.Invalid, "metadata is not JSON-serialisable", err)
		}
		s := string(b)
		m.Metadata = &s
	}
	return m, nil
}

func (m rewardClaim) toDomain() domain.Claim {
	d := domain.Claim{
		ID: m.ID, TenantID: m.TenantID, PlayerID: m.PlayerID, RewardID: m.RewardID,
		RewardSlug: m.RewardSlug, RewardType: m.RewardType, Status: m.Status,
		PointsCost: m.PointsCost, DebitKey: m.DebitKey,
		ClientRequestID: m.ClientRequestID, GrantKey: m.GrantKey,
		HoldExpiresAt: utcPtr(m.HoldExpiresAt), ClaimedAt: utcPtr(m.ClaimedAt),
		RedeemedAt: utcPtr(m.RedeemedAt), ExpiresAt: utcPtr(m.ExpiresAt), CancelledAt: utcPtr(m.CancelledAt),
		FulfilledAt: utcPtr(m.FulfilledAt), Code: m.Code, Version: m.Version,
		CreatedAt: m.CreatedAt.UTC(), UpdatedAt: m.UpdatedAt.UTC(),
	}
	if m.RejectReason != nil {
		d.RejectReason = *m.RejectReason
	}
	return d
}

func claimFromDomain(d domain.Claim) rewardClaim {
	m := rewardClaim{
		ID: d.ID, TenantID: d.TenantID, PlayerID: d.PlayerID, RewardID: d.RewardID,
		RewardSlug: d.RewardSlug, RewardType: d.RewardType, Status: d.Status,
		PointsCost: d.PointsCost, DebitKey: d.DebitKey,
		ClientRequestID: d.ClientRequestID, GrantKey: d.GrantKey,
		HoldExpiresAt: d.HoldExpiresAt, ClaimedAt: d.ClaimedAt, RedeemedAt: d.RedeemedAt,
		ExpiresAt: d.ExpiresAt, CancelledAt: d.CancelledAt, FulfilledAt: d.FulfilledAt, Code: d.Code, Version: d.Version,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if d.RejectReason != "" {
		reason := d.RejectReason
		m.RejectReason = &reason
	}
	return m
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// constraintOf returns the violated unique constraint's name, "" otherwise.
func constraintOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pgErr.ConstraintName
	}
	return ""
}
