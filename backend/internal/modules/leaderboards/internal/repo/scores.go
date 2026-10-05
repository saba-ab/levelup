package repo

import (
	"context"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/leaderboards/internal/app"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/shared/errs"
)

// visibleExpr decides whether score row s of board l is shown: the player
// is not hidden, and program boards show enrolled players only. Aliases: s =
// leaderboard_scores, l = leaderboards.
const visibleExpr = `NOT EXISTS (SELECT 1 FROM ` + tHidden + ` h
	WHERE h.tenant_id = s.tenant_id AND h.player_id = s.player_id AND (h.deactivated OR h.deleted))
AND (l.program_id IS NULL OR EXISTS (SELECT 1 FROM ` + tMembers + ` m
	WHERE m.program_id = l.program_id AND m.player_id = s.player_id AND m.enrolled))`

// rankedCTE ranks the visible rows of one board period: competition rank
// over score, list position over (score desc, player_id asc).
const rankedCTE = `WITH ranked AS (
	SELECT s.player_id::text AS player_id, s.score,
		RANK() OVER (ORDER BY s.score DESC) AS rank,
		ROW_NUMBER() OVER (ORDER BY s.score DESC, s.player_id ASC) - 1 AS position
	FROM ` + tScores + ` s JOIN ` + tBoards + ` l ON l.id = s.leaderboard_id
	WHERE s.leaderboard_id = ?::uuid AND s.period_start = ?::timestamptz AND ` + visibleExpr + `
)`

type standingRow struct {
	PlayerID string
	Score    int64
	Rank     int64
	Position int64
}

func (r standingRow) toDomain() domain.Standing {
	return domain.Standing{PlayerID: r.PlayerID, Score: r.Score, Rank: r.Rank, Position: r.Position}
}

func toStandings(rows []standingRow) []domain.Standing {
	out := make([]domain.Standing, len(rows))
	for i, r := range rows {
		out[i] = r.toDomain()
	}
	return out
}

func (r *Postgres) IsHidden(ctx context.Context, tx *gorm.DB, tenantID, playerID string) (bool, error) {
	if !validUUID(playerID) {
		return false, errs.New(errs.Invalid, "player_id is not a uuid")
	}
	var n int64
	err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM `+tHidden+`
		WHERE tenant_id = ? AND player_id = ? AND (deactivated OR deleted)`, tenantID, playerID).Scan(&n).Error
	if err != nil {
		return false, errs.Wrap(errs.Internal, "load hidden player", err)
	}
	return n > 0, nil
}

func (r *Postgres) IsMember(ctx context.Context, tx *gorm.DB, programID, playerID string) (bool, error) {
	var n int64
	err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM `+tMembers+`
		WHERE program_id = ? AND player_id = ? AND enrolled`, programID, playerID).Scan(&n).Error
	if err != nil {
		return false, errs.Wrap(errs.Internal, "load program membership", err)
	}
	return n > 0, nil
}

// ApplyScore records the (event, board) pair, then upserts the score. A
// redelivered event hits the applied_events primary key and changes nothing.
func (r *Postgres) ApplyScore(ctx context.Context, tx *gorm.DB, c app.ScoreChange) (app.ScoreResult, error) {
	db := tx.WithContext(ctx)
	res := db.Exec(`INSERT INTO `+tApplied+` (event_id, leaderboard_id, applied_at)
		VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, c.EventID, c.Period.LeaderboardID, c.Now)
	if res.Error != nil {
		return app.ScoreResult{}, errs.Wrap(errs.Internal, "record applied event", res.Error)
	}
	if res.RowsAffected == 0 {
		return app.ScoreResult{}, nil
	}

	var end *time.Time
	if c.Period.HasEnd() {
		e := c.Period.End
		end = &e
	}
	if err := db.Exec(`INSERT INTO `+tPeriods+` (leaderboard_id, period_start, tenant_id, period_end)
		VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		c.Period.LeaderboardID, c.Period.Start, c.Period.TenantID, end).Error; err != nil {
		return app.ScoreResult{}, errs.Wrap(errs.Internal, "record leaderboard period", err)
	}

	var q string
	switch c.Op.Kind {
	case domain.OpIncrement:
		q = `INSERT INTO ` + tScores + ` AS cur
			(leaderboard_id, period_start, player_id, tenant_id, score, last_event_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (leaderboard_id, period_start, player_id) DO UPDATE SET
				score = cur.score + EXCLUDED.score,
				last_event_at = GREATEST(cur.last_event_at, EXCLUDED.last_event_at),
				updated_at = EXCLUDED.updated_at
			RETURNING score`
	case domain.OpSet:
		q = `INSERT INTO ` + tScores + ` AS cur
			(leaderboard_id, period_start, player_id, tenant_id, score, last_event_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (leaderboard_id, period_start, player_id) DO UPDATE SET
				score = EXCLUDED.score,
				last_event_at = EXCLUDED.last_event_at,
				updated_at = EXCLUDED.updated_at
			WHERE cur.last_event_at IS NULL OR cur.last_event_at < EXCLUDED.last_event_at
			RETURNING score`
	default:
		return app.ScoreResult{}, errs.New(errs.Internal, "unknown score op")
	}
	var scores []int64
	if err := db.Raw(q, c.Period.LeaderboardID, c.Period.Start, c.PlayerID, c.Period.TenantID,
		c.Op.Value, c.At, c.Now).Scan(&scores).Error; err != nil {
		return app.ScoreResult{}, errs.Wrap(errs.Internal, "upsert leaderboard score", err)
	}
	if len(scores) == 0 {
		return app.ScoreResult{Applied: true}, nil // an older balance lost to a newer one
	}
	return app.ScoreResult{Applied: true, Changed: true, Score: scores[0]}, nil
}

