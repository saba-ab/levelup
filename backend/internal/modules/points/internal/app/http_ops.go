package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"levelup/internal/modules/points/contracts"
	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
	"levelup/internal/shared/money"
)

// manualKeyPrefix namespaces Idempotency-Key header values in the ledger so
// they can never collide with keys derived by other modules.
const manualKeyPrefix = "manual:"

// defaultTransferDescription keeps Laravel's wording.
const defaultTransferDescription = "Point transfer"

// ManualMove is an admin credit or debit over HTTP.
type ManualMove struct {
	PlayerID       string
	Amount         int64
	Kind           string
	Description    string
	IdempotencyKey string // raw Idempotency-Key header value
}

// Credit is POST /players/{id}/wallet/credit. replay is true when the key
// had already been applied (the original entry is returned, nothing new).
func (s *Service) Credit(ctx context.Context, m ManualMove) (entry domain.LedgerEntry, replay bool, err error) {
	return s.manual(ctx, contracts.PermCredit, domain.Credit, m)
}

// Debit is POST /players/{id}/wallet/debit. Insufficient balance is a 422
// with code insufficient_balance; nothing is recorded.
func (s *Service) Debit(ctx context.Context, m ManualMove) (entry domain.LedgerEntry, replay bool, err error) {
	return s.manual(ctx, contracts.PermDebit, domain.Debit, m)
}

func (s *Service) manual(ctx context.Context, perm authz.Permission, dir domain.Direction, m ManualMove) (domain.LedgerEntry, bool, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.LedgerEntry{}, false, err
	}
	if err := s.authz.Authorize(ctx, p, perm, nil); err != nil {
		return domain.LedgerEntry{}, false, err
	}
	key, err := manualKey(m.IdempotencyKey)
	if err != nil {
		return domain.LedgerEntry{}, false, err
	}
	if err := domain.CheckKind(dir, m.Kind); err != nil {
		return domain.LedgerEntry{}, false, err
	}
	if m.Amount <= 0 {
		return domain.LedgerEntry{}, false, domain.ErrNonPositiveAmount
	}

	matches := func(e domain.LedgerEntry) bool {
		return e.PlayerID == m.PlayerID && e.Direction == dir && e.Amount.Minor() == m.Amount && e.Kind == m.Kind
	}
	// Replay before the player check: a retried request returns its
	// original outcome even if the player changed since.
	var prior moveResult
	if settled, err := s.settledInTx(ctx, nil, p.TenantID, key, &prior); err != nil {
		return domain.LedgerEntry{}, false, err
	} else if settled {
		if prior.Rejection != nil || !matches(prior.Entry) {
			return domain.LedgerEntry{}, false, domain.ErrIdempotencyKeyReused
		}
		return prior.Entry, true, nil
	}

	if err := s.requireActivePlayer(ctx, p.TenantID, m.PlayerID); err != nil {
		return domain.LedgerEntry{}, false, err
	}
	res, err := s.move(ctx, moveReq{
		TenantID: p.TenantID,
		PlayerID: m.PlayerID,
		Dir:      dir,
		Amount:   money.Amount(m.Amount),
		Spec: domain.EntrySpec{
			IdempotencyKey: key,
			Kind:           m.Kind,
			Source:         effect.Source{Kind: effect.SourceManual, ID: p.UserID},
			Description:    m.Description,
			CreatedBy:      actor(p),
		},
	})
	if err != nil {
		return domain.LedgerEntry{}, false, err
	}
	if res.Rejection != nil || (res.Replay && !matches(res.Entry)) {
		return domain.LedgerEntry{}, false, domain.ErrIdempotencyKeyReused
	}
	return res.Entry, res.Replay, nil
}

// TransferReq is POST /wallets/transfer.
type TransferReq struct {
	FromPlayerID   string
	ToPlayerID     string
	Amount         int64
	Description    string
	IdempotencyKey string
}

// TransferResult carries both legs.
type TransferResult struct {
	TransferID string
	Out        domain.LedgerEntry
	In         domain.LedgerEntry
	Replay     bool
}

