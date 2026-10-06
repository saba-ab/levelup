package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/streaks/internal/app"
	"levelup/internal/modules/streaks/internal/domain"
	"levelup/internal/shared/errs"
)

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

// ---- streak definitions ----

func (r *Postgres) CreateStreak(ctx context.Context, tx *gorm.DB, s domain.Streak) error {
	m := streakFromDomain(s)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		return mapStreakWriteErr(err, "insert streak")
	}
	return nil
}

func (r *Postgres) SaveStreak(ctx context.Context, tx *gorm.DB, s domain.Streak) error {
	m := streakFromDomain(s)
	res := tx.WithContext(ctx).Model(&streak{}).
		Where("id = ? AND tenant_id = ? AND version = ? AND deleted_at IS NULL", m.ID, m.TenantID, m.Version).
		Updates(map[string]any{
			"slug":              m.Slug,
			"name":              m.Name,
			"description":       m.Description,
			"activity_key":      m.ActivityKey,
			"grace_periods":     m.GracePeriods,
			"points_per_period": m.PointsPerPeriod,
			"milestones":        m.Milestones,
			"is_active":         m.IsActive,
			"auto_record":       m.AutoRecord,
			"updated_at":        m.UpdatedAt,
			"version":           gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return mapStreakWriteErr(res.Error, "save streak")
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) SoftDeleteStreak(ctx context.Context, tx *gorm.DB, tenantID, id string, at time.Time) error {
	res := tx.WithContext(ctx).Model(&streak{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Updates(map[string]any{"deleted_at": at, "updated_at": at, "version": gorm.Expr("version + 1")})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "delete streak", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrStreakNotFound
	}
	return nil
}

func (r *Postgres) StreakByID(ctx context.Context, tenantID, id string) (domain.Streak, error) {
	if !isUUID(tenantID) || !isUUID(id) {
		return domain.Streak{}, domain.ErrStreakNotFound
	}
	return r.oneStreak(ctx, "id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID)
}

func (r *Postgres) StreakByActivityKey(ctx context.Context, tenantID, key string) (domain.Streak, error) {
	if !isUUID(tenantID) {
		return domain.Streak{}, domain.ErrStreakNotFound
	}
	return r.oneStreak(ctx, "activity_key = ? AND tenant_id = ? AND deleted_at IS NULL", key, tenantID)
}

func (r *Postgres) oneStreak(ctx context.Context, where string, args ...any) (domain.Streak, error) {
	var m streak
	err := r.db.WithContext(ctx).Where(where, args...).First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Streak{}, domain.ErrStreakNotFound
	case err != nil:
		return domain.Streak{}, errs.Wrap(errs.Internal, "load streak", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) StreaksByIDs(ctx context.Context, tenantID string, ids []string) (map[string]domain.Streak, error) {
	out := map[string]domain.Streak{}
	ids = filterUUIDs(ids)
	if len(ids) == 0 || !isUUID(tenantID) {
		return out, nil
	}
	var ms []streak
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id IN ? AND deleted_at IS NULL", tenantID, ids).
		Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load streaks", err)
	}
	for _, m := range ms {
		out[m.ID] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) ListStreaks(ctx context.Context, tenantID string, f app.ListFilter, p app.Page) ([]domain.Streak, error) {
	if !isUUID(tenantID) {
		return nil, nil
	}
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("created_at DESC, id DESC").
		Limit(p.Limit)
	if f.Active != nil {
		q = q.Where("is_active = ?", *f.Active)
	}
	if f.Period != "" {
		q = q.Where("period = ?", f.Period)
	}
	if !p.Before.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", p.Before, p.BeforeID)
	}
	var ms []streak
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list streaks", err)
	}
	out := make([]domain.Streak, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// ---- player streaks ----

// EnsurePlayerStreak inserts the row unless (player_id, streak_id) exists:
// concurrent first records no longer race into a 500 (S8).
func (r *Postgres) EnsurePlayerStreak(ctx context.Context, tx *gorm.DB, ps domain.PlayerStreak) error {
	m := playerStreakFromDomain(ps)
	err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "player_id"}, {Name: "streak_id"}},
			DoNothing: true,
		}).
		Create(&m).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "insert player streak", err)
	}
	return nil
}

