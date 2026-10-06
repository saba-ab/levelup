package repo_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/rewards/contracts"
	"levelup/internal/modules/rewards/internal/app"
	"levelup/internal/modules/rewards/internal/domain"
	"levelup/internal/modules/rewards/internal/ports"
	"levelup/internal/modules/rewards/internal/repo"
	"levelup/internal/modules/rewards/migrations"
	"levelup/internal/platform/authz"
	authzmigrations "levelup/internal/platform/authz/migrations"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

func setup(t *testing.T) (*repo.Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "authz", authzmigrations.FS))
	require.NoError(t, postgres.Apply(ctx, dsn, "rewards", migrations.FS, migrations.Go()...))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 16)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "rewards")
	return repo.NewPostgres(moduleDB), moduleDB
}

func ptr[T any](v T) *T { return &v }

func seedReward(t *testing.T, r *repo.Postgres, db *gorm.DB, tenant string, mut func(*domain.RewardPatch)) domain.Reward {
	t.Helper()
	p := domain.RewardPatch{Name: ptr("Reward " + id.NewID()), Status: ptr(contracts.RewardActive), PointsCost: ptr(int64(100)),
		Metadata: map[string]any{"color": "red"}, Value: ptr("10.00"), ValueType: ptr("fixed")}
	if mut != nil {
		mut(&p)
	}
	rw, err := domain.NewReward(tenant, p, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		return r.CreateReward(context.Background(), tx, rw)
	}))
	return rw
}

type nopOutbox struct {
	mu     sync.Mutex
	topics []string
}

func (o *nopOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, _ any) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.topics = append(o.topics, topic)
	return nil
}

type allow struct{}

func (allow) Authorize(context.Context, authz.Principal, authz.Permission, any) error { return nil }

type players struct{}

func (players) PlayersByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, i := range ids {
		out[i] = ports.PlayerSnapshot{ID: i, TenantID: tenantID, Active: true}
	}
	return out, nil
}

type levels struct{}

func (levels) LevelsByPlayerIDs(context.Context, string, []string) (map[string]int, error) {
	return map[string]int{}, nil
}

type noOutcome struct{}

func (noOutcome) OutcomeByKey(context.Context, string, string) (ports.PaymentOutcome, bool, error) {
	return ports.PaymentOutcome{}, false, nil
}

func TestRewardRoundTripAndSlugUniqueness(t *testing.T) {
	r, db := setup(t)
	ctx := context.Background()
	tenant := id.NewID()
	rw := seedReward(t, r, db, tenant, func(p *domain.RewardPatch) { p.Slug = ptr("mug") })

	got, err := r.RewardByID(ctx, tenant, rw.ID, false)
	require.NoError(t, err)
	require.Equal(t, "mug", got.Slug)
	require.Equal(t, "10.00", *got.Value)
	require.Equal(t, "red", got.Metadata["color"])

	_, err = r.RewardByID(ctx, id.NewID(), rw.ID, false)
	require.ErrorIs(t, err, domain.ErrRewardNotFound, "another tenant's row is not found")
	_, err = r.RewardByID(ctx, tenant, "not-a-uuid", false)
	require.ErrorIs(t, err, domain.ErrRewardNotFound)

	dup, err := domain.NewReward(tenant, domain.RewardPatch{Name: ptr("Mug"), Slug: ptr("mug")}, time.Now().UTC())
	require.NoError(t, err)
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateReward(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrSlugTaken)

	// Stale version is refused.
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveReward(ctx, tx, got) }))
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveReward(ctx, tx, got) })
	require.ErrorIs(t, err, domain.ErrVersionConflict)

	// Soft delete frees the slug.
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		return r.SoftDeleteReward(ctx, tx, tenant, rw.ID, time.Now().UTC())
	}))
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateReward(ctx, tx, dup) }))
	_, err = r.RewardByID(ctx, tenant, rw.ID, true)
	require.NoError(t, err, "settlement still reads deleted rewards")
}

