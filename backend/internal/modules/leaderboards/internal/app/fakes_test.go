package app

import (
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/leaderboards/contracts"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/modules/leaderboards/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// ---- repository fake: mirrors the SQL semantics of internal/repo ----

type scoreKey struct {
	board  string
	start  int64
	player string
}

type scoreRow struct {
	tenant    string
	score     int64
	lastEvent time.Time
}

type periodKey struct {
	board string
	start int64
}

type periodRow struct {
	p      domain.Period
	closed bool
}

type statusRow struct {
	deactivated, deleted bool
	at                   time.Time
}

type memberRow struct {
	enrolled bool
	at       time.Time
}

type fakeRepo struct {
	mu        sync.Mutex
	boards    map[string]domain.Leaderboard
	scores    map[scoreKey]scoreRow
	periods   map[periodKey]*periodRow
	applied   map[string]time.Time // event|board
	hidden    map[string]statusRow // tenant|player
	members   map[string]memberRow // program|player
	snapshots map[periodKey][]domain.Standing
	marks     map[string]time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		boards: map[string]domain.Leaderboard{}, scores: map[scoreKey]scoreRow{},
		periods: map[periodKey]*periodRow{}, applied: map[string]time.Time{},
		hidden: map[string]statusRow{}, members: map[string]memberRow{},
		snapshots: map[periodKey][]domain.Standing{}, marks: map[string]time.Time{},
	}
}

func (f *fakeRepo) Create(_ context.Context, _ *gorm.DB, lb domain.Leaderboard) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range f.boards {
		if b.TenantID == lb.TenantID && b.Slug == lb.Slug && b.DeletedAt == nil {
			return domain.ErrSlugTaken
		}
	}
	f.boards[lb.ID] = lb
	return nil
}

func (f *fakeRepo) ByID(_ context.Context, tenantID, id string) (domain.Leaderboard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.boards[id]
	if !ok || b.TenantID != tenantID || b.DeletedAt != nil {
		return domain.Leaderboard{}, domain.ErrNotFound
	}
	return b, nil
}

