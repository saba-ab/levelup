// Package domain holds points' entities and invariants: a per-player
// wallet that never overdraws, and an append-only ledger whose every entry
// balances (after = before + direction*amount). No framework tags.
package domain

import (
	"time"

	"levelup/internal/modules/points/contracts"
	"levelup/internal/shared/id"
	"levelup/internal/shared/money"
)

// Direction is the sign of a ledger entry.
type Direction int8

const (
	Credit Direction = 1
	Debit  Direction = -1
)

func (d Direction) Valid() bool { return d == Credit || d == Debit }

// creditKinds and debitKinds are the kinds a caller may choose (manual API
// and job commands). transfer and refund are written only by the service.
// Restricting kinds per direction fixes Laravel's B6 ("penalty" credited).
var (
	creditKinds = map[string]bool{
		contracts.KindEarn: true, contracts.KindBonus: true,
		contracts.KindReward: true, contracts.KindAdjustment: true,
	}
	debitKinds = map[string]bool{
		contracts.KindSpend: true, contracts.KindRedeem: true,
		contracts.KindPenalty: true, contracts.KindExpire: true,
	}
)

// CheckKind refuses a kind that does not belong to the direction.
func CheckKind(dir Direction, kind string) error {
	switch {
	case dir == Credit && creditKinds[kind]:
		return nil
	case dir == Debit && debitKinds[kind]:
		return nil
	}
	return ErrKindNotAllowed
}

// CreditKinds lists the caller-selectable credit kinds (documentation, DTOs).
func CreditKinds() []string {
	return []string{contracts.KindEarn, contracts.KindBonus, contracts.KindReward, contracts.KindAdjustment}
}

// DebitKinds lists the caller-selectable debit kinds.
func DebitKinds() []string {
	return []string{contracts.KindSpend, contracts.KindRedeem, contracts.KindPenalty, contracts.KindExpire}
}

// Wallet is one player's points balance inside one tenant.
type Wallet struct {
	ID             string
	TenantID       string
	PlayerID       string // bare uuid — no FK to player_svc
	Balance        money.Amount
	LifetimeEarned money.Amount
	LifetimeSpent  money.Amount
	Active         bool
	// Version is the optimistic-concurrency token as READ; the repository
	// writes version+1 only where version still equals it.
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewWallet(tenantID, playerID string, now time.Time) Wallet {
	return Wallet{
		ID:        id.NewID(),
		TenantID:  tenantID,
		PlayerID:  playerID,
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Credit adds amount and counts it as earned.
func (w *Wallet) Credit(amount money.Amount, now time.Time) (before, after money.Amount, err error) {
	if amount <= 0 {
		return 0, 0, ErrNonPositiveAmount
	}
	if !w.Active {
		return 0, 0, ErrWalletInactive
	}
	next, err := w.Balance.Add(amount)
	if err != nil {
		return 0, 0, ErrAmountOverflow
	}
	earned, err := w.LifetimeEarned.Add(amount)
	if err != nil {
		return 0, 0, ErrAmountOverflow
	}
	before = w.Balance
	w.Balance, w.LifetimeEarned, w.UpdatedAt = next, earned, now
	return before, next, nil
}

// Debit removes amount; never below zero (B1). Balance may reach exactly 0.
func (w *Wallet) Debit(amount money.Amount, now time.Time) (before, after money.Amount, err error) {
	if amount <= 0 {
		return 0, 0, ErrNonPositiveAmount
	}
	if !w.Active {
		return 0, 0, ErrWalletInactive
	}
	if amount > w.Balance {
		return 0, 0, ErrInsufficientBalance
	}
	spent, err := w.LifetimeSpent.Add(amount)
	if err != nil {
		return 0, 0, ErrAmountOverflow
	}
	before = w.Balance
	w.Balance, w.LifetimeSpent, w.UpdatedAt = w.Balance-amount, spent, now
	return before, w.Balance, nil
}

// Refund gives back a previously debited amount. It undoes spending, so it
// lowers lifetime_spent instead of raising lifetime_earned (the reconcile
// job checks exactly this).
func (w *Wallet) Refund(amount money.Amount, now time.Time) (before, after money.Amount, err error) {
	if amount <= 0 {
		return 0, 0, ErrNonPositiveAmount
	}
	if !w.Active {
		return 0, 0, ErrWalletInactive
	}
	next, err := w.Balance.Add(amount)
	if err != nil {
		return 0, 0, ErrAmountOverflow
	}
	spent := w.LifetimeSpent - amount
	if spent < 0 {
		spent = 0
	}
	before = w.Balance
	w.Balance, w.LifetimeSpent, w.UpdatedAt = next, spent, now
	return before, next, nil
}

// NextVersion is the version the wallet carries once the pending write
// commits; ledger entries record it so the chain can be ordered.
func (w Wallet) NextVersion() int { return w.Version + 1 }

func (w Wallet) Snapshot() contracts.WalletSnapshot {
	return contracts.WalletSnapshot{
		ID:             w.ID,
		TenantID:       w.TenantID,
		PlayerID:       w.PlayerID,
		Balance:        w.Balance.Minor(),
		LifetimeEarned: w.LifetimeEarned.Minor(),
		LifetimeSpent:  w.LifetimeSpent.Minor(),
		Active:         w.Active,
	}
}
