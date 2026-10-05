package app

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"levelup/internal/modules/points/contracts"
	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/modules/points/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

var (
	tenantA  = id.Derive("tenant", "a")
	tenantB  = id.Derive("tenant", "b")
	alice    = id.Derive("player", "alice")
	bob      = id.Derive("player", "bob")
	sleepy   = id.Derive("player", "inactive")
	foreign  = id.Derive("player", "foreign") // lives in tenantB
	adminID  = id.Derive("user", "admin")
	allPerms = allowKeys{
		contracts.PermViewWallet.Key(): true, contracts.PermViewTransactions.Key(): true,
		contracts.PermCredit.Key(): true, contracts.PermDebit.Key(): true,
		contracts.PermTransfer.Key(): true, contracts.PermManageWallet.Key(): true,
	}
)

func players() *fakePlayers {
	return &fakePlayers{players: map[string]ports.PlayerSnapshot{
		alice:   {ID: alice, TenantID: tenantA, Active: true},
		bob:     {ID: bob, TenantID: tenantA, Active: true},
		sleepy:  {ID: sleepy, TenantID: tenantA, Active: false},
		foreign: {ID: foreign, TenantID: tenantB, Active: true},
	}}
}

type fixture struct {
	svc     *Service
	repo    *fakeRepo
	ob      *fakeOutbox
	players *fakePlayers
}

func newFixture(t *testing.T, enf authz.Enforcer) fixture {
	t.Helper()
	repo, ob, pl := newFakeRepo(), &fakeOutbox{}, players()
	svc, _ := newTestService(repo, pl, ob, enf)
	return fixture{svc: svc, repo: repo, ob: ob, players: pl}
}

func asAdmin() context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: adminID, TenantID: tenantA, RoleIDs: []int64{2}})
}

func creditCmd(key, player string, amount int64) contracts.CreditCmdV1 {
	return contracts.CreditCmdV1{
		IdempotencyKey: key, TenantID: tenantA, PlayerID: player, Amount: amount,
		Kind: contracts.KindEarn, Source: effect.Source{Kind: effect.SourceRule, ID: "r1"},
		OccurredAt: testNow,
	}
}

func debitCmd(key, player string, amount int64) contracts.DebitCmdV1 {
	return contracts.DebitCmdV1{
		IdempotencyKey: key, TenantID: tenantA, PlayerID: player, Amount: amount,
		Kind: contracts.KindRedeem, Source: effect.Source{Kind: effect.SourceReward, ID: "claim-1"},
	}
}

func balance(t *testing.T, f fixture, tenantID, playerID string) int64 {
	t.Helper()
	w, ok := f.repo.wallet(tenantID, playerID)
	require.True(t, ok, "wallet must exist")
	return w.Balance.Minor()
}

// ---- job: credit ----

func TestCreditCommandOpensWalletAndPublishes(t *testing.T) {
	f := newFixture(t, allowKeys{})
	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("k1", alice, 100)))

	require.Equal(t, int64(100), balance(t, f, tenantA, alice))
	require.Equal(t, []string{contracts.TopicWalletOpened, contracts.TopicCredited}, f.ob.topics())
	ev := f.ob.last(contracts.TopicCredited).(contracts.LedgerMovedV1)
	require.Equal(t, int64(100), ev.BalanceAfter)
	require.Equal(t, "k1", ev.IdempotencyKey)
	require.Equal(t, effect.SourceRule, ev.Source.Kind)
	require.Len(t, f.repo.entries, 1)
	require.Equal(t, 1, f.repo.entries[0].WalletVersion)
}