// Transfer moves points between two players of the caller's tenant: two
// ledger legs sharing transfer_id, both wallets locked in id order, one
// transaction, one points.transferred.v1.
func (s *Service) Transfer(ctx context.Context, req TransferReq) (TransferResult, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return TransferResult{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermTransfer, nil); err != nil {
		return TransferResult{}, err
	}
	key, err := manualKey(req.IdempotencyKey)
	if err != nil {
		return TransferResult{}, err
	}
	if req.FromPlayerID == req.ToPlayerID {
		return TransferResult{}, domain.ErrSelfTransfer
	}
	if req.Amount <= 0 {
		return TransferResult{}, domain.ErrNonPositiveAmount
	}
	outKey, inKey := key+":out", key+":in"

	if res, ok, err := s.transferReplay(ctx, p.TenantID, outKey, inKey, req); err != nil || ok {
		return res, err
	}

	// Both players must exist in the caller's tenant (fixes B8: no wallet
	// is ever opened for a foreign or unknown player).
	got, err := s.players.PlayersByIDs(ctx, p.TenantID, []string{req.FromPlayerID, req.ToPlayerID})
	if err != nil {
		return TransferResult{}, err
	}
	for _, pid := range []string{req.FromPlayerID, req.ToPlayerID} {
		snap, ok := got[pid]
		if !ok || snap.TenantID != p.TenantID {
			return TransferResult{}, errs.WithCode(errs.Wrap(errs.NotFound, "player "+pid, domain.ErrPlayerNotFound), "player_not_found")
		}
		if !snap.Active {
			return TransferResult{}, errs.WithCode(errs.Wrap(errs.Invalid, "player "+pid, domain.ErrPlayerInactive), "player_inactive")
		}
	}

	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = defaultTransferDescription
	}
	transferID := id.NewID()
	amount := money.Amount(req.Amount)
	var out TransferResult
	err = s.tx(ctx, func(tx *gorm.DB) error {
		out = TransferResult{}
		// Open in a fixed order so two first-time opposite transfers cannot
		// deadlock on the unique-index inserts either.
		pair := []string{req.FromPlayerID, req.ToPlayerID}
		sort.Strings(pair)
		for _, pid := range pair {
			if err := s.ensureWallet(ctx, tx, p.TenantID, pid); err != nil {
				return err
			}
		}
		locked, err := s.repo.WalletsForUpdate(ctx, tx, p.TenantID, []string{req.FromPlayerID, req.ToPlayerID})
		if err != nil {
			return err
		}
		var src, dst *domain.Wallet
		for i := range locked {
			switch locked[i].PlayerID {
			case req.FromPlayerID:
				src = &locked[i]
			case req.ToPlayerID:
				dst = &locked[i]
			}
		}
		if src == nil || dst == nil {
			return domain.ErrWalletNotFound
		}
		var prior moveResult
		if settled, err := s.settledInTx(ctx, tx, p.TenantID, outKey, &prior); err != nil {
			return err
		} else if settled {
			out.Replay = true
			return nil
		}

		now := s.clock.Now()
		if !dst.Active {
			return errs.WithCode(errs.Wrap(errs.Invalid, "destination wallet", domain.ErrWalletInactive), "wallet_inactive")
		}
		srcBefore, srcAfter, err := src.Debit(amount, now)
		if err != nil {
			if errors.Is(err, domain.ErrInsufficientBalance) {
				return errs.WithCode(errs.Wrap(errs.Invalid,
					fmt.Sprintf("requested %d, available %d", req.Amount, src.Balance.Minor()), err), "insufficient_balance")
			}
			return err
		}
		dstBefore, dstAfter, err := dst.Credit(amount, now)
		if err != nil {
			return err
		}
		spec := domain.EntrySpec{
			Kind:        contracts.KindTransfer,
			Source:      effect.Source{Kind: effect.SourceTransfer, ID: transferID},
			Description: description,
			TransferID:  transferID,
			CreatedBy:   actor(p),
		}
		outSpec, inSpec := spec, spec
		outSpec.IdempotencyKey, inSpec.IdempotencyKey = outKey, inKey
		outEntry, err := domain.NewEntry(*src, domain.Debit, amount, srcBefore, srcAfter, outSpec, now)
		if err != nil {
			return err
		}
		inEntry, err := domain.NewEntry(*dst, domain.Credit, amount, dstBefore, dstAfter, inSpec, now)
		if err != nil {
			return err
		}
		if err := s.appendAndSave(ctx, tx, []domain.LedgerEntry{outEntry, inEntry}, src, dst); err != nil {
			return err
		}
		out = TransferResult{TransferID: transferID, Out: outEntry, In: inEntry}
		return s.outbox.Publish(ctx, tx, contracts.TopicTransferred, contracts.TransferredV1{
			TransferID:     transferID,
			TenantID:       p.TenantID,
			FromPlayerID:   req.FromPlayerID,
			ToPlayerID:     req.ToPlayerID,
			Amount:         req.Amount,
			FromBalance:    srcAfter.Minor(),
			ToBalance:      dstAfter.Minor(),
			IdempotencyKey: key,
			At:             now,
		})
	})
	if errors.Is(err, errKeyRace) || (err == nil && out.Replay) {
		res, ok, rerr := s.transferReplay(ctx, p.TenantID, outKey, inKey, req)
		if rerr != nil {
			return TransferResult{}, rerr
		}
		if !ok {
			return TransferResult{}, errs.New(errs.Conflict, "transfer key settled concurrently, retry")
		}
		return res, nil
	}
	if err != nil {
		return TransferResult{}, err
	}
	return out, nil
}

