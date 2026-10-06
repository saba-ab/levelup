// Package repo implements missions' persistence. Models stay unexported;
// table names derive from struct names under the missions_svc. prefix (no
// TableName overrides).
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

	"levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/missions/internal/app"
	"levelup/internal/modules/missions/internal/domain"
	"levelup/internal/shared/errs"
)

// mission → missions_svc.missions.
type mission struct {
	ID                      string `gorm:"primaryKey;type:uuid"`
	TenantID                string `gorm:"type:uuid"`
	Slug                    string
	Name                    string
	Description             string
	Type                    string
	Status                  string
	Target                  int64
	Criteria                string `gorm:"type:jsonb"`
	PointsReward            int64
	XPReward                int64   `gorm:"column:xp_reward"`
	BadgeRewardID           *string `gorm:"type:uuid"`
	MaxCompletionsPerPlayer *int    `gorm:"column:max_completions_per_player"`
	StartsAt                *time.Time
	EndsAt                  *time.Time
	Version                 int
	CreatedAt               time.Time
	UpdatedAt               time.Time
	DeletedAt               *time.Time
}

// missionAttempt → missions_svc.mission_attempts.
type missionAttempt struct {
	ID           string `gorm:"primaryKey;type:uuid"`
	TenantID     string `gorm:"type:uuid"`
	MissionID    string `gorm:"type:uuid"`
	PlayerID     string `gorm:"type:uuid"`
	Status       string
	Progress     int64
	Target       int64
	PeriodKey    string
	PeriodEndsAt *time.Time
	StartedAt    time.Time
	CompletedAt  *time.Time
	Version      int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// progressEvent → missions_svc.progress_events.
type progressEvent struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid"`
	IdempotencyKey string
	MissionID      string  `gorm:"type:uuid"`
	PlayerID       string  `gorm:"type:uuid"`
	AttemptID      *string `gorm:"type:uuid"`
	Increment      int64
	Status         string
	Reason         string
	SourceKind     string
	SourceID       string
	CreatedAt      time.Time
}

// reconcileMarker → missions_svc.reconcile_markers.
type reconcileMarker struct {
	Job     string `gorm:"primaryKey"`
	LastRun time.Time
}

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

// ---- missions ----

func (r *Postgres) CreateMission(ctx context.Context, tx *gorm.DB, m domain.Mission) error {
	row, err := missionFromDomain(m)
	if err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "insert mission", err)
	}
	return nil
}

func (r *Postgres) MissionByID(ctx context.Context, tenantID, id string) (domain.Mission, error) {
	return r.loadMission(r.db.WithContext(ctx), tenantID, id, false)
}

func (r *Postgres) MissionInTx(ctx context.Context, tx *gorm.DB, tenantID, id string, lock bool) (domain.Mission, error) {
	return r.loadMission(tx.WithContext(ctx), tenantID, id, lock)
}

func (r *Postgres) loadMission(db *gorm.DB, tenantID, id string, lock bool) (domain.Mission, error) {
	if !validUUID(id) {
		return domain.Mission{}, domain.ErrMissionNotFound
	}
	if lock {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []mission
	err := db.Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantID, id).Limit(1).Find(&rows).Error
	if err != nil {
		return domain.Mission{}, errs.Wrap(errs.Internal, "load mission", err)
	}
	if len(rows) == 0 {
		return domain.Mission{}, domain.ErrMissionNotFound
	}
	return rows[0].toDomain()
}