func TestRedeliveredCreditInsertsOnce(t *testing.T) {
	f := newFixture(t, allowKeys{})
	cmd := creditCmd("k1", alice, 100)
	require.NoError(t, f.svc.HandleCredit(context.Background(), cmd))
	calls := f.players.calls
	require.NoError(t, f.svc.HandleCredit(context.Background(), cmd))

	require.Len(t, f.repo.entries, 1)
	require.Equal(t, int64(100), balance(t, f, tenantA, alice))
	require.Equal(t, 1, f.ob.count(contracts.TopicCredited))
	require.Equal(t, calls, f.players.calls, "a settled key must not even consult the player port")
}

func TestCreditCommandMalformedIsInvalid(t *testing.T) {
	f := newFixture(t, allowKeys{})
	bad := []contracts.CreditCmdV1{
		func() contracts.CreditCmdV1 { c := creditCmd("", alice, 1); return c }(),
		func() contracts.CreditCmdV1 { c := creditCmd("k", alice, 1); c.TenantID = "nope"; return c }(),
		func() contracts.CreditCmdV1 { c := creditCmd("k", "nope", 1); return c }(),
		func() contracts.CreditCmdV1 { c := creditCmd("k", alice, 0); return c }(),
		// B6: a debit kind on the credit command never adds points.
		func() contracts.CreditCmdV1 { c := creditCmd("k", alice, 5); c.Kind = contracts.KindPenalty; return c }(),
	}
	for _, c := range bad {
		err := f.svc.HandleCredit(context.Background(), c)
		require.Equal(t, errs.Invalid, errs.KindOf(err), "%+v", c)
	}
	require.Empty(t, f.ob.topics())
	require.Empty(t, f.repo.entries)
}

func TestCreditCommandRejectsUnknownAndInactivePlayer(t *testing.T) {
	f := newFixture(t, allowKeys{})
	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("k-unknown", id.Derive("player", "ghost"), 10)))
	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("k-sleepy", sleepy, 10)))
	// A player of another tenant is unknown here.
	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("k-foreign", foreign, 10)))

	require.Equal(t, 3, f.ob.count(contracts.TopicCreditRejected))
	require.Empty(t, f.repo.entries)
	require.Empty(t, f.repo.wallets, "no wallet is opened for a rejected player")

	out, ok, err := f.svc.OutcomeByKey(context.Background(), tenantA, "k-sleepy")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, contracts.OutcomeRejected, out.Status)
	require.Equal(t, effect.ReasonPlayerInactive, out.Reason)

	out, _, _ = f.svc.OutcomeByKey(context.Background(), tenantA, "k-foreign")
	require.Equal(t, effect.ReasonPlayerNotFound, out.Reason)

	// Redelivery of a rejected command publishes nothing new.
	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("k-sleepy", sleepy, 10)))
	require.Equal(t, 3, f.ob.count(contracts.TopicCreditRejected))
}

func TestCreditCommandRejectsInactiveWallet(t *testing.T) {
	f := newFixture(t, allPerms)
	_, err := f.svc.SetActive(asAdmin(), alice, false)
	require.NoError(t, err)

	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("k1", alice, 10)))
	ev := f.ob.last(contracts.TopicCreditRejected).(contracts.MoveRejectedV1)
	require.Equal(t, effect.ReasonWalletInactive, ev.Reason)
	require.Equal(t, int64(0), balance(t, f, tenantA, alice))
}

func TestTransientPlayerErrorIsReturnedAndRecordsNothing(t *testing.T) {
	f := newFixture(t, allowKeys{})
	f.players.err = errs.New(errs.Unavailable, "player module down")
	err := f.svc.HandleCredit(context.Background(), creditCmd("k1", alice, 10))
	require.Equal(t, errs.Unavailable, errs.KindOf(err))
	require.Empty(t, f.ob.topics())
	require.Empty(t, f.repo.rejections)
}

// ---- job: debit ----

