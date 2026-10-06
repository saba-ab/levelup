package app

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/modules/rewards/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

const (
	tenantA = "0198d000-0000-7000-8000-00000000000a"
	tenantB = "0198d000-0000-7000-8000-00000000000b"
	player1 = "0198d000-0000-7000-8000-000000000001"
	player2 = "0198d000-0000-7000-8000-000000000002"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

// ---- fake repository with transactional snapshots ----

type fakeRepo struct {
	mu      sync.Mutex
	rewards map[string]domain.Reward
	claims  map[string]domain.Claim
	markers map[string]time.Time
	deleted map[string]bool
	// missClientRequestOnce makes the next ClaimByClientRequest miss, to
	// simulate a concurrent request winning between pre-check and insert.
	missClientRequestOnce bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		rewards: map[string]domain.Reward{}, claims: map[string]domain.Claim{},
		markers: map[string]time.Time{}, deleted: map[string]bool{},
	}
}

type repoSnap struct {
	rewards map[string]domain.Reward
	claims  map[string]domain.Claim
	deleted map[string]bool
}

func (f *fakeRepo) snapshot() repoSnap {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := repoSnap{rewards: map[string]domain.Reward{}, claims: map[string]domain.Claim{}, deleted: map[string]bool{}}
	for k, v := range f.rewards {
		s.rewards[k] = v
	}
	for k, v := range f.claims {
		s.claims[k] = v
	}
	for k, v := range f.deleted {
		s.deleted[k] = v
	}
	return s
}

func (f *fakeRepo) restore(s repoSnap) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rewards, f.claims, f.deleted = s.rewards, s.claims, s.deleted
}

func (f *fakeRepo) CreateReward(_ context.Context, _ *gorm.DB, r domain.Reward) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, x := range f.rewards {
		if x.TenantID == r.TenantID && x.Slug == r.Slug && !f.deleted[id] {
			return domain.ErrSlugTaken
		}
	}
	f.rewards[r.ID] = r
	return nil
}

func (f *fakeRepo) RewardByID(_ context.Context, tenantID, id string, includeDeleted bool) (domain.Reward, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rewards[id]
	if !ok || r.TenantID != tenantID || (f.deleted[id] && !includeDeleted) {
		return domain.Reward{}, domain.ErrRewardNotFound
	}
	return r, nil
}

func (f *fakeRepo) RewardForUpdate(ctx context.Context, _ *gorm.DB, tenantID, id string, includeDeleted bool) (domain.Reward, error) {
	return f.RewardByID(ctx, tenantID, id, includeDeleted)
}

func (f *fakeRepo) SaveReward(_ context.Context, _ *gorm.DB, r domain.Reward) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.rewards[r.ID]
	if !ok || cur.Version != r.Version {
		return domain.ErrVersionConflict
	}
	r.Version++
	f.rewards[r.ID] = r
	return nil
}

func (f *fakeRepo) SoftDeleteReward(_ context.Context, _ *gorm.DB, tenantID, id string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rewards[id]
	if !ok || r.TenantID != tenantID || f.deleted[id] {
		return domain.ErrRewardNotFound
	}
	f.deleted[id] = true
	return nil
}

