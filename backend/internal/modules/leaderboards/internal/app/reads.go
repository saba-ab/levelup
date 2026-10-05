package app

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/shared/errs"
)

const (
	defaultAround = 2
	maxAround     = 10
	warmLockTTL   = 30 * time.Second
	warmTimeout   = 30 * time.Second
)

// Entry is a hydrated ranking row.
type Entry struct {
	domain.Standing
	ExternalID  string
	DisplayName string
}

// Page is one page of a ranking.
type Page struct {
	Leaderboard domain.Leaderboard
	Period      domain.Period
	Entries     []Entry
	NextCursor  string
}

// PlayerRank is a player's standing with its neighbours.
type PlayerRank struct {
	Leaderboard domain.Leaderboard
	Period      domain.Period
	Entry       Entry
	Neighbours  []Entry
}

// ResolvePeriod maps ?period= to the board's period: "" or "current" is the
// period containing now; an RFC 3339 time selects the period containing it.
func ResolvePeriod(lb domain.Leaderboard, param string, now time.Time) (domain.Period, error) {
	switch strings.TrimSpace(param) {
	case "", "current":
		return domain.PeriodOf(lb, now), nil
	}
	t, err := time.Parse(time.RFC3339, param)
	if err != nil {
		return domain.Period{}, domain.ErrInvalidPeriod
	}
	return domain.PeriodOf(lb, t), nil
}

// Position cursors: rankings are positional by nature and the redis path
// addresses positions in O(log n). The cursor is opaque to clients.
func encodePosCursor(pos int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte("p" + strconv.FormatInt(pos, 10)))
}

func decodePosCursor(c string) (int64, error) {
	if c == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil || len(raw) < 2 || raw[0] != 'p' {
		return 0, errs.New(errs.Invalid, "malformed cursor")
	}
	pos, err := strconv.ParseInt(string(raw[1:]), 10, 64)
	if err != nil || pos < 0 {
		return 0, errs.New(errs.Invalid, "malformed cursor")
	}
	return pos, nil
}

