package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/app"
	"levelup/internal/modules/notifications/internal/domain"
	"levelup/internal/shared/errs"
)

// schema qualifies raw SQL: Deps.DB pins the prefix only for model-derived
// table names, and search_path is not usable behind PgBouncer.
const schema = "notifications_svc."

// Postgres implements app.Repository.
type Postgres struct{ db *gorm.DB }

var _ app.Repository = (*Postgres)(nil)

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ---- templates ----

func (r *Postgres) CreateTemplate(ctx context.Context, tx *gorm.DB, t domain.Template) error {
	m := templateFromDomain(t)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrTemplateNameTaken
		}
		return errs.Wrap(errs.Internal, "insert notification template", err)
	}
	return nil
}

func (r *Postgres) loadTemplate(ctx context.Context, q *gorm.DB, tenantID, id string) (domain.Template, error) {
	var m notificationTemplate
	err := q.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Template{}, domain.ErrTemplateNotFound
	case err != nil:
		return domain.Template{}, errs.Wrap(errs.Internal, "load notification template", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) TemplateByID(ctx context.Context, tenantID, id string) (domain.Template, error) {
	return r.loadTemplate(ctx, r.db, tenantID, id)
}

func (r *Postgres) TemplateByIDForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Template, error) {
	return r.loadTemplate(ctx, tx.Clauses(clause.Locking{Strength: "UPDATE"}), tenantID, id)
}

func (r *Postgres) SaveTemplate(ctx context.Context, tx *gorm.DB, t domain.Template) error {
	m := templateFromDomain(t)
	res := tx.WithContext(ctx).Model(&notificationTemplate{}).
		Where("id = ? AND tenant_id = ? AND version = ?", m.ID, m.TenantID, m.Version).
		Updates(map[string]any{
			"name":           m.Name,
			"trigger":        m.Trigger,
			"channels":       m.Channels,
			"title_template": m.TitleTemplate,
			"body_template":  m.BodyTemplate,
			"is_active":      m.IsActive,
			"updated_at":     m.UpdatedAt,
			"deleted_at":     m.DeletedAt,
			"version":        gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return domain.ErrTemplateNameTaken
		}
		return errs.Wrap(errs.Internal, "save notification template", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) ListTemplates(ctx context.Context, tenantID string, f app.TemplateFilter) ([]domain.Template, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("created_at DESC, id DESC").
		Limit(f.Limit)
	if f.Trigger != "" {
		q = q.Where("trigger = ?", f.Trigger)
	}
	if f.Active != nil {
		q = q.Where("is_active = ?", *f.Active)
	}
	if !f.Cursor.Before.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", f.Cursor.Before, f.Cursor.BeforeID)
	}
	var ms []notificationTemplate
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list notification templates", err)
	}
	return templatesToDomain(ms), nil
}

func (r *Postgres) ActiveTemplatesByTrigger(ctx context.Context, tenantID, trigger string) ([]domain.Template, error) {
	var ms []notificationTemplate
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND trigger = ? AND is_active AND deleted_at IS NULL", tenantID, trigger).
		Order("created_at, id").
		Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load templates by trigger", err)
	}
	return templatesToDomain(ms), nil
}

func (r *Postgres) TemplatesByIDs(ctx context.Context, tenantID string, ids []string) ([]domain.Template, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var ms []notificationTemplate
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND id IN ?", tenantID, ids).
		Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load notification templates", err)
	}
	return templatesToDomain(ms), nil
}

func templatesToDomain(ms []notificationTemplate) []domain.Template {
	out := make([]domain.Template, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out
}

// ---- channel settings ----

func (r *Postgres) ChannelSettings(ctx context.Context, tenantID string) (domain.ChannelSettings, bool, error) {
	var m channelSetting
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.ChannelSettings{}, false, nil
	case err != nil:
		return domain.ChannelSettings{}, false, errs.Wrap(errs.Internal, "load channel settings", err)
	}
	return domain.ChannelSettings{TenantID: m.TenantID, EmailEnabled: m.EmailEnabled,
		EmailFromName: m.EmailFromName, UpdatedAt: m.UpdatedAt.UTC()}, true, nil
}

