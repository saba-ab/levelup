// Package repo implements points' persistence. Models stay unexported;
// table names derive from struct names under the points_svc. prefix.
package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/points/internal/app"
	"levelup/internal/modules/points/internal/domain"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/money"
)

// wallet → points_svc.wallets.
type wallet struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid"`
	PlayerID       string `gorm:"type:uuid"`
	Balance        int64
	LifetimeEarned int64
	LifetimeSpent  int64
	IsActive       bool
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ledgerEntry → points_svc.ledger_entries.
type ledgerEntry struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid"`
	WalletID       string `gorm:"type:uuid"`
	PlayerID       string `gorm:"type:uuid"`
	IdempotencyKey string
	Kind           string
	Direction      int16
	Amount         int64
	BalanceBefore  int64
	BalanceAfter   int64
	WalletVersion  int
	SourceKind     string
	SourceID       string
	ActivityID     string
	Description    string
	TransferID     *string `gorm:"type:uuid"`
	ReversalOf     *string `gorm:"type:uuid"`
	OccurredAt     time.Time
	CreatedBy      *string `gorm:"type:uuid"`
	CreatedAt      time.Time
}

// rejection → points_svc.rejections.
type rejection struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid"`
	IdempotencyKey string
	Command        string
	PlayerID       *string `gorm:"type:uuid"`
	Amount         int64
	Reason         string
	Available      int64
	SourceKind     string
	SourceID       string
	CreatedAt      time.Time
}

func (m wallet) toDomain() domain.Wallet {
	return domain.Wallet{
		ID:             m.ID,
		TenantID:       m.TenantID,
		PlayerID:       m.PlayerID,
		Balance:        money.Amount(m.Balance),
		LifetimeEarned: money.Amount(m.LifetimeEarned),
		LifetimeSpent:  money.Amount(m.LifetimeSpent),
		Active:         m.IsActive,
		Version:        m.Version,
		CreatedAt:      m.CreatedAt.UTC(),
		UpdatedAt:      m.UpdatedAt.UTC(),
	}
}

func walletFromDomain(w domain.Wallet) wallet {
	return wallet{
		ID:             w.ID,
		TenantID:       w.TenantID,
		PlayerID:       w.PlayerID,
		Balance:        w.Balance.Minor(),
		LifetimeEarned: w.LifetimeEarned.Minor(),
		LifetimeSpent:  w.LifetimeSpent.Minor(),
		IsActive:       w.Active,
		Version:        w.Version,
		CreatedAt:      w.CreatedAt,
		UpdatedAt:      w.UpdatedAt,
	}
}

func (m ledgerEntry) toDomain() domain.LedgerEntry {
	return domain.LedgerEntry{
		ID:             m.ID,
		TenantID:       m.TenantID,
		WalletID:       m.WalletID,
		PlayerID:       m.PlayerID,
		IdempotencyKey: m.IdempotencyKey,
		Kind:           m.Kind,
		Direction:      domain.Direction(m.Direction),
		Amount:         money.Amount(m.Amount),
		BalanceBefore:  money.Amount(m.BalanceBefore),
		BalanceAfter:   money.Amount(m.BalanceAfter),
		WalletVersion:  m.WalletVersion,
		Source:         effect.Source{Kind: m.SourceKind, ID: m.SourceID, ActivityID: m.ActivityID},
		Description:    m.Description,
		TransferID:     deref(m.TransferID),
		ReversalOf:     deref(m.ReversalOf),
		OccurredAt:     m.OccurredAt.UTC(),
		CreatedBy:      deref(m.CreatedBy),
		CreatedAt:      m.CreatedAt.UTC(),
	}
}