func TestClaimUniquenessMapsToDomainErrors(t *testing.T) {
	r, db := setup(t)
	ctx := context.Background()
	tenant := id.NewID()
	rw := seedReward(t, r, db, tenant, nil)

	mk := func() domain.Claim {
		return domain.NewClaim(rw, id.NewID(), false, time.Minute, nil, time.Now().UTC())
	}
	insert := func(c domain.Claim) error {
		return postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.InsertClaim(ctx, tx, c) })
	}

	a := mk()
	a.ClientRequestID = ptr("req-1")
	a.Code = ptr("AAAA-BBBB-CCCC")
	require.NoError(t, insert(a))

	b := mk()
	b.ClientRequestID = ptr("req-1")
	require.ErrorIs(t, insert(b), domain.ErrDuplicateRequest)

	g1 := mk()
	g1.GrantKey = ptr("rule:1")
	require.NoError(t, insert(g1))
	g2 := mk()
	g2.GrantKey = ptr("rule:1")
	require.ErrorIs(t, insert(g2), domain.ErrDuplicateRequest)

	c := mk()
	c.Code = ptr("AAAA-BBBB-CCCC")
	require.ErrorIs(t, insert(c), domain.ErrCodeCollision)

	// Same key in another tenant is fine.
	other := seedReward(t, r, db, id.NewID(), nil)
	d := domain.NewClaim(other, id.NewID(), false, time.Minute, nil, time.Now().UTC())
	d.ClientRequestID = ptr("req-1")
	d.Code = ptr("AAAA-BBBB-CCCC")
	require.NoError(t, insert(d))

	got, err := r.ClaimByClientRequest(ctx, tenant, "req-1")
	require.NoError(t, err)
	require.Equal(t, a.ID, got.ID)
}

func TestSweepQueriesAndMarkers(t *testing.T) {
	r, db := setup(t)
	ctx := context.Background()
	tenant := id.NewID()
	rw := seedReward(t, r, db, tenant, nil)
	now := time.Now().UTC()

	pending := domain.NewClaim(rw, id.NewID(), false, -time.Minute, nil, now)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.InsertClaim(ctx, tx, pending) }))

	due, err := r.DuePendingClaims(ctx, now, 1000)
	require.NoError(t, err)
	require.Contains(t, claimIDs(due), pending.ID)

	var n int
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		var err error
		n, err = r.CountHeldClaims(ctx, tx, tenant, rw.ID, pending.PlayerID)
		return err
	}))
	require.Equal(t, 1, n)

	job := app.JobClaimsReconcile + "." + id.NewID()
	last, err := r.LastRun(ctx, job)
	require.NoError(t, err)
	require.True(t, last.IsZero())
	require.NoError(t, r.MarkRun(ctx, job, now))
	require.NoError(t, r.MarkRun(ctx, job, now.Add(time.Minute)))
	last, err = r.LastRun(ctx, job)
	require.NoError(t, err)
	require.WithinDuration(t, now.Add(time.Minute), last, time.Millisecond)

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) }))
	_, err = r.ClaimByID(ctx, tenant, pending.ID)
	require.ErrorIs(t, err, domain.ErrClaimNotFound)
}

// The last unit of stock with concurrent claims: exactly one wins, the
// counter never exceeds the cap (doc 05 R1/R2).
func TestConcurrentClaimsOnLastUnit(t *testing.T) {
	r, db := setup(t)
	tenant := id.NewID()
	rw := seedReward(t, r, db, tenant, func(p *domain.RewardPatch) { p.MaxRedemptions = ptr(1) })

	ob := &nopOutbox{}
	svc := app.NewService(r, players{}, levels{}, noOutcome{}, ob, allow{}, db, clock.System(), app.Settings{})
	ctx := authz.Into(context.Background(), authz.Principal{UserID: "u", TenantID: tenant})

	const n = 12
	var wg sync.WaitGroup
	results := make(chan error, n)
	for range n {
		wg.Go(func() {
			_, _, err := svc.Claim(ctx, rw.ID, id.NewID(), "")
			results <- err
		})
	}
	wg.Wait()
	close(results)

	wins := 0
	for err := range results {
		if err == nil {
			wins++
			continue
		}
		require.ErrorIs(t, err, domain.ErrRewardDepleted)
	}
	require.Equal(t, 1, wins)
	got, err := r.RewardByID(context.Background(), tenant, rw.ID, false)
	require.NoError(t, err)
	require.Equal(t, 1, got.StockUsed)
	require.Equal(t, contracts.RewardDepleted, got.Status)
}

// Concurrent replays of one Idempotency-Key produce one claim.
func TestConcurrentSameIdempotencyKey(t *testing.T) {
	r, db := setup(t)
	tenant := id.NewID()
	rw := seedReward(t, r, db, tenant, nil)
	svc := app.NewService(r, players{}, levels{}, noOutcome{}, &nopOutbox{}, allow{}, db, clock.System(), app.Settings{})
	ctx := authz.Into(context.Background(), authz.Principal{UserID: "u", TenantID: tenant})
	player := id.NewID()

	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errc := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			c, _, err := svc.Claim(ctx, rw.ID, player, "same-key")
			errc <- err
			ids <- c.ID
		})
	}
	wg.Wait()
	close(ids)
	close(errc)
	for err := range errc {
		require.NoError(t, err)
	}
	seen := map[string]bool{}
	for i := range ids {
		seen[i] = true
	}
	require.Len(t, seen, 1)
	got, err := r.RewardByID(context.Background(), tenant, rw.ID, false)
	require.NoError(t, err)
	require.Equal(t, 1, got.StockUsed)
}