func (r *Postgres) UpsertChannelSettings(ctx context.Context, tx *gorm.DB, s domain.ChannelSettings) error {
	m := channelSetting{TenantID: s.TenantID, EmailEnabled: s.EmailEnabled, EmailFromName: s.EmailFromName, UpdatedAt: s.UpdatedAt}
	if err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"email_enabled", "email_from_name", "updated_at"}),
		}).
		Create(&m).Error; err != nil {
		return errs.Wrap(errs.Internal, "upsert channel settings", err)
	}
	return nil
}

// ---- notifications ----

func (r *Postgres) InsertNotification(ctx context.Context, tx *gorm.DB, n domain.Notification) (bool, error) {
	m := notificationFromDomain(n)
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "template_id"}, {Name: "event_id"}, {Name: "channel"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert notification", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) NotificationByID(ctx context.Context, tenantID, id string) (domain.Notification, bool, error) {
	var m notification
	err := r.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Notification{}, false, nil
	case err != nil:
		return domain.Notification{}, false, errs.Wrap(errs.Internal, "load notification", err)
	}
	return m.toDomain(), true, nil
}

const claimSQL = `
UPDATE ` + schema + `notifications
SET attempts = attempts + 1, lease_until = ?, updated_at = ?
WHERE id = ? AND tenant_id = ? AND status = 'pending' AND channel = 'email'
  AND (lease_until IS NULL OR lease_until < ?)
RETURNING *`

func (r *Postgres) ClaimEmail(ctx context.Context, tx *gorm.DB, tenantID, id string, now, leaseUntil time.Time) (domain.Notification, bool, error) {
	var ms []notification
	if err := tx.WithContext(ctx).Raw(claimSQL, leaseUntil, now, id, tenantID, now).Scan(&ms).Error; err != nil {
		return domain.Notification{}, false, errs.Wrap(errs.Internal, "claim email notification", err)
	}
	if len(ms) == 0 {
		return domain.Notification{}, false, nil
	}
	return ms[0].toDomain(), true, nil
}

func (r *Postgres) SettleNotification(ctx context.Context, tx *gorm.DB, n domain.Notification) (bool, error) {
	res := tx.WithContext(ctx).Model(&notification{}).
		Where("id = ? AND tenant_id = ? AND status = ?", n.ID, n.TenantID, contracts.StatusPending).
		Updates(map[string]any{
			"status":       n.Status,
			"reason":       n.Reason,
			"last_error":   n.LastError,
			"lease_until":  n.LeaseUntil,
			"delivered_at": n.DeliveredAt,
			"updated_at":   n.UpdatedAt,
		})
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "settle notification", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) ListHistory(ctx context.Context, tenantID string, f app.HistoryFilter) ([]domain.Notification, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC, id DESC").
		Limit(f.Limit)
	if f.TemplateID != "" {
		q = q.Where("template_id = ?", f.TemplateID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Channel != "" {
		q = q.Where("channel = ?", f.Channel)
	}
	if f.PlayerID != "" {
		q = q.Where("player_id = ?", f.PlayerID)
	}
	if !f.Cursor.Before.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", f.Cursor.Before, f.Cursor.BeforeID)
	}
	return findNotifications(q, "list notification history")
}

func (r *Postgres) ListFeed(ctx context.Context, tenantID, playerID string, f app.FeedFilter) ([]domain.Notification, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND player_id = ? AND channel = ? AND status = ?",
			tenantID, playerID, contracts.ChannelInApp, contracts.StatusDelivered).
		Order("created_at DESC, id DESC").
		Limit(f.Limit)
	if f.UnreadOnly {
		q = q.Where("read_at IS NULL")
	}
	if !f.Cursor.Before.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", f.Cursor.Before, f.Cursor.BeforeID)
	}
	return findNotifications(q, "list notification feed")
}