func entryFromDomain(e domain.LedgerEntry) ledgerEntry {
	return ledgerEntry{
		ID:             e.ID,
		TenantID:       e.TenantID,
		WalletID:       e.WalletID,
		PlayerID:       e.PlayerID,
		IdempotencyKey: e.IdempotencyKey,
		Kind:           e.Kind,
		Direction:      int16(e.Direction),
		Amount:         e.Amount.Minor(),
		BalanceBefore:  e.BalanceBefore.Minor(),
		BalanceAfter:   e.BalanceAfter.Minor(),
		WalletVersion:  e.WalletVersion,
		SourceKind:     e.Source.Kind,
		SourceID:       e.Source.ID,
		ActivityID:     e.Source.ActivityID,
		Description:    e.Description,
		TransferID:     ptr(e.TransferID),
		ReversalOf:     ptr(e.ReversalOf),
		OccurredAt:     e.OccurredAt,
		CreatedBy:      ptr(e.CreatedBy),
		CreatedAt:      e.CreatedAt,
	}
}

func (m rejection) toDomain() domain.Rejection {
	return domain.Rejection{
		ID:             m.ID,
		TenantID:       m.TenantID,
		IdempotencyKey: m.IdempotencyKey,
		Command:        m.Command,
		PlayerID:       deref(m.PlayerID),
		Amount:         money.Amount(m.Amount),
		Reason:         m.Reason,
		Available:      money.Amount(m.Available),
		Source:         effect.Source{Kind: m.SourceKind, ID: m.SourceID},
		CreatedAt:      m.CreatedAt.UTC(),
	}
}

