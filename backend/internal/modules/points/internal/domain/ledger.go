package domain

import (
	"time"

	"levelup/internal/shared/effect"
	"levelup/internal/shared/id"
	"levelup/internal/shared/money"
)

// LedgerEntry is one append-only balance movement.
type LedgerEntry struct {
	ID             string
	TenantID       string
	WalletID       string
	PlayerID       string
	IdempotencyKey string
	Kind           string
	Direction      Direction
	Amount         money.Amount
	BalanceBefore  money.Amount
	BalanceAfter   money.Amount
	WalletVersion  int
	Source         effect.Source
	Description    string
	TransferID     string // shared by both legs of a transfer
	ReversalOf     string // the debit entry a refund reverses
	OccurredAt     time.Time
	CreatedBy      string // acting user; empty for job-driven movements
	CreatedAt      time.Time
}

// EntrySpec is what a movement knows before the wallet arithmetic runs.
type EntrySpec struct {
	IdempotencyKey string
	Kind           string
	Source         effect.Source
	Description    string
	TransferID     string
	ReversalOf     string
	OccurredAt     time.Time
	CreatedBy      string
}

// NewEntry records a movement already applied to w (w carries the
// post-movement balance). It enforces after = before + direction*amount.
func NewEntry(w Wallet, dir Direction, amount, before, after money.Amount, spec EntrySpec, now time.Time) (LedgerEntry, error) {
	if !dir.Valid() {
		return LedgerEntry{}, ErrBrokenEntry
	}
	if amount <= 0 {
		return LedgerEntry{}, ErrNonPositiveAmount
	}
	if int64(after) != int64(before)+int64(dir)*int64(amount) {
		return LedgerEntry{}, ErrBrokenEntry
	}
	occurred := spec.OccurredAt
	if occurred.IsZero() {
		occurred = now
	}
	return LedgerEntry{
		ID:             id.NewID(),
		TenantID:       w.TenantID,
		WalletID:       w.ID,
		PlayerID:       w.PlayerID,
		IdempotencyKey: spec.IdempotencyKey,
		Kind:           spec.Kind,
		Direction:      dir,
		Amount:         amount,
		BalanceBefore:  before,
		BalanceAfter:   after,
		WalletVersion:  w.NextVersion(),
		Source:         spec.Source,
		Description:    spec.Description,
		TransferID:     spec.TransferID,
		ReversalOf:     spec.ReversalOf,
		OccurredAt:     occurred.UTC(),
		CreatedBy:      spec.CreatedBy,
		CreatedAt:      now,
	}, nil
}

// Command names a rejected command's type.
const (
	CommandCredit = "credit"
	CommandDebit  = "debit"
	CommandRefund = "refund"
)

// Rejection is the settled "no" to a job command, stored under the
// command's idempotency key so OutcomeByKey answers for it and a
// redelivery is a no-op.
type Rejection struct {
	ID             string
	TenantID       string
	IdempotencyKey string
	Command        string
	PlayerID       string
	Amount         money.Amount
	Reason         string
	Available      money.Amount
	Source         effect.Source
	CreatedAt      time.Time
}

// Drift is one wallet whose stored counters disagree with its ledger.
type Drift struct {
	WalletID       string
	TenantID       string
	PlayerID       string
	Balance        int64
	LedgerBalance  int64
	LifetimeEarned int64
	LedgerEarned   int64
	LifetimeSpent  int64
	LedgerSpent    int64
	ChainBreaks    int64
}
