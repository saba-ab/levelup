package app

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/rules/internal/domain"
	"levelup/internal/modules/rules/internal/domain/eval"
	"levelup/internal/modules/rules/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

// fakeRepo is an in-memory Repository. snapshot/restore let the test tx
// runner roll back like Postgres would.
type fakeRepo struct {
	mu          sync.Mutex
	rules       map[string]domain.Rule
	versions    map[string]domain.Version
	generations map[string]int64
	decisions   map[string]domain.Decision // by id
	executions  []domain.Execution
	effects     []domain.Effect
	counters    map[string]counter
	liveLoads   int
	lastAttempt map[string]time.Time
}

type counter struct {
	count int64
	last  time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		rules: map[string]domain.Rule{}, versions: map[string]domain.Version{},
		generations: map[string]int64{}, decisions: map[string]domain.Decision{}, counters: map[string]counter{},
		lastAttempt: map[string]time.Time{},
	}
}

type repoState struct {
	rules       map[string]domain.Rule
	versions    map[string]domain.Version
	generations map[string]int64
	decisions   map[string]domain.Decision
	executions  []domain.Execution
	effects     []domain.Effect
	counters    map[string]counter
}

func (f *fakeRepo) snapshot() repoState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return repoState{maps.Clone(f.rules), maps.Clone(f.versions), maps.Clone(f.generations), maps.Clone(f.decisions),
		slices.Clone(f.executions), slices.Clone(f.effects), maps.Clone(f.counters)}
}

func (f *fakeRepo) restore(s repoState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules, f.versions, f.generations, f.decisions = s.rules, s.versions, s.generations, s.decisions
	f.executions, f.effects, f.counters = s.executions, s.effects, s.counters
}

func (f *fakeRepo) CreateRule(_ context.Context, _ *gorm.DB, r domain.Rule) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.rules {
		if x.TenantID == r.TenantID && x.Slug == r.Slug && x.DeletedAt == nil {
			return domain.ErrSlugTaken
		}
	}
	f.rules[r.ID] = r
	return nil
}

func (f *fakeRepo) RuleByID(_ context.Context, tenantID, id string) (domain.Rule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rules[id]
	if !ok || r.TenantID != tenantID || r.DeletedAt != nil {
		return domain.Rule{}, domain.ErrRuleNotFound
	}
	return r, nil
}

func (f *fakeRepo) RuleForUpdate(ctx context.Context, _ *gorm.DB, tenantID, id string) (domain.Rule, error) {
	return f.RuleByID(ctx, tenantID, id)
}

func (f *fakeRepo) SaveRule(_ context.Context, _ *gorm.DB, r domain.Rule) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.rules[r.ID]; !ok {
		return domain.ErrRuleNotFound
	}
	f.rules[r.ID] = r
	return nil
}

func (f *fakeRepo) ListRules(_ context.Context, tenantID string, flt RuleFilter, p Page) ([]domain.Rule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Rule
	for _, r := range f.rules {
		if r.TenantID != tenantID || r.DeletedAt != nil || (flt.Status != "" && r.Status != flt.Status) ||
			(flt.TriggerEvent != "" && r.TriggerEvent != flt.TriggerEvent) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if p.AfterID != "" {
		i := slices.IndexFunc(out, func(r domain.Rule) bool { return r.ID < p.AfterID })
		if i < 0 {
			return nil, nil
		}
		out = out[i:]
	}
	if len(out) > p.Limit {
		out = out[:p.Limit]
	}
	return out, nil
}

func (f *fakeRepo) CreateVersion(_ context.Context, _ *gorm.DB, v domain.Version) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.versions {
		if x.RuleID == v.RuleID && x.Version == v.Version {
			return domain.ErrVersionConflict
		}
	}
	f.versions[v.ID] = v
	return nil
}

func (f *fakeRepo) SaveDraftVersion(_ context.Context, _ *gorm.DB, v domain.Version) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.versions[v.ID]
	if !ok || cur.Published() {
		return domain.ErrVersionPublished
	}
	cur.Conditions, cur.Actions, cur.Limits = v.Conditions, v.Actions, v.Limits
	f.versions[v.ID] = cur
	return nil
}