// transferReplay returns the already-applied transfer for the key, or
// ErrIdempotencyKeyReused when the key was used for something else.
func (s *Service) transferReplay(ctx context.Context, tenantID, outKey, inKey string, req TransferReq) (TransferResult, bool, error) {
	outEntry, ok, err := s.repo.EntryByKey(ctx, nil, tenantID, outKey)
	if err != nil || !ok {
		return TransferResult{}, false, err
	}
	inEntry, ok, err := s.repo.EntryByKey(ctx, nil, tenantID, inKey)
	if err != nil {
		return TransferResult{}, false, err
	}
	if !ok || outEntry.PlayerID != req.FromPlayerID || inEntry.PlayerID != req.ToPlayerID ||
		outEntry.Amount.Minor() != req.Amount {
		return TransferResult{}, false, domain.ErrIdempotencyKeyReused
	}
	return TransferResult{TransferID: outEntry.TransferID, Out: outEntry, In: inEntry, Replay: true}, true, nil
}

// SetActive is PATCH /players/{id}/wallet {is_active}: an admin switch.
// Opening the wallet when absent keeps "deactivate before first credit"
// meaningful.
func (s *Service) SetActive(ctx context.Context, playerID string, active bool) (domain.Wallet, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Wallet{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermManageWallet, nil); err != nil {
		return domain.Wallet{}, err
	}
	if _, err := s.player(ctx, p.TenantID, playerID); err != nil {
		return domain.Wallet{}, err
	}
	var out domain.Wallet
	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.ensureWallet(ctx, tx, p.TenantID, playerID); err != nil {
			return err
		}
		w, err := s.repo.WalletForUpdate(ctx, tx, p.TenantID, playerID)
		if err != nil {
			return err
		}
		if w.Active != active {
			w.Active = active
			w.UpdatedAt = s.clock.Now()
			if err := s.repo.SaveWallet(ctx, tx, w); err != nil {
				return err
			}
			w.Version++
		}
		out = w
		return nil
	})
	if err != nil {
		return domain.Wallet{}, err
	}
	return out, nil
}

func (s *Service) requireActivePlayer(ctx context.Context, tenantID, playerID string) error {
	snap, err := s.player(ctx, tenantID, playerID)
	if err != nil {
		return err
	}
	if !snap.Active {
		return domain.ErrPlayerInactive
	}
	return nil
}

func manualKey(header string) (string, error) {
	k := strings.TrimSpace(header)
	if k == "" {
		return "", domain.ErrIdempotencyKeyRequired
	}
	if len(k) > 200 {
		return "", errs.WithCode(errs.New(errs.Invalid, "Idempotency-Key must be at most 200 characters"), "invalid_idempotency_key")
	}
	return manualKeyPrefix + k, nil
}

// actor is the created_by stamp: the acting user when it is a uuid.
func actor(p authz.Principal) string {
	if isUUID(p.UserID) {
		return p.UserID
	}
	return ""
}