func findNotifications(q *gorm.DB, what string) ([]domain.Notification, error) {
	var ms []notification
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, what, err)
	}
	out := make([]domain.Notification, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) CountUnread(ctx context.Context, tenantID, playerID string) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&notification{}).
		Where("tenant_id = ? AND player_id = ? AND channel = ? AND status = ? AND read_at IS NULL",
			tenantID, playerID, contracts.ChannelInApp, contracts.StatusDelivered).
		Count(&n).Error; err != nil {
		return 0, errs.Wrap(errs.Internal, "count unread notifications", err)
	}
	return n, nil
}

const markReadSQL = `
UPDATE ` + schema + `notifications
SET read_at = COALESCE(read_at, ?)
WHERE id = ? AND tenant_id = ? AND player_id = ? AND channel = 'in_app' AND status = 'delivered'
RETURNING *`

func (r *Postgres) MarkRead(ctx context.Context, tx *gorm.DB, tenantID, playerID, id string, now time.Time) (domain.Notification, bool, error) {
	var ms []notification
	if err := tx.WithContext(ctx).Raw(markReadSQL, now, id, tenantID, playerID).Scan(&ms).Error; err != nil {
		return domain.Notification{}, false, errs.Wrap(errs.Internal, "mark notification read", err)
	}
	if len(ms) == 0 {
		return domain.Notification{}, false, nil
	}
	return ms[0].toDomain(), true, nil
}

func (r *Postgres) MarkAllRead(ctx context.Context, tx *gorm.DB, tenantID, playerID string, now time.Time) (int64, error) {
	res := tx.WithContext(ctx).Model(&notification{}).
		Where("tenant_id = ? AND player_id = ? AND channel = ? AND status = ? AND read_at IS NULL",
			tenantID, playerID, contracts.ChannelInApp, contracts.StatusDelivered).
		UpdateColumn("read_at", now)
	if res.Error != nil {
		return 0, errs.Wrap(errs.Internal, "mark all notifications read", res.Error)
	}
	return res.RowsAffected, nil
}

type statsRow struct {
	TemplateID string
	Channel    string
	Status     string
	Count      int64
	ReadCount  int64
}

func (r *Postgres) Stats(ctx context.Context, tenantID string, from, to time.Time) ([]domain.StatsRow, error) {
	q := r.db.WithContext(ctx).Model(&notification{}).
		Select("template_id, channel, status, COUNT(*) AS count, COUNT(read_at) AS read_count").
		Where("tenant_id = ?", tenantID).
		Group("template_id, channel, status")
	if !from.IsZero() {
		q = q.Where("created_at >= ?", from)
	}
	if !to.IsZero() {
		q = q.Where("created_at < ?", to)
	}
	var rows []statsRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "notification stats", err)
	}
	out := make([]domain.StatsRow, len(rows))
	for i, s := range rows {
		out[i] = domain.StatsRow{TemplateID: s.TemplateID, Channel: s.Channel, Status: s.Status, Count: s.Count, Read: s.ReadCount}
	}
	return out, nil
}

func (r *Postgres) FailStalePending(ctx context.Context, tx *gorm.DB, cutoff, now time.Time) (int64, error) {
	res := tx.WithContext(ctx).Model(&notification{}).
		Where("status = ? AND created_at < ?", contracts.StatusPending, cutoff).
		Updates(map[string]any{
			"status":      contracts.StatusFailed,
			"reason":      contracts.ReasonStale,
			"lease_until": nil,
			"updated_at":  now,
		})
	if res.Error != nil {
		return 0, errs.Wrap(errs.Internal, "fail stale email notifications", res.Error)
	}
	return res.RowsAffected, nil
}

// ---- purge ----

// PurgeTenant deletes every row of the tenant, children first. Running it
// twice deletes nothing the second time.
func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	for _, model := range []any{&notification{}, &notificationTemplate{}, &channelSetting{}} {
		if err := tx.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(model).Error; err != nil {
			return errs.Wrap(errs.Internal, "purge tenant", err)
		}
	}
	return nil
}

func (r *Postgres) PurgePlayer(ctx context.Context, tx *gorm.DB, tenantID, playerID string) error {
	if err := tx.WithContext(ctx).Where("tenant_id = ? AND player_id = ?", tenantID, playerID).
		Delete(&notification{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge player notifications", err)
	}
	return nil
}
