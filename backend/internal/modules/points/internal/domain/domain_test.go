package domain

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/points/contracts"
	"levelup/internal/shared/money"
)

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func wallet(balance int64, active bool) Wallet {
	w := NewWallet("t", "p", now)
	w.Balance = money.Amount(balance)
	w.Active = active
	return w
}

func TestCredit(t *testing.T) {
	cases := []struct {
		name    string
		w       Wallet
		amount  money.Amount
		wantErr error
		after   money.Amount
	}{
		{"adds", wallet(500, true), 100, nil, 600},
		{"from zero", wallet(0, true), 1, nil, 1},
		{"zero amount", wallet(0, true), 0, ErrNonPositiveAmount, 0},
		{"negative amount", wallet(0, true), -5, ErrNonPositiveAmount, 0},
		{"inactive", wallet(0, false), 10, ErrWalletInactive, 0},
		{"overflow", wallet(math.MaxInt64, true), 1, ErrAmountOverflow, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.w
			beforeBal := w.Balance
			before, after, err := w.Credit(tc.amount, now)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, beforeBal, w.Balance, "a refused credit changes nothing")
				return
			}
			require.NoError(t, err)
			require.Equal(t, beforeBal, before)
			require.Equal(t, tc.after, after)
			require.Equal(t, tc.after, w.Balance)
			require.Equal(t, tc.amount, w.LifetimeEarned)
		})
	}
}

func TestDebit(t *testing.T) {
	cases := []struct {
		name    string
		w       Wallet
		amount  money.Amount
		wantErr error
		after   money.Amount
	}{
		{"subtracts", wallet(500, true), 100, nil, 400},
		{"to exactly zero", wallet(100, true), 100, nil, 0},
		{"overdraw refused", wallet(50, true), 100, ErrInsufficientBalance, 0},
		{"empty wallet", wallet(0, true), 1, ErrInsufficientBalance, 0},
		{"zero amount", wallet(10, true), 0, ErrNonPositiveAmount, 0},
		{"inactive", wallet(500, false), 10, ErrWalletInactive, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.w
			beforeBal := w.Balance
			_, after, err := w.Debit(tc.amount, now)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, beforeBal, w.Balance)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.after, after)
			require.Equal(t, tc.amount, w.LifetimeSpent)
			require.GreaterOrEqual(t, int64(w.Balance), int64(0))
		})
	}
}

func TestRefundLowersLifetimeSpent(t *testing.T) {
	w := wallet(500, true)
	_, _, err := w.Debit(200, now)
	require.NoError(t, err)
	before, after, err := w.Refund(200, now)
	require.NoError(t, err)
	require.Equal(t, money.Amount(300), before)
	require.Equal(t, money.Amount(500), after)
	require.Equal(t, money.Amount(0), w.LifetimeSpent)
	require.Equal(t, money.Amount(0), w.LifetimeEarned)

	inactive := wallet(0, false)
	_, _, err = inactive.Refund(10, now)
	require.ErrorIs(t, err, ErrWalletInactive)
}

// B6: a caller cannot credit with a debit kind or debit with a credit kind.
func TestCheckKindMatchesDirection(t *testing.T) {
	cases := []struct {
		dir  Direction
		kind string
		ok   bool
	}{
		{Credit, contracts.KindEarn, true},
		{Credit, contracts.KindBonus, true},
		{Credit, contracts.KindReward, true},
		{Credit, contracts.KindAdjustment, true},
		{Credit, contracts.KindPenalty, false},
		{Credit, contracts.KindSpend, false},
		{Credit, contracts.KindTransfer, false},
		{Credit, contracts.KindRefund, false},
		{Debit, contracts.KindSpend, true},
		{Debit, contracts.KindRedeem, true},
		{Debit, contracts.KindPenalty, true},
		{Debit, contracts.KindExpire, true},
		{Debit, contracts.KindEarn, false},
		{Debit, "", false},
	}
	for _, tc := range cases {
		err := CheckKind(tc.dir, tc.kind)
		if tc.ok {
			require.NoError(t, err, "%d %s", tc.dir, tc.kind)
		} else {
			require.ErrorIs(t, err, ErrKindNotAllowed, "%d %s", tc.dir, tc.kind)
		}
	}
}

func TestNewEntryEnforcesBalanceEquation(t *testing.T) {
	w := wallet(100, true)
	e, err := NewEntry(w, Credit, 50, 100, 150, EntrySpec{IdempotencyKey: "k", Kind: contracts.KindEarn}, now)
	require.NoError(t, err)
	require.Equal(t, w.Version+1, e.WalletVersion)
	require.Equal(t, now, e.OccurredAt, "occurred_at defaults to now")

	_, err = NewEntry(w, Debit, 50, 100, 150, EntrySpec{}, now)
	require.ErrorIs(t, err, ErrBrokenEntry)
	_, err = NewEntry(w, Direction(0), 50, 100, 150, EntrySpec{}, now)
	require.ErrorIs(t, err, ErrBrokenEntry)
	_, err = NewEntry(w, Credit, 0, 100, 100, EntrySpec{}, now)
	require.ErrorIs(t, err, ErrNonPositiveAmount)
}