func (f *fakeRepo) PublishVersion(_ context.Context, _ *gorm.DB, v domain.Version) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.versions[v.ID]
	if !ok {
		return domain.ErrVersionNotFound
	}
	if cur.PublishedAt == nil {
		cur.PublishedAt = v.PublishedAt
	}
	f.versions[v.ID] = cur
	return nil
}

func (f *fakeRepo) VersionByNumberTx(_ context.Context, _ *gorm.DB, tenantID, ruleID string, n int) (domain.Version, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.versions {
		if v.TenantID == tenantID && v.RuleID == ruleID && v.Version == n {
			return v, nil
		}
	}
	return domain.Version{}, domain.ErrVersionNotFound
}

func (f *fakeRepo) LatestVersionTx(_ context.Context, _ *gorm.DB, tenantID, ruleID string) (domain.Version, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var best *domain.Version
	for _, v := range f.versions {
		if v.TenantID == tenantID && v.RuleID == ruleID && (best == nil || v.Version > best.Version) {
			vv := v
			best = &vv
		}
	}
	if best == nil {
		return domain.Version{}, domain.ErrVersionNotFound
	}
	return *best, nil
}

func (f *fakeRepo) LatestVersion(ctx context.Context, tenantID, ruleID string) (domain.Version, error) {
	return f.LatestVersionTx(ctx, nil, tenantID, ruleID)
}

func (f *fakeRepo) VersionsByIDs(_ context.Context, tenantID string, ids []string) (map[string]domain.Version, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]domain.Version{}
	for _, id := range ids {
		if v, ok := f.versions[id]; ok && v.TenantID == tenantID {
			out[id] = v
		}
	}
	return out, nil
}

func (f *fakeRepo) ListVersions(_ context.Context, tenantID, ruleID string, p Page) ([]domain.Version, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Version
	for _, v := range f.versions {
		if v.TenantID == tenantID && v.RuleID == ruleID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	if len(out) > p.Limit {
		out = out[:p.Limit]
	}
	return out, nil
}

func (f *fakeRepo) BumpGeneration(_ context.Context, _ *gorm.DB, tenantID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.generations[tenantID]++
	return f.generations[tenantID], nil
}

func (f *fakeRepo) Generation(_ context.Context, tenantID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.generations[tenantID], nil
}

func (f *fakeRepo) LiveRuleSources(_ context.Context, tenantID, trigger string) ([]eval.RuleSource, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.liveLoads++
	var out []eval.RuleSource
	for _, r := range f.rules {
		if r.TenantID != tenantID || r.TriggerEvent != trigger || !r.Live() {
			continue
		}
		v := f.versions[r.CurrentVersionID]
		out = append(out, eval.RuleSource{RuleID: r.ID, RuleVersionID: v.ID, Name: r.Name, ProgramID: r.ProgramID,
			Priority: r.Priority, Conditions: v.Conditions, Actions: v.Actions, Limits: v.Limits})
	}
	return out, nil
}

func (f *fakeRepo) DecisionExists(_ context.Context, activityID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.decisions {
		if d.ActivityID == activityID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) InsertDecision(_ context.Context, _ *gorm.DB, d domain.Decision) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.decisions {
		if x.ActivityID == d.ActivityID {
			return false, nil
		}
	}
	f.decisions[d.ID] = d
	return true, nil
}

func (f *fakeRepo) SetDecisionOutcome(_ context.Context, _ *gorm.DB, d domain.Decision) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur := f.decisions[d.ID]
	cur.Outcome, cur.Reason = d.Outcome, d.Reason
	f.decisions[d.ID] = cur
	return nil
}

func (f *fakeRepo) InsertExecutions(_ context.Context, _ *gorm.DB, es []domain.Execution) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executions = append(f.executions, es...)
	return nil
}

func (f *fakeRepo) InsertEffects(_ context.Context, _ *gorm.DB, es []domain.Effect) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range es {
		dup := slices.ContainsFunc(f.effects, func(x domain.Effect) bool {
			return x.TenantID == e.TenantID && x.IdempotencyKey == e.IdempotencyKey
		})
		if !dup {
			f.effects = append(f.effects, e)
		}
	}
	return nil
}

