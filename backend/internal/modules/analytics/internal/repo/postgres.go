// Package repo implements analytics' persistence in raw SQL: the projection
// is upserts and aggregates, which GORM models would only obscure. Days are
// passed as 'YYYY-MM-DD' text cast to DATE so the session time zone never
// shifts them.
package repo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/analytics/internal/app"
	"levelup/internal/modules/analytics/internal/domain"
	"levelup/internal/shared/errs"
)

const (
	tApplied   = "analytics_svc.applied_events"
	tCounters  = "analytics_svc.daily_counters"
	tDays      = "analytics_svc.player_days"
	tEventDays = "analytics_svc.player_event_days"
	tFirstSeen = "analytics_svc.player_first_seen"
	tMarkers   = "analytics_svc.reconcile_markers"

	pruneBatch = 5000
)

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

func day(t time.Time) string { return domain.DayOf(t).Format(time.DateOnly) }

func (r *Postgres) Apply(ctx context.Context, tx *gorm.DB, f domain.Fact, now time.Time) (bool, error) {
	db := tx.WithContext(ctx)
	res := db.Exec(`INSERT INTO `+tApplied+` (event_id, tenant_id, applied_at) VALUES (?, ?, ?)
		ON CONFLICT DO NOTHING`, f.EventID, f.TenantID, now)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "record applied event", res.Error)
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	d := day(f.Day)
	for _, c := range f.Counters {
		if err := db.Exec(`INSERT INTO `+tCounters+` (tenant_id, metric, day, dimension, value)
			VALUES (?, ?, ?::date, ?, ?)
			ON CONFLICT (tenant_id, metric, day, dimension)
			DO UPDATE SET value = `+tCounters+`.value + EXCLUDED.value`,
			f.TenantID, c.Metric, d, c.Dimension, c.Value).Error; err != nil {
			return false, errs.Wrap(errs.Internal, "upsert daily counter", err)
		}
	}
	if !f.IsActivity() {
		return true, nil
	}
	if err := db.Exec(`INSERT INTO `+tDays+` (tenant_id, day, player_id) VALUES (?, ?::date, ?)
		ON CONFLICT DO NOTHING`, f.TenantID, d, f.PlayerID).Error; err != nil {
		return false, errs.Wrap(errs.Internal, "record player day", err)
	}
	if err := db.Exec(`INSERT INTO `+tEventDays+` (tenant_id, event_type, day, player_id) VALUES (?, ?, ?::date, ?)
		ON CONFLICT DO NOTHING`, f.TenantID, f.EventType, d, f.PlayerID).Error; err != nil {
		return false, errs.Wrap(errs.Internal, "record player event day", err)
	}
	if err := db.Exec(`INSERT INTO `+tFirstSeen+` (tenant_id, player_id, first_day) VALUES (?, ?, ?::date)
		ON CONFLICT (tenant_id, player_id)
		DO UPDATE SET first_day = LEAST(`+tFirstSeen+`.first_day, EXCLUDED.first_day)`,
		f.TenantID, f.PlayerID, d).Error; err != nil {
		return false, errs.Wrap(errs.Internal, "record player first seen", err)
	}
	return true, nil
}

func (r *Postgres) Counters(ctx context.Context, tenantID string, rg domain.Range, metrics []string) ([]app.CounterRow, error) {
	if len(metrics) == 0 {
		return nil, nil
	}
	var rows []struct {
		Day       time.Time
		Metric    string
		Dimension string
		Value     int64
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT day, metric, dimension, value FROM `+tCounters+`
		WHERE tenant_id = ? AND metric IN ? AND day BETWEEN ?::date AND ?::date`,
		tenantID, metrics, day(rg.From), day(rg.To)).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load daily counters", err)
	}
	out := make([]app.CounterRow, len(rows))
	for i, x := range rows {
		out[i] = app.CounterRow{Day: domain.DayOf(x.Day), Metric: x.Metric, Dimension: x.Dimension, Value: x.Value}
	}
	return out, nil
}

func (r *Postgres) ActivePlayersByDay(ctx context.Context, tenantID string, rg domain.Range) (map[time.Time]int64, error) {
	var rows []struct {
		Day     time.Time
		Players int64
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT day, count(*) AS players FROM `+tDays+`
		WHERE tenant_id = ? AND day BETWEEN ?::date AND ?::date GROUP BY day`,
		tenantID, day(rg.From), day(rg.To)).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load active players", err)
	}
	out := make(map[time.Time]int64, len(rows))
	for _, x := range rows {
		out[domain.DayOf(x.Day)] = x.Players
	}
	return out, nil
}

func (r *Postgres) DistinctActivePlayers(ctx context.Context, tenantID string, rg domain.Range) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Raw(`SELECT count(DISTINCT player_id) FROM `+tDays+`
		WHERE tenant_id = ? AND day BETWEEN ?::date AND ?::date`,
		tenantID, day(rg.From), day(rg.To)).Scan(&n).Error; err != nil {
		return 0, errs.Wrap(errs.Internal, "count active players", err)
	}
	return n, nil
}

// weekOf truncates a DATE column to its Monday without the session time zone.
func weekOf(col string) string { return "date_trunc('week', " + col + "::timestamp)::date" }

