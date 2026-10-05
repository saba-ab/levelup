// Package transport is points' HTTP layer: DTO shape validation here,
// invariants in the service and domain.
package transport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"levelup/internal/modules/points/internal/app"
	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/validate"
)

const idempotencyHeader = "Idempotency-Key"

type Handler struct {
	svc *app.Service
	val *validate.Validator
}

func NewHandler(svc *app.Service, val *validate.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// Mount registers full paths inside one auth group instead of
// r.Route("/players", ...): the player module owns that prefix, and a
// second Route/Mount on it would panic in chi.
func (h *Handler) Mount(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/players/{playerID}/wallet", h.getWallet)
		r.Patch("/players/{playerID}/wallet", h.patchWallet)
		r.Get("/players/{playerID}/wallet/transactions", h.listTransactions)
		r.Post("/players/{playerID}/wallet/credit", h.credit)
		r.Post("/players/{playerID}/wallet/debit", h.debit)
		r.Post("/wallets/transfer", h.transfer)
	})
}

// WalletResp is a wallet. A never-opened wallet has opened=false, an empty
// id and zero counters.
type WalletResp struct {
	ID             string     `json:"id"`
	PlayerID       string     `json:"player_id"`
	Balance        int64      `json:"balance"`
	LifetimeEarned int64      `json:"lifetime_earned"`
	LifetimeSpent  int64      `json:"lifetime_spent"`
	IsActive       bool       `json:"is_active"`
	Opened         bool       `json:"opened"`
	Version        int        `json:"version"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
}

// EntryResp is one ledger entry.
type EntryResp struct {
	ID            string    `json:"id"`
	WalletID      string    `json:"wallet_id"`
	PlayerID      string    `json:"player_id"`
	Kind          string    `json:"kind"`
	Direction     string    `json:"direction"` // credit | debit
	Amount        int64     `json:"amount"`
	BalanceBefore int64     `json:"balance_before"`
	BalanceAfter  int64     `json:"balance_after"`
	Description   string    `json:"description,omitempty"`
	SourceKind    string    `json:"source_kind,omitempty"`
	SourceID      string    `json:"source_id,omitempty"`
	ActivityID    string    `json:"activity_id,omitempty"`
	TransferID    string    `json:"transfer_id,omitempty"`
	ReversalOf    string    `json:"reversal_of,omitempty"`
	CreatedBy     string    `json:"created_by,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// EntryListResp is a cursor page of ledger entries.
type EntryListResp struct {
	Data       []EntryResp `json:"data"`
	NextCursor string      `json:"next_cursor"`
}

// TransferResp carries both legs of a transfer.
type TransferResp struct {
	TransferID string    `json:"transfer_id"`
	From       EntryResp `json:"from"`
	To         EntryResp `json:"to"`
}

// CreditReq is a manual credit. kind is restricted to credit kinds.
type CreditReq struct {
	Amount      int64  `json:"amount"      validate:"required,gt=0"`
	Kind        string `json:"kind"        validate:"required,oneof=earn bonus reward adjustment"`
	Description string `json:"description" validate:"max=255"`
}

// DebitReq is a manual debit. kind is restricted to debit kinds.
type DebitReq struct {
	Amount      int64  `json:"amount"      validate:"required,gt=0"`
	Kind        string `json:"kind"        validate:"required,oneof=spend redeem penalty expire"`
	Description string `json:"description" validate:"max=255"`
}

// TransferReq moves points between two players of the caller's tenant.
type TransferReq struct {
	FromPlayerID string `json:"from_player_id" validate:"required,uuid"`
	ToPlayerID   string `json:"to_player_id"   validate:"required,uuid,nefield=FromPlayerID"`
	Amount       int64  `json:"amount"         validate:"required,gt=0"`
	Description  string `json:"description"    validate:"max=255"`
}

// PatchWalletReq toggles a wallet; omitted fields stay untouched.
type PatchWalletReq struct {
	IsActive *bool `json:"is_active" validate:"required"`
}

// @Summary      Get a player's wallet (zero balance when never opened)
// @Tags         points
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path string true "Player id (uuid)"
// @Success      200 {object} WalletResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /players/{playerID}/wallet [get]
func (h *Handler) getWallet(w http.ResponseWriter, r *http.Request) {
	playerID, err := playerParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	wal, opened, err := h.svc.Wallet(r.Context(), playerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toWalletResp(wal, opened))
}

// @Summary      Activate or deactivate a player's wallet
// @Tags         points
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path string true "Player id (uuid)"
// @Param        body body PatchWalletReq true "Wallet state"
// @Success      200 {object} WalletResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /players/{playerID}/wallet [patch]
func (h *Handler) patchWallet(w http.ResponseWriter, r *http.Request) {
	playerID, err := playerParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req PatchWalletReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	wal, err := h.svc.SetActive(r.Context(), playerID, *req.IsActive)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toWalletResp(wal, true))
}