// SetPlayerStatus is last-writer-wins on the event time.
func (r *Postgres) SetPlayerStatus(ctx context.Context, tx *gorm.DB, tenantID, playerID string, active bool, at time.Time) error {
	err := tx.WithContext(ctx).Exec(`INSERT INTO `+tHidden+` AS cur (tenant_id, player_id, deactivated, deleted, changed_at)
		VALUES (?, ?, ?, false, ?)
		ON CONFLICT (tenant_id, player_id) DO UPDATE SET
			deactivated = EXCLUDED.deactivated, changed_at = EXCLUDED.changed_at
		WHERE cur.changed_at <= EXCLUDED.changed_at`, tenantID, playerID, !active, at).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "project player status", err)
	}
	return nil
}

// MarkPlayerDeleted is terminal: no later event un-deletes a player.
func (r *Postgres) MarkPlayerDeleted(ctx context.Context, tx *gorm.DB, tenantID, playerID string, at time.Time) error {
	err := tx.WithContext(ctx).Exec(`INSERT INTO `+tHidden+` AS cur (tenant_id, player_id, deactivated, deleted, changed_at)
		VALUES (?, ?, false, true, ?)
		ON CONFLICT (tenant_id, player_id) DO UPDATE SET
			deleted = true, changed_at = GREATEST(cur.changed_at, EXCLUDED.changed_at)`, tenantID, playerID, at).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "project player deletion", err)
	}
	return nil
}

// SetMembership is last-writer-wins on the event time.
func (r *Postgres) SetMembership(ctx context.Context, tx *gorm.DB, tenantID, programID, playerID string, enrolled bool, at time.Time) error {
	err := tx.WithContext(ctx).Exec(`INSERT INTO `+tMembers+` AS cur (program_id, player_id, tenant_id, enrolled, changed_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (program_id, player_id) DO UPDATE SET
			enrolled = EXCLUDED.enrolled, changed_at = EXCLUDED.changed_at
		WHERE cur.changed_at <= EXCLUDED.changed_at`, programID, playerID, tenantID, enrolled, at).Error
	if err != nil {
		return errs.Wrap(errs.Internal, "project program membership", err)
	}
	return nil
}

type playerScoreRow struct {
	LeaderboardID string
	TenantID      string
	PeriodStart   time.Time
	PeriodEnd     *time.Time
	Score         int64
	Visible       bool
}

func (r *Postgres) PlayerScores(ctx context.Context, tenantID, playerID string, since time.Time) ([]app.PlayerScore, error) {
	var rows []playerScoreRow
	err := r.db.WithContext(ctx).Raw(`SELECT s.leaderboard_id::text AS leaderboard_id, s.tenant_id::text AS tenant_id,
			s.period_start, p.period_end, s.score, (`+visibleExpr+`) AS visible
		FROM `+tScores+` s
		JOIN `+tPeriods+` p ON p.leaderboard_id = s.leaderboard_id AND p.period_start = s.period_start
		JOIN `+tBoards+` l ON l.id = s.leaderboard_id
		WHERE s.tenant_id = ? AND s.player_id = ? AND l.deleted_at IS NULL
		  AND (p.period_end IS NULL OR p.period_end > ?)`, tenantID, playerID, since).Scan(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load player scores", err)
	}
	out := make([]app.PlayerScore, len(rows))
	for i, row := range rows {
		p := domain.Period{LeaderboardID: row.LeaderboardID, TenantID: row.TenantID, Start: row.PeriodStart.UTC()}
		if row.PeriodEnd != nil {
			p.End = row.PeriodEnd.UTC()
		}
		out[i] = app.PlayerScore{Period: p, Score: row.Score, Visible: row.Visible}
	}
	return out, nil
}

