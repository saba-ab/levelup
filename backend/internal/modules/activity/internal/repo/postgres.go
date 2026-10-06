// Package repo implements activity's persistence. Models stay unexported
// and map to domain types; tables derive from struct names (no
// TableName(), the module session prefixes activity_svc.).
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/activity/internal/app"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/shared/errs"
)

// activity → activity_svc.activities.
type activity struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	TenantID          string `gorm:"type:uuid"`
	EventID           string
	EventType         string
	PlayerExternalID  *string
	PlayerID          *string `gorm:"type:uuid"`
	Properties        string  `gorm:"type:jsonb"`
	Context           string  `gorm:"type:jsonb"`
	OccurredAt        time.Time
	ReceivedAt        time.Time
	Status            string
	DecisionID        *string `gorm:"type:uuid"`
	Outcome           *string
	Reason            *string
	DecidedAt         *time.Time
	CausationDepth    int
	SourceEventID     *string
	RepublishCount    int
	LastRepublishedAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// reconcileMarker → activity_svc.reconcile_markers.
type reconcileMarker struct {
	Name    string `gorm:"primaryKey"`
	LastRun time.Time
}

func fromDomain(a domain.Activity) (activity, error) {
	props, err := marshalObject(a.Properties)
	if err != nil {
		return activity{}, err
	}
	ctxData, err := marshalObject(a.Context)
	if err != nil {
		return activity{}, err
	}
	m := activity{
		ID:               a.ID,
		TenantID:         a.TenantID,
		EventID:          a.EventID,
		EventType:        a.EventType,
		PlayerExternalID: nullable(a.PlayerExternalID),
		PlayerID:         nullable(a.PlayerID),
		Properties:       props,
		Context:          ctxData,
		OccurredAt:       a.OccurredAt,
		ReceivedAt:       a.ReceivedAt,
		Status:           a.Status,
		DecisionID:       nullable(a.DecisionID),
		Outcome:          nullable(a.Outcome),
		Reason:           nullable(a.Reason),
		CausationDepth:   a.CausationDepth,
		SourceEventID:    nullable(a.SourceEventID),
		RepublishCount:   a.RepublishCount,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
	}
	if !a.DecidedAt.IsZero() {
		at := a.DecidedAt
		m.DecidedAt = &at
	}
	return m, nil
}

func (m activity) toDomain() (domain.Activity, error) {
	a := domain.Activity{
		ID:               m.ID,
		TenantID:         m.TenantID,
		EventID:          m.EventID,
		EventType:        m.EventType,
		PlayerExternalID: deref(m.PlayerExternalID),
		PlayerID:         deref(m.PlayerID),
		OccurredAt:       m.OccurredAt.UTC(),
		ReceivedAt:       m.ReceivedAt.UTC(),
		Status:           m.Status,
		DecisionID:       deref(m.DecisionID),
		Outcome:          deref(m.Outcome),
		Reason:           deref(m.Reason),
		CausationDepth:   m.CausationDepth,
		SourceEventID:    deref(m.SourceEventID),
		RepublishCount:   m.RepublishCount,
		CreatedAt:        m.CreatedAt.UTC(),
		UpdatedAt:        m.UpdatedAt.UTC(),
	}
	if m.DecidedAt != nil {
		a.DecidedAt = m.DecidedAt.UTC()
	}
	if err := unmarshalObject(m.Properties, &a.Properties); err != nil {
		return domain.Activity{}, err
	}
	if err := unmarshalObject(m.Context, &a.Context); err != nil {
		return domain.Activity{}, err
	}
	return a, nil
}

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

// Insert is the idempotent hot-path write: a second report of the same
// (tenant_id, event_id) writes nothing.
func (r *Postgres) Insert(ctx context.Context, tx *gorm.DB, a domain.Activity) (bool, error) {
	m, err := fromDomain(a)
	if err != nil {
		return false, err
	}
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "event_id"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert activity", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) ByEventIDs(ctx context.Context, tx *gorm.DB, tenantID string, eventIDs []string) (map[string]domain.Activity, error) {
	out := map[string]domain.Activity{}
	if len(eventIDs) == 0 {
		return out, nil
	}
	var ms []activity
	err := tx.WithContext(ctx).
		Where("tenant_id = ? AND event_id IN ?", tenantID, eventIDs).
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load activities by event id", err)
	}
	for _, m := range ms {
		a, err := m.toDomain()
		if err != nil {
			return nil, err
		}
		out[a.EventID] = a
	}
	return out, nil
}

func (r *Postgres) ByID(ctx context.Context, tenantID, id string) (domain.Activity, error) {
	var m activity
	err := r.db.WithContext(ctx).First(&m, "tenant_id = ? AND id = ?", tenantID, id).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Activity{}, domain.ErrNotFound
	case err != nil:
		return domain.Activity{}, errs.Wrap(errs.Internal, "load activity", err)
	}
	return m.toDomain()
}

func (r *Postgres) List(ctx context.Context, tenantID string, f app.ListFilter, before time.Time, beforeID string, limit int) ([]domain.Activity, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ?", tenantID).
		Order("created_at DESC, id DESC").
		Limit(limit)
	if f.EventType != "" {
		q = q.Where("event_type = ?", f.EventType)
	}
	if f.PlayerExternalID != "" {
		q = q.Where("player_external_id = ?", f.PlayerExternalID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if !before.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", before, beforeID)
	}
	var ms []activity
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list activities", err)
	}
	return toDomains(ms)
}

