package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/progression/internal/app"
	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/shared/errs"
)

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var (
	_ app.Repository = (*Postgres)(nil)
	_ app.Reconciler = (*Postgres)(nil)
)

var errProgressNotFound = errs.New(errs.NotFound, "player progress not found")

func (r *Postgres) conn(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

// table resolves a model's prefixed table name for the few raw statements.
func (r *Postgres) table(structName string) string {
	return r.db.NamingStrategy.TableName(structName)
}

func (r *Postgres) LockLadder(ctx context.Context, tx *gorm.DB, tenantID string) error {
	err := tx.WithContext(ctx).
		Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "progression.levels:"+tenantID).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "lock ladder", err)
	}
	return nil
}

func (r *Postgres) Ladder(ctx context.Context, tx *gorm.DB, tenantID string, includeInactive bool) (domain.Ladder, error) {
	q := r.conn(tx).WithContext(ctx).Where("tenant_id = ?", tenantID).Order("level_number ASC")
	if !includeInactive {
		q = q.Where("is_active")
	}
	var ms []level
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load ladder", err)
	}
	out := make([]domain.Level, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return domain.NewLadder(out), nil
}

func (r *Postgres) LevelByID(ctx context.Context, tenantID, levelID string) (domain.Level, error) {
	var m level
	err := r.db.WithContext(ctx).First(&m, "tenant_id = ? AND id = ?", tenantID, levelID).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Level{}, domain.ErrLevelNotFound
	case err != nil:
		return domain.Level{}, errs.Wrap(errs.Internal, "load level", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) CreateLevel(ctx context.Context, tx *gorm.DB, l domain.Level) error {
	m := levelFromDomain(l)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrLevelNumberTaken
		}
		return errs.Wrap(errs.Internal, "insert level", err)
	}
	return nil
}

func (r *Postgres) UpdateLevel(ctx context.Context, tx *gorm.DB, l domain.Level) error {
	m := levelFromDomain(l)
	res := tx.WithContext(ctx).Model(&level{}).
		Where("tenant_id = ? AND id = ?", m.TenantID, m.ID).
		Updates(map[string]any{
			"level_number":    m.LevelNumber,
			"name":            m.Name,
			"description":     m.Description,
			"xp_required":     m.XPRequired,
			"points_reward":   m.PointsReward,
			"badge_reward_id": m.BadgeRewardID,
			"perks":           m.Perks,
			"icon_url":        m.IconURL,
			"is_active":       m.IsActive,
			"updated_at":      m.UpdatedAt,
		})
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return domain.ErrLevelNumberTaken
		}
		return errs.Wrap(errs.Internal, "update level", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrLevelNotFound
	}
	return nil
}

func (r *Postgres) SoftDeleteLevel(ctx context.Context, tx *gorm.DB, tenantID, levelID string, at time.Time) error {
	res := tx.WithContext(ctx).Model(&level{}).
		Where("tenant_id = ? AND id = ?", tenantID, levelID).
		Updates(map[string]any{"deleted_at": at, "updated_at": at})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "delete level", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrLevelNotFound
	}
	return nil
}

func (r *Postgres) InsertGrant(ctx context.Context, tx *gorm.DB, g domain.XPGrant) (bool, error) {
	m := grantFromDomain(g)
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert xp grant", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) GrantByKey(ctx context.Context, tx *gorm.DB, tenantID, key string) (domain.XPGrant, error) {
	var m xpGrant
	err := r.conn(tx).WithContext(ctx).First(&m, "tenant_id = ? AND idempotency_key = ?", tenantID, key).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.XPGrant{}, errs.New(errs.NotFound, "xp grant not found")
	case err != nil:
		return domain.XPGrant{}, errs.Wrap(errs.Internal, "load xp grant", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) ListGrants(ctx context.Context, tenantID, playerID string, before time.Time, beforeID string, limit int) ([]domain.XPGrant, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND player_id = ?", tenantID, playerID).
		Order("created_at DESC, id DESC").
		Limit(limit)
	if !before.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", before, beforeID)
	}
	var ms []xpGrant
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list xp grants", err)
	}
	out := make([]domain.XPGrant, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) EnsureProgress(ctx context.Context, tx *gorm.DB, p domain.Progress) error {
	m := progressFromDomain(p)
	err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "player_id"}},
			DoNothing: true,
		}).
		Create(&m).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "ensure player progress", err)
	}
	return nil
}

func (r *Postgres) ProgressForUpdate(ctx context.Context, tx *gorm.DB, tenantID, playerID string) (domain.Progress, error) {
	var m playerProgress
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&m, "tenant_id = ? AND player_id = ?", tenantID, playerID).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Progress{}, errProgressNotFound
	case err != nil:
		return domain.Progress{}, errs.Wrap(errs.Internal, "lock player progress", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) SaveProgress(ctx context.Context, tx *gorm.DB, p domain.Progress) error {
	m := progressFromDomain(p)
	res := tx.WithContext(ctx).Model(&playerProgress{}).
		Where("tenant_id = ? AND player_id = ? AND version = ?", m.TenantID, m.PlayerID, m.Version).
		Updates(map[string]any{
			"total_xp":     m.TotalXP,
			"level_id":     m.LevelID,
			"level_number": m.LevelNumber,
			"updated_at":   m.UpdatedAt,
			"version":      gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "save player progress", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) ProgressByPlayers(ctx context.Context, tenantID string, playerIDs []string) (map[string]domain.Progress, error) {
	out := make(map[string]domain.Progress, len(playerIDs))
	if len(playerIDs) == 0 {
		return out, nil
	}
	var ms []playerProgress
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND player_id IN ?", tenantID, playerIDs).
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load player progress", err)
	}
	for _, m := range ms {
		out[m.PlayerID] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) InsertLevelReward(ctx context.Context, tx *gorm.DB, lr domain.LevelReward) (bool, error) {
	m := levelReward{ID: lr.ID, TenantID: lr.TenantID, PlayerID: lr.PlayerID, LevelID: lr.LevelID, GrantedAt: lr.GrantedAt}
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "player_id"}, {Name: "level_id"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert level reward", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) InsertRejection(ctx context.Context, tx *gorm.DB, rj domain.GrantRejection) (bool, error) {
	m := grantRejection{
		TenantID:       rj.TenantID,
		IdempotencyKey: rj.IdempotencyKey,
		PlayerID:       rj.PlayerID,
		Reason:         rj.Reason,
		CreatedAt:      rj.CreatedAt,
	}
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert grant rejection", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// PurgeTenant hard-deletes every row of the tenant, children first.
func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	for _, model := range []any{&levelReward{}, &playerProgress{}, &xpGrant{}, &grantRejection{}, &level{}} {
		if err := tx.WithContext(ctx).Unscoped().Where("tenant_id = ?", tenantID).Delete(model).Error; err != nil {
			return errs.Wrap(errs.Internal, "purge tenant", err)
		}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