func TestDebitCommandApplies(t *testing.T) {
	f := newFixture(t, allowKeys{})
	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("c1", alice, 500)))
	require.NoError(t, f.svc.HandleDebit(context.Background(), debitCmd("d1", alice, 100)))

	w, _ := f.repo.wallet(tenantA, alice)
	require.Equal(t, int64(400), w.Balance.Minor())
	require.Equal(t, int64(100), w.LifetimeSpent.Minor())
	require.Equal(t, int64(500), w.LifetimeEarned.Minor())
	require.Equal(t, 1, f.ob.count(contracts.TopicDebited))
}

func TestInsufficientBalanceIsRejectionFactNotError(t *testing.T) {
	f := newFixture(t, allowKeys{})
	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("c1", alice, 50)))

	err := f.svc.HandleDebit(context.Background(), debitCmd("d1", alice, 100))
	require.NoError(t, err, "a business rejection must ack, never retry or DLQ")

	ev := f.ob.last(contracts.TopicDebitRejected).(contracts.MoveRejectedV1)
	require.Equal(t, effect.ReasonInsufficientBalance, ev.Reason)
	require.Equal(t, int64(50), ev.Available)
	require.Equal(t, int64(100), ev.Amount)
	require.Equal(t, int64(50), balance(t, f, tenantA, alice))
	require.Len(t, f.repo.entries, 1)

	// The rejection is final: even after a top-up the redelivered debit
	// does not apply late.
	require.NoError(t, f.svc.HandleCredit(context.Background(), creditCmd("c2", alice, 500)))
	require.NoError(t, f.svc.HandleDebit(context.Background(), debitCmd("d1", alice, 100)))
	require.Equal(t, int64(550), balance(t, f, tenantA, alice))
	require.Equal(t, 1, f.ob.count(contracts.TopicDebitRejected))

	out, ok, err := f.svc.OutcomeByKey(context.Background(), tenantA, "d1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, contracts.OutcomeRejected, out.Status)
}

func TestDebitWithoutWalletIsInsufficient(t *testing.T) {
	f := newFixture(t, allowKeys{})
	require.NoError(t, f.svc.HandleDebit(context.Background(), debitCmd("d1", bob, 1)))
	ev := f.ob.last(contracts.TopicDebitRejected).(contracts.MoveRejectedV1)
	require.Equal(t, effect.ReasonInsufficientBalance, ev.Reason)
	require.Equal(t, int64(0), ev.Available)
}

func TestDebitCommandWithCreditKindIsInvalid(t *testing.T) {
	f := newFixture(t, allowKeys{})
	c := debitCmd("d1", alice, 1)
	c.Kind = contracts.KindBonus
	require.Equal(t, errs.Invalid, errs.KindOf(f.svc.HandleDebit(context.Background(), c)))
}

// ---- job: refund ----

func TestRefundReversesDebitExactlyOnce(t *testing.T) {
	f := newFixture(t, allowKeys{})
	ctx := context.Background()
	require.NoError(t, f.svc.HandleCredit(ctx, creditCmd("c1", alice, 500)))
	require.NoError(t, f.svc.HandleDebit(ctx, debitCmd("d1", alice, 200)))

	refund := contracts.RefundCmdV1{IdempotencyKey: "r1", TenantID: tenantA, DebitIdempotencyKey: "d1", Reason: "claim cancelled"}
	require.NoError(t, f.svc.HandleRefund(ctx, refund))
	require.NoError(t, f.svc.HandleRefund(ctx, refund)) // redelivery

	w, _ := f.repo.wallet(tenantA, alice)
	require.Equal(t, int64(500), w.Balance.Minor())
	require.Equal(t, int64(0), w.LifetimeSpent.Minor())
	require.Equal(t, 1, f.ob.count(contracts.TopicRefunded))

	// A second refund under another key is refused: a debit is refunded once.
	require.NoError(t, f.svc.HandleRefund(ctx, contracts.RefundCmdV1{IdempotencyKey: "r2", TenantID: tenantA, DebitIdempotencyKey: "d1"}))
	ev := f.ob.last(contracts.TopicRefundRejected).(contracts.MoveRejectedV1)
	require.Equal(t, contracts.ReasonAlreadyRefunded, ev.Reason)
	require.Equal(t, int64(500), balance(t, f, tenantA, alice))
	require.Equal(t, 1, f.ob.count(contracts.TopicRefunded))
}