// Entries returns a page of the board's ranking for a period. Redis serves
// it when the period's set is loaded; otherwise Postgres does and the set is
// warmed in the background. The board's max_entries caps the visible list.
func (s *Service) Entries(ctx context.Context, leaderboardID, periodParam, cursor string, limit int) (Page, error) {
	_, lb, err := s.load(ctx, contracts.PermView, leaderboardID)
	if err != nil {
		return Page{}, err
	}
	period, err := ResolvePeriod(lb, periodParam, s.clock.Now())
	if err != nil {
		return Page{}, err
	}
	start, err := decodePosCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	page := Page{Leaderboard: lb, Period: period, Entries: []Entry{}}
	remaining := int64(lb.MaxEntries) - start
	if remaining <= 0 {
		return page, nil
	}
	n := min(int64(limit), remaining)

	// One extra row tells whether another page exists.
	rows, err := s.rangeRows(ctx, lb, period, start, n+1)
	if err != nil {
		return Page{}, err
	}
	if int64(len(rows)) > n {
		rows = rows[:n]
		if start+n < int64(lb.MaxEntries) {
			page.NextCursor = encodePosCursor(start + n)
		}
	}
	page.Entries, err = s.hydrate(ctx, lb.TenantID, rows)
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

// rangeRows returns ranked standings at positions [start, start+count).
func (s *Service) rangeRows(ctx context.Context, lb domain.Leaderboard, p domain.Period, start, count int64) ([]domain.Standing, error) {
	if s.ready(ctx, p) {
		rows, err := s.redisRange(ctx, p, start, count)
		if err == nil {
			return rows, nil
		}
		s.log.Warn("leaderboard read model unavailable; serving from postgres", zap.Error(err))
	} else {
		s.warm(ctx, lb, p)
	}
	return s.repo.RangeByPosition(ctx, lb, p.Start, start, count)
}

func (s *Service) redisRange(ctx context.Context, p domain.Period, start, count int64) ([]domain.Standing, error) {
	rows, err := s.ranks.Range(ctx, p, start, start+count-1)
	if err != nil || len(rows) == 0 {
		return rows, err
	}
	above, err := s.ranks.CountAbove(ctx, p, rows[0].Score)
	if err != nil {
		return nil, err
	}
	domain.AssignRanks(rows, start, above+1)
	return rows, nil
}

func (s *Service) ready(ctx context.Context, p domain.Period) bool {
	ok, err := s.ranks.Ready(ctx, p)
	if err != nil {
		s.log.Warn("leaderboard read model unavailable", zap.Error(err))
		return false
	}
	return ok
}

// warm loads the period into redis off the request path. A lock keeps a
// stampede of cache misses from loading the same period N times.
func (s *Service) warm(ctx context.Context, lb domain.Leaderboard, p domain.Period) {
	expireAt := s.expiry(p)
	if !expireAt.After(s.clock.Now()) {
		return // the key would be dead on arrival: serve Postgres only
	}
	locked, err := s.ranks.TryLock(ctx, p, warmLockTTL)
	if err != nil || !locked {
		return
	}
	bg := context.WithoutCancel(ctx)
	s.async(func() {
		wctx, cancel := context.WithTimeout(bg, warmTimeout)
		defer cancel()
		if err := s.loadPeriod(wctx, lb, p); err != nil {
			s.log.Warn("warm leaderboard read model", zap.String("leaderboard_id", lb.ID), zap.Error(err))
		}
	})
}

// loadPeriod replaces the period's redis set with Postgres' visible rows.
func (s *Service) loadPeriod(ctx context.Context, lb domain.Leaderboard, p domain.Period) error {
	rows, err := s.repo.VisibleScores(ctx, lb, p.Start)
	if err != nil {
		return err
	}
	return s.ranks.Replace(ctx, p, rows, s.expiry(p))
}

func (s *Service) hydrate(ctx context.Context, tenantID string, rows []domain.Standing) ([]Entry, error) {
	out := make([]Entry, len(rows))
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.PlayerID
	}
	snaps, err := s.players.ByIDs(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	for i, r := range rows {
		snap := snaps[r.PlayerID]
		out[i] = Entry{Standing: r, ExternalID: snap.ExternalID, DisplayName: snap.DisplayName}
	}
	return out, nil
}

// PlayerStanding returns a player's rank and score plus up to `around`
// neighbours on each side.
func (s *Service) PlayerStanding(ctx context.Context, leaderboardID, playerID, periodParam string, around int) (PlayerRank, error) {
	_, lb, err := s.load(ctx, contracts.PermView, leaderboardID)
	if err != nil {
		return PlayerRank{}, err
	}
	period, err := ResolvePeriod(lb, periodParam, s.clock.Now())
	if err != nil {
		return PlayerRank{}, err
	}
	if around < 0 {
		around = defaultAround
	}
	around = min(around, maxAround)

	snaps, err := s.players.ByIDs(ctx, lb.TenantID, []string{playerID})
	if err != nil {
		return PlayerRank{}, err
	}
	if _, ok := snaps[playerID]; !ok {
		return PlayerRank{}, domain.ErrPlayerNotFound
	}

	me, found, err := s.position(ctx, lb, period, playerID)
	if err != nil {
		return PlayerRank{}, err
	}
	if !found {
		return PlayerRank{}, domain.ErrPlayerNotRanked
	}
	from := max(0, me.Position-int64(around))
	window, err := s.rangeRows(ctx, lb, period, from, me.Position-from+int64(around)+1)
	if err != nil {
		return PlayerRank{}, err
	}
	entries, err := s.hydrate(ctx, lb.TenantID, window)
	if err != nil {
		return PlayerRank{}, err
	}
	out := PlayerRank{Leaderboard: lb, Period: period, Neighbours: []Entry{}}
	for _, e := range entries {
		if e.PlayerID == playerID {
			out.Entry = e
			continue
		}
		out.Neighbours = append(out.Neighbours, e)
	}
	if out.Entry.PlayerID == "" {
		// The window moved between the two reads; report the lookup itself.
		snap := snaps[playerID]
		out.Entry = Entry{Standing: me, ExternalID: snap.ExternalID, DisplayName: snap.DisplayName}
	}
	return out, nil
}

func (s *Service) position(ctx context.Context, lb domain.Leaderboard, p domain.Period, playerID string) (domain.Standing, bool, error) {
	if s.ready(ctx, p) {
		st, found, err := s.redisPosition(ctx, p, playerID)
		if err == nil {
			return st, found, nil
		}
		s.log.Warn("leaderboard read model unavailable; serving from postgres", zap.Error(err))
	} else {
		s.warm(ctx, lb, p)
	}
	return s.repo.PositionOf(ctx, lb, p.Start, playerID)
}

func (s *Service) redisPosition(ctx context.Context, p domain.Period, playerID string) (domain.Standing, bool, error) {
	pos, score, found, err := s.ranks.Position(ctx, p, playerID)
	if err != nil || !found {
		return domain.Standing{}, false, err
	}
	above, err := s.ranks.CountAbove(ctx, p, score)
	if err != nil {
		return domain.Standing{}, false, err
	}
	return domain.Standing{PlayerID: playerID, Score: score, Rank: above + 1, Position: pos}, true, nil
}

// RebuildResult reports a rebuild.
type RebuildResult struct {
	Periods int
	Entries int
}

// Rebuild (admin) reloads every open period of a board from Postgres.
func (s *Service) Rebuild(ctx context.Context, leaderboardID string) (RebuildResult, error) {
	_, lb, err := s.load(ctx, contracts.PermRebuild, leaderboardID)
	if err != nil {
		return RebuildResult{}, err
	}
	return s.rebuildBoard(ctx, lb)
}

func (s *Service) rebuildBoard(ctx context.Context, lb domain.Leaderboard) (RebuildResult, error) {
	periods, err := s.repo.Periods(ctx, lb.TenantID, lb.ID, true)
	if err != nil {
		return RebuildResult{}, err
	}
	current := domain.PeriodOf(lb, s.clock.Now())
	seen := false
	for _, p := range periods {
		if p.Start.Equal(current.Start) {
			seen = true
		}
	}
	if !seen {
		periods = append(periods, current)
	}
	var res RebuildResult
	for _, p := range periods {
		expireAt := s.expiry(p)
		if !expireAt.After(s.clock.Now()) {
			continue
		}
		rows, err := s.repo.VisibleScores(ctx, lb, p.Start)
		if err != nil {
			return res, err
		}
		if err := s.ranks.Replace(ctx, p, rows, expireAt); err != nil {
			return res, errs.Wrap(errs.Unavailable, "rebuild leaderboard read model", err)
		}
		res.Periods++
		res.Entries += len(rows)
	}
	return res, nil
}