func (f *fakeRepo) List(_ context.Context, tenantID string, flt ListFilter, beforeAt time.Time, beforeID string, limit int) ([]domain.Leaderboard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Leaderboard
	for _, b := range f.boards {
		if b.TenantID != tenantID || b.DeletedAt != nil {
			continue
		}
		if flt.Type != "" && b.Type != flt.Type {
			continue
		}
		if flt.Active != nil && b.Active != *flt.Active {
			continue
		}
		olderThanCursor := b.CreatedAt.Before(beforeAt) || (b.CreatedAt.Equal(beforeAt) && b.ID < beforeID)
		if !beforeAt.IsZero() && !olderThanCursor {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) Save(_ context.Context, _ *gorm.DB, lb domain.Leaderboard) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.boards[lb.ID]
	if !ok || cur.Version != lb.Version {
		return domain.ErrVersionConflict
	}
	lb.Version++
	f.boards[lb.ID] = lb
	return nil
}

func (f *fakeRepo) SoftDelete(_ context.Context, _ *gorm.DB, lb domain.Leaderboard, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.boards[lb.ID]
	b.DeletedAt = &at
	f.boards[lb.ID] = b
	return nil
}

func (f *fakeRepo) ActiveByType(_ context.Context, tenantID, typ string) ([]domain.Leaderboard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Leaderboard
	for _, b := range f.boards {
		if b.TenantID == tenantID && b.Type == typ && b.Active && b.DeletedAt == nil {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeRepo) ActiveForActivity(_ context.Context, tenantID, eventType string) ([]domain.Leaderboard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Leaderboard
	for _, b := range f.boards {
		if b.TenantID == tenantID && b.Type == contracts.TypeActivity && b.Activity != nil &&
			b.Activity.EventType == eventType && b.Active && b.DeletedAt == nil {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeRepo) ActiveBoards(_ context.Context, afterID string, limit int) ([]domain.Leaderboard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Leaderboard
	for _, b := range f.boards {
		if b.Active && b.DeletedAt == nil && b.ID > afterID {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) isHidden(tenantID, playerID string) bool {
	h := f.hidden[tenantID+"|"+playerID]
	return h.deactivated || h.deleted
}

func (f *fakeRepo) isMember(programID, playerID string) bool {
	return f.members[programID+"|"+playerID].enrolled
}

func (f *fakeRepo) IsHidden(_ context.Context, _ *gorm.DB, tenantID, playerID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isHidden(tenantID, playerID), nil
}

func (f *fakeRepo) IsMember(_ context.Context, _ *gorm.DB, programID, playerID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.isMember(programID, playerID), nil
}

func (f *fakeRepo) ApplyScore(_ context.Context, _ *gorm.DB, c ScoreChange) (ScoreResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ak := c.EventID + "|" + c.Period.LeaderboardID
	if _, done := f.applied[ak]; done {
		return ScoreResult{}, nil
	}
	f.applied[ak] = c.Now
	pk := periodKey{c.Period.LeaderboardID, c.Period.Start.Unix()}
	if _, ok := f.periods[pk]; !ok {
		f.periods[pk] = &periodRow{p: c.Period}
	}
	sk := scoreKey{c.Period.LeaderboardID, c.Period.Start.Unix(), c.PlayerID}
	row, exists := f.scores[sk]
	switch c.Op.Kind {
	case domain.OpIncrement:
		row.score += c.Op.Value
		if c.At.After(row.lastEvent) {
			row.lastEvent = c.At
		}
	case domain.OpSet:
		if exists && !row.lastEvent.Before(c.At) {
			return ScoreResult{Applied: true}, nil
		}
		row.score, row.lastEvent = c.Op.Value, c.At
	}
	row.tenant = c.Period.TenantID
	f.scores[sk] = row
	return ScoreResult{Applied: true, Changed: true, Score: row.score}, nil
}

func (f *fakeRepo) SetPlayerStatus(_ context.Context, _ *gorm.DB, tenantID, playerID string, active bool, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := tenantID + "|" + playerID
	cur, ok := f.hidden[k]
	if ok && cur.at.After(at) {
		return nil
	}
	cur.deactivated, cur.at = !active, at
	f.hidden[k] = cur
	return nil
}

func (f *fakeRepo) MarkPlayerDeleted(_ context.Context, _ *gorm.DB, tenantID, playerID string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := tenantID + "|" + playerID
	cur := f.hidden[k]
	cur.deleted = true
	if at.After(cur.at) {
		cur.at = at
	}
	f.hidden[k] = cur
	return nil
}

func (f *fakeRepo) SetMembership(_ context.Context, _ *gorm.DB, _, programID, playerID string, enrolled bool, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := programID + "|" + playerID
	cur, ok := f.members[k]
	if ok && cur.at.After(at) {
		return nil
	}
	f.members[k] = memberRow{enrolled: enrolled, at: at}
	return nil
}

func (f *fakeRepo) visible(lb domain.Leaderboard, tenant, player string) bool {
	if f.isHidden(tenant, player) {
		return false
	}
	return lb.ProgramID == "" || f.isMember(lb.ProgramID, player)
}

func (f *fakeRepo) PlayerScores(_ context.Context, tenantID, playerID string, since time.Time) ([]PlayerScore, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []PlayerScore
	for k, row := range f.scores {
		if row.tenant != tenantID || k.player != playerID {
			continue
		}
		lb := f.boards[k.board]
		if lb.DeletedAt != nil {
			continue
		}
		p := f.periods[periodKey{k.board, k.start}].p
		if p.HasEnd() && !p.End.After(since) {
			continue
		}
		out = append(out, PlayerScore{Period: p, Score: row.score, Visible: f.visible(lb, tenantID, playerID)})
	}
	return out, nil
}

func (f *fakeRepo) ranked(lb domain.Leaderboard, start time.Time) []domain.Standing {
	var out []domain.Standing
	for k, row := range f.scores {
		if k.board != lb.ID || k.start != start.Unix() || !f.visible(lb, row.tenant, k.player) {
			continue
		}
		out = append(out, domain.Standing{PlayerID: k.player, Score: row.score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].PlayerID < out[j].PlayerID
	})
	domain.AssignRanks(out, 0, 1)
	return out
}

func (f *fakeRepo) RangeByPosition(_ context.Context, lb domain.Leaderboard, start time.Time, offset, limit int64) ([]domain.Standing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.ranked(lb, start)
	if offset >= int64(len(all)) {
		return nil, nil
	}
	return all[offset:min(int64(len(all)), offset+limit)], nil
}

func (f *fakeRepo) PositionOf(_ context.Context, lb domain.Leaderboard, start time.Time, playerID string) (domain.Standing, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.ranked(lb, start) {
		if s.PlayerID == playerID {
			return s, true, nil
		}
	}
	return domain.Standing{}, false, nil
}

func (f *fakeRepo) VisibleScores(_ context.Context, lb domain.Leaderboard, start time.Time) ([]domain.Standing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ranked(lb, start), nil
}

func (f *fakeRepo) Periods(_ context.Context, tenantID, leaderboardID string, openOnly bool) ([]domain.Period, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Period
	for _, p := range f.periods {
		if p.p.TenantID == tenantID && p.p.LeaderboardID == leaderboardID && (!openOnly || !p.closed) {
			out = append(out, p.p)
		}
	}
	return out, nil
}

func (f *fakeRepo) DuePeriods(_ context.Context, before time.Time, limit int) ([]domain.Period, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Period
	for _, p := range f.periods {
		if !p.closed && p.p.HasEnd() && !p.p.End.After(before) && f.boards[p.p.LeaderboardID].DeletedAt == nil {
			out = append(out, p.p)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) ClosePeriod(_ context.Context, _ *gorm.DB, lb domain.Leaderboard, p domain.Period, _ time.Time, snapshotLimit, topN int) (bool, []domain.Standing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pk := periodKey{p.LeaderboardID, p.Start.Unix()}
	row := f.periods[pk]
	if row == nil || row.closed {
		return false, nil, nil
	}
	row.closed = true
	all := f.ranked(lb, p.Start)
	f.snapshots[pk] = all[:min(len(all), snapshotLimit)]
	return true, all[:min(len(all), topN)], nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) ([]domain.Leaderboard, []domain.Period, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var boards []domain.Leaderboard
	var periods []domain.Period
	for id, b := range f.boards {
		if b.TenantID == tenantID {
			boards = append(boards, b)
			delete(f.boards, id)
		}
	}
	for k, p := range f.periods {
		if p.p.TenantID == tenantID {
			periods = append(periods, p.p)
			delete(f.periods, k)
		}
	}
	for k, s := range f.scores {
		if s.tenant == tenantID {
			delete(f.scores, k)
		}
	}
	for k := range f.hidden {
		if strings.HasPrefix(k, tenantID+"|") {
			delete(f.hidden, k)
		}
	}
	return boards, periods, nil
}

func (f *fakeRepo) PruneAppliedEvents(_ context.Context, before time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for k, at := range f.applied {
		if at.Before(before) {
			delete(f.applied, k)
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) LastRun(_ context.Context, job string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.marks[job], nil
}

func (f *fakeRepo) MarkRun(_ context.Context, job string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marks[job] = at
	return nil
}

func (f *fakeRepo) score(board string, start time.Time, player string) (int64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.scores[scoreKey{board, start.Unix(), player}]
	return r.score, ok
}

// ---- rank store fake: an in-memory sorted set per period ----

func pkey(p domain.Period) string { return p.LeaderboardID + "|" + p.Start.UTC().Format(time.RFC3339) }

type fakeRanks struct {
	mu      sync.Mutex
	sets    map[string]map[string]int64
	ready   map[string]bool
	deleted []string
	fail    bool
}

func newFakeRanks() *fakeRanks {
	return &fakeRanks{sets: map[string]map[string]int64{}, ready: map[string]bool{}}
}

var errRedisDown = errs.New(errs.Unavailable, "redis down")

func (r *fakeRanks) Ready(_ context.Context, p domain.Period) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return false, errRedisDown
	}
	return r.ready[pkey(p)], nil
}

func (r *fakeRanks) Increment(_ context.Context, p domain.Period, player string, delta int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errRedisDown
	}
	k := pkey(p)
	if !r.ready[k] {
		return nil
	}
	if r.sets[k] == nil {
		r.sets[k] = map[string]int64{}
	}
	r.sets[k][player] += delta
	return nil
}

func (r *fakeRanks) Set(_ context.Context, p domain.Period, player string, score int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errRedisDown
	}
	k := pkey(p)
	if !r.ready[k] {
		return nil
	}
	if r.sets[k] == nil {
		r.sets[k] = map[string]int64{}
	}
	r.sets[k][player] = score
	return nil
}

func (r *fakeRanks) Remove(_ context.Context, p domain.Period, players ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, pl := range players {
		delete(r.sets[pkey(p)], pl)
	}
	return nil
}

func (r *fakeRanks) ordered(p domain.Period) []domain.Standing {
	var out []domain.Standing
	for pl, s := range r.sets[pkey(p)] {
		out = append(out, domain.Standing{PlayerID: pl, Score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].PlayerID < out[j].PlayerID
	})
	return out
}

func (r *fakeRanks) Range(_ context.Context, p domain.Period, start, stop int64) ([]domain.Standing, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return nil, errRedisDown
	}
	all := r.ordered(p)
	if start >= int64(len(all)) {
		return nil, nil
	}
	return slices.Clone(all[start:min(int64(len(all)), stop+1)]), nil
}

func (r *fakeRanks) Position(_ context.Context, p domain.Period, player string) (int64, int64, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, s := range r.ordered(p) {
		if s.PlayerID == player {
			return int64(i), s.Score, true, nil
		}
	}
	return 0, 0, false, nil
}

func (r *fakeRanks) CountAbove(_ context.Context, p domain.Period, score int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, s := range r.sets[pkey(p)] {
		if s > score {
			n++
		}
	}
	return n, nil
}

func (r *fakeRanks) Replace(_ context.Context, p domain.Period, rows []domain.Standing, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errRedisDown
	}
	set := map[string]int64{}
	for _, row := range rows {
		set[row.PlayerID] = row.Score
	}
	r.sets[pkey(p)] = set
	r.ready[pkey(p)] = true
	return nil
}

func (r *fakeRanks) Delete(_ context.Context, periods ...domain.Period) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range periods {
		k := pkey(p)
		delete(r.sets, k)
		delete(r.ready, k)
		r.deleted = append(r.deleted, k)
	}
	return nil
}

func (r *fakeRanks) TryLock(context.Context, domain.Period, time.Duration) (bool, error) {
	return !r.fail, nil
}

func (r *fakeRanks) snapshot(p domain.Period) []domain.Standing {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ordered(p)
}

// ---- outbox, authz, players ----

type recorded struct {
	topic   string
	payload any
}

type fakeOutbox struct{ published []recorded }

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.published = append(f.published, recorded{topic, payload})
	return nil
}

type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

type fakePlayers struct {
	players map[string]ports.PlayerSnapshot
}

func (f fakePlayers) IDByExternalID(_ context.Context, _ string, externalID string) (string, bool, error) {
	for id, p := range f.players {
		if p.ExternalID == externalID {
			return id, true, nil
		}
	}
	return "", false, nil
}

func (f fakePlayers) ByIDs(_ context.Context, _ string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, id := range ids {
		if p, ok := f.players[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}