func TestPermissionSeedGrantsRoles(t *testing.T) {
	_, db := setup(t)
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM authz_svc.permissions WHERE module = 'rewards'`).Scan(&n).Error)
	require.EqualValues(t, len(contracts.AllPermissions), n)
	require.NoError(t, db.Raw(`SELECT count(*) FROM authz_svc.casbin_rule WHERE v0 = 'role:6' AND v1 = 'rewards:cancel'`).Scan(&n).Error)
	require.Zero(t, n, "cancel is admin-only")
	require.NoError(t, db.Raw(`SELECT count(*) FROM authz_svc.casbin_rule WHERE v0 = 'role:4' AND v1 = 'rewards:cancel'`).Scan(&n).Error)
	require.EqualValues(t, 1, n)
}

func claimIDs(cs []domain.Claim) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

// The saga's settle and cancel paths against real SQL: claimed with a
// voucher code, rejected releases stock, late debit after cancel refunds.
func TestSagaSettlementAgainstPostgres(t *testing.T) {
	r, db := setup(t)
	tenant := id.NewID()
	rw := seedReward(t, r, db, tenant, func(p *domain.RewardPatch) {
		p.Type = ptr(contracts.TypeDiscount)
		p.ClaimTTLDays = ptr(30)
		p.MaxRedemptions = ptr(3)
	})
	ob := &nopOutbox{}
	svc := app.NewService(r, players{}, levels{}, noOutcome{}, ob, allow{}, db, clock.System(), app.Settings{})
	ctx := authz.Into(context.Background(), authz.Principal{UserID: "u", TenantID: tenant})
	bg := context.Background()

	paid, _, err := svc.Claim(ctx, rw.ID, id.NewID(), "")
	require.NoError(t, err)
	require.NoError(t, svc.OnDebited(bg, pointscontracts.LedgerMovedV1{IdempotencyKey: paid.DebitKey, TenantID: tenant}))
	got, err := r.ClaimByID(bg, tenant, paid.ID)
	require.NoError(t, err)
	require.Equal(t, contracts.ClaimClaimed, got.Status)
	require.NotNil(t, got.Code)
	require.NotNil(t, got.ExpiresAt)

	refused, _, err := svc.Claim(ctx, rw.ID, id.NewID(), "")
	require.NoError(t, err)
	require.NoError(t, svc.OnDebitRejected(bg, pointscontracts.MoveRejectedV1{IdempotencyKey: refused.DebitKey, TenantID: tenant, Reason: "insufficient_balance"}))
	got, err = r.ClaimByID(bg, tenant, refused.ID)
	require.NoError(t, err)
	require.Equal(t, contracts.ClaimRejected, got.Status)

	late, _, err := svc.Claim(ctx, rw.ID, id.NewID(), "")
	require.NoError(t, err)
	_, err = svc.Cancel(ctx, late.ID)
	require.NoError(t, err)
	require.NoError(t, svc.OnDebited(bg, pointscontracts.LedgerMovedV1{IdempotencyKey: late.DebitKey, TenantID: tenant}))
	got, err = r.ClaimByID(bg, tenant, late.ID)
	require.NoError(t, err)
	require.Equal(t, contracts.ClaimRefundPending, got.Status)

	reward, err := r.RewardByID(bg, tenant, rw.ID, false)
	require.NoError(t, err)
	require.Equal(t, 1, reward.StockUsed, "only the settled claim holds stock")

	redeemed, err := svc.Redeem(ctx, paid.ID)
	require.NoError(t, err)
	require.Equal(t, contracts.ClaimRedeemed, redeemed.Status)
	list, _, err := svc.ListPlayerClaims(ctx, paid.PlayerID, "", 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
}

// Fulfilment, the tenant-wide history and the stats aggregate against real
// SQL: fulfilled_at persists, a redelivered grant delivers once, the
// filters and the keyset work, and the FILTER aggregates match.
func TestFulfilmentHistoryAndStatsAgainstPostgres(t *testing.T) {
	r, db := setup(t)
	tenant := id.NewID()
	points := seedReward(t, r, db, tenant, func(p *domain.RewardPatch) {
		p.PointsCost = ptr(int64(0))
		p.Value = ptr("300")
	})
	paid := seedReward(t, r, db, tenant, func(p *domain.RewardPatch) { p.PointsCost = ptr(int64(40)) })
	ob := &nopOutbox{}
	svc := app.NewService(r, players{}, levels{}, noOutcome{}, ob, allow{}, db, clock.System(), app.Settings{})
	ctx := authz.Into(context.Background(), authz.Principal{UserID: "u", TenantID: tenant})
	bg := context.Background()
	p1, p2 := id.NewID(), id.NewID()

	claimed, _, err := svc.Claim(ctx, points.ID, p1, "")
	require.NoError(t, err)
	redeemed, err := svc.Redeem(ctx, claimed.ID)
	require.NoError(t, err)
	require.NotNil(t, redeemed.FulfilledAt)
	got, err := r.ClaimByID(bg, tenant, claimed.ID)
	require.NoError(t, err)
	require.NotNil(t, got.FulfilledAt, "fulfilled_at round-trips")
	require.WithinDuration(t, *redeemed.FulfilledAt, *got.FulfilledAt, time.Millisecond)

	ob.topics = nil
	cmd := contracts.GrantCmdV1{IdempotencyKey: "rule:" + id.NewID(), TenantID: tenant, PlayerID: p2, RewardID: points.ID}
	require.NoError(t, svc.Grant(bg, cmd))
	require.NoError(t, svc.Grant(bg, cmd))
	require.Equal(t, []string{pointscontracts.Topic(pointscontracts.JobCredit), contracts.TopicClaimed}, ob.topics)
	granted, err := r.ClaimByGrantKey(bg, tenant, cmd.IdempotencyKey)
	require.NoError(t, err)
	require.NotNil(t, granted.FulfilledAt)

	pc, _, err := svc.Claim(ctx, paid.ID, p1, "")
	require.NoError(t, err)
	require.NoError(t, svc.OnDebited(bg, pointscontracts.LedgerMovedV1{IdempotencyKey: pc.DebitKey, TenantID: tenant}))

	all, next, err := svc.ListClaims(ctx, app.ClaimFilter{}, "", 2)
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.NotEmpty(t, next)
	rest, next, err := svc.ListClaims(ctx, app.ClaimFilter{}, next, 2)
	require.NoError(t, err)
	require.Len(t, rest, 1)
	require.Empty(t, next)
	require.ElementsMatch(t, []string{claimed.ID, granted.ID, pc.ID}, append(claimIDs(all), claimIDs(rest)...))

	byStatus, _, err := svc.ListClaims(ctx, app.ClaimFilter{Status: contracts.ClaimRedeemed}, "", 10)
	require.NoError(t, err)
	require.Equal(t, []string{claimed.ID}, claimIDs(byStatus))
	byPlayer, _, err := svc.ListClaims(ctx, app.ClaimFilter{PlayerID: p1, RewardID: paid.ID}, "", 10)
	require.NoError(t, err)
	require.Equal(t, []string{pc.ID}, claimIDs(byPlayer))
	future := time.Now().Add(time.Hour)
	none, _, err := svc.ListClaims(ctx, app.ClaimFilter{From: &future}, "", 10)
	require.NoError(t, err)
	require.Empty(t, none)
	past := time.Now().Add(-time.Hour)
	window, _, err := svc.ListClaims(ctx, app.ClaimFilter{From: &past, To: &future}, "", 10)
	require.NoError(t, err)
	require.Len(t, window, 3)
	other, err := r.ListClaims(bg, id.NewID(), app.ClaimFilter{}, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, other)

	// A deleted reward without claims drops out of the stats.
	gone := seedReward(t, r, db, tenant, nil)
	require.NoError(t, postgres.InTx(bg, db, func(tx *gorm.DB) error {
		return r.SoftDeleteReward(bg, tx, tenant, gone.ID, time.Now())
	}))
	rep, err := svc.Stats(ctx)
	require.NoError(t, err)
	require.Len(t, rep.Rewards, 2)
	byID := map[string]app.RewardStats{}
	for _, s := range rep.Rewards {
		byID[s.RewardID] = s
	}
	require.Equal(t, app.RewardStats{RewardID: points.ID, Slug: points.Slug, Name: points.Name, Type: points.Type,
		Claimed: 2, Redeemed: 1}, byID[points.ID])
	require.Equal(t, app.RewardStats{RewardID: paid.ID, Slug: paid.Slug, Name: paid.Name, Type: paid.Type,
		Claimed: 1, PointsSpent: 40}, byID[paid.ID])
	require.Equal(t, app.RewardStats{Claimed: 3, Redeemed: 1, PointsSpent: 40}, rep.Totals)
}