// PlayerStreakForUpdate takes the row lock: records, resets and the sweep
// serialize on the player streak row.
func (r *Postgres) PlayerStreakForUpdate(ctx context.Context, tx *gorm.DB, tenantID, playerID, streakID string) (domain.PlayerStreak, error) {
	if !isUUID(tenantID) || !isUUID(playerID) || !isUUID(streakID) {
		return domain.PlayerStreak{}, domain.ErrPlayerStreakNotFound
	}
	var m playerStreak
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND player_id = ? AND streak_id = ?", tenantID, playerID, streakID).
		First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.PlayerStreak{}, domain.ErrPlayerStreakNotFound
	case err != nil:
		return domain.PlayerStreak{}, errs.Wrap(errs.Internal, "lock player streak", err)
	}
	return m.toDomain(), nil
}

// SavePlayerStreak writes back guarded by the optimistic version.
func (r *Postgres) SavePlayerStreak(ctx context.Context, tx *gorm.DB, ps domain.PlayerStreak) error {
	m := playerStreakFromDomain(ps)
	res := tx.WithContext(ctx).Model(&playerStreak{}).
		Where("id = ? AND version = ?", m.ID, m.Version).
		Updates(map[string]any{
			"current_count":         m.CurrentCount,
			"longest_count":         m.LongestCount,
			"last_period_start":     m.LastPeriodStart,
			"run_started_at":        m.RunStartedAt,
			"run_floor":             m.RunFloor,
			"broken_at":             m.BrokenAt,
			"broken_run_started_at": m.BrokenRunStartedAt,
			"updated_at":            m.UpdatedAt,
			"version":               gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "save player streak", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) PlayerStreaksByPlayers(ctx context.Context, tenantID string, playerIDs []string) ([]domain.PlayerStreak, error) {
	playerIDs = filterUUIDs(playerIDs)
	if len(playerIDs) == 0 || !isUUID(tenantID) {
		return nil, nil
	}
	var ms []playerStreak
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND player_id IN ?", tenantID, playerIDs).
		Order("current_count DESC, id").
		Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load player streaks", err)
	}
	out := make([]domain.PlayerStreak, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// ---- buckets ----

// InsertPeriod reports whether the bucket is new. ON CONFLICT DO NOTHING on
// (player_streak_id, period_start): the same period twice is a no-op.
func (r *Postgres) InsertPeriod(ctx context.Context, tx *gorm.DB, b domain.PeriodBucket) (bool, error) {
	m := streakPeriod{
		ID:             b.ID,
		TenantID:       b.TenantID,
		PlayerStreakID: b.PlayerStreakID,
		PeriodStart:    b.PeriodStart,
		IdempotencyKey: b.IdempotencyKey,
		RecordedAt:     b.RecordedAt,
	}
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "player_streak_id"}, {Name: "period_start"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert streak period", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) PeriodStarts(ctx context.Context, tx *gorm.DB, playerStreakID string) ([]time.Time, error) {
	var starts []time.Time
	if err := tx.WithContext(ctx).Model(&streakPeriod{}).
		Where("player_streak_id = ?", playerStreakID).
		Order("period_start").
		Pluck("period_start", &starts).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load streak periods", err)
	}
	for i := range starts {
		starts[i] = starts[i].UTC()
	}
	return starts, nil
}

// ---- milestone awards ----

func (r *Postgres) AwardedMilestonesBetween(ctx context.Context, tx *gorm.DB, playerStreakID string, from, to time.Time) (map[int]bool, error) {
	var ms []int
	if err := tx.WithContext(ctx).Model(&streakMilestoneAward{}).
		Where("player_streak_id = ? AND run_started_at BETWEEN ? AND ?", playerStreakID, from, to).
		Pluck("milestone", &ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load milestone awards", err)
	}
	out := make(map[int]bool, len(ms))
	for _, m := range ms {
		out[m] = true
	}
	return out, nil
}

func (r *Postgres) InsertMilestoneAward(ctx context.Context, tx *gorm.DB, a domain.MilestoneAward) (bool, error) {
	m := streakMilestoneAward{
		ID:             a.ID,
		TenantID:       a.TenantID,
		PlayerStreakID: a.PlayerStreakID,
		Milestone:      a.Milestone,
		BonusPoints:    a.BonusPoints,
		RunStartedAt:   a.RunStartedAt,
		AwardedAt:      a.AwardedAt,
	}
	res := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert milestone award", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// ---- record requests ----

func (r *Postgres) RequestExists(ctx context.Context, tenantID, key string) (bool, error) {
	if !isUUID(tenantID) {
		return false, nil
	}
	var n int64
	if err := r.db.WithContext(ctx).Model(&recordRequest{}).
		Where("tenant_id = ? AND idempotency_key = ?", tenantID, key).
		Count(&n).Error; err != nil {
		return false, errs.Wrap(errs.Internal, "load record request", err)
	}
	return n > 0, nil
}

func (r *Postgres) InsertRequest(ctx context.Context, tx *gorm.DB, req domain.RecordRequest) (bool, error) {
	if !isUUID(req.TenantID) {
		return false, errs.New(errs.Invalid, "tenant_id must be a uuid")
	}
	m := recordRequest{
		TenantID:       req.TenantID,
		IdempotencyKey: req.IdempotencyKey,
		PlayerID:       req.PlayerID,
		StreakID:       req.StreakID,
		ActivityKey:    req.ActivityKey,
		Status:         req.Status,
		Reason:         req.Reason,
		CreatedAt:      req.CreatedAt,
	}
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert record request", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) PruneRequests(ctx context.Context, before time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&recordRequest{})
	if res.Error != nil {
		return 0, errs.Wrap(errs.Internal, "prune record requests", res.Error)
	}
	return res.RowsAffected, nil
}

// ---- sweep ----

func (r *Postgres) LiveStreakRefs(ctx context.Context) ([]app.StreakRef, error) {
	var rows []struct {
		TenantID string
		StreakID string
	}
	if err := r.db.WithContext(ctx).Model(&playerStreak{}).
		Distinct("tenant_id", "streak_id").
		Where("current_count > 0").
		Find(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load live streaks", err)
	}
	out := make([]app.StreakRef, len(rows))
	for i, x := range rows {
		out[i] = app.StreakRef{TenantID: x.TenantID, StreakID: x.StreakID}
	}
	return out, nil
}

// LapsedForUpdate locks a batch of live runs whose newest bucket is before
// the threshold. SKIP LOCKED: a row a record holds right now is left to the
// next tick instead of blocking the sweep.
func (r *Postgres) LapsedForUpdate(ctx context.Context, tx *gorm.DB, tenantID, streakID string, before time.Time, limit int) ([]domain.PlayerStreak, error) {
	var ms []playerStreak
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("tenant_id = ? AND streak_id = ? AND current_count > 0 AND last_period_start < ?", tenantID, streakID, before).
		Order("id").
		Limit(limit).
		Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "lock lapsed player streaks", err)
	}
	out := make([]domain.PlayerStreak, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) LastRun(ctx context.Context, job string) (time.Time, error) {
	var m reconcileMarker
	err := r.db.WithContext(ctx).First(&m, "job = ?", job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil // never ran
	}
	if err != nil {
		return time.Time{}, errs.Wrap(errs.Internal, "load reconcile marker", err)
	}
	return m.LastRun.UTC(), nil
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

// ---- purge ----

func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	if !isUUID(tenantID) {
		return errs.New(errs.Invalid, "tenant_id must be a uuid")
	}
	db := tx.WithContext(ctx)
	for _, model := range []any{&streakMilestoneAward{}, &streakPeriod{}, &playerStreak{}, &recordRequest{}, &streak{}} {
		if err := db.Where("tenant_id = ?", tenantID).Delete(model).Error; err != nil {
			return errs.Wrap(errs.Internal, "purge tenant", err)
		}
	}
	return nil
}

func (r *Postgres) DeletePlayer(ctx context.Context, tx *gorm.DB, tenantID, playerID string) error {
	if !isUUID(tenantID) || !isUUID(playerID) {
		return errs.New(errs.Invalid, "tenant_id and player_id must be uuids")
	}
	db := tx.WithContext(ctx)
	owned := db.Model(&playerStreak{}).Select("id").Where("tenant_id = ? AND player_id = ?", tenantID, playerID)
	if err := db.Where("player_streak_id IN (?)", owned).Delete(&streakMilestoneAward{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "delete player milestone awards", err)
	}
	if err := db.Where("player_streak_id IN (?)", owned).Delete(&streakPeriod{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "delete player streak periods", err)
	}
	if err := db.Where("tenant_id = ? AND player_id = ?", tenantID, playerID).Delete(&playerStreak{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "delete player streaks", err)
	}
	return nil
}

// ---- helpers ----

// isUUID guards uuid columns: a malformed id is "not found", never a 500
// from Postgres rejecting the cast.
func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func filterUUIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if isUUID(id) {
			out = append(out, id)
		}
	}
	return out
}

func mapStreakWriteErr(err error, op string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "ux_streaks_tenant_slug":
			return domain.ErrSlugTaken
		case "ux_streaks_tenant_activity_key":
			return domain.ErrActivityKeyTaken
		}
		return errs.Wrap(errs.AlreadyExists, op, err)
	}
	return errs.Wrap(errs.Internal, op, err)
}
