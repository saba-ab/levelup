package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/badges/internal/app"
	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/shared/errs"
)

// schema qualifies raw SQL: Deps.DB pins the prefix only for model-derived
// table names, and search_path is not usable behind PgBouncer.
const schema = "badges_svc."

// Postgres implements app.Repository and app.Reconciler.
type Postgres struct{ db *gorm.DB }

var (
	_ app.Repository = (*Postgres)(nil)
	_ app.Reconciler = (*Postgres)(nil)
)

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ---- badges ----

func (r *Postgres) CreateBadge(ctx context.Context, tx *gorm.DB, b domain.Badge) error {
	m, err := badgeFromDomain(b)
	if err != nil {
		return errs.Wrap(errs.Invalid, "encode badge requirements", err)
	}
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "insert badge", err)
	}
	return nil
}

func (r *Postgres) loadBadge(ctx context.Context, q *gorm.DB, tenantID, id string) (domain.Badge, error) {
	var m badge
	err := q.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Badge{}, domain.ErrBadgeNotFound
	case err != nil:
		return domain.Badge{}, errs.Wrap(errs.Internal, "load badge", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) BadgeByID(ctx context.Context, tenantID, id string) (domain.Badge, error) {
	return r.loadBadge(ctx, r.db, tenantID, id)
}

func (r *Postgres) BadgeByIDTx(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Badge, error) {
	return r.loadBadge(ctx, tx, tenantID, id)
}

func (r *Postgres) BadgeByIDForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Badge, error) {
	return r.loadBadge(ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}), tenantID, id)
}

func (r *Postgres) SaveBadge(ctx context.Context, tx *gorm.DB, b domain.Badge) error {
	m, err := badgeFromDomain(b)
	if err != nil {
		return errs.Wrap(errs.Invalid, "encode badge requirements", err)
	}
	res := tx.WithContext(ctx).Model(&badge{}).
		Where("id = ? AND tenant_id = ? AND version = ?", m.ID, m.TenantID, m.Version).
		Updates(map[string]any{
			"slug":         m.Slug,
			"name":         m.Name,
			"description":  m.Description,
			"icon_url":     m.IconURL,
			"tier":         m.Tier,
			"category":     m.Category,
			"points_value": m.PointsValue,
			"is_stackable": m.IsStackable,
			"max_awards":   m.MaxAwards,
			"requirements": m.Requirements,
			"is_active":    m.IsActive,
			"is_secret":    m.IsSecret,
			"sort_order":   m.SortOrder,
			"updated_at":   m.UpdatedAt,
			"deleted_at":   m.DeletedAt,
			"version":      gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "save badge", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) ListBadges(ctx context.Context, tenantID string, f app.BadgeFilter) ([]domain.Badge, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("created_at DESC, id DESC").
		Limit(f.Limit)
	if f.Tier != "" {
		q = q.Where("tier = ?", f.Tier)
	}
	if f.Category != "" {
		q = q.Where("category = ?", f.Category)
	}
	if f.Active != nil {
		q = q.Where("is_active = ?", *f.Active)
	}
	if !f.Before.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", f.Before, f.BeforeID)
	}
	var ms []badge
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list badges", err)
	}
	out := make([]domain.Badge, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) BadgesByIDs(ctx context.Context, tenantID string, ids []string, includeDeleted bool) ([]domain.Badge, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tenantID, ids)
	if !includeDeleted {
		q = q.Where("deleted_at IS NULL")
	}
	var ms []badge
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load badges", err)
	}
	out := make([]domain.Badge, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// ---- player badges ----

// LockOrCreatePlayerBadge serializes every award of one (player, badge):
// the placeholder insert waits on a concurrent first award's uncommitted
// row and then does nothing, and FOR UPDATE queues the rest. This is what
// makes "concurrent stackable awards never exceed max_awards" true (B16)
// and turns the Laravel unique-violation 500 into a normal second award.
func (r *Postgres) LockOrCreatePlayerBadge(ctx context.Context, tx *gorm.DB, placeholder domain.PlayerBadge) (domain.PlayerBadge, error) {
	m := playerBadgeFromDomain(placeholder)
	if err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "player_id"}, {Name: "badge_id"}},
			DoNothing: true,
		}).
		Create(&m).Error; err != nil {
		return domain.PlayerBadge{}, errs.Wrap(errs.Internal, "insert player badge", err)
	}
	pb, found, err := r.LockPlayerBadge(ctx, tx, placeholder.TenantID, placeholder.PlayerID, placeholder.BadgeID)
	if err != nil {
		return domain.PlayerBadge{}, err
	}
	if !found {
		// The conflicting row belongs to another tenant: impossible while
		// badge ids are tenant-scoped, so treat it as corruption.
		return domain.PlayerBadge{}, errs.New(errs.Internal, "player badge row exists outside the tenant")
	}
	return pb, nil
}