func TestRefundOfRejectedDebitIsRejected(t *testing.T) {
	f := newFixture(t, allowKeys{})
	ctx := context.Background()
	require.NoError(t, f.svc.HandleDebit(ctx, debitCmd("d1", alice, 200))) // rejected: empty wallet
	require.NoError(t, f.svc.HandleRefund(ctx, contracts.RefundCmdV1{IdempotencyKey: "r1", TenantID: tenantA, DebitIdempotencyKey: "d1"}))

	ev := f.ob.last(contracts.TopicRefundRejected).(contracts.MoveRejectedV1)
	require.Equal(t, effect.ReasonTargetNotFound, ev.Reason)
	require.Equal(t, 0, f.ob.count(contracts.TopicRefunded))
}

func TestRefundBeforeDebitRetries(t *testing.T) {
	f := newFixture(t, allowKeys{})
	err := f.svc.HandleRefund(context.Background(), contracts.RefundCmdV1{IdempotencyKey: "r1", TenantID: tenantA, DebitIdempotencyKey: "later"})
	require.Equal(t, errs.Unavailable, errs.KindOf(err), "unordered delivery: retry until the debit settles")
	require.Empty(t, f.repo.rejections)
}

func TestRefundOfCreditIsInvalid(t *testing.T) {
	f := newFixture(t, allowKeys{})
	ctx := context.Background()
	require.NoError(t, f.svc.HandleCredit(ctx, creditCmd("c1", alice, 10)))
	err := f.svc.HandleRefund(ctx, contracts.RefundCmdV1{IdempotencyKey: "r1", TenantID: tenantA, DebitIdempotencyKey: "c1"})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

// ---- subscriptions ----

func TestOpenForPlayerIsIdempotent(t *testing.T) {
	f := newFixture(t, allowKeys{})
	ctx := context.Background()
	require.NoError(t, f.svc.OpenForPlayer(ctx, tenantA, alice))
	require.NoError(t, f.svc.OpenForPlayer(ctx, tenantA, alice))
	require.Len(t, f.repo.wallets, 1)
	require.Equal(t, 1, f.ob.count(contracts.TopicWalletOpened))
	require.Equal(t, errs.Invalid, errs.KindOf(f.svc.OpenForPlayer(ctx, "x", alice)))
}

func TestPurgeTenantIsIdempotentAndScoped(t *testing.T) {
	f := newFixture(t, allowKeys{})
	ctx := context.Background()
	require.NoError(t, f.svc.HandleCredit(ctx, creditCmd("c1", alice, 10)))
	other := creditCmd("c1", foreign, 10)
	other.TenantID = tenantB
	require.NoError(t, f.svc.HandleCredit(ctx, other))

	require.NoError(t, f.svc.PurgeTenant(ctx, tenantA))
	require.NoError(t, f.svc.PurgeTenant(ctx, tenantA))
	_, ok := f.repo.wallet(tenantA, alice)
	require.False(t, ok)
	require.Equal(t, int64(10), balance(t, f, tenantB, foreign))
	require.Len(t, f.repo.entries, 1)
}

// ---- HTTP: manual credit / debit ----

func manual(player string, amount int64, kind, key string) ManualMove {
	return ManualMove{PlayerID: player, Amount: amount, Kind: kind, IdempotencyKey: key}
}

func TestManualCreditHappyPathAndReplay(t *testing.T) {
	f := newFixture(t, allPerms)
	e, replay, err := f.svc.Credit(asAdmin(), manual(alice, 100, contracts.KindBonus, "abc"))
	require.NoError(t, err)
	require.False(t, replay)
	require.Equal(t, int64(0), e.BalanceBefore.Minor())
	require.Equal(t, int64(100), e.BalanceAfter.Minor())
	require.Equal(t, "manual:abc", e.IdempotencyKey)
	require.Equal(t, adminID, e.CreatedBy)
	require.Equal(t, effect.SourceManual, e.Source.Kind)

	again, replay, err := f.svc.Credit(asAdmin(), manual(alice, 100, contracts.KindBonus, "abc"))
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, e.ID, again.ID)
	require.Equal(t, int64(100), balance(t, f, tenantA, alice))
	require.Equal(t, 1, f.ob.count(contracts.TopicCredited))

	_, _, err = f.svc.Credit(asAdmin(), manual(alice, 999, contracts.KindBonus, "abc"))
	require.Equal(t, "idempotency_key_reused", errs.CodeOf(err))
}

