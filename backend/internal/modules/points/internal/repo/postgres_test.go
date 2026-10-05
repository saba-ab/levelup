package repo

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/points/contracts"
	"levelup/internal/modules/points/internal/app"
	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/modules/points/internal/ports"
	"levelup/internal/modules/points/migrations"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/id"
)

// syncOutbox is a goroutine-safe recording outbox (the platform outbox
// table is not needed to prove points' SQL).
type syncOutbox struct {
	mu     sync.Mutex
	topics []string
}

func (o *syncOutbox) Publish(_ context.Context, tx *gorm.DB, topic string, _ any) error {
	if tx == nil {
		return fmt.Errorf("publish outside a transaction")
	}
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

// everyone treats every player as an active member of the asked tenant.
type everyone struct{}

func (everyone) PlayersByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, i := range ids {
		out[i] = ports.PlayerSnapshot{ID: i, TenantID: tenantID, Active: true}
	}
	return out, nil
}

type allowAll struct{}

func (allowAll) Authorize(context.Context, authz.Principal, authz.Permission, any) error { return nil }

var (
	setupOnce sync.Once
	sharedDB  *gorm.DB
)

func setup(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	setupOnce.Do(func() {
		ctx := context.Background()
		require.NoError(t, postgres.Apply(ctx, dsn, "points", migrations.FS))
		db, _, err := postgres.NewFromDSNs(ctx, dsn, "", 32)
		require.NoError(t, err)
		base, err := db.GormBase(nil)
		require.NoError(t, err)
		sharedDB = postgres.NewModuleDB(base, "points")
	})
	require.NotNil(t, sharedDB)
	return NewPostgres(sharedDB), sharedDB
}

func newService(t *testing.T) (*app.Service, *Postgres, *syncOutbox, string) {
	r, db := setup(t)
	ob := &syncOutbox{}
	svc := app.NewService(r, everyone{}, ob, allowAll{}, db, clock.System(), nil, nil)
	return svc, r, ob, id.NewID() // fresh tenant per test isolates rows
}

func credit(tenantID, playerID, key string, amount int64) contracts.CreditCmdV1 {
	return contracts.CreditCmdV1{IdempotencyKey: key, TenantID: tenantID, PlayerID: playerID, Amount: amount,
		Kind: contracts.KindEarn, Source: effect.Source{Kind: effect.SourceRule, ID: key}}
}

func debit(tenantID, playerID, key string, amount int64) contracts.DebitCmdV1 {
	return contracts.DebitCmdV1{IdempotencyKey: key, TenantID: tenantID, PlayerID: playerID, Amount: amount,
		Kind: contracts.KindSpend, Source: effect.Source{Kind: effect.SourceReward, ID: key}}
}

func admin(tenantID string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: id.NewID(), TenantID: tenantID})
}

func TestConcurrentDebitsNeverGoNegative(t *testing.T) {
	svc, r, ob, tenant := newService(t)
	ctx := context.Background()
	player := id.NewID()
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, player, "seed", 100)))

	var wg sync.WaitGroup
	errsCh := make(chan error, 20)
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errsCh <- svc.HandleDebit(ctx, debit(tenant, player, fmt.Sprintf("d%d", i), 30))
		}(i)
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		require.NoError(t, err, "rejections are results, never errors")
	}

	w, ok, err := r.WalletByPlayer(ctx, tenant, player)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(10), w.Balance.Minor(), "exactly three 30-point debits fit into 100")
	require.Equal(t, 3, ob.count(contracts.TopicDebited))
	require.Equal(t, 17, ob.count(contracts.TopicDebitRejected))
	require.Equal(t, 4, w.Version, "one credit + three debits")

	// The chain is gap-free.
	drift, err := r.FindDrift(ctx, time.Time{})
	require.NoError(t, err)
	for _, d := range drift {
		require.NotEqual(t, tenant, d.TenantID, "unexpected drift %+v", d)
	}
}

