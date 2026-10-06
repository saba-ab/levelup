package repo

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/badges/internal/app"
	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/modules/badges/internal/ports"
	missionscontracts "levelup/internal/modules/missions/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/id"
)

func applyStats(t *testing.T, r *Postgres, db *gorm.DB, tenant, player string, u app.StatsUpdate) {
	t.Helper()
	if u.At.IsZero() {
		u.At = now()
	}
	require.NoError(t, postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		return r.ApplyPlayerStats(context.Background(), tx, tenant, player, u)
	}))
}

func TestPlayerStatsUpsertSemantics(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	base := now()
	lp := func(v int64) *int64 { return &v }

	st, err := r.PlayerStats(ctx, tenant, player)
	require.NoError(t, err)
	require.Zero(t, st.LifetimePoints, "never seen: all zero")
	require.Empty(t, st.ActivityCounts)

	applyStats(t, r, db, tenant, player, app.StatsUpdate{LifetimePoints: lp(900), LifetimeAt: base.Add(time.Minute)})
	applyStats(t, r, db, tenant, player, app.StatsUpdate{LifetimePoints: lp(400), LifetimeAt: base}) // late, older
	applyStats(t, r, db, tenant, player, app.StatsUpdate{MissionsCompleted: 1, MaxStreak: 9, Level: 4})
	applyStats(t, r, db, tenant, player, app.StatsUpdate{MissionsCompleted: 1, MaxStreak: 3, Level: 2, BadgesEarned: 1})
	applyStats(t, r, db, tenant, player, app.StatsUpdate{ActivityType: "purchase"})
	applyStats(t, r, db, tenant, player, app.StatsUpdate{ActivityType: "purchase"})
	applyStats(t, r, db, tenant, player, app.StatsUpdate{ActivityType: "login"})

	st, err = r.PlayerStats(ctx, tenant, player)
	require.NoError(t, err)
	require.EqualValues(t, 900, st.LifetimePoints, "the newest ledger time wins")
	require.EqualValues(t, 2, st.MissionsCompleted)
	require.EqualValues(t, 9, st.MaxStreak)
	require.EqualValues(t, 4, st.Level)
	require.EqualValues(t, 1, st.BadgesEarned)
	require.Equal(t, map[string]int64{"purchase": 2, "login": 1}, st.ActivityCounts)

	applyStats(t, r, db, tenant, player, app.StatsUpdate{LifetimePoints: lp(1300), LifetimeAt: base.Add(2 * time.Minute)})
	applyStats(t, r, db, tenant, player, app.StatsUpdate{LifetimePoints: lp(1200), LifetimeAt: base.Add(2 * time.Minute)})
	st, err = r.PlayerStats(ctx, tenant, player)
	require.NoError(t, err)
	require.EqualValues(t, 1300, st.LifetimePoints, "a tie keeps the larger value")

	other, err := r.PlayerStats(ctx, id.NewID(), player)
	require.NoError(t, err)
	require.Zero(t, other.Level, "scoped per tenant")
}

func TestMarkEventAppliedAndPrune(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	mark := func(tenantID, key string, at time.Time) bool {
		var fresh bool
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			var err error
			fresh, err = r.MarkEventApplied(ctx, tx, tenantID, key, at)
			return err
		}))
		return fresh
	}
	old := now().Add(-1000 * time.Hour)
	require.True(t, mark(tenant, "k1", old))
	require.False(t, mark(tenant, "k1", now()), "second sighting")
	require.True(t, mark(id.NewID(), "k1", now()), "per tenant")
	require.True(t, mark(tenant, "k2", now()))

	n, err := r.PruneAppliedEvents(ctx, now().Add(-999*time.Hour), 100)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 1)
	require.True(t, mark(tenant, "k1", now()), "pruned key is gone")
	require.False(t, mark(tenant, "k2", now()), "recent key kept")
}

func TestAutoAwardBadgesAndPurge(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	with := seedBadge(t, r, db, tenant, nil)
	seedBadge(t, r, db, tenant, func(p *domain.NewBadgeParams) { p.Requirements = nil })
	seedBadge(t, r, db, tenant, func(p *domain.NewBadgeParams) { p.Active = false })
	seedBadge(t, r, db, id.NewID(), nil)

	got, err := r.AutoAwardBadges(ctx, tenant)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, with.ID, got[0].ID)
	_, ok := got[0].AutoRequirements()
	require.True(t, ok)

	applyStats(t, r, db, tenant, player, app.StatsUpdate{Level: 3, ActivityType: "x"})
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		_, err := r.MarkEventApplied(ctx, tx, tenant, "k", now())
		return err
	}))
	for range 2 {
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) }))
	}
	st, err := r.PlayerStats(ctx, tenant, player)
	require.NoError(t, err)
	require.Zero(t, st.Level)
	require.Empty(t, st.ActivityCounts)
	var fresh bool
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		fresh, err = r.MarkEventApplied(ctx, tx, tenant, "k", now())
		return err
	}))
	require.True(t, fresh, "dedupe keys purged with the tenant")
}

type allowAll struct{}

func (allowAll) Authorize(context.Context, authz.Principal, authz.Permission, any) error { return nil }

type countingOutbox struct {
	mu     sync.Mutex
	topics []string
}

func (o *countingOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, _ any) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.topics = append(o.topics, topic)
	return nil
}

func (o *countingOutbox) count(topic string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := 0
	for _, t := range o.topics {
		if t == topic {
			n++
		}
	}
	return n
}

type activePlayers struct{}

func (activePlayers) ByID(_ context.Context, tenantID, playerID string) (ports.PlayerSnapshot, bool, error) {
	return ports.PlayerSnapshot{ID: playerID, TenantID: tenantID, Active: true}, true, nil
}