func (r *Postgres) CohortSizes(ctx context.Context, tenantID string, from, until time.Time) (map[time.Time]int64, error) {
	var rows []struct {
		Cohort  time.Time
		Players int64
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT `+weekOf("first_day")+` AS cohort, count(*) AS players
		FROM `+tFirstSeen+`
		WHERE tenant_id = ? AND first_day >= ?::date AND first_day < ?::date
		GROUP BY 1`, tenantID, day(from), day(until)).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load cohort sizes", err)
	}
	out := make(map[time.Time]int64, len(rows))
	for _, x := range rows {
		out[domain.DayOf(x.Cohort)] = x.Players
	}
	return out, nil
}

func (r *Postgres) CohortActivity(ctx context.Context, tenantID string, from, until time.Time) ([]domain.CohortCell, error) {
	var rows []struct {
		Cohort  time.Time
		Offset  int
		Players int64
	}
	if err := r.db.WithContext(ctx).Raw(`SELECT `+weekOf("f.first_day")+` AS cohort,
			(`+weekOf("d.day")+` - `+weekOf("f.first_day")+`) / 7 AS "offset",
			count(DISTINCT d.player_id) AS players
		FROM `+tFirstSeen+` f
		JOIN `+tDays+` d ON d.tenant_id = f.tenant_id AND d.player_id = f.player_id
		WHERE f.tenant_id = ? AND f.first_day >= ?::date AND f.first_day < ?::date
		  AND d.day >= f.first_day AND d.day < ?::date
		GROUP BY 1, 2`, tenantID, day(from), day(until), day(until)).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load cohort activity", err)
	}
	out := make([]domain.CohortCell, len(rows))
	for i, x := range rows {
		out[i] = domain.CohortCell{Cohort: domain.DayOf(x.Cohort), Offset: x.Offset, Players: x.Players}
	}
	return out, nil
}

// Funnel chains one CTE per step: s1 is each player's first day doing step
// 1 in the range; s(k) is the first day doing step k on or after s(k-1).
func (r *Postgres) Funnel(ctx context.Context, tenantID string, steps []string, rg domain.Range) ([]int64, error) {
	if len(steps) == 0 {
		return nil, nil
	}
	var (
		b    strings.Builder
		args []any
	)
	b.WriteString("WITH ")
	for i, st := range steps {
		if i > 0 {
			b.WriteString(", ")
		}
		if i == 0 {
			fmt.Fprintf(&b, `s1 AS (SELECT player_id, min(day) AS d FROM %s
				WHERE tenant_id = ? AND event_type = ? AND day BETWEEN ?::date AND ?::date
				GROUP BY player_id)`, tEventDays)
			args = append(args, tenantID, st, day(rg.From), day(rg.To))
			continue
		}
		fmt.Fprintf(&b, `s%d AS (SELECT e.player_id, min(e.day) AS d FROM %s e
			JOIN s%d p ON p.player_id = e.player_id AND e.day >= p.d
			WHERE e.tenant_id = ? AND e.event_type = ? AND e.day <= ?::date
			GROUP BY e.player_id)`, i+1, tEventDays, i)
		args = append(args, tenantID, st, day(rg.To))
	}
	b.WriteString(" SELECT ")
	for i := range steps {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "(SELECT count(*) FROM s%d) AS c%d", i+1, i+1)
	}
	rows, err := r.db.WithContext(ctx).Raw(b.String(), args...).Rows()
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "compute funnel", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]int64, len(steps))
	if rows.Next() {
		dst := make([]any, len(steps))
		for i := range out {
			dst[i] = &out[i]
		}
		if err := rows.Scan(dst...); err != nil {
			return nil, errs.Wrap(errs.Internal, "scan funnel", err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(errs.Internal, "read funnel", err)
	}
	return out, nil
}

// Prune deletes in batches so no single statement holds locks for long.
func (r *Postgres) Prune(ctx context.Context, before, appliedBefore time.Time) (int64, error) {
	var total int64
	d := day(before)
	for _, q := range []struct {
		table, where string
		arg          any
	}{
		{tCounters, "day < ?::date", d},
		{tDays, "day < ?::date", d},
		{tEventDays, "day < ?::date", d},
		{tApplied, "applied_at < ?", appliedBefore},
	} {
		for {
			res := r.db.WithContext(ctx).Exec(`DELETE FROM `+q.table+` WHERE ctid IN (
				SELECT ctid FROM `+q.table+` WHERE `+q.where+` LIMIT ?)`, q.arg, pruneBatch)
			if res.Error != nil {
				return total, errs.Wrap(errs.Internal, "prune "+q.table, res.Error)
			}
			total += res.RowsAffected
			if res.RowsAffected < pruneBatch {
				break
			}
		}
	}
	return total, nil
}

func (r *Postgres) MarkRun(ctx context.Context, job string, at time.Time) error {
	if err := r.db.WithContext(ctx).Exec(`INSERT INTO `+tMarkers+` (job, last_run) VALUES (?, ?)
		ON CONFLICT (job) DO UPDATE SET last_run = EXCLUDED.last_run`, job, at).Error; err != nil {
		return errs.Wrap(errs.Internal, "mark run", err)
	}
	return nil
}

func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	db := tx.WithContext(ctx)
	for _, t := range []string{tCounters, tDays, tEventDays, tFirstSeen, tApplied} {
		if err := db.Exec(`DELETE FROM `+t+` WHERE tenant_id = ?`, tenantID).Error; err != nil {
			return errs.Wrap(errs.Internal, "purge "+t, err)
		}
	}
	return nil
}
