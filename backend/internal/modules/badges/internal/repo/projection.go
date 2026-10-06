package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/badges/internal/app"
	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/shared/errs"
)

// ---- requirements projection ----

func (r *Postgres) MarkEventApplied(ctx context.Context, tx *gorm.DB, tenantID, key string, at time.Time) (bool, error) {
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&appliedEvent{TenantID: tenantID, EventKey: key, AppliedAt: at})
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "record applied event", res.Error)
	}
	return res.RowsAffected == 1, nil
}

// applyStatsSQL upserts one projection row. The inserted values are the
// deltas of a first sighting; on conflict increments add, maxima take
// GREATEST, and lifetime_points is replaced only by a newer ledger time
// (ties keep the larger value, so the outcome is order-independent).
const applyStatsSQL = `
INSERT INTO ` + schema + `badge_player_stats AS s
    (tenant_id, player_id, lifetime_points, lifetime_points_at, missions_completed, max_streak, level, badges_earned, updated_at)
VALUES (@tenant, @player, @lifetime, @lifetime_at, @missions, @streak, @level, @badges, @at)
ON CONFLICT (tenant_id, player_id) DO UPDATE SET
    lifetime_points = CASE
        WHEN EXCLUDED.lifetime_points_at IS NOT NULL AND (
             s.lifetime_points_at IS NULL
          OR EXCLUDED.lifetime_points_at > s.lifetime_points_at
          OR (EXCLUDED.lifetime_points_at = s.lifetime_points_at AND EXCLUDED.lifetime_points > s.lifetime_points))
        THEN EXCLUDED.lifetime_points ELSE s.lifetime_points END,
    lifetime_points_at = CASE
        WHEN EXCLUDED.lifetime_points_at IS NOT NULL AND (
             s.lifetime_points_at IS NULL OR EXCLUDED.lifetime_points_at > s.lifetime_points_at)
        THEN EXCLUDED.lifetime_points_at ELSE s.lifetime_points_at END,
    missions_completed = s.missions_completed + EXCLUDED.missions_completed,
    max_streak = GREATEST(s.max_streak, EXCLUDED.max_streak),
    level = GREATEST(s.level, EXCLUDED.level),
    badges_earned = s.badges_earned + EXCLUDED.badges_earned,
    updated_at = EXCLUDED.updated_at`

const applyActivitySQL = `
INSERT INTO ` + schema + `badge_player_activity_counts AS c (tenant_id, player_id, event_type, count, updated_at)
VALUES (?, ?, ?, 1, ?)
ON CONFLICT (tenant_id, player_id, event_type) DO UPDATE SET
    count = c.count + 1,
    updated_at = EXCLUDED.updated_at`

func (r *Postgres) ApplyPlayerStats(ctx context.Context, tx *gorm.DB, tenantID, playerID string, u app.StatsUpdate) error {
	var (
		lifetime   int64
		lifetimeAt *time.Time
	)
	if u.LifetimePoints != nil {
		lifetime = *u.LifetimePoints
		at := u.LifetimeAt.UTC()
		lifetimeAt = &at
	}
	err := tx.WithContext(ctx).Exec(applyStatsSQL, map[string]any{
		"tenant":      tenantID,
		"player":      playerID,
		"lifetime":    lifetime,
		"lifetime_at": lifetimeAt,
		"missions":    u.MissionsCompleted,
		"streak":      u.MaxStreak,
		"level":       u.Level,
		"badges":      u.BadgesEarned,
		"at":          u.At,
	}).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "apply player stats", err)
	}
	if u.ActivityType == "" {
		return nil
	}
	if err := tx.WithContext(ctx).Exec(applyActivitySQL, tenantID, playerID, u.ActivityType, u.At).Error; err != nil {
		return errs.Wrap(errs.Internal, "apply activity count", err)
	}
	return nil
}

func (r *Postgres) PlayerStats(ctx context.Context, tenantID, playerID string) (domain.PlayerStats, error) {
	out := domain.PlayerStats{TenantID: tenantID, PlayerID: playerID, ActivityCounts: map[string]int64{}}
	var m badgePlayerStat
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND player_id = ?", tenantID, playerID).First(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
	case err != nil:
		return domain.PlayerStats{}, errs.Wrap(errs.Internal, "load player stats", err)
	default:
		out.LifetimePoints = m.LifetimePoints
		out.MissionsCompleted = m.MissionsCompleted
		out.MaxStreak = m.MaxStreak
		out.Level = m.Level
		out.BadgesEarned = m.BadgesEarned
	}
	var counts []badgePlayerActivityCount
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND player_id = ?", tenantID, playerID).Find(&counts).Error; err != nil {
		return domain.PlayerStats{}, errs.Wrap(errs.Internal, "load activity counts", err)
	}
	for _, c := range counts {
		out.ActivityCounts[c.EventType] = c.Count
	}
	return out, nil
}