func (r *Postgres) RangeByPosition(ctx context.Context, lb domain.Leaderboard, periodStart time.Time, offset, limit int64) ([]domain.Standing, error) {
	var rows []standingRow
	err := r.db.WithContext(ctx).Raw(rankedCTE+`
		SELECT player_id, score, rank, position FROM ranked
		WHERE position >= ? ORDER BY position LIMIT ?`, lb.ID, periodStart, offset, limit).Scan(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "rank leaderboard", err)
	}
	return toStandings(rows), nil
}

func (r *Postgres) PositionOf(ctx context.Context, lb domain.Leaderboard, periodStart time.Time, playerID string) (domain.Standing, bool, error) {
	if !validUUID(playerID) {
		return domain.Standing{}, false, nil
	}
	var rows []standingRow
	err := r.db.WithContext(ctx).Raw(rankedCTE+`
		SELECT player_id, score, rank, position FROM ranked WHERE player_id = ?`,
		lb.ID, periodStart, playerID).Scan(&rows).Error
	if err != nil {
		return domain.Standing{}, false, errs.Wrap(errs.Internal, "rank player", err)
	}
	if len(rows) == 0 {
		return domain.Standing{}, false, nil
	}
	return rows[0].toDomain(), true, nil
}

func (r *Postgres) VisibleScores(ctx context.Context, lb domain.Leaderboard, periodStart time.Time) ([]domain.Standing, error) {
	var rows []standingRow
	err := r.db.WithContext(ctx).Raw(`SELECT s.player_id::text AS player_id, s.score
		FROM `+tScores+` s JOIN `+tBoards+` l ON l.id = s.leaderboard_id
		WHERE s.leaderboard_id = ? AND s.period_start = ? AND `+visibleExpr+`
		ORDER BY s.score DESC, s.player_id ASC`, lb.ID, periodStart).Scan(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load leaderboard scores", err)
	}
	out := toStandings(rows)
	domain.AssignRanks(out, 0, 1)
	return out, nil
}

type periodRow struct {
	LeaderboardID string
	TenantID      string
	PeriodStart   time.Time
	PeriodEnd     *time.Time
}

func (p periodRow) toDomain() domain.Period {
	out := domain.Period{LeaderboardID: p.LeaderboardID, TenantID: p.TenantID, Start: p.PeriodStart.UTC()}
	if p.PeriodEnd != nil {
		out.End = p.PeriodEnd.UTC()
	}
	return out
}

func toPeriods(rows []periodRow) []domain.Period {
	out := make([]domain.Period, len(rows))
	for i, p := range rows {
		out[i] = p.toDomain()
	}
	return out
}

const periodCols = `p.leaderboard_id::text AS leaderboard_id, p.tenant_id::text AS tenant_id, p.period_start, p.period_end`

func (r *Postgres) Periods(ctx context.Context, tenantID, leaderboardID string, openOnly bool) ([]domain.Period, error) {
	q := `SELECT ` + periodCols + ` FROM ` + tPeriods + ` p WHERE p.tenant_id = ? AND p.leaderboard_id = ?`
	if openOnly {
		q += ` AND p.closed_at IS NULL`
	}
	var rows []periodRow
	if err := r.db.WithContext(ctx).Raw(q+` ORDER BY p.period_start`, tenantID, leaderboardID).Scan(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list leaderboard periods", err)
	}
	return toPeriods(rows), nil
}

func (r *Postgres) DuePeriods(ctx context.Context, before time.Time, limit int) ([]domain.Period, error) {
	var rows []periodRow
	err := r.db.WithContext(ctx).Raw(`SELECT `+periodCols+`
		FROM `+tPeriods+` p JOIN `+tBoards+` l ON l.id = p.leaderboard_id
		WHERE p.closed_at IS NULL AND p.period_end IS NOT NULL AND p.period_end <= ? AND l.deleted_at IS NULL
		ORDER BY p.period_end, p.leaderboard_id LIMIT ?`, before, limit).Scan(&rows).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "list due leaderboard periods", err)
	}
	return toPeriods(rows), nil
}