func (r *Postgres) SaveMission(ctx context.Context, tx *gorm.DB, m domain.Mission) error {
	row, err := missionFromDomain(m)
	if err != nil {
		return err
	}
	res := tx.WithContext(ctx).Model(&mission{}).
		Where("tenant_id = ? AND id = ? AND version = ?", row.TenantID, row.ID, row.Version).
		Updates(map[string]any{
			"slug":                       row.Slug,
			"name":                       row.Name,
			"description":                row.Description,
			"type":                       row.Type,
			"status":                     row.Status,
			"target":                     row.Target,
			"criteria":                   row.Criteria,
			"points_reward":              row.PointsReward,
			"xp_reward":                  row.XPReward,
			"badge_reward_id":            row.BadgeRewardID,
			"max_completions_per_player": row.MaxCompletionsPerPlayer,
			"starts_at":                  row.StartsAt,
			"ends_at":                    row.EndsAt,
			"deleted_at":                 row.DeletedAt,
			"updated_at":                 row.UpdatedAt,
			"version":                    gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "save mission", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) ListMissions(ctx context.Context, tenantID string, f app.MissionFilter, after app.Cursor, limit int) ([]domain.Mission, error) {
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	q = keyset(q, after)
	var rows []mission
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list missions", err)
	}
	out := make([]domain.Mission, 0, len(rows))
	for _, row := range rows {
		m, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *Postgres) MissionsByIDs(ctx context.Context, tenantID string, ids []string) (map[string]domain.Mission, error) {
	ids = onlyUUIDs(ids)
	out := make(map[string]domain.Mission, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []mission
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tenantID, ids).Find(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load missions", err)
	}
	for _, row := range rows {
		m, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		out[m.ID] = m
	}
	return out, nil
}

// AutoMissions uses ix_missions_auto_event_type.
func (r *Postgres) AutoMissions(ctx context.Context, tenantID, eventType string) ([]domain.Mission, error) {
	if !validUUID(tenantID) || eventType == "" {
		return nil, nil
	}
	var rows []mission
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL AND status = ? AND criteria ->> 'event_type' = ?",
			tenantID, contracts.MissionActive, eventType).
		Order("created_at, id").
		Find(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load auto missions", err)
	}
	out := make([]domain.Mission, 0, len(rows))
	for _, row := range rows {
		m, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// ---- attempts ----

func (r *Postgres) OpenAttemptForUpdate(ctx context.Context, tx *gorm.DB, tenantID, missionID, playerID, periodKey string) (domain.Attempt, bool, error) {
	var rows []missionAttempt
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND mission_id = ? AND player_id = ? AND period_key = ? AND status = ?",
			tenantID, missionID, playerID, periodKey, contracts.AttemptInProgress).
		Limit(1).Find(&rows).Error
	if err != nil {
		return domain.Attempt{}, false, errs.Wrap(errs.Internal, "lock open attempt", err)
	}
	if len(rows) == 0 {
		return domain.Attempt{}, false, nil
	}
	return rows[0].toDomain(), true, nil
}

func (r *Postgres) LatestAttempt(ctx context.Context, tx *gorm.DB, tenantID, missionID, playerID, periodKey string) (domain.Attempt, bool, error) {
	var rows []missionAttempt
	err := tx.WithContext(ctx).
		Where("tenant_id = ? AND mission_id = ? AND player_id = ? AND period_key = ?",
			tenantID, missionID, playerID, periodKey).
		Order("created_at DESC, id DESC").Limit(1).Find(&rows).Error
	if err != nil {
		return domain.Attempt{}, false, errs.Wrap(errs.Internal, "load latest attempt", err)
	}
	if len(rows) == 0 {
		return domain.Attempt{}, false, nil
	}
	return rows[0].toDomain(), true, nil
}

func (r *Postgres) AttemptCounts(ctx context.Context, tx *gorm.DB, tenantID, missionID, playerID, periodKey string) (int, bool, error) {
	var got struct {
		Total    int64
		InPeriod int64
	}
	err := tx.WithContext(ctx).Model(&missionAttempt{}).
		Select("COUNT(*) AS total, COUNT(*) FILTER (WHERE period_key = ?) AS in_period", periodKey).
		Where("tenant_id = ? AND mission_id = ? AND player_id = ? AND status = ?",
			tenantID, missionID, playerID, contracts.AttemptCompleted).
		Scan(&got).Error
	if err != nil {
		return 0, false, errs.Wrap(errs.Internal, "count attempts", err)
	}
	return int(got.Total), got.InPeriod > 0, nil
}

func (r *Postgres) InsertAttempt(ctx context.Context, tx *gorm.DB, a domain.Attempt) (bool, error) {
	row := attemptFromDomain(a)
	res := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert attempt", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) SaveAttempt(ctx context.Context, tx *gorm.DB, a domain.Attempt) error {
	res := tx.WithContext(ctx).Model(&missionAttempt{}).
		Where("tenant_id = ? AND id = ? AND version = ?", a.TenantID, a.ID, a.Version).
		Updates(map[string]any{
			"status":       a.Status,
			"progress":     a.Progress,
			"completed_at": a.CompletedAt,
			"updated_at":   a.UpdatedAt,
			"version":      gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "save attempt", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) AttemptByID(ctx context.Context, tenantID, id string) (domain.Attempt, error) {
	if !validUUID(id) {
		return domain.Attempt{}, domain.ErrAttemptNotFound
	}
	var rows []missionAttempt
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).Limit(1).Find(&rows).Error
	if err != nil {
		return domain.Attempt{}, errs.Wrap(errs.Internal, "load attempt", err)
	}
	if len(rows) == 0 {
		return domain.Attempt{}, domain.ErrAttemptNotFound
	}
	return rows[0].toDomain(), nil
}

func (r *Postgres) ListAttempts(ctx context.Context, tenantID string, f app.AttemptFilter, after app.Cursor, limit int) ([]domain.Attempt, error) {
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if f.MissionID != "" {
		if !validUUID(f.MissionID) {
			return nil, nil
		}
		q = q.Where("mission_id = ?", f.MissionID)
	}
	if f.PlayerID != "" {
		if !validUUID(f.PlayerID) {
			return nil, nil
		}
		q = q.Where("player_id = ?", f.PlayerID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	q = keyset(q, after)
	var rows []missionAttempt
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list attempts", err)
	}
	out := make([]domain.Attempt, len(rows))
	for i, row := range rows {
		out[i] = row.toDomain()
	}
	return out, nil
}

func (r *Postgres) CompletedCounts(ctx context.Context, tenantID string, playerIDs []string) (map[string]int, error) {
	playerIDs = onlyUUIDs(playerIDs)
	out := make(map[string]int, len(playerIDs))
	if len(playerIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		PlayerID string
		N        int64
	}
	err := r.db.WithContext(ctx).Model(&missionAttempt{}).
		Select("player_id, COUNT(*) AS n").
		Where("tenant_id = ? AND status = ? AND player_id IN ?", tenantID, contracts.AttemptCompleted, playerIDs).
		Group("player_id").Scan(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "count completions", err)
	}
	for _, row := range rows {
		out[row.PlayerID] = int(row.N)
	}
	return out, nil
}

// AttemptStats is one grouped scan over ix_mission_attempts_stats.
func (r *Postgres) AttemptStats(ctx context.Context, tenantID string, missionIDs []string) (map[string]app.AttemptStats, error) {
	missionIDs = onlyUUIDs(missionIDs)
	out := make(map[string]app.AttemptStats, len(missionIDs))
	if len(missionIDs) == 0 || !validUUID(tenantID) {
		return out, nil
	}
	var rows []struct {
		MissionID  string
		Started    int64
		InProgress int64
		Completed  int64
		AvgHours   *float64
	}
	err := r.db.WithContext(ctx).Model(&missionAttempt{}).
		Select(`mission_id,
			COUNT(*) AS started,
			COUNT(*) FILTER (WHERE status = ?) AS in_progress,
			COUNT(*) FILTER (WHERE status = ?) AS completed,
			AVG(EXTRACT(EPOCH FROM (completed_at - started_at)) / 3600.0)
				FILTER (WHERE status = ?) AS avg_hours`,
			contracts.AttemptInProgress, contracts.AttemptCompleted, contracts.AttemptCompleted).
		Where("tenant_id = ? AND mission_id IN ?", tenantID, missionIDs).
		Group("mission_id").Scan(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "aggregate mission attempts", err)
	}
	for _, row := range rows {
		out[row.MissionID] = app.AttemptStats{
			Started:            row.Started,
			InProgress:         row.InProgress,
			Completed:          row.Completed,
			AvgHoursToComplete: row.AvgHours,
		}
	}
	return out, nil
}

// ---- progress events ----

func (r *Postgres) InsertProgressEvent(ctx context.Context, tx *gorm.DB, e domain.ProgressEvent) (bool, error) {
	row := progressFromDomain(e)
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).
		Create(&row)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert progress event", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) FinishProgressEvent(ctx context.Context, tx *gorm.DB, e domain.ProgressEvent) error {
	row := progressFromDomain(e)
	err := tx.WithContext(ctx).Model(&progressEvent{}).
		Where("tenant_id = ? AND id = ?", row.TenantID, row.ID).
		Updates(map[string]any{
			"attempt_id": row.AttemptID,
			"status":     row.Status,
			"reason":     row.Reason,
		}).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "finish progress event", err)
	}
	return nil
}

