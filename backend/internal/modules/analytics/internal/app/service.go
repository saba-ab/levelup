// Package app holds analytics' use cases: applying consumed facts to the
// projection (idempotent per event id) and the bounded portal queries.
package app

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/analytics/contracts"
	"levelup/internal/modules/analytics/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/errs"
)

// CounterRow is one aggregated daily_counters row.
type CounterRow struct {
	Day       time.Time
	Metric    string
	Dimension string
	Value     int64
}

// Repository is implemented by internal/repo. Write methods take the tx.
type Repository interface {
	// Apply records f once: false (and no change) when EventID was already
	// applied.
	Apply(ctx context.Context, tx *gorm.DB, f domain.Fact, now time.Time) (bool, error)

	Counters(ctx context.Context, tenantID string, r domain.Range, metrics []string) ([]CounterRow, error)
	ActivePlayersByDay(ctx context.Context, tenantID string, r domain.Range) (map[time.Time]int64, error)
	DistinctActivePlayers(ctx context.Context, tenantID string, r domain.Range) (int64, error)
	CohortSizes(ctx context.Context, tenantID string, from, until time.Time) (map[time.Time]int64, error)
	CohortActivity(ctx context.Context, tenantID string, from, until time.Time) ([]domain.CohortCell, error)
	Funnel(ctx context.Context, tenantID string, steps []string, r domain.Range) ([]int64, error)

	Prune(ctx context.Context, before, appliedBefore time.Time) (int64, error)
	MarkRun(ctx context.Context, job string, at time.Time) error
	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
}

// Settings are the retention knobs from Config.
type Settings struct {
	Retention        time.Duration
	AppliedRetention time.Duration
}

type Service struct {
	repo  Repository
	authz authz.Enforcer
	clock clock.Clock
	set   Settings

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, enf authz.Enforcer, db *gorm.DB, c clock.Clock, set Settings) *Service {
	s := &Service{repo: repo, authz: enf, clock: c, set: set}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, db, fn)
	}
	return s
}

func (s *Service) guard(ctx context.Context) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, nil); err != nil {
		return authz.Principal{}, err
	}
	return p, nil
}

// Record applies one consumed fact. Idempotent: a redelivered event id
// changes nothing. Reordering is harmless: counters are commutative sums,
// activity days are set inserts and first-seen keeps the earliest day.
func (s *Service) Record(ctx context.Context, f domain.Fact) error {
	if err := f.Validate(); err != nil {
		return err
	}
	f.Day = domain.DayOf(f.Day)
	return s.tx(ctx, func(tx *gorm.DB) error {
		_, err := s.repo.Apply(ctx, tx, f, s.clock.Now())
		return err
	})
}

// ---- overview ----

// DailyOverview is one day of the overview series.
type DailyOverview struct {
	Day            time.Time
	Activities     int64
	ActivePlayers  int64
	PointsCredited int64
	PointsDebited  int64
	NewPlayers     int64
}

// Totals sums the range.
type Totals struct {
	Activities        int64
	ActivePlayers     int64 // distinct over the range
	NewPlayers        int64
	PointsCredited    int64
	PointsDebited     int64
	BadgesAwarded     int64
	MissionsStarted   int64
	MissionsCompleted int64
	LevelsReached     int64
	RewardsClaimed    int64
}

type Overview struct {
	Range  domain.Range
	Totals Totals
	Daily  []DailyOverview
	DAU    int64 // active on Range.To
	WAU    int64 // distinct active over the 7 days ending Range.To
	MAU    int64 // distinct active over the 30 days ending Range.To
}

var overviewMetrics = []string{
	contracts.MetricActivities, contracts.MetricPointsCredited, contracts.MetricPointsDebited,
	contracts.MetricPlayersCreated, contracts.MetricBadgesAwarded, contracts.MetricMissionsStarted,
	contracts.MetricMissionsCompleted, contracts.MetricLevelsReached, contracts.MetricRewardsClaimed,
}

