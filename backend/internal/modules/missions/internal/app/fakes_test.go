package app

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/missions/internal/domain"
	"levelup/internal/modules/missions/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// fakeRepo is an in-memory Repository with the same uniqueness rules as
// the SQL schema: (tenant, slug) among live missions, one open attempt per
// (tenant, mission, player, period), (tenant, idempotency_key) on events.
type fakeRepo struct {
	missions map[string]domain.Mission
	attempts map[string]domain.Attempt
	events   map[string]domain.ProgressEvent // by id
	markers  map[string]time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		missions: map[string]domain.Mission{},
		attempts: map[string]domain.Attempt{},
		events:   map[string]domain.ProgressEvent{},
		markers:  map[string]time.Time{},
	}
}

func (f *fakeRepo) CreateMission(_ context.Context, _ *gorm.DB, m domain.Mission) error {
	for _, x := range f.missions {
		if x.TenantID == m.TenantID && x.Slug == m.Slug && x.DeletedAt == nil {
			return domain.ErrSlugTaken
		}
	}
	f.missions[m.ID] = m
	return nil
}

func (f *fakeRepo) MissionByID(_ context.Context, tenantID, id string) (domain.Mission, error) {
	m, ok := f.missions[id]
	if !ok || m.TenantID != tenantID || m.DeletedAt != nil {
		return domain.Mission{}, domain.ErrMissionNotFound
	}
	return m, nil
}

func (f *fakeRepo) MissionInTx(ctx context.Context, _ *gorm.DB, tenantID, id string, _ bool) (domain.Mission, error) {
	return f.MissionByID(ctx, tenantID, id)
}

func (f *fakeRepo) SaveMission(_ context.Context, _ *gorm.DB, m domain.Mission) error {
	cur, ok := f.missions[m.ID]
	if !ok || cur.Version != m.Version {
		return domain.ErrVersionConflict
	}
	for _, x := range f.missions {
		if x.ID != m.ID && x.TenantID == m.TenantID && x.Slug == m.Slug && x.DeletedAt == nil && m.DeletedAt == nil {
			return domain.ErrSlugTaken
		}
	}
	m.Version++
	f.missions[m.ID] = m
	return nil
}