func (r *Postgres) LockPlayerBadge(ctx context.Context, tx *gorm.DB, tenantID, playerID, badgeID string) (domain.PlayerBadge, bool, error) {
	return r.findPlayerBadge(ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}), tenantID, playerID, badgeID)
}

func (r *Postgres) PlayerBadge(ctx context.Context, tenantID, playerID, badgeID string) (domain.PlayerBadge, bool, error) {
	return r.findPlayerBadge(ctx, r.db, tenantID, playerID, badgeID)
}

func (r *Postgres) findPlayerBadge(ctx context.Context, q *gorm.DB, tenantID, playerID, badgeID string) (domain.PlayerBadge, bool, error) {
	var m playerBadge
	err := q.WithContext(ctx).
		Where("tenant_id = ? AND player_id = ? AND badge_id = ?", tenantID, playerID, badgeID).
		First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.PlayerBadge{}, false, nil
	case err != nil:
		return domain.PlayerBadge{}, false, errs.Wrap(errs.Internal, "load player badge", err)
	}
	return m.toDomain(), true, nil
}

func (r *Postgres) SavePlayerBadge(ctx context.Context, tx *gorm.DB, pb domain.PlayerBadge) error {
	res := tx.WithContext(ctx).Model(&playerBadge{}).
		Where("id = ? AND version = ?", pb.ID, pb.Version).
		Updates(map[string]any{
			"earned_count":     pb.EarnedCount,
			"first_awarded_at": pb.FirstAwardedAt,
			"last_awarded_at":  pb.LastAwardedAt,
			"updated_at":       pb.UpdatedAt,
			"version":          gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "save player badge", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) DeletePlayerBadge(ctx context.Context, tx *gorm.DB, pb domain.PlayerBadge) error {
	if err := tx.WithContext(ctx).Where("id = ?", pb.ID).Delete(&playerBadge{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "delete player badge", err)
	}
	return nil
}

func (r *Postgres) ListPlayerBadges(ctx context.Context, tenantID, playerID string, cur app.PageCursor, limit int) ([]domain.PlayerBadge, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND player_id = ? AND earned_count > 0", tenantID, playerID).
		Order("created_at DESC, id DESC").
		Limit(limit)
	if !cur.Before.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", cur.Before, cur.BeforeID)
	}
	var ms []playerBadge
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list player badges", err)
	}
	out := make([]domain.PlayerBadge, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) PlayerBadgesByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) ([]domain.PlayerBadge, error) {
	if len(playerIDs) == 0 {
		return nil, nil
	}
	var ms []playerBadge
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND player_id IN ? AND earned_count > 0", tenantID, playerIDs).
		Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load player badges", err)
	}
	out := make([]domain.PlayerBadge, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// ---- award ledger ----

func (r *Postgres) findAward(ctx context.Context, q *gorm.DB, tenantID, key string) (domain.Award, bool, error) {
	var m badgeAward
	err := q.WithContext(ctx).
		Where("tenant_id = ? AND idempotency_key = ?", tenantID, key).
		First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Award{}, false, nil
	case err != nil:
		return domain.Award{}, false, errs.Wrap(errs.Internal, "load badge award", err)
	}
	return m.toDomain(), true, nil
}

func (r *Postgres) AwardByKey(ctx context.Context, tenantID, key string) (domain.Award, bool, error) {
	return r.findAward(ctx, r.db, tenantID, key)
}