func (s *Service) Overview(ctx context.Context, from, to *time.Time) (Overview, error) {
	p, err := s.guard(ctx)
	if err != nil {
		return Overview{}, err
	}
	r, err := domain.NewRange(from, to, s.clock.Now())
	if err != nil {
		return Overview{}, err
	}
	rows, err := s.repo.Counters(ctx, p.TenantID, r, overviewMetrics)
	if err != nil {
		return Overview{}, err
	}
	active, err := s.repo.ActivePlayersByDay(ctx, p.TenantID, r)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Range: r}
	byDay := map[time.Time]*DailyOverview{}
	for _, d := range r.EachDay() {
		out.Daily = append(out.Daily, DailyOverview{Day: d, ActivePlayers: active[d]})
	}
	for i := range out.Daily {
		byDay[out.Daily[i].Day] = &out.Daily[i]
	}
	t := &out.Totals
	for _, row := range rows {
		d := byDay[domain.DayOf(row.Day)]
		switch row.Metric {
		case contracts.MetricActivities:
			t.Activities += row.Value
			if d != nil {
				d.Activities += row.Value
			}
		case contracts.MetricPointsCredited:
			t.PointsCredited += row.Value
			if d != nil {
				d.PointsCredited += row.Value
			}
		case contracts.MetricPointsDebited:
			t.PointsDebited += row.Value
			if d != nil {
				d.PointsDebited += row.Value
			}
		case contracts.MetricPlayersCreated:
			t.NewPlayers += row.Value
			if d != nil {
				d.NewPlayers += row.Value
			}
		case contracts.MetricBadgesAwarded:
			t.BadgesAwarded += row.Value
		case contracts.MetricMissionsStarted:
			t.MissionsStarted += row.Value
		case contracts.MetricMissionsCompleted:
			t.MissionsCompleted += row.Value
		case contracts.MetricLevelsReached:
			t.LevelsReached += row.Value
		case contracts.MetricRewardsClaimed:
			t.RewardsClaimed += row.Value
		}
	}
	if t.ActivePlayers, err = s.repo.DistinctActivePlayers(ctx, p.TenantID, r); err != nil {
		return Overview{}, err
	}
	if out.DAU, err = s.repo.DistinctActivePlayers(ctx, p.TenantID, domain.Trailing(r.To, 1)); err != nil {
		return Overview{}, err
	}
	if out.WAU, err = s.repo.DistinctActivePlayers(ctx, p.TenantID, domain.Trailing(r.To, 7)); err != nil {
		return Overview{}, err
	}
	if out.MAU, err = s.repo.DistinctActivePlayers(ctx, p.TenantID, domain.Trailing(r.To, 30)); err != nil {
		return Overview{}, err
	}
	return out, nil
}

// ---- engagement ----

// DayCount is a value on one day.
type DayCount struct {
	Day   time.Time
	Count int64
}

// KeyCount is a value per dimension key.
type KeyCount struct {
	Key   string
	Count int64
}

// DailyEngagement is one day of the engagement series.
type DailyEngagement struct {
	Day               time.Time
	BadgesAwarded     int64
	MissionsStarted   int64
	MissionsCompleted int64
	LevelsReached     int64
	RewardsClaimed    int64
}

type Engagement struct {
	Range             domain.Range
	BadgesPerDay      []DayCount
	MissionsStarted   int64
	MissionsCompleted int64
	LevelsReached     int64
	RewardsClaimed    int64
	TopEventTypes     []KeyCount
	TopBadges         []KeyCount
	TopMissions       []KeyCount
	LevelsByNumber    []KeyCount
	Daily             []DailyEngagement
}

const topN = 10

var engagementMetrics = []string{
	contracts.MetricActivities, contracts.MetricBadgesAwarded, contracts.MetricMissionsStarted,
	contracts.MetricMissionsCompleted, contracts.MetricLevelsReached, contracts.MetricRewardsClaimed,
}

func (s *Service) Engagement(ctx context.Context, from, to *time.Time) (Engagement, error) {
	p, err := s.guard(ctx)
	if err != nil {
		return Engagement{}, err
	}
	r, err := domain.NewRange(from, to, s.clock.Now())
	if err != nil {
		return Engagement{}, err
	}
	rows, err := s.repo.Counters(ctx, p.TenantID, r, engagementMetrics)
	if err != nil {
		return Engagement{}, err
	}
	out := Engagement{Range: r}
	byDay := map[time.Time]*DailyEngagement{}
	for _, d := range r.EachDay() {
		out.Daily = append(out.Daily, DailyEngagement{Day: d})
	}
	for i := range out.Daily {
		byDay[out.Daily[i].Day] = &out.Daily[i]
	}
	events, badges, missions, levels := map[string]int64{}, map[string]int64{}, map[string]int64{}, map[string]int64{}
	for _, row := range rows {
		d := byDay[domain.DayOf(row.Day)]
		if d == nil {
			d = &DailyEngagement{}
		}
		switch row.Metric {
		case contracts.MetricActivities:
			events[row.Dimension] += row.Value
		case contracts.MetricBadgesAwarded:
			badges[row.Dimension] += row.Value
			d.BadgesAwarded += row.Value
		case contracts.MetricMissionsStarted:
			out.MissionsStarted += row.Value
			d.MissionsStarted += row.Value
		case contracts.MetricMissionsCompleted:
			out.MissionsCompleted += row.Value
			missions[row.Dimension] += row.Value
			d.MissionsCompleted += row.Value
		case contracts.MetricLevelsReached:
			out.LevelsReached += row.Value
			levels[row.Dimension] += row.Value
			d.LevelsReached += row.Value
		case contracts.MetricRewardsClaimed:
			out.RewardsClaimed += row.Value
			d.RewardsClaimed += row.Value
		}
	}
	out.BadgesPerDay = make([]DayCount, len(out.Daily))
	for i, d := range out.Daily {
		out.BadgesPerDay[i] = DayCount{Day: d.Day, Count: d.BadgesAwarded}
	}
	out.TopEventTypes = top(events, topN)
	out.TopBadges = top(badges, topN)
	out.TopMissions = top(missions, topN)
	out.LevelsByNumber = top(levels, 0)
	return out, nil
}