// ApplyLimits mirrors the SQL semantics (all-or-nothing per rule).
func (f *fakeRepo) ApplyLimits(_ context.Context, _ *gorm.DB, c LimitCheck) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := func(w string) string { return c.TenantID + "|" + c.RuleID + "|" + c.PlayerID + "|" + w }
	type upd struct {
		k string
		v counter
	}
	var ups []upd
	check := func(w string, maxCount int64) bool {
		cur := f.counters[key(w)]
		if cur.count >= maxCount {
			return false
		}
		ups = append(ups, upd{key(w), counter{cur.count + 1, laterOf(cur.last, c.At)}})
		return true
	}
	if c.Limits.MaxPerPlayer > 0 && !check(domain.WindowLifetime, c.Limits.MaxPerPlayer) {
		return false, nil
	}
	if c.Limits.MaxPerPlayerPerDay > 0 && !check(domain.DayWindow(c.At), c.Limits.MaxPerPlayerPerDay) {
		return false, nil
	}
	if c.Limits.MaxPerPlayerPerWeek > 0 && !check(domain.WeekWindow(c.At), c.Limits.MaxPerPlayerPerWeek) {
		return false, nil
	}
	if c.Limits.CooldownSeconds > 0 {
		cur, ok := f.counters[key(domain.WindowCooldown)]
		cd := time.Duration(c.Limits.CooldownSeconds) * time.Second
		if ok && c.At.Before(cur.last.Add(cd)) && c.At.After(cur.last.Add(-cd)) {
			return false, nil
		}
		ups = append(ups, upd{key(domain.WindowCooldown), counter{cur.count + 1, laterOf(cur.last, c.At)}})
	}
	for _, u := range ups {
		f.counters[u.k] = u.v
	}
	return true, nil
}