func TestManualMoveRequiresIdempotencyKey(t *testing.T) {
	f := newFixture(t, allPerms)
	_, _, err := f.svc.Credit(asAdmin(), manual(alice, 1, contracts.KindBonus, " "))
	require.Equal(t, "idempotency_key_required", errs.CodeOf(err))
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

// B6 fixed: a penalty can never be posted as a credit.
func TestManualCreditRejectsDebitKind(t *testing.T) {
	f := newFixture(t, allPerms)
	_, _, err := f.svc.Credit(asAdmin(), manual(alice, 10, contracts.KindPenalty, "k"))
	require.Equal(t, "invalid_kind", errs.CodeOf(err))
	_, _, err = f.svc.Debit(asAdmin(), manual(alice, 10, contracts.KindEarn, "k"))
	require.Equal(t, "invalid_kind", errs.CodeOf(err))
	require.Empty(t, f.ob.topics())
}

// B7: dedicated permissions, not "any tenant user".
func TestManualMovesNeedPermission(t *testing.T) {
	f := newFixture(t, allowKeys{contracts.PermViewWallet.Key(): true})
	_, _, err := f.svc.Credit(asAdmin(), manual(alice, 10, contracts.KindBonus, "k"))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, _, err = f.svc.Debit(asAdmin(), manual(alice, 10, contracts.KindSpend, "k"))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = f.svc.Transfer(asAdmin(), TransferReq{FromPlayerID: alice, ToPlayerID: bob, Amount: 1, IdempotencyKey: "t"})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = f.svc.SetActive(asAdmin(), alice, false)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Empty(t, f.ob.topics())
}

func TestPrincipalWithoutTenantIsDenied(t *testing.T) {
	f := newFixture(t, allPerms)
	ctx := authz.Into(context.Background(), authz.Principal{UserID: adminID, RoleIDs: []int64{1}})
	_, _, err := f.svc.Wallet(ctx, alice)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, _, err = f.svc.Credit(context.Background(), manual(alice, 1, contracts.KindBonus, "k"))
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
}

// B8-style: no cross-tenant wallet creation or reads.
func TestCrossTenantPlayerIsNotFound(t *testing.T) {
	f := newFixture(t, allPerms)
	_, _, err := f.svc.Credit(asAdmin(), manual(foreign, 10, contracts.KindBonus, "k"))
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	require.Equal(t, "player_not_found", errs.CodeOf(err))
	_, _, err = f.svc.Wallet(asAdmin(), foreign)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	_, _, err = f.svc.Ledger(asAdmin(), foreign, LedgerQuery{})
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	require.Empty(t, f.repo.wallets)
}

func TestManualCreditToInactivePlayerIsRefused(t *testing.T) {
	f := newFixture(t, allPerms)
	_, _, err := f.svc.Credit(asAdmin(), manual(sleepy, 10, contracts.KindBonus, "k"))
	require.Equal(t, "player_inactive", errs.CodeOf(err))
}

func TestManualDebitInsufficientRecordsNothing(t *testing.T) {
	f := newFixture(t, allPerms)
	_, _, err := f.svc.Debit(asAdmin(), manual(alice, 100, contracts.KindSpend, "d"))
	require.Equal(t, errs.Invalid, errs.KindOf(err), "422")
	require.Equal(t, "insufficient_balance", errs.CodeOf(err))
	require.Empty(t, f.ob.topics(), "the HTTP path records and publishes nothing on rejection")
	require.Empty(t, f.repo.rejections)
	require.Empty(t, f.repo.wallets, "the lazily-opened wallet rolls back with the refused debit")

	_, _, err = f.svc.Credit(asAdmin(), manual(alice, 50, contracts.KindBonus, "c"))
	require.NoError(t, err)
	_, _, err = f.svc.Debit(asAdmin(), manual(alice, 100, contracts.KindSpend, "d"))
	require.Contains(t, err.Error(), "requested 100, available 50")

	e, _, err := f.svc.Debit(asAdmin(), manual(alice, 50, contracts.KindSpend, "d2"))
	require.NoError(t, err)
	require.Equal(t, int64(0), e.BalanceAfter.Minor())
}

func TestManualDebitOnInactiveWallet(t *testing.T) {
	f := newFixture(t, allPerms)
	_, _, err := f.svc.Credit(asAdmin(), manual(alice, 50, contracts.KindBonus, "c"))
	require.NoError(t, err)
	_, err = f.svc.SetActive(asAdmin(), alice, false)
	require.NoError(t, err)
	_, _, err = f.svc.Debit(asAdmin(), manual(alice, 10, contracts.KindSpend, "d"))
	require.Equal(t, "wallet_inactive", errs.CodeOf(err))
	_, _, err = f.svc.Credit(asAdmin(), manual(alice, 10, contracts.KindBonus, "c2"))
	require.Equal(t, "wallet_inactive", errs.CodeOf(err))
}

// ---- HTTP: transfer ----

func TestTransferMovesAtomically(t *testing.T) {
	f := newFixture(t, allPerms)
	_, _, err := f.svc.Credit(asAdmin(), manual(alice, 500, contracts.KindBonus, "seed"))
	require.NoError(t, err)

	res, err := f.svc.Transfer(asAdmin(), TransferReq{FromPlayerID: alice, ToPlayerID: bob, Amount: 100, IdempotencyKey: "t1"})
	require.NoError(t, err)
	require.False(t, res.Replay)
	require.Equal(t, int64(400), balance(t, f, tenantA, alice))
	require.Equal(t, int64(100), balance(t, f, tenantA, bob), "destination wallet opened on the fly")
	require.Equal(t, res.TransferID, res.Out.TransferID)
	require.Equal(t, res.TransferID, res.In.TransferID)
	require.Equal(t, domain.Debit, res.Out.Direction)
	require.Equal(t, domain.Credit, res.In.Direction)
	require.Equal(t, defaultTransferDescription, res.Out.Description)
	require.Equal(t, 1, f.ob.count(contracts.TopicTransferred))
	ev := f.ob.last(contracts.TopicTransferred).(contracts.TransferredV1)
	require.Equal(t, int64(400), ev.FromBalance)
	require.Equal(t, int64(100), ev.ToBalance)

	again, err := f.svc.Transfer(asAdmin(), TransferReq{FromPlayerID: alice, ToPlayerID: bob, Amount: 100, IdempotencyKey: "t1"})
	require.NoError(t, err)
	require.True(t, again.Replay)
	require.Equal(t, res.TransferID, again.TransferID)
	require.Equal(t, int64(400), balance(t, f, tenantA, alice))
	require.Equal(t, 1, f.ob.count(contracts.TopicTransferred))
}

func TestTransferInsufficientMovesNothing(t *testing.T) {
	f := newFixture(t, allPerms)
	_, _, err := f.svc.Credit(asAdmin(), manual(alice, 50, contracts.KindBonus, "seed"))
	require.NoError(t, err)
	published := len(f.ob.topics())

	_, err = f.svc.Transfer(asAdmin(), TransferReq{FromPlayerID: alice, ToPlayerID: bob, Amount: 100, IdempotencyKey: "t1"})
	require.Equal(t, "insufficient_balance", errs.CodeOf(err))
	require.Equal(t, int64(50), balance(t, f, tenantA, alice))
	_, ok := f.repo.wallet(tenantA, bob)
	require.False(t, ok, "the destination wallet rolls back too")
	require.Len(t, f.ob.topics(), published)
}

// B8 fixed: a foreign or unknown destination never gets a wallet.
func TestTransferToForeignPlayerIsNotFound(t *testing.T) {
	f := newFixture(t, allPerms)
	_, _, err := f.svc.Credit(asAdmin(), manual(alice, 50, contracts.KindBonus, "seed"))
	require.NoError(t, err)
	_, err = f.svc.Transfer(asAdmin(), TransferReq{FromPlayerID: alice, ToPlayerID: foreign, Amount: 10, IdempotencyKey: "t1"})
	require.Equal(t, "player_not_found", errs.CodeOf(err))
	_, ok := f.repo.wallet(tenantA, foreign)
	require.False(t, ok)
	_, ok = f.repo.wallet(tenantB, foreign)
	require.False(t, ok)
}

func TestTransferValidation(t *testing.T) {
	f := newFixture(t, allPerms)
	_, err := f.svc.Transfer(asAdmin(), TransferReq{FromPlayerID: alice, ToPlayerID: alice, Amount: 10, IdempotencyKey: "t"})
	require.Equal(t, "self_transfer", errs.CodeOf(err))
	_, err = f.svc.Transfer(asAdmin(), TransferReq{FromPlayerID: alice, ToPlayerID: bob, Amount: 10})
	require.Equal(t, "idempotency_key_required", errs.CodeOf(err))
	_, err = f.svc.Transfer(asAdmin(), TransferReq{FromPlayerID: alice, ToPlayerID: sleepy, Amount: 10, IdempotencyKey: "t"})
	require.Equal(t, "player_inactive", errs.CodeOf(err))
}

// ---- reads ----

// B17 fixed: GET never creates a wallet.
func TestGetWalletReturnsZeroViewWithoutWriting(t *testing.T) {
	f := newFixture(t, allPerms)
	w, opened, err := f.svc.Wallet(asAdmin(), alice)
	require.NoError(t, err)
	require.False(t, opened)
	require.Equal(t, int64(0), w.Balance.Minor())
	require.True(t, w.Active)
	require.Empty(t, f.repo.wallets)
	require.Empty(t, f.ob.topics())

	_, _, err = f.svc.Credit(asAdmin(), manual(alice, 7, contracts.KindBonus, "c"))
	require.NoError(t, err)
	w, opened, err = f.svc.Wallet(asAdmin(), alice)
	require.NoError(t, err)
	require.True(t, opened)
	require.Equal(t, int64(7), w.Balance.Minor())
}

func TestGetWalletNeedsPermission(t *testing.T) {
	f := newFixture(t, allowKeys{})
	_, _, err := f.svc.Wallet(asAdmin(), alice)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, _, err = f.svc.Ledger(asAdmin(), alice, LedgerQuery{})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestLedgerPagesAndFilters(t *testing.T) {
	repo, ob, pl := newFakeRepo(), &fakeOutbox{}, players()
	svc, clk := newTestService(repo, pl, ob, allPerms)
	for i := range 5 {
		_, _, err := svc.Credit(asAdmin(), manual(alice, int64(10+i), contracts.KindBonus, "c"+string(rune('a'+i))))
		require.NoError(t, err)
		clk.Advance(time.Second)
	}
	_, _, err := svc.Debit(asAdmin(), manual(alice, 5, contracts.KindSpend, "d"))
	require.NoError(t, err)

	page1, next, err := svc.Ledger(asAdmin(), alice, LedgerQuery{Limit: 4})
	require.NoError(t, err)
	require.Len(t, page1, 4)
	require.NotEmpty(t, next)
	require.Equal(t, domain.Debit, page1[0].Direction, "newest first")

	page2, next, err := svc.Ledger(asAdmin(), alice, LedgerQuery{Limit: 4, Cursor: next})
	require.NoError(t, err)
	require.Len(t, page2, 2)
	require.Empty(t, next)

	credits, _, err := svc.Ledger(asAdmin(), alice, LedgerQuery{Direction: "credit"})
	require.NoError(t, err)
	require.Len(t, credits, 5)
	spends, _, err := svc.Ledger(asAdmin(), alice, LedgerQuery{Kind: contracts.KindSpend})
	require.NoError(t, err)
	require.Len(t, spends, 1)

	_, _, err = svc.Ledger(asAdmin(), alice, LedgerQuery{Direction: "sideways"})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	_, _, err = svc.Ledger(asAdmin(), alice, LedgerQuery{Kind: "nope"})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	_, _, err = svc.Ledger(asAdmin(), alice, LedgerQuery{Cursor: "garbage!"})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestSetActiveToggles(t *testing.T) {
	f := newFixture(t, allPerms)
	w, err := f.svc.SetActive(asAdmin(), alice, false)
	require.NoError(t, err)
	require.False(t, w.Active)
	w, err = f.svc.SetActive(asAdmin(), alice, true)
	require.NoError(t, err)
	require.True(t, w.Active)
	_, err = f.svc.SetActive(asAdmin(), foreign, false)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
}

func TestReaderWalletsAndOutcome(t *testing.T) {
	f := newFixture(t, allowKeys{})
	ctx := context.Background()
	require.NoError(t, f.svc.HandleCredit(ctx, creditCmd("c1", alice, 42)))

	ws, err := f.svc.WalletsByPlayerIDs(ctx, tenantA, []string{alice, bob})
	require.NoError(t, err)
	require.Len(t, ws, 1)
	require.Equal(t, int64(42), ws[0].Balance)

	out, ok, err := f.svc.OutcomeByKey(ctx, tenantA, "c1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, contracts.OutcomeApplied, out.Status)
	require.Equal(t, int64(42), out.Amount)
	require.NotEmpty(t, out.EntryID)

	_, ok, err = f.svc.OutcomeByKey(ctx, tenantB, "c1")
	require.NoError(t, err)
	require.False(t, ok, "outcomes are tenant-scoped")
}

// ---- reconcile ----

func TestReconcileCountsDriftAndMarksRun(t *testing.T) {
	repo, ob := newFakeRepo(), &fakeOutbox{}
	counter := &countingCounter{Counter: prometheus.NewCounter(prometheus.CounterOpts{Name: "test_drift_total"})}
	svc := NewService(repo, players(), ob, allowKeys{}, nil, nil, nil, counter)
	clk := newClock()
	svc.clock = clk

	last := testNow.Add(-15 * time.Minute)
	rec := &fakeReconciler{last: last, drift: []domain.Drift{{WalletID: "w1", Balance: 10, LedgerBalance: 5}}}
	require.NoError(t, svc.Reconcile(context.Background(), rec))
	require.Equal(t, 1, counter.incs)
	require.Equal(t, testNow, rec.marked)
	require.True(t, rec.since.Before(last), "sweep overlaps the previous run")

	first := &fakeReconciler{}
	require.NoError(t, svc.Reconcile(context.Background(), first))
	require.True(t, first.since.IsZero(), "first run sweeps everything")
	require.Empty(t, ob.topics(), "reconcile never writes or publishes")
}

// countingCounter records Inc calls without pulling prometheus/testutil.
type countingCounter struct {
	prometheus.Counter
	incs int
}

func (c *countingCounter) Inc() {
	c.incs++
	c.Counter.Inc()
}
