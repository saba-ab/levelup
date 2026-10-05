package repo_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/missions/internal/app"
	"levelup/internal/modules/missions/internal/domain"
	"levelup/internal/modules/missions/internal/ports"
	"levelup/internal/modules/missions/internal/repo"
	"levelup/internal/modules/missions/migrations"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*repo.Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "missions", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 8)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "missions")
	return repo.NewPostgres(moduleDB), moduleDB
}

func seedMission(t *testing.T, r *repo.Postgres, db *gorm.DB, tenantID string, mod func(*domain.NewMissionParams)) domain.Mission {
	t.Helper()
	p := domain.NewMissionParams{
		TenantID: tenantID, Name: "M " + id.NewID()[24:], Type: contracts.TypeRepeating,
		Status: contracts.MissionActive, Target: 3, Criteria: map[string]any{"event_type": "purchase_completed"},
	}
	if mod != nil {
		mod(&p)
	}
	m, err := domain.NewMission(p, time.Now().UTC().Truncate(time.Microsecond))
	require.NoError(t, err)
	require.NoError(t, postgres.InTx(context.Background(), db, func(tx *gorm.DB) error {
		return r.CreateMission(context.Background(), tx, m)
	}))
	return m
}

func TestMissionRoundTripAndSlugUniquePerTenant(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, other := id.NewID(), id.NewID()
	m := seedMission(t, r, db, tenant, func(p *domain.NewMissionParams) { p.Slug = "same" })

	got, err := r.MissionByID(ctx, tenant, m.ID)
	require.NoError(t, err)
	require.Equal(t, "purchase_completed", got.Criteria["event_type"], "JSONB criteria round-trips")
	_, err = r.MissionByID(ctx, other, m.ID)
	require.ErrorIs(t, err, domain.ErrMissionNotFound, "another tenant's mission is not found")
	_, err = r.MissionByID(ctx, tenant, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrMissionNotFound)

	dup, err := domain.NewMission(domain.NewMissionParams{TenantID: tenant, Slug: "same", Name: "x",
		Type: contracts.TypeDaily, Target: 1}, time.Now())
	require.NoError(t, err)
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateMission(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrSlugTaken)

	seedMission(t, r, db, other, func(p *domain.NewMissionParams) { p.Slug = "same" }) // other tenant may reuse it
}

func TestSaveMissionVersionGuard(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	m := seedMission(t, r, db, id.NewID(), nil)
	m.Name = "renamed"
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveMission(ctx, tx, m) }))
	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveMission(ctx, tx, m) })
	require.ErrorIs(t, err, domain.ErrVersionConflict, "a stale version is refused")
}

func TestProgressEventInsertIsIdempotent(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	ev := domain.ProgressEvent{
		ID: id.NewID(), TenantID: tenant, IdempotencyKey: "k", MissionID: id.NewID(), PlayerID: id.NewID(),
		Increment: 1, Status: domain.ProgressApplied, CreatedAt: time.Now().UTC(),
	}
	var first, second bool
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) (err error) {
		first, err = r.InsertProgressEvent(ctx, tx, ev)
		return err
	}))
	ev.ID = id.NewID()
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) (err error) {
		second, err = r.InsertProgressEvent(ctx, tx, ev)
		return err
	}))
	require.True(t, first)
	require.False(t, second, "same (tenant, key) inserts nothing")
}