func (f *fakeRepo) ListRewards(_ context.Context, tenantID string, flt RewardFilter, p Page) ([]domain.Reward, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Reward
	for id, r := range f.rewards {
		if r.TenantID != tenantID || f.deleted[id] || (flt.Status != "" && r.Status != flt.Status) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	var page []domain.Reward
	for _, r := range out {
		if p.AfterID != "" && r.ID >= p.AfterID {
			continue
		}
		page = append(page, r)
	}
	if len(page) > p.Limit {
		page = page[:p.Limit]
	}
	return page, nil
}

func (f *fakeRepo) EndedRewards(_ context.Context, now time.Time, limit int) ([]domain.Reward, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Reward
	for id, r := range f.rewards {
		if !f.deleted[id] && r.EndAt != nil && !now.Before(*r.EndAt) &&
			(r.Status == contracts.RewardActive || r.Status == contracts.RewardPaused || r.Status == contracts.RewardDepleted) {
			out = append(out, r)
		}
	}
	return capN(out, limit), nil
}

func (f *fakeRepo) InsertClaim(_ context.Context, _ *gorm.DB, c domain.Claim) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.claims {
		if x.TenantID != c.TenantID {
			continue
		}
		if c.ClientRequestID != nil && x.ClientRequestID != nil && *x.ClientRequestID == *c.ClientRequestID {
			return domain.ErrDuplicateRequest
		}
		if c.GrantKey != nil && x.GrantKey != nil && *x.GrantKey == *c.GrantKey {
			return domain.ErrDuplicateRequest
		}
		if c.Code != nil && x.Code != nil && *x.Code == *c.Code {
			return domain.ErrCodeCollision
		}
	}
	f.claims[c.ID] = c
	return nil
}

func (f *fakeRepo) ClaimByID(_ context.Context, tenantID, id string) (domain.Claim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.claims[id]
	if !ok || c.TenantID != tenantID {
		return domain.Claim{}, domain.ErrClaimNotFound
	}
	return c, nil
}

func (f *fakeRepo) ClaimForUpdate(ctx context.Context, _ *gorm.DB, tenantID, id string) (domain.Claim, error) {
	return f.ClaimByID(ctx, tenantID, id)
}

func (f *fakeRepo) findClaim(match func(domain.Claim) bool) (domain.Claim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.claims {
		if match(c) {
			return c, nil
		}
	}
	return domain.Claim{}, domain.ErrClaimNotFound
}

func (f *fakeRepo) ClaimByClientRequest(_ context.Context, tenantID, key string) (domain.Claim, error) {
	f.mu.Lock()
	miss := f.missClientRequestOnce
	f.missClientRequestOnce = false
	f.mu.Unlock()
	if miss {
		return domain.Claim{}, domain.ErrClaimNotFound
	}
	return f.findClaim(func(c domain.Claim) bool {
		return c.TenantID == tenantID && c.ClientRequestID != nil && *c.ClientRequestID == key
	})
}

func (f *fakeRepo) ClaimByGrantKey(_ context.Context, tenantID, key string) (domain.Claim, error) {
	return f.findClaim(func(c domain.Claim) bool {
		return c.TenantID == tenantID && c.GrantKey != nil && *c.GrantKey == key
	})
}

func (f *fakeRepo) SaveClaim(_ context.Context, _ *gorm.DB, c domain.Claim) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.claims[c.ID]
	if !ok || cur.Version != c.Version {
		return domain.ErrVersionConflict
	}
	if c.Code != nil {
		for id, x := range f.claims {
			if id != c.ID && x.TenantID == c.TenantID && x.Code != nil && *x.Code == *c.Code {
				return domain.ErrCodeCollision
			}
		}
	}
	c.Version++
	f.claims[c.ID] = c
	return nil
}