func rejectionFromDomain(r domain.Rejection) rejection {
	return rejection{
		ID:             r.ID,
		TenantID:       r.TenantID,
		IdempotencyKey: r.IdempotencyKey,
		Command:        r.Command,
		PlayerID:       ptr(r.PlayerID),
		Amount:         r.Amount.Minor(),
		Reason:         r.Reason,
		Available:      r.Available.Minor(),
		SourceKind:     r.Source.Kind,
		SourceID:       r.Source.ID,
		CreatedAt:      r.CreatedAt,
	}
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

func (r *Postgres) handle(tx *gorm.DB) *gorm.DB {
	if tx != nil {
		return tx
	}
	return r.db
}

func (r *Postgres) EnsureWallet(ctx context.Context, tx *gorm.DB, w domain.Wallet) (bool, error) {
	m := walletFromDomain(w)
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "player_id"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert wallet", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) WalletForUpdate(ctx context.Context, tx *gorm.DB, tenantID, playerID string) (domain.Wallet, error) {
	var m wallet
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND player_id = ?", tenantID, playerID).
		Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Wallet{}, domain.ErrWalletNotFound
	case err != nil:
		return domain.Wallet{}, errs.Wrap(errs.Internal, "lock wallet", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) WalletsForUpdate(ctx context.Context, tx *gorm.DB, tenantID string, playerIDs []string) ([]domain.Wallet, error) {
	var ms []wallet
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND player_id IN ?", tenantID, playerIDs).
		Order("id").
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "lock wallets", err)
	}
	out := make([]domain.Wallet, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) SaveWallet(ctx context.Context, tx *gorm.DB, w domain.Wallet) error {
	m := walletFromDomain(w)
	res := tx.WithContext(ctx).Model(&wallet{}).
		Where("id = ? AND tenant_id = ? AND version = ?", m.ID, m.TenantID, m.Version).
		Updates(map[string]any{
			"balance":         m.Balance,
			"lifetime_earned": m.LifetimeEarned,
			"lifetime_spent":  m.LifetimeSpent,
			"is_active":       m.IsActive,
			"updated_at":      m.UpdatedAt,
			"version":         gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "save wallet", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) InsertEntries(ctx context.Context, tx *gorm.DB, entries ...domain.LedgerEntry) (bool, error) {
	if len(entries) == 0 {
		return true, nil
	}
	ms := make([]ledgerEntry, len(entries))
	for i, e := range entries {
		ms[i] = entryFromDomain(e)
	}
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).
		Create(&ms)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert ledger entries", res.Error)
	}
	return res.RowsAffected == int64(len(ms)), nil
}

func (r *Postgres) InsertRejection(ctx context.Context, tx *gorm.DB, rej domain.Rejection) (bool, error) {
	m := rejectionFromDomain(rej)
	res := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).
		Create(&m)
	if res.Error != nil {
		return false, errs.Wrap(errs.Internal, "insert rejection", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (r *Postgres) EntryByKey(ctx context.Context, tx *gorm.DB, tenantID, key string) (domain.LedgerEntry, bool, error) {
	return r.oneEntry(ctx, tx, "tenant_id = ? AND idempotency_key = ?", tenantID, key)
}

func (r *Postgres) RefundOf(ctx context.Context, tx *gorm.DB, tenantID, debitEntryID string) (domain.LedgerEntry, bool, error) {
	return r.oneEntry(ctx, tx, "tenant_id = ? AND reversal_of = ?", tenantID, debitEntryID)
}

func (r *Postgres) oneEntry(ctx context.Context, tx *gorm.DB, where string, args ...any) (domain.LedgerEntry, bool, error) {
	var ms []ledgerEntry
	if err := r.handle(tx).WithContext(ctx).Where(where, args...).Limit(1).Find(&ms).Error; err != nil {
		return domain.LedgerEntry{}, false, errs.Wrap(errs.Internal, "load ledger entry", err)
	}
	if len(ms) == 0 {
		return domain.LedgerEntry{}, false, nil
	}
	return ms[0].toDomain(), true, nil
}

func (r *Postgres) RejectionByKey(ctx context.Context, tx *gorm.DB, tenantID, key string) (domain.Rejection, bool, error) {
	var ms []rejection
	err := r.handle(tx).WithContext(ctx).
		Where("tenant_id = ? AND idempotency_key = ?", tenantID, key).
		Limit(1).Find(&ms).Error
	if err != nil {
		return domain.Rejection{}, false, errs.Wrap(errs.Internal, "load rejection", err)
	}
	if len(ms) == 0 {
		return domain.Rejection{}, false, nil
	}
	return ms[0].toDomain(), true, nil
}

// PurgeTenant removes every row of the tenant: ledger first (FK), then
// wallets. Running it twice is a no-op.
func (r *Postgres) PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error {
	db := tx.WithContext(ctx)
	// Refund entries reference debit entries of the same tenant.
	if err := db.Where("tenant_id = ? AND reversal_of IS NOT NULL", tenantID).Delete(&ledgerEntry{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge refund entries", err)
	}
	if err := db.Where("tenant_id = ?", tenantID).Delete(&ledgerEntry{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge ledger", err)
	}
	if err := db.Where("tenant_id = ?", tenantID).Delete(&rejection{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge rejections", err)
	}
	if err := db.Where("tenant_id = ?", tenantID).Delete(&wallet{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "purge wallets", err)
	}
	return nil
}

func (r *Postgres) WalletByPlayer(ctx context.Context, tenantID, playerID string) (domain.Wallet, bool, error) {
	var ms []wallet
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND player_id = ?", tenantID, playerID).
		Limit(1).Find(&ms).Error
	if err != nil {
		return domain.Wallet{}, false, errs.Wrap(errs.Internal, "load wallet", err)
	}
	if len(ms) == 0 {
		return domain.Wallet{}, false, nil
	}
	return ms[0].toDomain(), true, nil
}

func (r *Postgres) WalletsByPlayers(ctx context.Context, tenantID string, playerIDs []string) ([]domain.Wallet, error) {
	var ms []wallet
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND player_id IN ?", tenantID, playerIDs).
		Find(&ms).Error
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "load wallets", err)
	}
	out := make([]domain.Wallet, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) ListEntries(ctx context.Context, tenantID, playerID string, f app.LedgerFilter) ([]domain.LedgerEntry, error) {
	q := r.db.WithContext(ctx).
		Where("tenant_id = ? AND player_id = ?", tenantID, playerID).
		Order("created_at DESC, id DESC").
		Limit(f.Limit)
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.Direction != 0 {
		q = q.Where("direction = ?", int16(f.Direction))
	}
	if !f.BeforeAt.IsZero() {
		q = q.Where("(created_at, id) < (?, ?)", f.BeforeAt, f.BeforeID)
	}
	var ms []ledgerEntry
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list ledger", err)
	}
	out := make([]domain.LedgerEntry, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}