func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (f *fakeRepo) ListDecisions(_ context.Context, tenantID string, flt DecisionFilter, p Page) ([]domain.Decision, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Decision
	for _, d := range f.decisions {
		if d.TenantID == tenantID && (flt.ActivityID == "" || d.ActivityID == flt.ActivityID) &&
			(flt.PlayerID == "" || d.PlayerID == flt.PlayerID) {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	if len(out) > p.Limit {
		out = out[:p.Limit]
	}
	return out, nil
}

func (f *fakeRepo) DecisionByID(_ context.Context, tenantID, id string) (domain.Decision, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.decisions[id]
	if !ok || d.TenantID != tenantID {
		return domain.Decision{}, domain.ErrDecisionNotFound
	}
	return d, nil
}

func (f *fakeRepo) ExecutionsByDecision(_ context.Context, tenantID, decisionID string) ([]domain.Execution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Execution
	for _, e := range f.executions {
		if e.TenantID == tenantID && e.DecisionID == decisionID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeRepo) EffectsByDecision(_ context.Context, tenantID, decisionID string) ([]domain.Effect, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Effect
	for _, e := range f.effects {
		if e.TenantID == tenantID && e.DecisionID == decisionID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeRepo) SettleEffect(_ context.Context, _ *gorm.DB, tenantID, key, status, reason string, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, e := range f.effects {
		if e.TenantID == tenantID && e.IdempotencyKey == key && e.Status == domain.EffectRequested {
			e.Status, e.Reason, e.SettledAt = status, reason, &at
			f.effects[i] = e
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) PendingEffects(_ context.Context, before time.Time, maxAttempts, limit int) ([]domain.Effect, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Effect
	for _, e := range f.effects {
		last := e.RequestedAt
		if la, ok := f.lastAttempt[e.ID]; ok {
			last = la
		}
		if e.Status == domain.EffectRequested && last.Before(before) && e.Attempts < maxAttempts {
			out = append(out, e)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) MarkEffectsRetried(_ context.Context, _ *gorm.DB, ids []string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, e := range f.effects {
		if slices.Contains(ids, e.ID) {
			e.Attempts++
			f.effects[i] = e
			f.lastAttempt[e.ID] = at
		}
	}
	return nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	maps.DeleteFunc(f.rules, func(_ string, r domain.Rule) bool { return r.TenantID == tenantID })
	maps.DeleteFunc(f.versions, func(_ string, v domain.Version) bool { return v.TenantID == tenantID })
	maps.DeleteFunc(f.decisions, func(_ string, d domain.Decision) bool { return d.TenantID == tenantID })
	delete(f.generations, tenantID)
	f.executions = slices.DeleteFunc(f.executions, func(e domain.Execution) bool { return e.TenantID == tenantID })
	f.effects = slices.DeleteFunc(f.effects, func(e domain.Effect) bool { return e.TenantID == tenantID })
	maps.DeleteFunc(f.counters, func(k string, _ counter) bool { return strings.HasPrefix(k, tenantID+"|") })
	return nil
}

// fakeOutbox records publishes; failOn makes Publish fail for a topic.
type published struct {
	topic   string
	payload any
}

type fakeOutbox struct {
	mu     sync.Mutex
	events []published
	failOn string
}

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failOn != "" && topic == f.failOn {
		return errs.New(errs.Unavailable, "outbox down")
	}
	f.events = append(f.events, published{topic, payload})
	return nil
}

func (f *fakeOutbox) topics() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.events))
	for i, e := range f.events {
		out[i] = e.topic
	}
	return out
}

func (f *fakeOutbox) byTopic(topic string) []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []any
	for _, e := range f.events {
		if e.topic == topic {
			out = append(out, e.payload)
		}
	}
	return out
}

// fakeCache records loads and evictions; inTx is set by the tx runner so an
// eviction inside a transaction fails the test (R44).
type fakeCache struct {
	t       *testing.T
	inTx    *bool
	entries map[string][]eval.RuleSource
	evicted []string
}

func (c *fakeCache) key(tenant string, gen int64, trigger string) string {
	return fmt.Sprintf("%s:%d:%s", tenant, gen, trigger)
}

func (c *fakeCache) Load(ctx context.Context, tenant string, gen int64, trigger string,
	load func(context.Context) ([]eval.RuleSource, error)) ([]eval.RuleSource, error) {
	k := c.key(tenant, gen, trigger)
	if v, ok := c.entries[k]; ok {
		return v, nil
	}
	v, err := load(ctx)
	if err != nil {
		return nil, err
	}
	c.entries[k] = v
	return v, nil
}

func (c *fakeCache) Evict(_ context.Context, tenant string, gen int64, triggers ...string) {
	require.False(c.t, *c.inTx, "cache eviction inside the transaction (R44)")
	for _, tr := range triggers {
		k := c.key(tenant, gen, tr)
		delete(c.entries, k)
		c.evicted = append(c.evicted, k)
	}
}

// allowKeys enforces only the listed permission keys.
type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

type fakePlayers struct {
	players map[string]ports.PlayerSnapshot // by id
	err     error
	calls   int
}

func (f *fakePlayers) ByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	f.calls++
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

func (f *fakePlayers) ByExternalIDs(_ context.Context, tenantID string, ext []string) (map[string]ports.PlayerSnapshot, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]ports.PlayerSnapshot{}
	for _, p := range f.players {
		if p.TenantID == tenantID && slices.Contains(ext, p.ExternalID) {
			out[p.ExternalID] = p
		}
	}
	return out, nil
}

type fakeProgress struct {
	m     map[string]ports.ProgressSnapshot
	calls int
}

func (f *fakeProgress) ByPlayerIDs(_ context.Context, _ string, ids []string) (map[string]ports.ProgressSnapshot, error) {
	f.calls++
	out := map[string]ports.ProgressSnapshot{}
	for _, id := range ids {
		if p, ok := f.m[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

type fakePoints struct {
	m     map[string]ports.BalanceSnapshot
	calls int
}

func (f *fakePoints) ByPlayerIDs(_ context.Context, _ string, ids []string) (map[string]ports.BalanceSnapshot, error) {
	f.calls++
	out := map[string]ports.BalanceSnapshot{}
	for _, id := range ids {
		if p, ok := f.m[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

type fakePrograms struct{ enrolled map[string][]string }

func (f *fakePrograms) EnrolledProgramIDs(_ context.Context, _, playerID string) ([]string, error) {
	return f.enrolled[playerID], nil
}

// harness bundles a service under test with its fakes.
type harness struct {
	svc      *Service
	repo     *fakeRepo
	ob       *fakeOutbox
	cache    *fakeCache
	players  *fakePlayers
	progress *fakeProgress
	points   *fakePoints
	clock    *clock.Fake
}

var (
	t0       = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	tenantA  = "0198d000-0000-7000-8000-0000000000a0"
	tenantB  = "0198d000-0000-7000-8000-0000000000b0"
	playerP  = "0198d000-0000-7000-8000-000000000101"
	playerQ  = "0198d000-0000-7000-8000-000000000102" // inactive
	playerB  = "0198d000-0000-7000-8000-000000000201" // tenant B
	badgeID  = "0198d000-0000-7000-8000-0000000000ee"
	allPerms = allowKeys{
		"rules:view_any": true, "rules:view": true, "rules:create": true, "rules:update": true,
		"rules:delete": true, "rules:publish": true, "rules:simulate": true, "rules:view_decisions": true,
	}
)

func newHarness(t *testing.T, enf authz.Enforcer) *harness {
	t.Helper()
	repo, ob := newFakeRepo(), &fakeOutbox{}
	inTx := false
	cache := &fakeCache{t: t, inTx: &inTx, entries: map[string][]eval.RuleSource{}}
	players := &fakePlayers{players: map[string]ports.PlayerSnapshot{
		playerP: {ID: playerP, TenantID: tenantA, ExternalID: "ext-p", Active: true, Attributes: map[string]any{"tier": "gold"}},
		playerQ: {ID: playerQ, TenantID: tenantA, ExternalID: "ext-q", Active: false},
		playerB: {ID: playerB, TenantID: tenantB, ExternalID: "ext-p", Active: true},
	}}
	progress := &fakeProgress{m: map[string]ports.ProgressSnapshot{playerP: {PlayerID: playerP, TotalXP: 900, Level: 3}}}
	points := &fakePoints{m: map[string]ports.BalanceSnapshot{playerP: {PlayerID: playerP, Balance: 150}}}
	clk := clock.NewFake(t0)
	svc := NewService(Deps{Repo: repo, Cache: cache, Players: players, Progress: progress, Points: points,
		Outbox: ob, Authz: enf, Clock: clk}, Config{MaxCausationDepth: 3})
	svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error {
		snap := repo.snapshot()
		ob.mu.Lock()
		n := len(ob.events)
		ob.mu.Unlock()
		inTx = true
		err := fn(nil)
		inTx = false
		if err != nil {
			repo.restore(snap)
			ob.mu.Lock()
			ob.events = ob.events[:n]
			ob.mu.Unlock()
		}
		return err
	}
	return &harness{svc: svc, repo: repo, ob: ob, cache: cache, players: players, progress: progress, points: points, clock: clk}
}

func asTenant(tenantID string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "0198d000-0000-7000-8000-00000000c0de", TenantID: tenantID, RoleIDs: []int64{2}})
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

// liveRule creates and publishes a rule in tenant A.
func (h *harness) liveRule(t *testing.T, name, trigger string, priority int, conditions, actions, limits string) RuleView {
	t.Helper()
	ctx := asTenant(tenantA)
	v, err := h.svc.CreateRule(ctx, CreateRuleInput{Name: name, TriggerEvent: trigger, Priority: priority,
		Conditions: raw(conditions), Actions: raw(actions), Limits: raw(limits)})
	require.NoError(t, err)
	h.clock.Advance(time.Millisecond) // distinct uuidv7 order is by id; time just for realism
	pub, err := h.svc.Publish(ctx, v.Rule.ID, 1)
	require.NoError(t, err)
	return pub
}

func isKind(t *testing.T, err error, k errs.Kind) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, k, errs.KindOf(err), "err: %v", err)
}