func TestOneOpenAttemptPerPeriod(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	m := seedMission(t, r, db, id.NewID(), nil)
	player := id.NewID()
	now := time.Now().UTC().Truncate(time.Microsecond)

	insert := func(a domain.Attempt) bool {
		var ok bool
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) (err error) {
			ok, err = r.InsertAttempt(ctx, tx, a)
			return err
		}))
		return ok
	}
	a := domain.StartAttempt(m, player, now)
	require.True(t, insert(a))
	require.False(t, insert(domain.StartAttempt(m, player, now)), "second open attempt for the period is refused")

	_, err := a.AddProgress(3, now)
	require.NoError(t, err)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveAttempt(ctx, tx, a) }))
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveAttempt(ctx, tx, a) })
	require.ErrorIs(t, err, domain.ErrVersionConflict)

	require.True(t, insert(domain.StartAttempt(m, player, now)), "after completion a new attempt may open")
	var total int
	var inPeriod bool
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) (err error) {
		total, inPeriod, err = r.AttemptCounts(ctx, tx, m.TenantID, m.ID, player, domain.PeriodAll)
		return err
	}))
	require.Equal(t, 1, total)
	require.True(t, inPeriod)

	counts, err := r.CompletedCounts(ctx, m.TenantID, []string{player})
	require.NoError(t, err)
	require.Equal(t, map[string]int{player: 1}, counts)
}

func TestDueRowsAndPurge(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	starts := now.Add(-2 * time.Hour)
	ended := seedMission(t, r, db, tenant, func(p *domain.NewMissionParams) { p.StartsAt = &starts; p.EndsAt = &past })
	daily := seedMission(t, r, db, tenant, func(p *domain.NewMissionParams) { p.Type = contracts.TypeDaily })

	stale := domain.StartAttempt(daily, id.NewID(), now.Add(-48*time.Hour))
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		_, err := r.InsertAttempt(ctx, tx, stale)
		return err
	}))

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		due, err := r.DueMissions(ctx, tx, now, 100)
		require.NoError(t, err)
		ids := map[string]bool{}
		for _, m := range due {
			ids[m.ID] = true
		}
		require.True(t, ids[ended.ID])
		require.False(t, ids[daily.ID])

		attempts, err := r.DueAttempts(ctx, tx, now, 100)
		require.NoError(t, err)
		found := false
		for _, a := range attempts {
			found = found || a.ID == stale.ID
		}
		require.True(t, found, "an open attempt past its period is due")
		return nil
	}))

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) }))
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) }))
	_, err := r.MissionByID(ctx, tenant, daily.ID)
	require.ErrorIs(t, err, domain.ErrMissionNotFound)
}

type syncOutbox struct {
	mu     sync.Mutex
	topics []string
}

func (o *syncOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, _ any) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.topics = append(o.topics, topic)
	return nil
}

func (o *syncOutbox) count(topic string) int {
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

type onePlayer struct{ id, tenant string }

func (p onePlayer) PlayersByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, x := range ids {
		if x == p.id && tenantID == p.tenant {
			out[x] = ports.PlayerSnapshot{ID: x, TenantID: tenantID, Active: true}
		}
	}
	return out, nil
}

// Concurrent redelivery of one command plus concurrent distinct commands:
// the duplicate applies once, and the attempt completes exactly once.
func TestConcurrentProgressCompletesExactlyOnce(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant, player := id.NewID(), id.NewID()
	m := seedMission(t, r, db, tenant, func(p *domain.NewMissionParams) {
		p.Type = contracts.TypeOneTime
		p.Target = 5
		p.PointsReward = 10
	})
	ob := &syncOutbox{}
	svc := app.NewService(r, onePlayer{player, tenant}, ob, authz.AllowAll{}, db, clock.System(), 100)

	var wg sync.WaitGroup
	errsCh := make(chan error, 20)
	for i := range 20 {
		key := "dup"
		if i%2 == 0 {
			key = "k-" + id.NewID()
		}
		wg.Go(func() {
			errsCh <- svc.HandleProgressJob(ctx, contracts.ProgressCmdV1{
				IdempotencyKey: key, TenantID: tenant, PlayerID: player, MissionID: m.ID, Increment: 1,
				Source: effect.Source{Kind: effect.SourceRule, ID: "x"},
			})
		})
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		require.NoError(t, err)
	}
	require.Equal(t, 1, ob.count(contracts.TopicCompleted))
	require.Equal(t, 1, ob.count("job.points.credit"))
	counts, err := r.CompletedCounts(ctx, tenant, []string{player})
	require.NoError(t, err)
	require.Equal(t, 1, counts[player])
}
