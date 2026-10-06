package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"levelup/internal/modules/segments/internal/app"
	"levelup/internal/modules/segments/internal/domain"
	"levelup/internal/shared/errs"
)

const (
	tSegments = "segments_svc.segments"
	tMembers  = "segments_svc.segment_members"
	tMarkers  = "segments_svc.reconcile_markers"
)

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func filterUUIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, s := range ids {
		if isUUID(s) {
			out = append(out, s)
		}
	}
	return out
}

func mapWriteErr(err error, op string) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return domain.ErrNameTaken
	}
	return errs.Wrap(errs.Internal, op, err)
}

// ---- definitions ----

func (r *Postgres) CreateSegment(ctx context.Context, tx *gorm.DB, s domain.Segment) error {
	m, err := segmentFromDomain(s)
	if err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		return mapWriteErr(err, "insert segment")
	}
	return nil
}

func (r *Postgres) SaveSegment(ctx context.Context, tx *gorm.DB, s domain.Segment) error {
	m, err := segmentFromDomain(s)
	if err != nil {
		return err
	}
	res := tx.WithContext(ctx).Model(&segment{}).
		Where("id = ? AND tenant_id = ? AND version = ? AND deleted_at IS NULL", m.ID, m.TenantID, m.Version).
		Updates(map[string]any{
			"name":        m.Name,
			"description": m.Description,
			"conditions":  m.Conditions,
			"updated_at":  m.UpdatedAt,
			"version":     gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return mapWriteErr(res.Error, "save segment")
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) SoftDeleteSegment(ctx context.Context, tx *gorm.DB, tenantID, id string, at time.Time) error {
	db := tx.WithContext(ctx)
	res := db.Model(&segment{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Updates(map[string]any{
			"deleted_at": at, "updated_at": at, "member_count": 0,
			"refresh_lease_until": nil, "version": gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "delete segment", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrSegmentNotFound
	}
	if err := db.Exec(`DELETE FROM `+tMembers+` WHERE segment_id = ?`, id).Error; err != nil {
		return errs.Wrap(errs.Internal, "delete segment members", err)
	}
	return nil
}

func (r *Postgres) SegmentByID(ctx context.Context, tenantID, id string) (domain.Segment, error) {
	if !isUUID(tenantID) || !isUUID(id) {
		return domain.Segment{}, domain.ErrSegmentNotFound
	}
	var m segment
	err := r.db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Segment{}, domain.ErrSegmentNotFound
	case err != nil:
		return domain.Segment{}, errs.Wrap(errs.Internal, "load segment", err)
	}
	return m.toDomain()
}

func (r *Postgres) ListSegments(ctx context.Context, tenantID string, p app.Page) ([]domain.Segment, error) {
	if !isUUID(tenantID) {
		return nil, nil
	}
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("created_at DESC, id DESC").
		Limit(p.Limit)
	if !p.Before.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", p.Before, p.BeforeID)
	}
	var ms []segment
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list segments", err)
	}
	out := make([]domain.Segment, 0, len(ms))
	for _, m := range ms {
		s, err := m.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *Postgres) LiveSegmentsAfter(ctx context.Context, afterID string, limit int) ([]app.SegmentRef, error) {
	q := r.db.WithContext(ctx).Model(&segment{}).Select("tenant_id, id").
		Where("deleted_at IS NULL").Order("id").Limit(limit)
	if afterID != "" {
		q = q.Where("id > ?", afterID)
	}
	var rows []struct {
		TenantID string
		ID       string
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list live segments", err)
	}
	out := make([]app.SegmentRef, len(rows))
	for i, x := range rows {
		out[i] = app.SegmentRef{TenantID: x.TenantID, ID: x.ID}
	}
	return out, nil
}

// ---- refresh ----

func (r *Postgres) AcquireRefresh(ctx context.Context, tenantID, segmentID, runID string, now, until time.Time) (domain.Segment, bool, error) {
	if !isUUID(tenantID) || !isUUID(segmentID) {
		return domain.Segment{}, false, domain.ErrSegmentNotFound
	}
	var ms []segment
	if err := r.db.WithContext(ctx).Raw(`UPDATE `+tSegments+`
		SET refresh_run_id = ?, refresh_lease_until = ?
		WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL
		  AND (refresh_lease_until IS NULL OR refresh_lease_until <= ?)
		RETURNING *`, runID, until, segmentID, tenantID, now).Scan(&ms).Error; err != nil {
		return domain.Segment{}, false, errs.Wrap(errs.Internal, "acquire refresh lease", err)
	}
	if len(ms) == 1 {
		s, err := ms[0].toDomain()
		return s, err == nil, err
	}
	s, err := r.SegmentByID(ctx, tenantID, segmentID)
	if err != nil {
		return domain.Segment{}, false, err
	}
	return s, false, nil
}

func (r *Postgres) ExtendLease(ctx context.Context, tx *gorm.DB, segmentID, runID string, until time.Time) (bool, error) {
	res := tx.WithContext(ctx).Exec(`UPDATE `+tSegments+` SET refresh_lease_until = ?
		WHERE id = ? AND refresh_run_id = ? AND deleted_at IS NULL`, until, segmentID, runID)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "extend refresh lease", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) ApplyPage(ctx context.Context, tx *gorm.DB, tenantID, segmentID, runID string, pageIDs, matched []string, now time.Time) (app.PageResult, error) {
	db := tx.WithContext(ctx)
	out := app.PageResult{Added: []string{}, Removed: []string{}}
	matched = filterUUIDs(matched)
	if len(matched) > 0 {
		var (
			b    strings.Builder
			args = make([]any, 0, len(matched)*5)
		)
		b.WriteString(`INSERT INTO ` + tMembers + ` (segment_id, player_id, tenant_id, added_at, run_id) VALUES `)
		for i, p := range matched {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("(?, ?, ?, ?, ?)")
			args = append(args, segmentID, p, tenantID, now, runID)
		}
		b.WriteString(` ON CONFLICT (segment_id, player_id) DO UPDATE SET run_id = EXCLUDED.run_id
			RETURNING player_id, (xmax = 0) AS inserted`)
		var rows []struct {
			PlayerID string
			Inserted bool
		}
		if err := db.Raw(b.String(), args...).Scan(&rows).Error; err != nil {
			return app.PageResult{}, errs.Wrap(errs.Internal, "upsert segment members", err)
		}
		for _, x := range rows {
			if x.Inserted {
				out.Added = append(out.Added, x.PlayerID)
			}
		}
	}
	pageIDs = filterUUIDs(pageIDs)
	if len(pageIDs) == 0 {
		return out, nil
	}
	q := `DELETE FROM ` + tMembers + ` WHERE segment_id = ? AND player_id IN ?`
	args := []any{segmentID, pageIDs}
	if len(matched) > 0 {
		q += ` AND player_id NOT IN ?`
		args = append(args, matched)
	}
	if err := db.Raw(q+` RETURNING player_id`, args...).Scan(&out.Removed).Error; err != nil {
		return app.PageResult{}, errs.Wrap(errs.Internal, "remove segment members", err)
	}
	return out, nil
}

func (r *Postgres) SweepStale(ctx context.Context, tx *gorm.DB, segmentID, runID string, limit int) ([]string, error) {
	removed := []string{}
	if err := tx.WithContext(ctx).Raw(`DELETE FROM `+tMembers+` WHERE ctid IN (
			SELECT ctid FROM `+tMembers+` WHERE segment_id = ? AND run_id <> ? LIMIT ?)
		RETURNING player_id`, segmentID, runID, limit).Scan(&removed).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "sweep stale segment members", err)
	}
	return removed, nil
}

func (r *Postgres) FinishRefresh(ctx context.Context, segmentID, runID string, now time.Time) error {
	if err := r.db.WithContext(ctx).Exec(`UPDATE `+tSegments+` SET
			member_count = (SELECT count(*) FROM `+tMembers+` WHERE segment_id = ?),
			last_refreshed_at = ?, refresh_lease_until = NULL
		WHERE id = ? AND refresh_run_id = ?`, segmentID, now, segmentID, runID).Error; err != nil {
		return errs.Wrap(errs.Internal, "finish segment refresh", err)
	}
	return nil
}

// ---- members ----

func (r *Postgres) ListMembers(ctx context.Context, tenantID, segmentID string, p app.Page) ([]app.Member, error) {
	if !isUUID(tenantID) || !isUUID(segmentID) {
		return nil, nil
	}
	q := `SELECT segment_id, player_id, added_at FROM ` + tMembers + ` WHERE segment_id = ? AND tenant_id = ?`
	args := []any{segmentID, tenantID}
	if !p.Before.IsZero() {
		if !isUUID(p.BeforeID) {
			return nil, errs.New(errs.Invalid, "malformed cursor")
		}
		q += ` AND (added_at, player_id) < (?, ?)`
		args = append(args, p.Before, p.BeforeID)
	}
	q += ` ORDER BY added_at DESC, player_id DESC LIMIT ?`
	args = append(args, p.Limit)
	var rows []struct {
		SegmentID string
		PlayerID  string
		AddedAt   time.Time
	}
	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list segment members", err)
	}
	out := make([]app.Member, len(rows))
	for i, x := range rows {
		out[i] = app.Member{SegmentID: x.SegmentID, PlayerID: x.PlayerID, AddedAt: x.AddedAt.UTC()}
	}
	return out, nil
}