func (f *fakeRepo) CountHeldClaims(_ context.Context, _ *gorm.DB, tenantID, rewardID, playerID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.claims {
		if c.TenantID == tenantID && c.RewardID == rewardID && c.PlayerID == playerID && c.HoldsStock() {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) ListPlayerClaims(_ context.Context, tenantID, playerID string, p Page) ([]domain.Claim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Claim
	for _, c := range f.claims {
		if c.TenantID == tenantID && c.PlayerID == playerID && (p.AfterID == "" || c.ID < p.AfterID) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return capN(out, p.Limit), nil
}

func (f *fakeRepo) ListClaims(_ context.Context, tenantID string, flt ClaimFilter, p Page) ([]domain.Claim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Claim
	for _, c := range f.claims {
		switch {
		case c.TenantID != tenantID,
			flt.Status != "" && c.Status != flt.Status,
			flt.RewardID != "" && c.RewardID != flt.RewardID,
			flt.PlayerID != "" && c.PlayerID != flt.PlayerID,
			flt.From != nil && c.CreatedAt.Before(*flt.From),
			flt.To != nil && !c.CreatedAt.Before(*flt.To),
			p.AfterID != "" && c.ID >= p.AfterID:
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return capN(out, p.Limit), nil
}

func (f *fakeRepo) RewardStats(_ context.Context, tenantID string) ([]RewardStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []RewardStats
	for id, r := range f.rewards {
		if r.TenantID != tenantID {
			continue
		}
		st := RewardStats{RewardID: id, Slug: r.Slug, Name: r.Name, Type: r.Type, Deleted: f.deleted[id]}
		for _, c := range f.claims {
			if c.TenantID != tenantID || c.RewardID != id {
				continue
			}
			if c.ClaimedAt != nil {
				st.Claimed++
			}
			switch c.Status {
			case contracts.ClaimRedeemed:
				st.Redeemed++
			case contracts.ClaimExpired:
				st.Expired++
			case contracts.ClaimCancelled, contracts.ClaimRefundPending, contracts.ClaimRefunded:
				st.Cancelled++
			}
			switch c.Status {
			case contracts.ClaimClaimed, contracts.ClaimRedeemed, contracts.ClaimExpired:
				st.PointsSpent += c.PointsCost
			}
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RewardID > out[j].RewardID })
	return out, nil
}

func (f *fakeRepo) claimsWhere(match func(domain.Claim) bool, limit int) []domain.Claim {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Claim
	for _, c := range f.claims {
		if match(c) {
			out = append(out, c)
		}
	}
	return capN(out, limit)
}

func (f *fakeRepo) DuePendingClaims(_ context.Context, now time.Time, limit int) ([]domain.Claim, error) {
	return f.claimsWhere(func(c domain.Claim) bool {
		return c.Status == contracts.ClaimPendingPayment && c.HoldExpiresAt != nil && !now.Before(*c.HoldExpiresAt)
	}, limit), nil
}

func (f *fakeRepo) PaidCancelledSince(_ context.Context, since time.Time, limit int) ([]domain.Claim, error) {
	return f.claimsWhere(func(c domain.Claim) bool {
		return c.Status == contracts.ClaimCancelled && c.PointsCost > 0 && !c.UpdatedAt.Before(since)
	}, limit), nil
}

func (f *fakeRepo) RefundPendingClaims(_ context.Context, limit int) ([]domain.Claim, error) {
	return f.claimsWhere(func(c domain.Claim) bool { return c.Status == contracts.ClaimRefundPending }, limit), nil
}

func (f *fakeRepo) DueExpiringClaims(_ context.Context, now time.Time, limit int) ([]domain.Claim, error) {
	return f.claimsWhere(func(c domain.Claim) bool {
		return c.Status == contracts.ClaimClaimed && c.ExpiresAt != nil && !now.Before(*c.ExpiresAt)
	}, limit), nil
}

func (f *fakeRepo) LastRun(_ context.Context, job string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.markers[job], nil
}

func (f *fakeRepo) MarkRun(_ context.Context, job string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markers[job] = at
	return nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, c := range f.claims {
		if c.TenantID == tenantID {
			delete(f.claims, id)
		}
	}
	for id, r := range f.rewards {
		if r.TenantID == tenantID {
			delete(f.rewards, id)
		}
	}
	return nil
}

func capN[T any](s []T, n int) []T {
	if n > 0 && len(s) > n {
		return s[:n]
	}
	return s
}

// ---- outbox, authz, ports ----

type recorded struct {
	topic   string
	payload any
}

type fakeOutbox struct {
	mu        sync.Mutex
	published []recorded
}

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, recorded{topic, payload})
	return nil
}

func (f *fakeOutbox) topics() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.published))
	for i, p := range f.published {
		out[i] = p.topic
	}
	return out
}

func (f *fakeOutbox) count(topic string) int {
	n := 0
	for _, t := range f.topics() {
		if t == topic {
			n++
		}
	}
	return n
}

