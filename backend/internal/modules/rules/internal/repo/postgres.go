package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/rules/internal/app"
	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/shared/errs"
)

// Postgres implements app.Repository.
type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

// table resolves a model to its schema-qualified name for raw SQL
// (rules_svc.<table> through the module's TablePrefix).
func (r *Postgres) table(model string) string { return r.db.NamingStrategy.TableName(model) }

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func internal(msg string, err error) error { return errs.Wrap(errs.Internal, msg, err) }

func keyset(q *gorm.DB, timeCol string, p app.Page) *gorm.DB {
	if !p.AfterTime.IsZero() {
		q = q.Where("("+timeCol+", id) < (?, ?)", p.AfterTime, p.AfterID)
	}
	return q.Order(timeCol + " DESC, id DESC").Limit(p.Limit)
}

// --- rules ---------------------------------------------------------------

func (r *Postgres) CreateRule(ctx context.Context, tx *gorm.DB, d domain.Rule) error {
	m := fromRule(d)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrSlugTaken
		}
		return internal("insert rule", err)
	}
	return nil
}

func (r *Postgres) RuleByID(ctx context.Context, tenantID, id string) (domain.Rule, error) {
	return r.ruleBy(r.db.WithContext(ctx), tenantID, id)
}

// RuleForUpdate locks the rule row: versioning, publish and PATCH of one
// rule serialize here.
func (r *Postgres) RuleForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Rule, error) {
	return r.ruleBy(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}), tenantID, id)
}