func (r *Postgres) RemovePlayer(ctx context.Context, tx *gorm.DB, tenantID, playerID string) ([]string, error) {
	segs := []string{}
	if !isUUID(tenantID) || !isUUID(playerID) {
		return segs, nil
	}
	db := tx.WithContext(ctx)
	if err := db.Raw(`DELETE FROM `+tMembers+` WHERE tenant_id = ? AND player_id = ? RETURNING segment_id`,
		tenantID, playerID).Scan(&segs).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "remove player from segments", err)
	}
	if len(segs) == 0 {
		return segs, nil
	}
	if err := db.Exec(`UPDATE `+tSegments+` SET member_count = GREATEST(member_count - 1, 0) WHERE id IN ?`,
		segs).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "decrement segment member counts", err)
	}
	return segs, nil
}

func (r *Postgres) SegmentsOfPlayers(ctx context.Context, tenantID string, playerIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	playerIDs = filterUUIDs(playerIDs)
	if !isUUID(tenantID) || len(playerIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		PlayerID  string
		SegmentID string
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT m.player_id, m.segment_id FROM `+tMembers+` m
		JOIN `+tSegments+` s ON s.id = m.segment_id AND s.deleted_at IS NULL
		WHERE m.tenant_id = ? AND m.player_id IN ?
		ORDER BY m.player_id, m.segment_id`, tenantID, playerIDs).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load player segments", err)
	}
	for _, x := range rows {
		out[x.PlayerID] = append(out[x.PlayerID], x.SegmentID)
	}
	return out, nil
}

// ---- maintenance ----

func (r *Postgres) MarkRun(ctx context.Context, job string, at time.Time) error {
	if err := r.db.WithContext(ctx).Exec(`INSERT INTO `+tMarkers+` (job, last_run) VALUES (?, ?)
		ON CONFLICT (job) DO UPDATE SET last_run = EXCLUDED.last_run`, job, at).Error; err != nil {
		return errs.Wrap(errs.Internal, "mark run", err)
	}
	return nil
}

func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	if !isUUID(tenantID) {
		return nil
	}
	db := tx.WithContext(ctx)
	if err := db.Exec(`DELETE FROM `+tMembers+` WHERE tenant_id = ?`, tenantID).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge segment members", err)
	}
	if err := db.Exec(`DELETE FROM `+tSegments+` WHERE tenant_id = ?`, tenantID).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge segments", err)
	}
	return nil
}