func (f *fakeRepo) ListMissions(_ context.Context, tenantID string, flt MissionFilter, after Cursor, limit int) ([]domain.Mission, error) {
	var out []domain.Mission
	for _, m := range f.missions {
		if m.TenantID != tenantID || m.DeletedAt != nil ||
			(flt.Status != "" && m.Status != flt.Status) || (flt.Type != "" && m.Type != flt.Type) {
			continue
		}
		if after.ID != "" && !before(m.CreatedAt, m.ID, after) {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return !before(out[i].CreatedAt, out[i].ID, Cursor{out[j].CreatedAt, out[j].ID}) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) MissionsByIDs(_ context.Context, tenantID string, ids []string) (map[string]domain.Mission, error) {
	out := map[string]domain.Mission{}
	for _, id := range ids {
		if m, ok := f.missions[id]; ok && m.TenantID == tenantID {
			out[id] = m
		}
	}
	return out, nil
}

func (f *fakeRepo) OpenAttemptForUpdate(_ context.Context, _ *gorm.DB, tenantID, missionID, playerID, periodKey string) (domain.Attempt, bool, error) {
	for _, a := range f.attempts {
		if a.TenantID == tenantID && a.MissionID == missionID && a.PlayerID == playerID &&
			a.PeriodKey == periodKey && a.Status == contracts.AttemptInProgress {
			return a, true, nil
		}
	}
	return domain.Attempt{}, false, nil
}

func (f *fakeRepo) LatestAttempt(_ context.Context, _ *gorm.DB, tenantID, missionID, playerID, periodKey string) (domain.Attempt, bool, error) {
	var best domain.Attempt
	found := false
	for _, a := range f.attempts {
		if a.TenantID == tenantID && a.MissionID == missionID && a.PlayerID == playerID && a.PeriodKey == periodKey {
			if !found || a.CreatedAt.After(best.CreatedAt) || (a.CreatedAt.Equal(best.CreatedAt) && a.ID > best.ID) {
				best, found = a, true
			}
		}
	}
	return best, found, nil
}

func (f *fakeRepo) AttemptCounts(_ context.Context, _ *gorm.DB, tenantID, missionID, playerID, periodKey string) (int, bool, error) {
	total, inPeriod := 0, false
	for _, a := range f.attempts {
		if a.TenantID == tenantID && a.MissionID == missionID && a.PlayerID == playerID && a.Status == contracts.AttemptCompleted {
			total++
			if a.PeriodKey == periodKey {
				inPeriod = true
			}
		}
	}
	return total, inPeriod, nil
}

func (f *fakeRepo) InsertAttempt(ctx context.Context, tx *gorm.DB, a domain.Attempt) (bool, error) {
	if _, open, _ := f.OpenAttemptForUpdate(ctx, tx, a.TenantID, a.MissionID, a.PlayerID, a.PeriodKey); open {
		return false, nil
	}
	f.attempts[a.ID] = a
	return true, nil
}

func (f *fakeRepo) SaveAttempt(_ context.Context, _ *gorm.DB, a domain.Attempt) error {
	cur, ok := f.attempts[a.ID]
	if !ok || cur.Version != a.Version {
		return domain.ErrVersionConflict
	}
	a.Version++
	f.attempts[a.ID] = a
	return nil
}

func (f *fakeRepo) AttemptByID(_ context.Context, tenantID, id string) (domain.Attempt, error) {
	a, ok := f.attempts[id]
	if !ok || a.TenantID != tenantID {
		return domain.Attempt{}, domain.ErrAttemptNotFound
	}
	return a, nil
}

func (f *fakeRepo) ListAttempts(_ context.Context, tenantID string, flt AttemptFilter, after Cursor, limit int) ([]domain.Attempt, error) {
	var out []domain.Attempt
	for _, a := range f.attempts {
		if a.TenantID != tenantID || (flt.MissionID != "" && a.MissionID != flt.MissionID) ||
			(flt.PlayerID != "" && a.PlayerID != flt.PlayerID) || (flt.Status != "" && a.Status != flt.Status) {
			continue
		}
		if after.ID != "" && !before(a.CreatedAt, a.ID, after) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return !before(out[i].CreatedAt, out[i].ID, Cursor{out[j].CreatedAt, out[j].ID}) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) CompletedCounts(_ context.Context, tenantID string, playerIDs []string) (map[string]int, error) {
	want := map[string]bool{}
	for _, p := range playerIDs {
		want[p] = true
	}
	out := map[string]int{}
	for _, a := range f.attempts {
		if a.TenantID == tenantID && want[a.PlayerID] && a.Status == contracts.AttemptCompleted {
			out[a.PlayerID]++
		}
	}
	return out, nil
}

func (f *fakeRepo) InsertProgressEvent(_ context.Context, _ *gorm.DB, e domain.ProgressEvent) (bool, error) {
	for _, x := range f.events {
		if x.TenantID == e.TenantID && x.IdempotencyKey == e.IdempotencyKey {
			return false, nil
		}
	}
	f.events[e.ID] = e
	return true, nil
}

func (f *fakeRepo) FinishProgressEvent(_ context.Context, _ *gorm.DB, e domain.ProgressEvent) error {
	if _, ok := f.events[e.ID]; !ok {
		return errs.New(errs.Internal, "no such event")
	}
	f.events[e.ID] = e
	return nil
}

func (f *fakeRepo) ProgressEventByKey(_ context.Context, tenantID, key string) (domain.ProgressEvent, bool, error) {
	for _, x := range f.events {
		if x.TenantID == tenantID && x.IdempotencyKey == key {
			return x, true, nil
		}
	}
	return domain.ProgressEvent{}, false, nil
}

func (f *fakeRepo) DueMissions(_ context.Context, _ *gorm.DB, now time.Time, limit int) ([]domain.Mission, error) {
	var out []domain.Mission
	for _, m := range f.missions {
		if m.DeletedAt == nil && m.Due(now) && len(out) < limit {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeRepo) DueAttempts(_ context.Context, _ *gorm.DB, now time.Time, limit int) ([]domain.Attempt, error) {
	var out []domain.Attempt
	for _, a := range f.attempts {
		if a.Status != contracts.AttemptInProgress || len(out) >= limit {
			continue
		}
		m := f.missions[a.MissionID]
		over := m.Status == contracts.MissionExpired || m.Status == contracts.MissionArchived || m.DeletedAt != nil
		if (a.PeriodEndsAt != nil && !a.PeriodEndsAt.After(now)) || over {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepo) LastRun(_ context.Context, job string) (time.Time, error) {
	return f.markers[job], nil
}

func (f *fakeRepo) MarkRun(_ context.Context, job string, at time.Time) error {
	f.markers[job] = at
	return nil
}

func (f *fakeRepo) AbandonOpenAttempts(_ context.Context, _ *gorm.DB, tenantID, playerID string, now time.Time) (int64, error) {
	var n int64
	for id, a := range f.attempts {
		if a.TenantID == tenantID && a.PlayerID == playerID && a.Status == contracts.AttemptInProgress {
			a.Status, a.UpdatedAt = contracts.AttemptAbandoned, now
			a.Version++
			f.attempts[id] = a
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	for id, x := range f.events {
		if x.TenantID == tenantID {
			delete(f.events, id)
		}
	}
	for id, x := range f.attempts {
		if x.TenantID == tenantID {
			delete(f.attempts, id)
		}
	}
	for id, x := range f.missions {
		if x.TenantID == tenantID {
			delete(f.missions, id)
		}
	}
	return nil
}

// before reports (ts, id) < cursor in (created_at, id) DESC order.
func before(ts time.Time, id string, c Cursor) bool {
	return ts.Before(c.CreatedAt) || (ts.Equal(c.CreatedAt) && id < c.ID)
}

var _ Repository = (*fakeRepo)(nil)

type recorded struct {
	topic   string
	payload any
}

type fakeOutbox struct{ published []recorded }

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.published = append(f.published, recorded{topic, payload})
	return nil
}

func (f *fakeOutbox) topics() []string {
	out := make([]string, len(f.published))
	for i, r := range f.published {
		out[i] = r.topic
	}
	return out
}

func (f *fakeOutbox) byTopic(topic string) []any {
	var out []any
	for _, r := range f.published {
		if r.topic == topic {
			out = append(out, r.payload)
		}
	}
	return out
}

// allowKeys grants only the listed permission keys.
type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

func allowAll() allowKeys {
	out := allowKeys{}
	for _, p := range contracts.AllPermissions {
		out[p.Key()] = true
	}
	return out
}

type fakePlayers struct {
	players map[string]ports.PlayerSnapshot
	err     error
}

func (f *fakePlayers) PlayersByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]ports.PlayerSnapshot{}
	for _, id := range ids {
		if p, ok := f.players[id]; ok && p.TenantID == tenantID {
			out[id] = p
		}
	}
	return out, nil
}

func (f *fakePlayers) PlayersByExternalIDs(_ context.Context, tenantID string, ext []string) (map[string]ports.PlayerSnapshot, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]ports.PlayerSnapshot{}
	for _, p := range f.players {
		if p.TenantID != tenantID || p.ExternalID == "" {
			continue
		}
		for _, x := range ext {
			if x == p.ExternalID {
				out[x] = p
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) AutoMissions(_ context.Context, tenantID, eventType string) ([]domain.Mission, error) {
	var out []domain.Mission
	for _, m := range f.missions {
		et, _ := m.Criteria["event_type"].(string)
		if m.TenantID == tenantID && m.DeletedAt == nil && m.Status == contracts.MissionActive && et == eventType {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeRepo) AttemptStats(_ context.Context, tenantID string, missionIDs []string) (map[string]AttemptStats, error) {
	want := map[string]bool{}
	for _, x := range missionIDs {
		want[x] = true
	}
	out := map[string]AttemptStats{}
	hours := map[string][]float64{}
	for _, a := range f.attempts {
		if a.TenantID != tenantID || !want[a.MissionID] {
			continue
		}
		st := out[a.MissionID]
		st.Started++
		switch a.Status {
		case contracts.AttemptInProgress:
			st.InProgress++
		case contracts.AttemptCompleted:
			st.Completed++
			hours[a.MissionID] = append(hours[a.MissionID], a.CompletedAt.Sub(a.StartedAt).Hours())
		}
		out[a.MissionID] = st
	}
	for mid, hs := range hours {
		sum := 0.0
		for _, h := range hs {
			sum += h
		}
		avg := sum / float64(len(hs))
		st := out[mid]
		st.AvgHoursToComplete = &avg
		out[mid] = st
	}
	return out, nil
}