func (r *Postgres) ProgressEventByKey(ctx context.Context, tenantID, key string) (domain.ProgressEvent, bool, error) {
	var rows []progressEvent
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND idempotency_key = ?", tenantID, key).Limit(1).Find(&rows).Error
	if err != nil {
		return domain.ProgressEvent{}, false, errs.Wrap(errs.Internal, "load progress event", err)
	}
	if len(rows) == 0 {
		return domain.ProgressEvent{}, false, nil
	}
	return rows[0].toDomain(), true, nil
}

// ---- sweep ----

func (r *Postgres) DueMissions(ctx context.Context, tx *gorm.DB, now time.Time, limit int) ([]domain.Mission, error) {
	var rows []mission
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("deleted_at IS NULL AND status IN ? AND ends_at IS NOT NULL AND ends_at < ?",
			[]string{contracts.MissionActive, contracts.MissionPaused}, now).
		Order("ends_at, id").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "select due missions", err)
	}
	out := make([]domain.Mission, 0, len(rows))
	for _, row := range rows {
		m, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *Postgres) DueAttempts(ctx context.Context, tx *gorm.DB, now time.Time, limit int) ([]domain.Attempt, error) {
	over := r.db.Model(&mission{}).Select("id").
		Where("status IN ? OR deleted_at IS NOT NULL", []string{contracts.MissionExpired, contracts.MissionArchived})
	var rows []missionAttempt
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("status = ?", contracts.AttemptInProgress).
		Where("(period_ends_at IS NOT NULL AND period_ends_at <= ?) OR mission_id IN (?)", now, over).
		Order("created_at, id").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "select due attempts", err)
	}
	out := make([]domain.Attempt, len(rows))
	for i, row := range rows {
		out[i] = row.toDomain()
	}
	return out, nil
}