// ApplyDecision only touches a pending row: the WHERE clause is the
// projection's idempotency guard.
func (r *Postgres) ApplyDecision(ctx context.Context, tx *gorm.DB, a domain.Activity) (bool, error) {
	updates := map[string]any{
		"status":      a.Status,
		"decision_id": a.DecisionID,
		"outcome":     nullable(a.Outcome),
		"reason":      nullable(a.Reason),
		"decided_at":  a.DecidedAt,
		"updated_at":  a.UpdatedAt,
	}
	if a.PlayerID != "" {
		updates["player_id"] = gorm.Expr("COALESCE(player_id, ?::uuid)", a.PlayerID)
	}
	res := tx.WithContext(ctx).Model(&activity{}).
		Where("tenant_id = ? AND id = ? AND status = ?", a.TenantID, a.ID, domain.StatusPending).
		Updates(updates)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "apply decision", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// LockStuck sweeps across tenants (a system job; each row carries its
// tenant). SKIP LOCKED lets a concurrent run or an in-flight decision pass.
func (r *Postgres) LockStuck(ctx context.Context, tx *gorm.DB, q app.StuckQuery) ([]domain.Activity, error) {
	var ms []activity
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("status = ?", domain.StatusPending).
		Where("received_at < ?", q.ReceivedBefore).
		Where("republish_count < ?", q.MaxRepublishes).
		Where("(last_republished_at IS NULL OR last_republished_at < ?)", q.ReceivedBefore).
		Order("received_at ASC").
		Limit(q.Limit).
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "lock stuck activities", err)
	}
	return toDomains(ms)
}

func (r *Postgres) MarkRepublished(ctx context.Context, tx *gorm.DB, ids []string, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	err := tx.WithContext(ctx).Model(&activity{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"republish_count":     gorm.Expr("republish_count + 1"),
			"last_republished_at": at,
			"updated_at":          at,
		}).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "mark activities re-published", err)
	}
	return nil
}

func (r *Postgres) CountExhausted(ctx context.Context, receivedBefore time.Time, maxRepublishes int) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&activity{}).
		Where("status = ? AND received_at < ? AND republish_count >= ?", domain.StatusPending, receivedBefore, maxRepublishes).
		Count(&n).Error
	if err != nil {
		return 0, errs.Wrap(errs.Internal, "count exhausted activities", err)
	}
	return n, nil
}

func (r *Postgres) DeleteTenant(ctx context.Context, tx *gorm.DB, tenantID string) (int64, error) {
	res := tx.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(&activity{})
	if res.Error != nil {
		return 0, errs.Wrap(errs.Internal, "purge tenant activities", res.Error)
	}
	return res.RowsAffected, nil
}

func (r *Postgres) LastRun(ctx context.Context, name string) (time.Time, error) {
	var m reconcileMarker
	err := r.db.WithContext(ctx).First(&m, "name = ?", name).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return time.Time{}, nil
	case err != nil:
		return time.Time{}, errs.Wrap(errs.Internal, "load reconcile marker", err)
	}
	return m.LastRun.UTC(), nil
}

func (r *Postgres) MarkRun(ctx context.Context, name string, at time.Time) error {
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"last_run"}),
		}).
		Create(&reconcileMarker{Name: name, LastRun: at}).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "mark reconcile run", err)
	}
	return nil
}

func toDomains(ms []activity) ([]domain.Activity, error) {
	out := make([]domain.Activity, len(ms))
	for i, m := range ms {
		a, err := m.toDomain()
		if err != nil {
			return nil, err
		}
		out[i] = a
	}
	return out, nil
}

// JSONB travels as string: with pgx's exec mode (PgBouncer-safe), []byte
// would be sent as bytea.
func marshalObject(m map[string]any) (string, error) {
	if m == nil {
		return "{}", nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return "", errs.Wrap(errs.Invalid, "encode activity payload", err)
	}
	return string(raw), nil
}

func unmarshalObject(raw string, dst *map[string]any) error {
	if raw == "" {
		*dst = map[string]any{}
		return nil
	}
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		return errs.Wrap(errs.Internal, "decode activity payload", err)
	}
	if *dst == nil {
		*dst = map[string]any{}
	}
	return nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// lastSeenRow is a scan target, not a table model.
type lastSeenRow struct {
	PlayerID   string
	OccurredAt time.Time
	EventType  string
}

// LastSeen is one DISTINCT ON query served by
// ix_activities_tenant_player_occurred: per player the newest row by
// (occurred_at, id). Rows not yet resolved to a player id do not count.
func (r *Postgres) LastSeen(ctx context.Context, tenantID string, playerIDs []string) ([]domain.LastSeen, error) {
	if len(playerIDs) == 0 {
		return []domain.LastSeen{}, nil
	}
	var rows []lastSeenRow
	err := r.db.WithContext(ctx).Model(&activity{}).
		Select("DISTINCT ON (player_id) player_id, occurred_at, event_type").
		Where("tenant_id = ? AND player_id IN ?", tenantID, playerIDs).
		Order("player_id, occurred_at DESC, id DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load last seen", err)
	}
	out := make([]domain.LastSeen, len(rows))
	for i, row := range rows {
		out[i] = domain.LastSeen{PlayerID: row.PlayerID, At: row.OccurredAt.UTC(), EventType: row.EventType}
	}
	return out, nil
}