func (r *Postgres) ruleBy(q *gorm.DB, tenantID, id string) (domain.Rule, error) {
	if !eval.IsUUID(id) {
		return domain.Rule{}, domain.ErrRuleNotFound
	}
	var m rule
	err := q.Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantID, id).Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Rule{}, domain.ErrRuleNotFound
	case err != nil:
		return domain.Rule{}, internal("load rule", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) SaveRule(ctx context.Context, tx *gorm.DB, d domain.Rule) error {
	m := fromRule(d)
	res := tx.WithContext(ctx).Model(&rule{}).
		Where("tenant_id = ? AND id = ?", m.TenantID, m.ID).
		Updates(map[string]any{
			"name":               m.Name,
			"description":        m.Description,
			"trigger_event":      m.TriggerEvent,
			"program_id":         m.ProgramID,
			"priority":           m.Priority,
			"status":             m.Status,
			"current_version_id": m.CurrentVersionID,
			"updated_at":         m.UpdatedAt,
			"deleted_at":         m.DeletedAt,
		})
	if res.Error != nil {
		return internal("save rule", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrRuleNotFound
	}
	return nil
}

func (r *Postgres) ListRules(ctx context.Context, tenantID string, f app.RuleFilter, p app.Page) ([]domain.Rule, error) {
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.TriggerEvent != "" {
		q = q.Where("trigger_event = ?", f.TriggerEvent)
	}
	if f.ProgramID != "" {
		if !eval.IsUUID(f.ProgramID) {
			return nil, nil
		}
		q = q.Where("program_id = ?", f.ProgramID)
	}
	var ms []rule
	if err := keyset(q, "created_at", p).Find(&ms).Error; err != nil {
		return nil, internal("list rules", err)
	}
	out := make([]domain.Rule, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// --- versions ------------------------------------------------------------

func (r *Postgres) CreateVersion(ctx context.Context, tx *gorm.DB, v domain.Version) error {
	m := fromVersion(v)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrVersionConflict
		}
		return internal("insert rule version", err)
	}
	return nil
}

// SaveDraftVersion edits a draft; the published_at IS NULL guard makes a
// published version physically immutable.
func (r *Postgres) SaveDraftVersion(ctx context.Context, tx *gorm.DB, v domain.Version) error {
	m := fromVersion(v)
	res := tx.WithContext(ctx).Model(&ruleVersion{}).
		Where("tenant_id = ? AND id = ? AND published_at IS NULL", m.TenantID, m.ID).
		Updates(map[string]any{"conditions": m.Conditions, "actions": m.Actions, "limits": m.Limits,
			"schedule": m.Schedule, "stop_processing": m.StopProcessing})
	if res.Error != nil {
		return internal("save draft version", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionPublished
	}
	return nil
}

// PublishVersion stamps published_at once; later publishes keep the stamp.
func (r *Postgres) PublishVersion(ctx context.Context, tx *gorm.DB, v domain.Version) error {
	res := tx.WithContext(ctx).Model(&ruleVersion{}).
		Where("tenant_id = ? AND id = ?", v.TenantID, v.ID).
		Update("published_at", gorm.Expr("COALESCE(published_at, ?)", v.PublishedAt))
	if res.Error != nil {
		return internal("publish version", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionNotFound
	}
	return nil
}

func (r *Postgres) VersionByNumberTx(ctx context.Context, tx *gorm.DB, tenantID, ruleID string, number int) (domain.Version, error) {
	return r.versionBy(tx.WithContext(ctx).Where("tenant_id = ? AND rule_id = ? AND version = ?", tenantID, ruleID, number))
}

func (r *Postgres) LatestVersionTx(ctx context.Context, tx *gorm.DB, tenantID, ruleID string) (domain.Version, error) {
	return r.versionBy(tx.WithContext(ctx).Where("tenant_id = ? AND rule_id = ?", tenantID, ruleID).Order("version DESC"))
}

func (r *Postgres) LatestVersion(ctx context.Context, tenantID, ruleID string) (domain.Version, error) {
	return r.LatestVersionTx(ctx, r.db, tenantID, ruleID)
}

func (r *Postgres) versionBy(q *gorm.DB) (domain.Version, error) {
	var m ruleVersion
	err := q.Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Version{}, domain.ErrVersionNotFound
	case err != nil:
		return domain.Version{}, internal("load rule version", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) VersionsByIDs(ctx context.Context, tenantID string, ids []string) (map[string]domain.Version, error) {
	out := map[string]domain.Version{}
	if len(ids) == 0 {
		return out, nil
	}
	var ms []ruleVersion
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tenantID, ids).Find(&ms).Error; err != nil {
		return nil, internal("load rule versions", err)
	}
	for _, m := range ms {
		out[m.ID] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) ListVersions(ctx context.Context, tenantID, ruleID string, p app.Page) ([]domain.Version, error) {
	var ms []ruleVersion
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND rule_id = ?", tenantID, ruleID)
	if err := keyset(q, "created_at", p).Find(&ms).Error; err != nil {
		return nil, internal("list rule versions", err)
	}
	out := make([]domain.Version, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// --- ruleset -------------------------------------------------------------

// BumpGeneration increments the tenant's ruleset generation in the caller's
// tx and returns the new value.
func (r *Postgres) BumpGeneration(ctx context.Context, tx *gorm.DB, tenantID string) (int64, error) {
	t := r.table("rulesetGeneration")
	var gen int64
	err := tx.WithContext(ctx).Raw(
		`INSERT INTO `+t+` AS g (tenant_id, generation) VALUES (?, 1)
		 ON CONFLICT (tenant_id) DO UPDATE SET generation = g.generation + 1
		 RETURNING generation`, tenantID).Scan(&gen).Error
	if err != nil {
		return 0, internal("bump ruleset generation", err)
	}
	return gen, nil
}

func (r *Postgres) Generation(ctx context.Context, tenantID string) (int64, error) {
	var ms []rulesetGeneration
	if err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Limit(1).Find(&ms).Error; err != nil {
		return 0, internal("load ruleset generation", err)
	}
	if len(ms) == 0 {
		return 0, nil
	}
	return ms[0].Generation, nil
}

type liveRow struct {
	RuleID         string
	RuleVersionID  string
	Name           string
	ProgramID      *string
	Priority       int
	Conditions     string
	Actions        string
	Limits         string
	Schedule       string
	StopProcessing bool
}

// LiveRuleSources returns the ONE current version of every live rule for a
// trigger (G23: older versions never fire; G24: inactive/archived/deleted
// rules are not evaluated).
func (r *Postgres) LiveRuleSources(ctx context.Context, tenantID, triggerEvent string) ([]eval.RuleSource, error) {
	var rows []liveRow
	err := r.db.WithContext(ctx).Raw(
		`SELECT r.id AS rule_id, v.id AS rule_version_id, r.name, r.program_id, r.priority,
		        v.conditions::text AS conditions, v.actions::text AS actions, v.limits::text AS limits,
		        v.schedule::text AS schedule, v.stop_processing
		   FROM `+r.table("rule")+` r
		   JOIN `+r.table("ruleVersion")+` v ON v.id = r.current_version_id
		  WHERE r.tenant_id = ? AND r.trigger_event = ? AND r.status = 'active'
		    AND r.deleted_at IS NULL AND v.published_at IS NOT NULL
		  ORDER BY r.priority DESC, r.id ASC`, tenantID, triggerEvent).Scan(&rows).Error
	if err != nil {
		return nil, internal("load live rules", err)
	}
	out := make([]eval.RuleSource, len(rows))
	for i, row := range rows {
		out[i] = eval.RuleSource{
			RuleID: row.RuleID, RuleVersionID: row.RuleVersionID, Name: row.Name, ProgramID: val(row.ProgramID),
			Priority: row.Priority, Conditions: json.RawMessage(row.Conditions),
			Actions: json.RawMessage(row.Actions), Limits: json.RawMessage(row.Limits),
			Schedule: json.RawMessage(row.Schedule), StopProcessing: row.StopProcessing,
		}
	}
	return out, nil
}

// --- decisions -------------------------------------------------------------

func (r *Postgres) DecisionExists(ctx context.Context, activityID string) (bool, error) {
	var exists bool
	err := r.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM `+r.table("decision")+` WHERE activity_id = ?)`, activityID).Scan(&exists).Error
	if err != nil {
		return false, internal("check decision", err)
	}
	return exists, nil
}

// InsertDecision claims the activity: ON CONFLICT (activity_id) DO NOTHING,
// false when another delivery already decided it.
func (r *Postgres) InsertDecision(ctx context.Context, tx *gorm.DB, d domain.Decision) (bool, error) {
	m := fromDecision(d)
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "activity_id"}}, DoNothing: true}).
		Create(&m)
	if res.Error != nil {
		return false, internal("insert decision", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) SetDecisionOutcome(ctx context.Context, tx *gorm.DB, d domain.Decision) error {
	err := tx.WithContext(ctx).Model(&decision{}).Where("id = ?", d.ID).
		Updates(map[string]any{"outcome": d.Outcome, "reason": d.Reason}).Error
	if err != nil {
		return internal("set decision outcome", err)
	}
	return nil
}

func (r *Postgres) InsertExecutions(ctx context.Context, tx *gorm.DB, es []domain.Execution) error {
	if len(es) == 0 {
		return nil
	}
	ms := make([]ruleExecution, len(es))
	for i, e := range es {
		m, err := fromExecution(e)
		if err != nil {
			return internal("marshal execution", err)
		}
		ms[i] = m
	}
	if err := tx.WithContext(ctx).Create(&ms).Error; err != nil {
		return internal("insert executions", err)
	}
	return nil
}

func (r *Postgres) InsertEffects(ctx context.Context, tx *gorm.DB, es []domain.Effect) error {
	if len(es) == 0 {
		return nil
	}
	ms := make([]ruleExecutionEffect, len(es))
	for i, e := range es {
		m, err := fromEffect(e)
		if err != nil {
			return internal("marshal effect", err)
		}
		ms[i] = m
	}
	err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "idempotency_key"}}, DoNothing: true}).
		Create(&ms).Error
	if err != nil {
		return internal("insert effects", err)
	}
	return nil
}

const limitsSavepoint = "rules_limits"

// ApplyLimits counts one firing against every limit of the rule, atomically:
// each counter is a conditional upsert (INSERT ... ON CONFLICT DO UPDATE ...
// WHERE count < max), which takes the row lock and only increments while
// under the cap, so concurrent decisions for one player can never exceed it
// (no ordering needed, ADR-0012). If any limit refuses, the savepoint rolls
// back the counters already incremented for this rule.
func (r *Postgres) ApplyLimits(ctx context.Context, tx *gorm.DB, c app.LimitCheck) (bool, error) {
	if c.Limits.IsZero() {
		return true, nil
	}
	if err := tx.WithContext(ctx).SavePoint(limitsSavepoint).Error; err != nil {
		return false, internal("savepoint limits", err)
	}
	at := c.At.UTC()
	t := r.table("rulePlayerCounter")
	capped := func(window string, maxCount int64) (bool, error) {
		res := tx.WithContext(ctx).Exec(
			`INSERT INTO `+t+` AS c (tenant_id, rule_id, player_id, window_key, count, last_fired_at)
			 VALUES (?, ?, ?, ?, 1, ?)
			 ON CONFLICT (tenant_id, rule_id, player_id, window_key) DO UPDATE
			    SET count = c.count + 1, last_fired_at = GREATEST(c.last_fired_at, EXCLUDED.last_fired_at)
			  WHERE c.count < ?`,
			c.TenantID, c.RuleID, c.PlayerID, window, at, maxCount)
		return res.RowsAffected == 1, res.Error
	}
	type step struct {
		window string
		max    int64
	}
	var steps []step
	if c.Limits.MaxPerPlayer > 0 {
		steps = append(steps, step{domain.WindowLifetime, c.Limits.MaxPerPlayer})
	}
	if c.Limits.MaxPerPlayerPerDay > 0 {
		steps = append(steps, step{domain.DayWindow(at), c.Limits.MaxPerPlayerPerDay})
	}
	if c.Limits.MaxPerPlayerPerWeek > 0 {
		steps = append(steps, step{domain.WeekWindow(at), c.Limits.MaxPerPlayerPerWeek})
	}
	refuse := func() (bool, error) {
		if err := tx.WithContext(ctx).RollbackTo(limitsSavepoint).Error; err != nil {
			return false, internal("rollback limits", err)
		}
		return false, nil
	}
	for _, st := range steps {
		ok, err := capped(st.window, st.max)
		if err != nil {
			return false, internal("apply limit", err)
		}
		if !ok {
			return refuse()
		}
	}
	if c.Limits.CooldownSeconds > 0 {
		// Symmetric against the latest firing, compared on activity time:
		// a late-arriving older activity inside the cooldown is refused too.
		res := tx.WithContext(ctx).Exec(
			`INSERT INTO `+t+` AS c (tenant_id, rule_id, player_id, window_key, count, last_fired_at)
			 VALUES (?, ?, ?, ?, 1, ?)
			 ON CONFLICT (tenant_id, rule_id, player_id, window_key) DO UPDATE
			    SET count = c.count + 1, last_fired_at = GREATEST(c.last_fired_at, EXCLUDED.last_fired_at)
			  WHERE EXCLUDED.last_fired_at >= c.last_fired_at + make_interval(secs => ?)
			     OR EXCLUDED.last_fired_at <= c.last_fired_at - make_interval(secs => ?)`,
			c.TenantID, c.RuleID, c.PlayerID, domain.WindowCooldown, at, float64(c.Limits.CooldownSeconds), float64(c.Limits.CooldownSeconds))
		if res.Error != nil {
			return false, internal("apply cooldown", res.Error)
		}
		if res.RowsAffected != 1 {
			return refuse()
		}
	}
	if err := tx.WithContext(ctx).Exec("RELEASE SAVEPOINT " + limitsSavepoint).Error; err != nil {
		return false, internal("release limits savepoint", err)
	}
	return true, nil
}

// --- history projection (grammar v2) ----------------------------------------

const dayLayout = "2006-01-02"

// RecordPlayerEvent counts one evaluated activity into its UTC-day bucket.
// It runs in the decision tx right after the decision insert won, so a
// redelivered activity is never counted twice.
func (r *Postgres) RecordPlayerEvent(ctx context.Context, tx *gorm.DB, e app.PlayerEvent) error {
	err := tx.WithContext(ctx).Exec(
		`INSERT INTO `+r.table("rulePlayerEventDay")+` AS h (tenant_id, player_id, event_type, day, count)
		 VALUES (?, ?, ?, ?::date, 1)
		 ON CONFLICT (tenant_id, player_id, event_type, day) DO UPDATE SET count = h.count + 1`,
		e.TenantID, e.PlayerID, e.EventType, e.At.UTC().Format(dayLayout)).Error
	if err != nil {
		return internal("record player event", err)
	}
	return nil
}

type historyRow struct {
	EventType string
	C1d       int64 `gorm:"column:c1d"`
	C7d       int64 `gorm:"column:c7d"`
	C30d      int64 `gorm:"column:c30d"`
	C90d      int64 `gorm:"column:c90d"`
	CAll      int64 `gorm:"column:call"`
}

// PlayerHistory returns, in one query, the player's recorded activity per
// event type and window. Windows are UTC days ending on q.At's day
// (inclusive); later days (out-of-order arrivals) are excluded.
func (r *Postgres) PlayerHistory(ctx context.Context, q app.HistoryQuery) (map[string]map[string]int64, error) {
	out := map[string]map[string]int64{}
	if len(q.EventTypes) == 0 || !eval.IsUUID(q.PlayerID) {
		return out, nil
	}
	day := q.At.UTC().Truncate(24 * time.Hour)
	since := func(window string) string {
		return day.AddDate(0, 0, 1-eval.WindowDays[window]).Format(dayLayout)
	}
	var rows []historyRow
	err := r.db.WithContext(ctx).Raw(
		`SELECT event_type,
		        COALESCE(SUM(count) FILTER (WHERE day >= ?::date), 0) AS c1d,
		        COALESCE(SUM(count) FILTER (WHERE day >= ?::date), 0) AS c7d,
		        COALESCE(SUM(count) FILTER (WHERE day >= ?::date), 0) AS c30d,
		        COALESCE(SUM(count) FILTER (WHERE day >= ?::date), 0) AS c90d,
		        COALESCE(SUM(count), 0) AS call
		   FROM `+r.table("rulePlayerEventDay")+`
		  WHERE tenant_id = ? AND player_id = ? AND event_type IN ? AND day <= ?::date
		  GROUP BY event_type`,
		since(eval.Window1d), since(eval.Window7d), since(eval.Window30d), since(eval.Window90d),
		q.TenantID, q.PlayerID, q.EventTypes, day.Format(dayLayout)).Scan(&rows).Error
	if err != nil {
		return nil, internal("load player history", err)
	}
	for _, row := range rows {
		out[row.EventType] = map[string]int64{
			eval.Window1d: row.C1d, eval.Window7d: row.C7d, eval.Window30d: row.C30d,
			eval.Window90d: row.C90d, eval.WindowAll: row.CAll,
		}
	}
	return out, nil
}

// --- stats -------------------------------------------------------------------

type ruleStatRow struct {
	RuleID          string
	Name            string
	Fired           int64
	NotMatched      int64
	Limited         int64
	OutOfSchedule   int64
	SkippedByStop   int64
	EffectsApplied  int64
	EffectsRejected int64
	PointsAwarded   int64
	XpAwarded       int64
}

// RuleStats aggregates executions (by created_at) and their effects (by
// requested_at, which equals the execution time) over [from, to) per rule.
// points/xp sum the amounts of every credit_points/grant_xp effect of fired
// executions, whatever its settlement.
func (r *Postgres) RuleStats(ctx context.Context, tenantID string, from, to time.Time) ([]app.RuleStat, error) {
	if !eval.IsUUID(tenantID) {
		return nil, nil
	}
	var rows []ruleStatRow
	err := r.db.WithContext(ctx).Raw(
		`WITH ex AS (
		    SELECT rule_id,
		           count(*) FILTER (WHERE status = 'fired')           AS fired,
		           count(*) FILTER (WHERE status = 'not_matched')     AS not_matched,
		           count(*) FILTER (WHERE status = 'limited')         AS limited,
		           count(*) FILTER (WHERE status = 'out_of_schedule') AS out_of_schedule,
		           count(*) FILTER (WHERE status = 'skipped_by_stop') AS skipped_by_stop
		      FROM `+r.table("ruleExecution")+`
		     WHERE tenant_id = ? AND created_at >= ? AND created_at < ?
		     GROUP BY rule_id
		 ), ef AS (
		    SELECT rule_id,
		           count(*) FILTER (WHERE status = 'applied')  AS effects_applied,
		           count(*) FILTER (WHERE status = 'rejected') AS effects_rejected,
		           COALESCE(SUM((params->>'amount')::numeric) FILTER (WHERE type = 'credit_points'), 0)::bigint AS points_awarded,
		           COALESCE(SUM((params->>'amount')::numeric) FILTER (WHERE type = 'grant_xp'), 0)::bigint      AS xp_awarded
		      FROM `+r.table("ruleExecutionEffect")+`
		     WHERE tenant_id = ? AND requested_at >= ? AND requested_at < ?
		     GROUP BY rule_id
		 )
		 SELECT COALESCE(ex.rule_id, ef.rule_id) AS rule_id, COALESCE(r.name, '') AS name,
		        COALESCE(ex.fired, 0) AS fired, COALESCE(ex.not_matched, 0) AS not_matched,
		        COALESCE(ex.limited, 0) AS limited, COALESCE(ex.out_of_schedule, 0) AS out_of_schedule,
		        COALESCE(ex.skipped_by_stop, 0) AS skipped_by_stop,
		        COALESCE(ef.effects_applied, 0) AS effects_applied, COALESCE(ef.effects_rejected, 0) AS effects_rejected,
		        COALESCE(ef.points_awarded, 0) AS points_awarded, COALESCE(ef.xp_awarded, 0) AS xp_awarded
		   FROM ex FULL JOIN ef ON ef.rule_id = ex.rule_id
		   LEFT JOIN `+r.table("rule")+` r ON r.id = COALESCE(ex.rule_id, ef.rule_id) AND r.tenant_id = ?
		  ORDER BY fired DESC, rule_id`,
		tenantID, from, to, tenantID, from, to, tenantID).Scan(&rows).Error
	if err != nil {
		return nil, internal("rule stats", err)
	}
	out := make([]app.RuleStat, len(rows))
	for i, row := range rows {
		out[i] = app.RuleStat{
			RuleID: row.RuleID, Name: row.Name, Fired: row.Fired, NotMatched: row.NotMatched, Limited: row.Limited,
			OutOfSchedule: row.OutOfSchedule, SkippedByStop: row.SkippedByStop, EffectsApplied: row.EffectsApplied,
			EffectsRejected: row.EffectsRejected, PointsAwarded: row.PointsAwarded, XPAwarded: row.XpAwarded,
		}
	}
	return out, nil
}

func (r *Postgres) ListDecisions(ctx context.Context, tenantID string, f app.DecisionFilter, p app.Page) ([]domain.Decision, error) {
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if f.ActivityID != "" {
		if !eval.IsUUID(f.ActivityID) {
			return nil, nil
		}
		q = q.Where("activity_id = ?", f.ActivityID)
	}
	if f.PlayerID != "" {
		if !eval.IsUUID(f.PlayerID) {
			return nil, nil
		}
		q = q.Where("player_id = ?", f.PlayerID)
	}
	var ms []decision
	if err := keyset(q, "evaluated_at", p).Find(&ms).Error; err != nil {
		return nil, internal("list decisions", err)
	}
	out := make([]domain.Decision, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) DecisionByID(ctx context.Context, tenantID, id string) (domain.Decision, error) {
	if !eval.IsUUID(id) {
		return domain.Decision{}, domain.ErrDecisionNotFound
	}
	var m decision
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Decision{}, domain.ErrDecisionNotFound
	case err != nil:
		return domain.Decision{}, internal("load decision", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) ExecutionsByDecision(ctx context.Context, tenantID, decisionID string) ([]domain.Execution, error) {
	var ms []ruleExecution
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND decision_id = ?", tenantID, decisionID).
		Order("created_at, id").Find(&ms).Error
	if err != nil {
		return nil, internal("load executions", err)
	}
	out := make([]domain.Execution, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) EffectsByDecision(ctx context.Context, tenantID, decisionID string) ([]domain.Effect, error) {
	var ms []ruleExecutionEffect
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND decision_id = ?", tenantID, decisionID).
		Order("requested_at, rule_version_id, action_index").Find(&ms).Error
	if err != nil {
		return nil, internal("load effects", err)
	}
	out := make([]domain.Effect, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// SettleEffect moves a requested effect to applied/rejected. Only a
// requested row moves, so redelivered or reordered outcomes are no-ops.
func (r *Postgres) SettleEffect(ctx context.Context, tx *gorm.DB, tenantID, key, status, reason string, at time.Time) (bool, error) {
	if !eval.IsUUID(tenantID) {
		return false, nil
	}
	res := tx.WithContext(ctx).Model(&ruleExecutionEffect{}).
		Where("tenant_id = ? AND idempotency_key = ? AND status = ?", tenantID, key, domain.EffectRequested).
		Updates(map[string]any{"status": status, "reason": reason, "settled_at": at})
	if res.Error != nil {
		return false, internal("settle effect", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) PendingEffects(ctx context.Context, before time.Time, maxAttempts, limit int) ([]domain.Effect, error) {
	var ms []ruleExecutionEffect
	err := r.db.WithContext(ctx).
		Where("status = ? AND COALESCE(last_attempt_at, requested_at) < ? AND attempts < ?",
			domain.EffectRequested, before, maxAttempts).
		Order("requested_at, id").Limit(limit).Find(&ms).Error
	if err != nil {
		return nil, internal("load pending effects", err)
	}
	out := make([]domain.Effect, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) MarkEffectsRetried(ctx context.Context, tx *gorm.DB, ids []string, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	err := tx.WithContext(ctx).Model(&ruleExecutionEffect{}).Where("id IN ?", ids).
		Updates(map[string]any{"attempts": gorm.Expr("attempts + 1"), "last_attempt_at": at}).Error
	if err != nil {
		return internal("mark effects retried", err)
	}
	return nil
}

// PurgeTenant deletes every row of a tenant (tenant.deleted.v1). Idempotent.
func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	if !eval.IsUUID(tenantID) {
		return nil
	}
	q := tx.WithContext(ctx)
	for _, m := range []any{
		&ruleExecutionEffect{}, &ruleExecution{}, &decision{}, &rulePlayerCounter{}, &rulePlayerEventDay{},
	} {
		if err := q.Where("tenant_id = ?", tenantID).Delete(m).Error; err != nil {
			return internal("purge tenant", err)
		}
	}
	if err := q.Model(&rule{}).Where("tenant_id = ?", tenantID).Update("current_version_id", nil).Error; err != nil {
		return internal("purge tenant", err)
	}
	for _, m := range []any{&ruleVersion{}, &rule{}, &rulesetGeneration{}} {
		if err := q.Where("tenant_id = ?", tenantID).Delete(m).Error; err != nil {
			return internal("purge tenant", err)
		}
	}
	return nil
}
