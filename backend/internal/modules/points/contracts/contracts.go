// Package contracts is points' public surface. Points are whole numbers
// (int64) in a per-player wallet backed by an append-only ledger.
package contracts

import (
	"context"
	"time"

	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
)

// Facts.
const (
	TopicWalletOpened   = "points.wallet_opened.v1"
	TopicCredited       = "points.credited.v1"
	TopicDebited        = "points.debited.v1"
	TopicDebitRejected  = "points.debit_rejected.v1"
	TopicCreditRejected = "points.credit_rejected.v1"
	TopicTransferred    = "points.transferred.v1"
	TopicRefunded       = "points.refunded.v1"
	// TopicRefundRejected answers a refund that cannot apply (debit was
	// rejected, already refunded, wallet inactive). Payload MoveRejectedV1.
	TopicRefundRejected = "points.refund_rejected.v1"
)

// Points-specific rejection reasons, alongside effect.Reason*.
const (
	ReasonAlreadyRefunded = "already_refunded"
)

// MoveOutcome.Status values.
const (
	OutcomeApplied  = "applied"
	OutcomeRejected = "rejected"
)

// Commands (outbox topics "job.<name>"; the worker runs Jobs() named <name>).
const (
	JobCredit = "points.credit"
	JobDebit  = "points.debit"
	JobRefund = "points.refund"
)

// Topic returns the outbox topic of a job name: "job." + name (R46).
func Topic(job string) string { return "job." + job }

// Ledger entry kinds (the Laravel TransactionType values, normalised).
const (
	KindEarn       = "earn"
	KindBonus      = "bonus"
	KindReward     = "reward"
	KindAdjustment = "adjustment"
	KindSpend      = "spend"
	KindRedeem     = "redeem"
	KindPenalty    = "penalty"
	KindExpire     = "expire"
	KindTransfer   = "transfer"
	KindRefund     = "refund"
)

// CreditCmdV1 adds points. IdempotencyKey is unique per tenant on the
// ledger: a redelivered command inserts nothing and re-publishes nothing.
type CreditCmdV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	Amount         int64         `json:"amount"` // > 0
	Kind           string        `json:"kind"`   // earn | bonus | reward | adjustment
	Description    string        `json:"description,omitempty"`
	Source         effect.Source `json:"source"`
	OccurredAt     time.Time     `json:"occurred_at"`
}

// DebitCmdV1 removes points; insufficient balance is a rejection fact,
// never an error.
type DebitCmdV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	Amount         int64         `json:"amount"` // > 0
	Kind           string        `json:"kind"`   // spend | redeem | penalty
	Description    string        `json:"description,omitempty"`
	Source         effect.Source `json:"source"`
	OccurredAt     time.Time     `json:"occurred_at"`
}

// RefundCmdV1 reverses a previous debit identified by its idempotency key.
type RefundCmdV1 struct {
	IdempotencyKey      string        `json:"idempotency_key"`
	TenantID            string        `json:"tenant_id"`
	DebitIdempotencyKey string        `json:"debit_idempotency_key"`
	Reason              string        `json:"reason,omitempty"`
	Source              effect.Source `json:"source"`
	OccurredAt          time.Time     `json:"occurred_at"`
}

type WalletOpenedV1 struct {
	WalletID string    `json:"wallet_id"`
	TenantID string    `json:"tenant_id"`
	PlayerID string    `json:"player_id"`
	At       time.Time `json:"at"`
}

// LedgerMovedV1 is the payload of credited, debited and refunded.
type LedgerMovedV1 struct {
	EntryID        string        `json:"entry_id"`
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	WalletID       string        `json:"wallet_id"`
	Kind           string        `json:"kind"`
	Amount         int64         `json:"amount"` // always positive; the topic gives the direction
	BalanceAfter   int64         `json:"balance_after"`
	LifetimeEarned int64         `json:"lifetime_earned"`
	Source         effect.Source `json:"source"`
	OccurredAt     time.Time     `json:"occurred_at"`
	At             time.Time     `json:"at"`
}

// MoveRejectedV1 is the payload of debit_rejected and credit_rejected.
type MoveRejectedV1 struct {
	IdempotencyKey string        `json:"idempotency_key"`
	TenantID       string        `json:"tenant_id"`
	PlayerID       string        `json:"player_id"`
	Amount         int64         `json:"amount"`
	Reason         string        `json:"reason"` // effect.Reason*
	Available      int64         `json:"available"`
	Source         effect.Source `json:"source"`
	At             time.Time     `json:"at"`
}

type TransferredV1 struct {
	TransferID     string    `json:"transfer_id"`
	TenantID       string    `json:"tenant_id"`
	FromPlayerID   string    `json:"from_player_id"`
	ToPlayerID     string    `json:"to_player_id"`
	Amount         int64     `json:"amount"`
	FromBalance    int64     `json:"from_balance_after"`
	ToBalance      int64     `json:"to_balance_after"`
	IdempotencyKey string    `json:"idempotency_key"`
	At             time.Time `json:"at"`
}

const Module = "points"

var (
	PermViewWallet       = authz.Permission{Module: Module, Action: "view_wallet"}
	PermViewTransactions = authz.Permission{Module: Module, Action: "view_transactions"}
	PermCredit           = authz.Permission{Module: Module, Action: "credit"}
	PermDebit            = authz.Permission{Module: Module, Action: "debit"}
	PermTransfer         = authz.Permission{Module: Module, Action: "transfer"}
	PermManageWallet     = authz.Permission{Module: Module, Action: "manage_wallet"} // admin: activate/deactivate
)

var AllPermissions = []authz.Permission{
	PermViewWallet, PermViewTransactions, PermCredit, PermDebit, PermTransfer, PermManageWallet,
}

type WalletSnapshot struct {
	ID             string
	TenantID       string
	PlayerID       string
	Balance        int64
	LifetimeEarned int64
	LifetimeSpent  int64
	Active         bool
}

// MoveOutcome is the settled result of a command, looked up by key. Saga
// sweeps (rewards) use it to resolve rows whose outcome event never came.
type MoveOutcome struct {
	IdempotencyKey string
	Status         string // "applied" | "rejected"
	Reason         string
	EntryID        string
	Amount         int64
}

type Reader interface {
	WalletsByPlayerIDs(ctx context.Context, tenantID string, playerIDs []string) ([]WalletSnapshot, error)
	OutcomeByKey(ctx context.Context, tenantID, idempotencyKey string) (MoveOutcome, bool, error)
}