func TestRedeliveredCreditInsertsOnceConcurrently(t *testing.T) {
	svc, r, ob, tenant := newService(t)
	ctx := context.Background()
	player := id.NewID()
	cmd := credit(tenant, player, "same-key", 25)

	errsCh := make(chan error, 10)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errsCh <- svc.HandleCredit(ctx, cmd)
		}()
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		require.NoError(t, err)
	}

	w, _, err := r.WalletByPlayer(ctx, tenant, player)
	require.NoError(t, err)
	require.Equal(t, int64(25), w.Balance.Minor())
	require.Equal(t, 1, ob.count(contracts.TopicCredited))
	require.Equal(t, 1, ob.count(contracts.TopicWalletOpened))
	rows, err := r.ListEntries(ctx, tenant, player, app.LedgerFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestOppositeTransfersAreDeadlockFreeAndAtomic(t *testing.T) {
	svc, r, ob, tenant := newService(t)
	ctx := context.Background()
	a, b := id.NewID(), id.NewID()
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, a, "seed-a", 1000)))
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, b, "seed-b", 1000)))

	errsCh := make(chan error, 20)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from, to := a, b
			if i%2 == 1 {
				from, to = b, a
			}
			_, err := svc.Transfer(admin(tenant), app.TransferReq{
				FromPlayerID: from, ToPlayerID: to, Amount: 10, IdempotencyKey: fmt.Sprintf("t%d", i),
			})
			errsCh <- err
		}(i)
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		require.NoError(t, err, "opposite transfers must not deadlock")
	}

	wa, _, _ := r.WalletByPlayer(ctx, tenant, a)
	wb, _, _ := r.WalletByPlayer(ctx, tenant, b)
	require.Equal(t, int64(2000), wa.Balance.Minor()+wb.Balance.Minor(), "points are conserved")
	require.Equal(t, int64(1000), wa.Balance.Minor())
	require.Equal(t, 20, ob.count(contracts.TopicTransferred))

	legs, err := r.ListEntries(ctx, tenant, a, app.LedgerFilter{Kind: contracts.KindTransfer, Limit: 100})
	require.NoError(t, err)
	require.Len(t, legs, 20)
	for _, l := range legs {
		require.NotEmpty(t, l.TransferID)
	}
}

func TestTransferInsufficientRollsBackBothLegs(t *testing.T) {
	svc, r, _, tenant := newService(t)
	ctx := context.Background()
	a, b := id.NewID(), id.NewID()
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, a, "seed", 5)))
	_, err := svc.Transfer(admin(tenant), app.TransferReq{FromPlayerID: a, ToPlayerID: b, Amount: 10, IdempotencyKey: "t"})
	require.Error(t, err)
	_, ok, err := r.WalletByPlayer(ctx, tenant, b)
	require.NoError(t, err)
	require.False(t, ok)
	wa, _, _ := r.WalletByPlayer(ctx, tenant, a)
	require.Equal(t, int64(5), wa.Balance.Minor())
}

func TestRefundAppliesExactlyOnceConcurrently(t *testing.T) {
	svc, r, ob, tenant := newService(t)
	ctx := context.Background()
	player := id.NewID()
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, player, "seed", 100)))
	require.NoError(t, svc.HandleDebit(ctx, debit(tenant, player, "d1", 60)))

	errsCh := make(chan error, 8)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Half redeliver the same refund, half use other keys.
			key := "r-same"
			if i%2 == 1 {
				key = fmt.Sprintf("r-%d", i)
			}
			errsCh <- svc.HandleRefund(ctx, contracts.RefundCmdV1{
				IdempotencyKey: key, TenantID: tenant, DebitIdempotencyKey: "d1",
			})
		}(i)
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		require.NoError(t, err)
	}

	w, _, _ := r.WalletByPlayer(ctx, tenant, player)
	require.Equal(t, int64(100), w.Balance.Minor())
	require.Equal(t, int64(0), w.LifetimeSpent.Minor())
	require.Equal(t, 1, ob.count(contracts.TopicRefunded))

	drift, err := r.FindDrift(ctx, time.Time{})
	require.NoError(t, err)
	for _, d := range drift {
		require.NotEqual(t, tenant, d.TenantID, "refund must reconcile: %+v", d)
	}
}