// ClosePeriod flips closed_at under the row lock UPDATE takes, so two
// concurrent rollovers serialize and only the first writes the snapshot.
func (r *Postgres) ClosePeriod(ctx context.Context, tx *gorm.DB, lb domain.Leaderboard, p domain.Period, at time.Time, snapshotLimit, topN int) (bool, []domain.Standing, error) {
	db := tx.WithContext(ctx)
	res := db.Exec(`UPDATE `+tPeriods+` SET closed_at = ?
		WHERE leaderboard_id = ? AND period_start = ? AND closed_at IS NULL`, at, p.LeaderboardID, p.Start)
	if res.Error != nil {
		return false, nil, errs.Wrap(errs.Internal, "close leaderboard period", res.Error)
	}
	if res.RowsAffected == 0 {
		return false, nil, nil
	}
	if err := db.Exec(rankedCTE+`
		INSERT INTO `+tSnapshots+` (leaderboard_id, period_start, player_id, tenant_id, rank, score, closed_at)
		SELECT ?::uuid, ?::timestamptz, player_id::uuid, ?::uuid, rank, score, ?::timestamptz FROM ranked WHERE position < ?
		ON CONFLICT DO NOTHING`,
		lb.ID, p.Start, lb.ID, p.Start, lb.TenantID, at, snapshotLimit).Error; err != nil {
		return false, nil, errs.Wrap(errs.Internal, "snapshot leaderboard period", err)
	}
	var rows []standingRow
	if err := db.Raw(`SELECT player_id::text AS player_id, score, rank FROM `+tSnapshots+`
		WHERE leaderboard_id = ? AND period_start = ? ORDER BY rank, player_id LIMIT ?`,
		lb.ID, p.Start, topN).Scan(&rows).Error; err != nil {
		return false, nil, errs.Wrap(errs.Internal, "load leaderboard snapshot", err)
	}
	return true, toStandings(rows), nil
}

func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) ([]domain.Leaderboard, []domain.Period, error) {
	if !validUUID(tenantID) {
		return nil, nil, errs.New(errs.Invalid, "tenant_id is not a uuid")
	}
	db := tx.WithContext(ctx)
	var boards []leaderboard
	if err := db.Where("tenant_id = ?", tenantID).Find(&boards).Error; err != nil {
		return nil, nil, errs.Wrap(errs.Internal, "load tenant leaderboards", err)
	}
	var periods []periodRow
	if err := db.Raw(`SELECT `+periodCols+` FROM `+tPeriods+` p WHERE p.tenant_id = ?`, tenantID).Scan(&periods).Error; err != nil {
		return nil, nil, errs.Wrap(errs.Internal, "load tenant leaderboard periods", err)
	}
	stmts := []string{
		`DELETE FROM ` + tApplied + ` WHERE leaderboard_id IN (SELECT id FROM ` + tBoards + ` WHERE tenant_id = ?)`,
		`DELETE FROM ` + tSnapshots + ` WHERE tenant_id = ?`,
		`DELETE FROM ` + tScores + ` WHERE tenant_id = ?`,
		`DELETE FROM ` + tPeriods + ` WHERE tenant_id = ?`,
		`DELETE FROM ` + tMembers + ` WHERE tenant_id = ?`,
		`DELETE FROM ` + tHidden + ` WHERE tenant_id = ?`,
		`DELETE FROM ` + tBoards + ` WHERE tenant_id = ?`,
	}
	for _, q := range stmts {
		if err := db.Exec(q, tenantID).Error; err != nil {
			return nil, nil, errs.Wrap(errs.Internal, "purge tenant leaderboards", err)
		}
	}
	return toDomains(boards), toPeriods(periods), nil
}

const pruneBatch = 10_000

func (r *Postgres) PruneAppliedEvents(ctx context.Context, before time.Time) (int64, error) {
	var total int64
	for {
		res := r.db.WithContext(ctx).Exec(`DELETE FROM `+tApplied+` WHERE ctid IN (
			SELECT ctid FROM `+tApplied+` WHERE applied_at < ? LIMIT ?)`, before, pruneBatch)
		if res.Error != nil {
			return total, errs.Wrap(errs.Internal, "prune applied events", res.Error)
		}
		total += res.RowsAffected
		if res.RowsAffected < pruneBatch {
			return total, nil
		}
	}
}