func (r *Postgres) AwardByKeyTx(ctx context.Context, tx *gorm.DB, tenantID, key string) (domain.Award, bool, error) {
	return r.findAward(ctx, tx, tenantID, key)
}

func (r *Postgres) InsertAward(ctx context.Context, tx *gorm.DB, a domain.Award) (bool, error) {
	m := awardFromDomain(a)
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert badge award", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) InsertRevocation(ctx context.Context, tx *gorm.DB, rev domain.Revocation) error {
	m := badgeRevocation{
		ID:                 rev.ID,
		TenantID:           rev.TenantID,
		PlayerID:           rev.PlayerID,
		BadgeID:            rev.BadgeID,
		PlayerBadgeID:      rev.PlayerBadgeID,
		EarnedCountRemoved: rev.EarnedCountRemoved,
		RevokedBy:          nullable(rev.RevokedBy),
		RevokedAt:          rev.RevokedAt,
	}
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		return errs.Wrap(errs.Internal, "insert badge revocation", err)
	}
	return nil
}

// ---- tenant purge ----

// PurgeTenant deletes every row of the tenant, children first (the only FK
// is player_badges.badge_id → badges.id). Running it twice deletes nothing
// the second time.
func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	for _, model := range []any{
		&badgeAward{}, &badgeRevocation{}, &playerBadge{}, &badge{},
		&badgePlayerStat{}, &badgePlayerActivityCount{}, &appliedEvent{},
	} {
		if err := tx.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(model).Error; err != nil {
			return errs.Wrap(errs.Internal, "purge tenant", err)
		}
	}
	return nil
}

// ---- reconcile ----

func (r *Postgres) LastRun(ctx context.Context) (time.Time, error) {
	var m reconcileMarker
	err := r.db.WithContext(ctx).First(&m, "id = 1").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil // never ran: sweep everything
	}
	if err != nil {
		return time.Time{}, errs.Wrap(errs.Internal, "load reconcile marker", err)
	}
	return m.LastRun.UTC(), nil
}

func (r *Postgres) MarkRun(ctx context.Context, at time.Time) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_run"}),
		}).
		Create(&reconcileMarker{ID: 1, LastRun: at}).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "mark reconcile run", err)
	}
	return nil
}

type driftRow struct {
	ID            string
	TenantID      string
	PlayerID      string
	BadgeID       string
	EarnedCount   int
	AppliedAwards int
	IsStackable   bool
	MaxAwards     *int
}

const driftSQL = `
SELECT pb.id, pb.tenant_id, pb.player_id, pb.badge_id, pb.earned_count,
       b.is_stackable, b.max_awards, COALESCE(a.applied, 0) AS applied_awards
FROM ` + schema + `player_badges pb
JOIN ` + schema + `badges b ON b.id = pb.badge_id
LEFT JOIN (
    SELECT player_badge_id, COUNT(*) AS applied
    FROM ` + schema + `badge_awards
    WHERE status = 'applied' AND player_badge_id IS NOT NULL
    GROUP BY player_badge_id
) a ON a.player_badge_id = pb.id
WHERE pb.updated_at >= ?
  AND (
        pb.earned_count <> COALESCE(a.applied, 0)
     OR (b.is_stackable AND b.max_awards IS NOT NULL AND pb.earned_count > b.max_awards)
     OR (NOT b.is_stackable AND pb.earned_count > 1)
  )
ORDER BY pb.updated_at, pb.id`

func (r *Postgres) DriftSince(ctx context.Context, since time.Time) ([]domain.Drift, error) {
	var rows []driftRow
	if err := r.db.WithContext(ctx).Raw(driftSQL, since).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "badges reconcile sweep", err)
	}
	out := make([]domain.Drift, len(rows))
	for i, d := range rows {
		out[i] = domain.Drift{
			PlayerBadgeID: d.ID,
			TenantID:      d.TenantID,
			PlayerID:      d.PlayerID,
			BadgeID:       d.BadgeID,
			EarnedCount:   d.EarnedCount,
			AppliedAwards: d.AppliedAwards,
			Stackable:     d.IsStackable,
			MaxAwards:     d.MaxAwards,
		}
	}
	return out, nil
}