// @Summary      List a player's ledger entries, newest first
// @Tags         points
// @Produce      json
// @Security     BearerAuth
// @Param        playerID  path  string true  "Player id (uuid)"
// @Param        kind      query string false "Filter by kind"
// @Param        direction query string false "credit or debit"
// @Param        limit     query int    false "Page size (default 25, max 100)"
// @Param        cursor    query string false "Opaque cursor from next_cursor"
// @Success      200 {object} EntryListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /players/{playerID}/wallet/transactions [get]
func (h *Handler) listTransactions(w http.ResponseWriter, r *http.Request) {
	playerID, err := playerParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	q := r.URL.Query()
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 {
			httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "invalid limit"),
				map[string]string{"limit": "must be a positive integer"}))
			return
		}
	}
	rows, next, err := h.svc.Ledger(r.Context(), playerID, app.LedgerQuery{
		Kind:      q.Get("kind"),
		Direction: q.Get("direction"),
		Cursor:    q.Get("cursor"),
		Limit:     limit,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := EntryListResp{Data: make([]EntryResp, len(rows)), NextCursor: next}
	for i, e := range rows {
		out.Data[i] = toEntryResp(e)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Credit points to a player (manual)
// @Tags         points
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        playerID        path   string    true "Player id (uuid)"
// @Param        Idempotency-Key header string    true "Unique per operation; replays return the original entry"
// @Param        body            body   CreditReq true "Credit"
// @Success      201 {object} EntryResp
// @Success      200 {object} EntryResp "replayed key"
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /players/{playerID}/wallet/credit [post]
func (h *Handler) credit(w http.ResponseWriter, r *http.Request) {
	playerID, err := playerParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req CreditReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	entry, replay, err := h.svc.Credit(r.Context(), app.ManualMove{
		PlayerID:       playerID,
		Amount:         req.Amount,
		Kind:           req.Kind,
		Description:    req.Description,
		IdempotencyKey: r.Header.Get(idempotencyHeader),
	})
	writeEntry(w, r, entry, replay, err)
}

// @Summary      Debit points from a player (manual); never overdraws
// @Tags         points
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        playerID        path   string   true "Player id (uuid)"
// @Param        Idempotency-Key header string   true "Unique per operation; replays return the original entry"
// @Param        body            body   DebitReq true "Debit"
// @Success      201 {object} EntryResp
// @Success      200 {object} EntryResp "replayed key"
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem "insufficient_balance, wallet_inactive, player_inactive"
// @Router       /players/{playerID}/wallet/debit [post]
func (h *Handler) debit(w http.ResponseWriter, r *http.Request) {
	playerID, err := playerParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req DebitReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	entry, replay, err := h.svc.Debit(r.Context(), app.ManualMove{
		PlayerID:       playerID,
		Amount:         req.Amount,
		Kind:           req.Kind,
		Description:    req.Description,
		IdempotencyKey: r.Header.Get(idempotencyHeader),
	})
	writeEntry(w, r, entry, replay, err)
}

// @Summary      Transfer points between two players of the tenant
// @Tags         points
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        Idempotency-Key header string      true "Unique per operation; replays return the original legs"
// @Param        body            body   TransferReq true "Transfer"
// @Success      201 {object} TransferResp
// @Success      200 {object} TransferResp "replayed key"
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem "insufficient_balance, wallet_inactive, self_transfer"
// @Router       /wallets/transfer [post]
func (h *Handler) transfer(w http.ResponseWriter, r *http.Request) {
	var req TransferReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Transfer(r.Context(), app.TransferReq{
		FromPlayerID:   req.FromPlayerID,
		ToPlayerID:     req.ToPlayerID,
		Amount:         req.Amount,
		Description:    req.Description,
		IdempotencyKey: r.Header.Get(idempotencyHeader),
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	status := http.StatusCreated
	if res.Replay {
		status = http.StatusOK
	}
	httpx.JSON(w, status, TransferResp{TransferID: res.TransferID, From: toEntryResp(res.Out), To: toEntryResp(res.In)})
}

func writeEntry(w http.ResponseWriter, r *http.Request, e domain.LedgerEntry, replay bool, err error) {
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	status := http.StatusCreated
	if replay {
		status = http.StatusOK
	}
	httpx.JSON(w, status, toEntryResp(e))
}

func playerParam(r *http.Request) (string, error) {
	raw := chi.URLParam(r, "playerID")
	if _, err := uuid.Parse(raw); err != nil {
		return "", errs.WithFields(errs.New(errs.Invalid, "player id must be a uuid"),
			map[string]string{"playerID": "must be a valid UUID"})
	}
	return raw, nil
}

func toWalletResp(w domain.Wallet, opened bool) WalletResp {
	out := WalletResp{
		ID:             w.ID,
		PlayerID:       w.PlayerID,
		Balance:        w.Balance.Minor(),
		LifetimeEarned: w.LifetimeEarned.Minor(),
		LifetimeSpent:  w.LifetimeSpent.Minor(),
		IsActive:       w.Active,
		Opened:         opened,
		Version:        w.Version,
	}
	if opened {
		c, u := w.CreatedAt, w.UpdatedAt
		out.CreatedAt, out.UpdatedAt = &c, &u
	}
	return out
}

func toEntryResp(e domain.LedgerEntry) EntryResp {
	dir := "credit"
	if e.Direction == domain.Debit {
		dir = "debit"
	}
	return EntryResp{
		ID:            e.ID,
		WalletID:      e.WalletID,
		PlayerID:      e.PlayerID,
		Kind:          e.Kind,
		Direction:     dir,
		Amount:        e.Amount.Minor(),
		BalanceBefore: e.BalanceBefore.Minor(),
		BalanceAfter:  e.BalanceAfter.Minor(),
		Description:   e.Description,
		SourceKind:    e.Source.Kind,
		SourceID:      e.Source.ID,
		ActivityID:    e.Source.ActivityID,
		TransferID:    e.TransferID,
		ReversalOf:    e.ReversalOf,
		CreatedBy:     e.CreatedBy,
		OccurredAt:    e.OccurredAt,
		CreatedAt:     e.CreatedAt,
	}
}
