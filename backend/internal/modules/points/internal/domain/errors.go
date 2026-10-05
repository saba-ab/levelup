package domain

import "levelup/internal/shared/errs"

// Business errors. Every error a client may branch on carries a code
// (ADR-0016). Business rejections are errs.Invalid (422): on the job path
// they never reach the consumer loop — the service turns them into a
// *_rejected.v1 fact and acks.
var (
	ErrNonPositiveAmount   = errs.WithCode(errs.New(errs.Invalid, "amount must be positive"), "invalid_amount")
	ErrAmountOverflow      = errs.WithCode(errs.New(errs.Invalid, "amount overflows the wallet"), "amount_overflow")
	ErrWalletInactive      = errs.WithCode(errs.New(errs.Invalid, "wallet is inactive and cannot process transactions"), "wallet_inactive")
	ErrInsufficientBalance = errs.WithCode(errs.New(errs.Invalid, "insufficient balance for this transaction"), "insufficient_balance")
	ErrKindNotAllowed      = errs.WithCode(errs.New(errs.Invalid, "kind is not allowed for this direction"), "invalid_kind")
	ErrSelfTransfer        = errs.WithCode(errs.New(errs.Invalid, "cannot transfer points to the same player"), "self_transfer")
	ErrBrokenEntry         = errs.New(errs.Internal, "ledger entry does not balance")

	ErrWalletNotFound  = errs.WithCode(errs.New(errs.NotFound, "wallet not found"), "wallet_not_found")
	ErrPlayerNotFound  = errs.WithCode(errs.New(errs.NotFound, "player not found"), "player_not_found")
	ErrPlayerInactive  = errs.WithCode(errs.New(errs.Invalid, "player is inactive"), "player_inactive")
	ErrVersionConflict = errs.WithCode(errs.New(errs.Conflict, "wallet modified concurrently, retry"), "version_conflict")

	ErrIdempotencyKeyRequired = errs.WithCode(errs.New(errs.Invalid, "Idempotency-Key header is required"), "idempotency_key_required")
	ErrIdempotencyKeyReused   = errs.WithCode(errs.New(errs.Invalid, "Idempotency-Key was already used for a different operation"), "idempotency_key_reused")
)