// top sorts by count desc then key asc; n <= 0 keeps everything.
func top(m map[string]int64, n int) []KeyCount {
	out := make([]KeyCount, 0, len(m))
	for k, v := range m {
		out = append(out, KeyCount{Key: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// ---- retention ----

func (s *Service) Retention(ctx context.Context, cohort string, weeks int) (domain.Retention, error) {
	p, err := s.guard(ctx)
	if err != nil {
		return domain.Retention{}, err
	}
	if err := domain.ParseCohort(cohort, weeks); err != nil {
		return domain.Retention{}, err
	}
	starts := domain.CohortStarts(s.clock.Now(), weeks)
	from, until := starts[0], starts[len(starts)-1].AddDate(0, 0, 7)
	sizes, err := s.repo.CohortSizes(ctx, p.TenantID, from, until)
	if err != nil {
		return domain.Retention{}, err
	}
	cells, err := s.repo.CohortActivity(ctx, p.TenantID, from, until)
	if err != nil {
		return domain.Retention{}, err
	}
	return domain.BuildRetention(starts, sizes, cells), nil
}

// ---- funnel ----

// FunnelStep is one step's reach.
type FunnelStep struct {
	EventType      string
	Players        int64
	PctOfPrevious  float64
	PctOfFirstStep float64
}

type Funnel struct {
	Range domain.Range
	Steps []FunnelStep
}

// Funnel counts the players who did each step in order inside the range.
// Approximation (documented): order is resolved at UTC-day granularity from
// player_event_days, so step k counts when its first day on or after step
// k-1's day exists; two steps on the same day always count as in order.
func (s *Service) Funnel(ctx context.Context, rawSteps string, from, to *time.Time) (Funnel, error) {
	p, err := s.guard(ctx)
	if err != nil {
		return Funnel{}, err
	}
	steps, err := domain.ParseSteps(rawSteps)
	if err != nil {
		return Funnel{}, err
	}
	r, err := domain.NewRange(from, to, s.clock.Now())
	if err != nil {
		return Funnel{}, err
	}
	counts, err := s.repo.Funnel(ctx, p.TenantID, steps, r)
	if err != nil {
		return Funnel{}, err
	}
	if len(counts) != len(steps) {
		return Funnel{}, errs.New(errs.Internal, "funnel step count mismatch")
	}
	out := Funnel{Range: r, Steps: make([]FunnelStep, len(steps))}
	for i, st := range steps {
		prev := counts[0]
		if i > 0 {
			prev = counts[i-1]
		}
		out.Steps[i] = FunnelStep{
			EventType:      st,
			Players:        counts[i],
			PctOfPrevious:  domain.Pct(counts[i], prev),
			PctOfFirstStep: domain.Pct(counts[i], counts[0]),
		}
	}
	return out, nil
}

// ---- maintenance ----

// Prune drops projection rows older than the retention and idempotency rows
// older than their own retention. Reconciling (R47): it deletes by age, so
// a missed run is caught up by the next.
func (s *Service) Prune(ctx context.Context) (int64, error) {
	if s.set.Retention <= 0 || s.set.AppliedRetention <= 0 {
		return 0, errs.New(errs.Invalid, "analytics retention must be positive")
	}
	now := s.clock.Now()
	n, err := s.repo.Prune(ctx, domain.DayOf(now.Add(-s.set.Retention)), now.Add(-s.set.AppliedRetention))
	if err != nil {
		return n, err
	}
	return n, s.repo.MarkRun(ctx, contracts.JobPrune, now)
}

// PurgeTenant handles tenant.deleted.v1. Idempotent.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return errs.New(errs.Invalid, "tenant.deleted.v1 without tenant_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, tenantID)
	})
}