func (f *fakeOutbox) last(topic string) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.published) - 1; i >= 0; i-- {
		if f.published[i].topic == topic {
			return f.published[i].payload
		}
	}
	return nil
}

func (f *fakeOutbox) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = nil
}

type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}

func allowAll() allowKeys {
	a := allowKeys{}
	for _, p := range contracts.AllPermissions {
		a[p.Key()] = true
	}
	return a
}

type fakePlayers map[string]ports.PlayerSnapshot

func (f fakePlayers) PlayersByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, id := range ids {
		if p, ok := f[id]; ok && p.TenantID == tenantID {
			out[id] = p
		}
	}
	return out, nil
}

type fakeProgress map[string]int

func (f fakeProgress) LevelsByPlayerIDs(_ context.Context, _ string, ids []string) (map[string]int, error) {
	out := map[string]int{}
	for _, id := range ids {
		if l, ok := f[id]; ok {
			out[id] = l
		}
	}
	return out, nil
}

type fakePoints struct {
	mu       sync.Mutex
	outcomes map[string]ports.PaymentOutcome
}

func (f *fakePoints) set(key string, o ports.PaymentOutcome) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outcomes[key] = o
}

func (f *fakePoints) OutcomeByKey(_ context.Context, _ string, key string) (ports.PaymentOutcome, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o, ok := f.outcomes[key]
	return o, ok, nil
}

// ---- harness ----

type harness struct {
	svc    *Service
	repo   *fakeRepo
	ob     *fakeOutbox
	clock  *clock.Fake
	points *fakePoints
	codes  int
}

func newHarness(t *testing.T, enf authz.Enforcer) *harness {
	t.Helper()
	h := &harness{
		repo:   newFakeRepo(),
		ob:     &fakeOutbox{},
		clock:  clock.NewFake(t0),
		points: &fakePoints{outcomes: map[string]ports.PaymentOutcome{}},
	}
	players := fakePlayers{
		player1:                                {ID: player1, TenantID: tenantA, Active: true},
		player2:                                {ID: player2, TenantID: tenantA, Active: true},
		"0198d000-0000-7000-8000-0000000000ff": {ID: "0198d000-0000-7000-8000-0000000000ff", TenantID: tenantA, Active: false},
	}
	h.svc = NewService(h.repo, players, fakeProgress{player1: 5}, h.points, h.ob, enf, nil, h.clock,
		Settings{HoldTTL: 10 * time.Minute, SweepBatchSize: 100})
	h.svc.code = func() string {
		h.codes++
		return "CODE-" + string(rune('A'+h.codes))
	}
	// Pass-through tx with rollback: a failed closure leaves neither rows
	// nor published events behind, like a real transaction.
	h.svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error {
		snap := h.repo.snapshot()
		h.ob.mu.Lock()
		n := len(h.ob.published)
		h.ob.mu.Unlock()
		if err := fn(nil); err != nil {
			h.repo.restore(snap)
			h.ob.mu.Lock()
			h.ob.published = h.ob.published[:n]
			h.ob.mu.Unlock()
			return err
		}
		return nil
	}
	return h
}

func ctxFor(tenant string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "u1", TenantID: tenant, RoleIDs: []int64{2}})
}

func (h *harness) seedReward(t *testing.T, tenant string, mut func(*domain.RewardPatch)) domain.Reward {
	t.Helper()
	p := domain.RewardPatch{
		Name:       ptr(fmt.Sprintf("Reward %d", len(h.repo.rewards))),
		Status:     ptr(contracts.RewardActive),
		PointsCost: ptr(int64(100)),
	}
	if mut != nil {
		mut(&p)
	}
	r, err := domain.NewReward(tenant, p, t0)
	if err != nil {
		t.Fatalf("seed reward: %v", err)
	}
	h.repo.rewards[r.ID] = r
	return r
}

func (h *harness) reward(id string) domain.Reward { return h.repo.rewards[id] }
func (h *harness) claim(id string) domain.Claim   { return h.repo.claims[id] }