func (activePlayers) ByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, i := range ids {
		out[i] = ports.PlayerSnapshot{ID: i, TenantID: tenantID, Active: true}
	}
	return out, nil
}

// Many concurrent deliveries of distinct mission completions, against real
// SQL: the projection counts each once and the badge is awarded exactly
// once, under the auto key.
func TestAutoAwardExactlyOnceUnderConcurrency(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	b := seedBadge(t, r, db, tenant, func(p *domain.NewBadgeParams) {
		p.Stackable = true // the auto key, not "already earned", must stop the second award
		p.Requirements = missionsReq(2)
	})
	ob := &countingOutbox{}
	svc := app.NewService(r, activePlayers{}, ob, allowAll{}, db, clock.System(), nil, nil)

	envs := make([]bus.Envelope, 8)
	for i := range envs {
		raw, err := json.Marshal(missionscontracts.CompletedV1{AttemptID: id.NewID(), TenantID: tenant, PlayerID: player, At: now()})
		require.NoError(t, err)
		envs[i] = bus.Envelope{EventID: id.NewID(), Topic: missionscontracts.TopicCompleted, Payload: raw}
	}
	var wg sync.WaitGroup
	errsCh := make(chan error, 2*len(envs))
	for range 2 { // every envelope delivered twice, concurrently
		for _, e := range envs {
			wg.Add(1)
			go func(e bus.Envelope) {
				defer wg.Done()
				errsCh <- svc.OnMissionCompleted(ctx, e)
			}(e)
		}
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		if err != nil {
			// A lost key race surfaces as Unavailable and is retried by the
			// worker; retry here the same way.
			require.Contains(t, err.Error(), "retry")
		}
	}
	for _, e := range envs {
		require.NoError(t, svc.OnMissionCompleted(ctx, e))
	}

	st, err := r.PlayerStats(ctx, tenant, player)
	require.NoError(t, err)
	require.EqualValues(t, len(envs), st.MissionsCompleted)
	pb, found, err := r.PlayerBadge(ctx, tenant, player, b.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 1, pb.EarnedCount)
	require.Equal(t, 1, ob.count(contracts.TopicAwarded))
	a, found, err := r.AwardByKey(ctx, tenant, contracts.AutoAwardKey(player, b.ID))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, effect.Source{Kind: contracts.SourceRequirements, ID: b.ID}, a.Source)
}

func TestAwardStatsAggregates(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	hot := seedBadge(t, r, db, tenant, nil)
	cold := seedBadge(t, r, db, tenant, nil)
	gone := seedBadge(t, r, db, tenant, nil)
	goneUnused := seedBadge(t, r, db, tenant, nil)
	p1, p2 := id.NewID(), id.NewID()
	today := time.Now().UTC().Truncate(24 * time.Hour).Add(time.Hour)
	days := func(n int) time.Time { return today.AddDate(0, 0, -n) }

	insert := func(badgeID, playerID string, status domain.AwardStatus, at time.Time) {
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
			_, err := r.InsertAward(ctx, tx, domain.Award{ID: id.NewID(), TenantID: tenant, PlayerID: playerID, BadgeID: badgeID,
				IdempotencyKey: id.NewID(), OccurredAt: at, Status: status, CreatedAt: at})
			return err
		}))
	}
	insert(hot.ID, p1, domain.AwardApplied, days(0))
	insert(hot.ID, p1, domain.AwardApplied, days(0))
	insert(hot.ID, p2, domain.AwardApplied, days(3))
	insert(hot.ID, p2, domain.AwardRejected, days(1))
	insert(gone.ID, p1, domain.AwardApplied, days(40))
	insert(id.NewID(), p1, domain.AwardRejected, days(0)) // unknown badge, rejected
	for _, b := range []domain.Badge{gone, goneUnused} {
		fresh, err := r.BadgeByID(ctx, tenant, b.ID)
		require.NoError(t, err)
		at := now()
		fresh.DeletedAt = &at
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveBadge(ctx, tx, fresh) }))
	}

	agg, err := r.AwardStats(ctx, tenant, days(29).Truncate(24*time.Hour))
	require.NoError(t, err)
	byID := map[string]app.BadgeStat{}
	for _, s := range agg.Badges {
		byID[s.BadgeID] = s
	}
	require.Len(t, agg.Badges, 3, "live badges plus the deleted one that was awarded")
	require.EqualValues(t, 3, byID[hot.ID].AwardedCount)
	require.EqualValues(t, 2, byID[hot.ID].UniquePlayers)
	require.NotNil(t, byID[hot.ID].LastAwardedAt)
	require.WithinDuration(t, days(0), *byID[hot.ID].LastAwardedAt, time.Millisecond)
	require.Zero(t, byID[cold.ID].AwardedCount)
	require.Nil(t, byID[cold.ID].LastAwardedAt)
	require.True(t, byID[gone.ID].Deleted)
	require.EqualValues(t, 1, byID[gone.ID].AwardedCount)
	require.Equal(t, map[string]int64{
		days(0).Format(time.DateOnly): 2,
		days(3).Format(time.DateOnly): 1,
	}, agg.PerDay, "applied only, inside the window")
	require.EqualValues(t, 4, agg.TotalAwarded)
	require.EqualValues(t, 2, agg.UniquePlayers)

	svc := app.NewService(r, activePlayers{}, &countingOutbox{}, allowAll{}, db, clock.System(), nil, nil)
	rep, err := svc.Stats(authz.Into(ctx, authz.Principal{UserID: id.NewID(), TenantID: tenant}))
	require.NoError(t, err)
	require.Len(t, rep.AwardsPerDay, app.StatsDays)
	require.Equal(t, app.DayCount{Day: days(0).Format(time.DateOnly), Count: 2}, rep.AwardsPerDay[app.StatsDays-1])
}