func TestReconcileDetectsTamperedBalance(t *testing.T) {
	svc, r, _, tenant := newService(t)
	ctx := context.Background()
	player := id.NewID()
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, player, "c1", 100)))
	require.NoError(t, svc.HandleDebit(ctx, debit(tenant, player, "d1", 40)))

	require.NoError(t, sharedDB.Exec(
		`UPDATE points_svc.wallets SET balance = balance + 1000 WHERE tenant_id = ? AND player_id = ?`,
		tenant, player).Error)

	drift, err := r.FindDrift(ctx, time.Time{})
	require.NoError(t, err)
	var mine []domain.Drift
	for _, d := range drift {
		if d.TenantID == tenant {
			mine = append(mine, d)
		}
	}
	require.Len(t, mine, 1)
	require.Equal(t, int64(1060), mine[0].Balance)
	require.Equal(t, int64(60), mine[0].LedgerBalance)
	require.Equal(t, int64(0), mine[0].ChainBreaks)

	// The job itself never fixes the balance and records its run.
	require.NoError(t, svc.Reconcile(ctx, r))
	w, _, _ := r.WalletByPlayer(ctx, tenant, player)
	require.Equal(t, int64(1060), w.Balance.Minor())
	last, err := r.LastRun(ctx)
	require.NoError(t, err)
	require.False(t, last.IsZero())
}

func TestSaveWalletDetectsVersionConflict(t *testing.T) {
	r, db := setup(t)
	ctx := context.Background()
	w := domain.NewWallet(id.NewID(), id.NewID(), time.Now().UTC())
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		created, err := r.EnsureWallet(ctx, tx, w)
		require.True(t, created)
		return err
	}))
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveWallet(ctx, tx, w) }))
	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveWallet(ctx, tx, w) })
	require.ErrorIs(t, err, domain.ErrVersionConflict)
}

func TestBalanceCheckConstraintBacksTheDomain(t *testing.T) {
	r, db := setup(t)
	ctx := context.Background()
	w := domain.NewWallet(id.NewID(), id.NewID(), time.Now().UTC())
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		_, err := r.EnsureWallet(ctx, tx, w)
		return err
	}))
	w.Balance = -1
	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveWallet(ctx, tx, w) })
	require.Error(t, err, "CHECK (balance >= 0) refuses an overdraft even if the domain were bypassed")
}

func TestPurgeTenantRemovesEverythingIdempotently(t *testing.T) {
	svc, r, _, tenant := newService(t)
	ctx := context.Background()
	player := id.NewID()
	require.NoError(t, svc.HandleCredit(ctx, credit(tenant, player, "c1", 100)))
	require.NoError(t, svc.HandleDebit(ctx, debit(tenant, player, "d1", 40)))
	require.NoError(t, svc.HandleRefund(ctx, contracts.RefundCmdV1{IdempotencyKey: "r1", TenantID: tenant, DebitIdempotencyKey: "d1"}))
	require.NoError(t, svc.HandleDebit(ctx, debit(tenant, player, "d2", 1000))) // rejection row

	require.NoError(t, svc.PurgeTenant(ctx, tenant))
	require.NoError(t, svc.PurgeTenant(ctx, tenant))
	_, ok, err := r.WalletByPlayer(ctx, tenant, player)
	require.NoError(t, err)
	require.False(t, ok)
	_, ok, err = r.RejectionByKey(ctx, nil, tenant, "d2")
	require.NoError(t, err)
	require.False(t, ok)
}
