package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/webhooks/internal/app"
	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/shared/errs"
)

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

// ---- endpoints ----

func (r *Postgres) CreateEndpoint(ctx context.Context, tx *gorm.DB, e domain.Endpoint) error {
	m := endpointFromDomain(e)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		return errs.Wrap(errs.Internal, "insert webhook endpoint", err)
	}
	return nil
}

func (r *Postgres) SaveEndpoint(ctx context.Context, tx *gorm.DB, e domain.Endpoint, resetFailures bool) error {
	m := endpointFromDomain(e)
	cols := map[string]any{
		"url":             m.URL,
		"description":     m.Description,
		"event_types":     m.EventTypes,
		"secret":          m.Secret,
		"is_active":       m.IsActive,
		"disabled_reason": m.DisabledReason,
		"disabled_at":     m.DisabledAt,
		"updated_at":      m.UpdatedAt,
		"version":         gorm.Expr("version + 1"),
	}
	if resetFailures {
		cols["consecutive_failures"] = 0
	}
	res := tx.WithContext(ctx).Model(&endpoint{}).
		Where("id = ? AND tenant_id = ? AND version = ? AND deleted_at IS NULL", m.ID, m.TenantID, m.Version).
		Updates(cols)
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "save webhook endpoint", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) SoftDeleteEndpoint(ctx context.Context, tx *gorm.DB, tenantID, id string, at time.Time) error {
	if !isUUID(tenantID) || !isUUID(id) {
		return domain.ErrEndpointNotFound
	}
	res := tx.WithContext(ctx).Model(&endpoint{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Updates(map[string]any{"deleted_at": at, "updated_at": at, "is_active": false, "version": gorm.Expr("version + 1")})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "delete webhook endpoint", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrEndpointNotFound
	}
	return nil
}

func (r *Postgres) EndpointByID(ctx context.Context, tenantID, id string) (domain.Endpoint, error) {
	if !isUUID(tenantID) || !isUUID(id) {
		return domain.Endpoint{}, domain.ErrEndpointNotFound
	}
	var m endpoint
	err := r.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Endpoint{}, domain.ErrEndpointNotFound
	case err != nil:
		return domain.Endpoint{}, errs.Wrap(errs.Internal, "load webhook endpoint", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) CountEndpoints(ctx context.Context, tenantID string) (int64, error) {
	if !isUUID(tenantID) {
		return 0, nil
	}
	var n int64
	if err := r.db.WithContext(ctx).Model(&endpoint{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Count(&n).Error; err != nil {
		return 0, errs.Wrap(errs.Internal, "count webhook endpoints", err)
	}
	return n, nil
}

func (r *Postgres) ListEndpoints(ctx context.Context, tenantID string, p app.Page) ([]domain.Endpoint, error) {
	if !isUUID(tenantID) {
		return nil, nil
	}
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	q = keyset(q, p)
	var ms []endpoint
	if err := q.Order("created_at DESC, id DESC").Limit(p.Limit).Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list webhook endpoints", err)
	}
	out := make([]domain.Endpoint, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) ActiveEndpointsFor(ctx context.Context, tenantID, event string) ([]domain.Endpoint, error) {
	if !isUUID(tenantID) {
		return nil, nil
	}
	var ms []endpoint
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL AND is_active", tenantID).
		Where("event_types @> ARRAY[?]::text[] OR event_types @> ARRAY['*']::text[]", event).
		Order("created_at, id").
		Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load subscribed webhook endpoints", err)
	}
	out := make([]domain.Endpoint, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) IncrementFailures(ctx context.Context, tx *gorm.DB, tenantID, id string) (int, error) {
	var rows []endpoint
	err := tx.WithContext(ctx).Model(&rows).
		Clauses(clause.Returning{Columns: []clause.Column{{Name: "consecutive_failures"}}}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		UpdateColumn("consecutive_failures", gorm.Expr("consecutive_failures + 1")).Error
	if err != nil {
		return 0, errs.Wrap(errs.Internal, "count webhook endpoint failure", err)
	}
	if len(rows) == 0 {
		return 0, nil // purged
	}
	return rows[0].ConsecutiveFailures, nil
}

func (r *Postgres) ResetFailures(ctx context.Context, tx *gorm.DB, tenantID, id string) error {
	err := tx.WithContext(ctx).Model(&endpoint{}).
		Where("id = ? AND tenant_id = ? AND consecutive_failures <> 0", id, tenantID).
		Update("consecutive_failures", 0).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "reset webhook endpoint failures", err)
	}
	return nil
}

func (r *Postgres) DisableEndpoint(ctx context.Context, tx *gorm.DB, tenantID, id, reason string, at time.Time) (bool, error) {
	res := tx.WithContext(ctx).Model(&endpoint{}).
		Where("id = ? AND tenant_id = ? AND is_active AND deleted_at IS NULL", id, tenantID).
		Updates(map[string]any{
			"is_active":       false,
			"disabled_reason": reason,
			"disabled_at":     at,
			"updated_at":      at,
			"version":         gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "disable webhook endpoint", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) ActiveEndpointsOverThreshold(ctx context.Context, threshold, limit int) ([]domain.Endpoint, error) {
	var ms []endpoint
	if err := r.db.WithContext(ctx).
		Where("is_active AND deleted_at IS NULL AND consecutive_failures >= ?", threshold).
		Order("id").Limit(limit).
		Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load failing webhook endpoints", err)
	}
	out := make([]domain.Endpoint, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// ---- deliveries ----

func (r *Postgres) InsertDelivery(ctx context.Context, tx *gorm.DB, d domain.Delivery) (bool, error) {
	m := deliveryFromDomain(d)
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "endpoint_id"}, {Name: "event_id"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert webhook delivery", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) SaveDelivery(ctx context.Context, tx *gorm.DB, d domain.Delivery) error {
	m := deliveryFromDomain(d)
	res := tx.WithContext(ctx).Model(&delivery{}).
		Where("id = ? AND tenant_id = ?", m.ID, m.TenantID).
		Updates(map[string]any{
			"status":          m.Status,
			"attempts":        m.Attempts,
			"cycle_attempts":  m.CycleAttempts,
			"response_status": m.ResponseStatus,
			"response_body":   m.ResponseBody,
			"latency_ms":      m.LatencyMs,
			"last_error":      m.LastError,
			"lease_until":     m.LeaseUntil,
			"enqueued_at":     m.EnqueuedAt,
			"last_attempt_at": m.LastAttemptAt,
			"delivered_at":    m.DeliveredAt,
			"updated_at":      m.UpdatedAt,
		})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "save webhook delivery", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrDeliveryNotFound
	}
	return nil
}

func (r *Postgres) DeliveryByID(ctx context.Context, tenantID, id string) (domain.Delivery, error) {
	return r.oneDelivery(r.db.WithContext(ctx), tenantID, id)
}

func (r *Postgres) DeliveryForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Delivery, error) {
	return r.oneDelivery(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}), tenantID, id)
}

func (r *Postgres) oneDelivery(q *gorm.DB, tenantID, id string) (domain.Delivery, error) {
	if !isUUID(tenantID) || !isUUID(id) {
		return domain.Delivery{}, domain.ErrDeliveryNotFound
	}
	var m delivery
	err := q.Where("id = ? AND tenant_id = ?", id, tenantID).First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Delivery{}, domain.ErrDeliveryNotFound
	case err != nil:
		return domain.Delivery{}, errs.Wrap(errs.Internal, "load webhook delivery", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) ListDeliveries(ctx context.Context, tenantID string, f app.DeliveryFilter, p app.Page) ([]domain.Delivery, error) {
	if !isUUID(tenantID) {
		return nil, nil
	}
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if f.EndpointID != "" {
		if !isUUID(f.EndpointID) {
			return nil, nil
		}
		q = q.Where("endpoint_id = ?", f.EndpointID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Event != "" {
		q = q.Where("event = ?", f.Event)
	}
	q = keyset(q, p)
	var ms []delivery
	if err := q.Order("created_at DESC, id DESC").Limit(p.Limit).Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list webhook deliveries", err)
	}
	out := make([]domain.Delivery, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) StalePending(ctx context.Context, before, now time.Time, limit int) ([]domain.Delivery, error) {
	var ms []delivery
	if err := r.db.WithContext(ctx).
		Where("status = 'pending' AND enqueued_at < ?", before).
		Where("(last_attempt_at IS NULL OR last_attempt_at < ?)", before).
		Where("(lease_until IS NULL OR lease_until < ?)", now).
		Order("enqueued_at, id").Limit(limit).
		Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load stale webhook deliveries", err)
	}
	out := make([]domain.Delivery, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// ---- markers ----

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
	for _, model := range []any{&delivery{}, &endpoint{}} {
		if err := db.Where("tenant_id = ?", tenantID).Delete(model).Error; err != nil {
			return errs.Wrap(errs.Internal, "purge tenant", err)
		}
	}
	return nil
}

// ---- helpers ----

func keyset(q *gorm.DB, p app.Page) *gorm.DB {
	if p.BeforeID == "" {
		return q
	}
	return q.Where("(created_at, id) < (?, ?)", p.Before, p.BeforeID)
}

// isUUID guards uuid columns: a malformed id is "not found", never a 500
// from Postgres rejecting the cast.
func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