func (r *Postgres) AutoAwardBadges(ctx context.Context, tenantID string) ([]domain.Badge, error) {
	var ms []badge
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND requirements IS NOT NULL AND is_active AND deleted_at IS NULL", tenantID).
		Order("created_at, id").
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load auto-award badges", err)
	}
	out := make([]domain.Badge, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

const pruneAppliedSQL = `
DELETE FROM ` + schema + `applied_events
WHERE ctid IN (SELECT ctid FROM ` + schema + `applied_events WHERE applied_at < ? LIMIT ?)`

func (r *Postgres) PruneAppliedEvents(ctx context.Context, before time.Time, limit int) (int, error) {
	res := r.db.WithContext(ctx).Exec(pruneAppliedSQL, before, limit)
	if res.Error != nil {
		return 0, errs.Wrap(errs.Internal, "prune applied events", res.Error)
	}
	return int(res.RowsAffected), nil
}

// ---- award stats ----

const badgeStatsSQL = `
SELECT b.id AS badge_id, b.slug, b.name, b.tier, b.deleted_at IS NOT NULL AS deleted,
       COALESCE(a.awarded, 0) AS awarded_count, COALESCE(a.players, 0) AS unique_players,
       a.last_at AS last_awarded_at
FROM ` + schema + `badges b
LEFT JOIN (
    SELECT badge_id, COUNT(*) AS awarded, COUNT(DISTINCT player_id) AS players, MAX(created_at) AS last_at
    FROM ` + schema + `badge_awards
    WHERE tenant_id = ? AND status = 'applied'
    GROUP BY badge_id
) a ON a.badge_id = b.id
WHERE b.tenant_id = ? AND (b.deleted_at IS NULL OR a.badge_id IS NOT NULL)
ORDER BY b.sort_order, b.created_at DESC, b.id DESC`

const perDaySQL = `
SELECT to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day, COUNT(*) AS count
FROM ` + schema + `badge_awards
WHERE tenant_id = ? AND status = 'applied' AND created_at >= ?
GROUP BY 1`

const totalsSQL = `
SELECT COUNT(*) AS total_awarded, COUNT(DISTINCT player_id) AS unique_players
FROM ` + schema + `badge_awards
WHERE tenant_id = ? AND status = 'applied'`

type badgeStatRow struct {
	BadgeID       string
	Slug          string
	Name          string
	Tier          string
	Deleted       bool
	AwardedCount  int64
	UniquePlayers int64
	LastAwardedAt *time.Time
}

func (r *Postgres) AwardStats(ctx context.Context, tenantID string, since time.Time) (app.AwardStats, error) {
	db := r.db.WithContext(ctx)
	var rows []badgeStatRow
	if err := db.Raw(badgeStatsSQL, tenantID, tenantID).Scan(&rows).Error; err != nil {
		return app.AwardStats{}, errs.Wrap(errs.Internal, "badge stats", err)
	}
	out := app.AwardStats{Badges: make([]app.BadgeStat, len(rows)), PerDay: map[string]int64{}}
	for i, row := range rows {
		if row.LastAwardedAt != nil {
			t := row.LastAwardedAt.UTC()
			row.LastAwardedAt = &t
		}
		out.Badges[i] = app.BadgeStat(row)
	}
	var days []struct {
		Day   string
		Count int64
	}
	if err := db.Raw(perDaySQL, tenantID, since).Scan(&days).Error; err != nil {
		return app.AwardStats{}, errs.Wrap(errs.Internal, "badge awards per day", err)
	}
	for _, d := range days {
		out.PerDay[d.Day] = d.Count
	}
	var totals struct {
		TotalAwarded  int64
		UniquePlayers int64
	}
	if err := db.Raw(totalsSQL, tenantID).Scan(&totals).Error; err != nil {
		return app.AwardStats{}, errs.Wrap(errs.Internal, "badge award totals", err)
	}
	out.TotalAwarded, out.UniquePlayers = totals.TotalAwarded, totals.UniquePlayers
	return out, nil
}
