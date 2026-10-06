package app

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/analytics/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

const (
	tenantA = "0198d000-0000-7000-8000-00000000000a"
	tenantB = "0198d000-0000-7000-8000-00000000000b"
)

var now = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) // a Wednesday

type counterKey struct {
	tenant, metric, dim string
	day                 time.Time
}

type dayKey struct {
	tenant, player string
	day            time.Time
}

// fakeRepo is an in-memory projection good enough for the service's
// aggregation logic; the SQL itself is proven in internal/repo.
type fakeRepo struct {
	applied   map[string]string // event id → tenant
	counters  map[counterKey]int64
	days      map[dayKey]bool
	firstSeen map[[2]string]time.Time

	cohortSizes map[time.Time]int64
	cohortCells []domain.CohortCell
	funnel      []int64
	funnelSteps []string

	pruneBefore, pruneApplied time.Time
	marked                    []string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		applied:   map[string]string{},
		counters:  map[counterKey]int64{},
		days:      map[dayKey]bool{},
		firstSeen: map[[2]string]time.Time{},
	}
}

func (f *fakeRepo) Apply(_ context.Context, _ *gorm.DB, fact domain.Fact, _ time.Time) (bool, error) {
	if _, ok := f.applied[fact.EventID]; ok {
		return false, nil
	}
	f.applied[fact.EventID] = fact.TenantID
	for _, c := range fact.Counters {
		f.counters[counterKey{fact.TenantID, c.Metric, c.Dimension, fact.Day}] += c.Value
	}
	if fact.IsActivity() {
		f.days[dayKey{fact.TenantID, fact.PlayerID, fact.Day}] = true
		k := [2]string{fact.TenantID, fact.PlayerID}
		if cur, ok := f.firstSeen[k]; !ok || fact.Day.Before(cur) {
			f.firstSeen[k] = fact.Day
		}
	}
	return true, nil
}

func inRange(d time.Time, r domain.Range) bool { return !d.Before(r.From) && !d.After(r.To) }

func (f *fakeRepo) Counters(_ context.Context, tenantID string, r domain.Range, metrics []string) ([]CounterRow, error) {
	want := map[string]bool{}
	for _, m := range metrics {
		want[m] = true
	}
	var out []CounterRow
	for k, v := range f.counters {
		if k.tenant == tenantID && want[k.metric] && inRange(k.day, r) {
			out = append(out, CounterRow{Day: k.day, Metric: k.metric, Dimension: k.dim, Value: v})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day.Before(out[j].Day) })
	return out, nil
}

func (f *fakeRepo) ActivePlayersByDay(_ context.Context, tenantID string, r domain.Range) (map[time.Time]int64, error) {
	out := map[time.Time]int64{}
	for k := range f.days {
		if k.tenant == tenantID && inRange(k.day, r) {
			out[k.day]++
		}
	}
	return out, nil
}

func (f *fakeRepo) DistinctActivePlayers(_ context.Context, tenantID string, r domain.Range) (int64, error) {
	seen := map[string]bool{}
	for k := range f.days {
		if k.tenant == tenantID && inRange(k.day, r) {
			seen[k.player] = true
		}
	}
	return int64(len(seen)), nil
}

func (f *fakeRepo) CohortSizes(context.Context, string, time.Time, time.Time) (map[time.Time]int64, error) {
	return f.cohortSizes, nil
}

func (f *fakeRepo) CohortActivity(context.Context, string, time.Time, time.Time) ([]domain.CohortCell, error) {
	return f.cohortCells, nil
}

func (f *fakeRepo) Funnel(_ context.Context, _ string, steps []string, _ domain.Range) ([]int64, error) {
	f.funnelSteps = steps
	return f.funnel, nil
}

func (f *fakeRepo) Prune(_ context.Context, before, appliedBefore time.Time) (int64, error) {
	f.pruneBefore, f.pruneApplied = before, appliedBefore
	return 3, nil
}

func (f *fakeRepo) MarkRun(_ context.Context, job string, _ time.Time) error {
	f.marked = append(f.marked, job)
	return nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	for k := range f.counters {
		if k.tenant == tenantID {
			delete(f.counters, k)
		}
	}
	for k := range f.days {
		if k.tenant == tenantID {
			delete(f.days, k)
		}
	}
	for k, t := range f.applied {
		if t == tenantID {
			delete(f.applied, k)
		}
	}
	return nil
}

type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

func newTestService(repo Repository, enf authz.Enforcer) *Service {
	svc := NewService(repo, enf, nil, clock.NewFake(now), Settings{Retention: 2 * 365 * 24 * time.Hour, AppliedRetention: 30 * 24 * time.Hour})
	svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
	return svc
}

func asTenant(tenantID string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "u1", TenantID: tenantID, RoleIDs: []int64{2}})
}