func (r *Postgres) LastRun(ctx context.Context, job string) (time.Time, error) {
	var rows []reconcileMarker
	if err := r.db.WithContext(ctx).Where("job = ?", job).Limit(1).Find(&rows).Error; err != nil {
		return time.Time{}, errs.Wrap(errs.Internal, "load reconcile marker", err)
	}
	if len(rows) == 0 {
		return time.Time{}, nil // never ran: sweep everything
	}
	return rows[0].LastRun, nil
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

// ---- subscriptions ----

func (r *Postgres) AbandonOpenAttempts(ctx context.Context, tx *gorm.DB, tenantID, playerID string, now time.Time) (int64, error) {
	if !validUUID(playerID) || !validUUID(tenantID) {
		return 0, nil
	}
	res := tx.WithContext(ctx).Model(&missionAttempt{}).
		Where("tenant_id = ? AND player_id = ? AND status = ?", tenantID, playerID, contracts.AttemptInProgress).
		Updates(map[string]any{
			"status":     contracts.AttemptAbandoned,
			"updated_at": now,
			"version":    gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return 0, errs.Wrap(errs.Internal, "abandon attempts", res.Error)
	}
	return res.RowsAffected, nil
}

func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	if !validUUID(tenantID) {
		return nil
	}
	db := tx.WithContext(ctx)
	for _, model := range []any{&progressEvent{}, &missionAttempt{}, &mission{}} {
		if err := db.Where("tenant_id = ?", tenantID).Delete(model).Error; err != nil {
			return errs.Wrap(errs.Internal, "purge tenant", err)
		}
	}
	return nil
}

// ---- mapping ----

func missionFromDomain(m domain.Mission) (mission, error) {
	criteria := m.Criteria
	if criteria == nil {
		criteria = map[string]any{}
	}
	raw, err := json.Marshal(criteria)
	if err != nil {
		return mission{}, errs.Wrap(errs.Invalid, "criteria is not JSON-encodable", err)
	}
	var badge *string
	if m.BadgeRewardID != "" {
		b := m.BadgeRewardID
		badge = &b
	}
	return mission{
		ID:                      m.ID,
		TenantID:                m.TenantID,
		Slug:                    m.Slug,
		Name:                    m.Name,
		Description:             m.Description,
		Type:                    m.Type,
		Status:                  m.Status,
		Target:                  m.Target,
		Criteria:                string(raw),
		PointsReward:            m.PointsReward,
		XPReward:                m.XPReward,
		BadgeRewardID:           badge,
		MaxCompletionsPerPlayer: m.MaxCompletionsPerPlayer,
		StartsAt:                m.StartsAt,
		EndsAt:                  m.EndsAt,
		Version:                 m.Version,
		CreatedAt:               m.CreatedAt,
		UpdatedAt:               m.UpdatedAt,
		DeletedAt:               m.DeletedAt,
	}, nil
}

func (row mission) toDomain() (domain.Mission, error) {
	criteria := map[string]any{}
	if row.Criteria != "" {
		if err := json.Unmarshal([]byte(row.Criteria), &criteria); err != nil {
			return domain.Mission{}, errs.Wrap(errs.Internal, "decode mission criteria", err)
		}
	}
	m := domain.Mission{
		ID:                      row.ID,
		TenantID:                row.TenantID,
		Slug:                    row.Slug,
		Name:                    row.Name,
		Description:             row.Description,
		Type:                    row.Type,
		Status:                  row.Status,
		Target:                  row.Target,
		Criteria:                criteria,
		PointsReward:            row.PointsReward,
		XPReward:                row.XPReward,
		MaxCompletionsPerPlayer: row.MaxCompletionsPerPlayer,
		StartsAt:                utc(row.StartsAt),
		EndsAt:                  utc(row.EndsAt),
		Version:                 row.Version,
		CreatedAt:               row.CreatedAt.UTC(),
		UpdatedAt:               row.UpdatedAt.UTC(),
		DeletedAt:               utc(row.DeletedAt),
	}
	if row.BadgeRewardID != nil {
		m.BadgeRewardID = *row.BadgeRewardID
	}
	return m, nil
}

func attemptFromDomain(a domain.Attempt) missionAttempt {
	return missionAttempt{
		ID:           a.ID,
		TenantID:     a.TenantID,
		MissionID:    a.MissionID,
		PlayerID:     a.PlayerID,
		Status:       a.Status,
		Progress:     a.Progress,
		Target:       a.Target,
		PeriodKey:    a.PeriodKey,
		PeriodEndsAt: a.PeriodEndsAt,
		StartedAt:    a.StartedAt,
		CompletedAt:  a.CompletedAt,
		Version:      a.Version,
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
	}
}

func (row missionAttempt) toDomain() domain.Attempt {
	return domain.Attempt{
		ID:           row.ID,
		TenantID:     row.TenantID,
		MissionID:    row.MissionID,
		PlayerID:     row.PlayerID,
		Status:       row.Status,
		Progress:     row.Progress,
		Target:       row.Target,
		PeriodKey:    row.PeriodKey,
		PeriodEndsAt: utc(row.PeriodEndsAt),
		StartedAt:    row.StartedAt.UTC(),
		CompletedAt:  utc(row.CompletedAt),
		Version:      row.Version,
		CreatedAt:    row.CreatedAt.UTC(),
		UpdatedAt:    row.UpdatedAt.UTC(),
	}
}

func progressFromDomain(e domain.ProgressEvent) progressEvent {
	var attempt *string
	if e.AttemptID != "" {
		a := e.AttemptID
		attempt = &a
	}
	return progressEvent{
		ID:             e.ID,
		TenantID:       e.TenantID,
		IdempotencyKey: e.IdempotencyKey,
		MissionID:      e.MissionID,
		PlayerID:       e.PlayerID,
		AttemptID:      attempt,
		Increment:      e.Increment,
		Status:         e.Status,
		Reason:         e.Reason,
		SourceKind:     e.SourceKind,
		SourceID:       e.SourceID,
		CreatedAt:      e.CreatedAt,
	}
}

func (row progressEvent) toDomain() domain.ProgressEvent {
	e := domain.ProgressEvent{
		ID:             row.ID,
		TenantID:       row.TenantID,
		IdempotencyKey: row.IdempotencyKey,
		MissionID:      row.MissionID,
		PlayerID:       row.PlayerID,
		Increment:      row.Increment,
		Status:         row.Status,
		Reason:         row.Reason,
		SourceKind:     row.SourceKind,
		SourceID:       row.SourceID,
		CreatedAt:      row.CreatedAt.UTC(),
	}
	if row.AttemptID != nil {
		e.AttemptID = *row.AttemptID
	}
	return e
}

// ---- helpers ----

func keyset(q *gorm.DB, after app.Cursor) *gorm.DB {
	if after.ID == "" {
		return q
	}
	return q.Where("(created_at, id) < (?, ?)", after.CreatedAt, after.ID)
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func validUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func onlyUUIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if validUUID(id) {
			out = append(out, id)
		}
	}
	return out
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
