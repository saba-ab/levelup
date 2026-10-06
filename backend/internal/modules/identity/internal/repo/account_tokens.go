package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/shared/errs"
)

// passwordReset → identity_svc.password_resets.
type passwordReset struct {
	ID        string `gorm:"primaryKey;type:uuid"`
	UserID    string `gorm:"type:uuid"`
	Email     string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// emailVerification → identity_svc.email_verifications.
type emailVerification struct {
	ID        string `gorm:"primaryKey;type:uuid"`
	UserID    string `gorm:"type:uuid"`
	Email     string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// invitation → identity_svc.invitations. RoleIDs uses the bigint[] text
// form, like api keys.
type invitation struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid"`
	Email          string
	Name           string
	RoleIDs        string `gorm:"column:role_ids;type:bigint[]"`
	TokenHash      string
	InvitedBy      *string `gorm:"type:uuid"`
	ExpiresAt      time.Time
	AcceptedAt     *time.Time
	AcceptedUserID *string `gorm:"type:uuid"`
	RevokedAt      *time.Time
	CreatedAt      time.Time
}

var _ app.AccountTokenRepository = (*Postgres)(nil)

// UserByIDForUpdate locks the user row (tenancy ignored: token flows).
func (r *Postgres) UserByIDForUpdate(ctx context.Context, tx *gorm.DB, id string) (domain.User, error) {
	q := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"})
	return r.loadUserWith(ctx, q, tx, "id = ?", id)
}

// ---- password resets ----

func (r *Postgres) ReplacePasswordReset(ctx context.Context, tx *gorm.DB, pr domain.PasswordReset) error {
	db := tx.WithContext(ctx)
	if err := db.Where("user_id = ?", pr.UserID).Delete(&passwordReset{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "delete earlier password resets", err)
	}
	m := passwordReset(pr)
	if err := db.Create(&m).Error; err != nil {
		return errs.Wrap(errs.Internal, "insert password reset", err)
	}
	return nil
}

func (r *Postgres) PasswordResetByHash(ctx context.Context, hash string) (domain.PasswordReset, error) {
	return loadPasswordReset(r.db.WithContext(ctx), hash)
}

func (r *Postgres) PasswordResetByHashForUpdate(ctx context.Context, tx *gorm.DB, hash string) (domain.PasswordReset, error) {
	return loadPasswordReset(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}), hash)
}

func loadPasswordReset(q *gorm.DB, hash string) (domain.PasswordReset, error) {
	var m passwordReset
	err := q.Where("token_hash = ?", hash).Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.PasswordReset{}, domain.ErrInvalidResetToken
	case err != nil:
		return domain.PasswordReset{}, errs.Wrap(errs.Internal, "load password reset", err)
	}
	m.ExpiresAt, m.CreatedAt = m.ExpiresAt.UTC(), m.CreatedAt.UTC()
	return domain.PasswordReset(m), nil
}

func (r *Postgres) MarkPasswordResetUsed(ctx context.Context, tx *gorm.DB, id string, at time.Time) error {
	if err := tx.WithContext(ctx).Model(&passwordReset{}).Where("id = ? AND used_at IS NULL", id).
		Update("used_at", at).Error; err != nil {
		return errs.Wrap(errs.Internal, "mark password reset used", err)
	}
	return nil
}

// ---- email verifications ----

func (r *Postgres) ReplaceEmailVerification(ctx context.Context, tx *gorm.DB, v domain.EmailVerification) error {
	db := tx.WithContext(ctx)
	if err := db.Where("user_id = ?", v.UserID).Delete(&emailVerification{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "delete earlier email verifications", err)
	}
	m := emailVerification(v)
	if err := db.Create(&m).Error; err != nil {
		return errs.Wrap(errs.Internal, "insert email verification", err)
	}
	return nil
}

func (r *Postgres) EmailVerificationByHashForUpdate(ctx context.Context, tx *gorm.DB, hash string) (domain.EmailVerification, error) {
	var m emailVerification
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("token_hash = ?", hash).Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.EmailVerification{}, domain.ErrInvalidVerificationToken
	case err != nil:
		return domain.EmailVerification{}, errs.Wrap(errs.Internal, "load email verification", err)
	}
	m.ExpiresAt, m.CreatedAt = m.ExpiresAt.UTC(), m.CreatedAt.UTC()
	return domain.EmailVerification(m), nil
}

func (r *Postgres) MarkEmailVerificationUsed(ctx context.Context, tx *gorm.DB, id string, at time.Time) error {
	if err := tx.WithContext(ctx).Model(&emailVerification{}).Where("id = ? AND used_at IS NULL", id).
		Update("used_at", at).Error; err != nil {
		return errs.Wrap(errs.Internal, "mark email verification used", err)
	}
	return nil
}

// ---- invitations ----

func (r *Postgres) CreateInvitation(ctx context.Context, tx *gorm.DB, inv domain.Invitation) error {
	m := invitationFromDomain(inv)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if uniqueViolation(err, "invitations_open_key") {
			// A concurrent invite of the same address won the race.
			return domain.ErrVersionConflict
		}
		return errs.Wrap(errs.Internal, "insert invitation", err)
	}
	return nil
}

func (r *Postgres) RevokeOpenInvitations(ctx context.Context, tx *gorm.DB, tenantID, email string, at time.Time) error {
	if err := tx.WithContext(ctx).Model(&invitation{}).
		Where("tenant_id = ? AND email = ? AND accepted_at IS NULL AND revoked_at IS NULL", tenantID, email).
		Update("revoked_at", at).Error; err != nil {
		return errs.Wrap(errs.Internal, "revoke open invitations", err)
	}
	return nil
}

func (r *Postgres) OpenInvitations(ctx context.Context, tenantID string, page app.Page) ([]domain.Invitation, error) {
	var ms []invitation
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND accepted_at IS NULL AND revoked_at IS NULL", tenantID)
	if err := keyset(q, page).Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list invitations", err)
	}
	out := make([]domain.Invitation, 0, len(ms))
	for _, m := range ms {
		inv, err := m.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, nil
}

func (r *Postgres) InvitationInTenantForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Invitation, error) {
	if !validUUID(id) {
		return domain.Invitation{}, domain.ErrInvitationNotFound
	}
	return loadInvitation(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}),
		"tenant_id = ? AND id = ?", tenantID, id)
}

func (r *Postgres) InvitationByHash(ctx context.Context, hash string) (domain.Invitation, error) {
	return loadInvitation(r.db.WithContext(ctx), "token_hash = ?", hash)
}

func (r *Postgres) InvitationByHashForUpdate(ctx context.Context, tx *gorm.DB, hash string) (domain.Invitation, error) {
	return loadInvitation(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}), "token_hash = ?", hash)
}

func loadInvitation(q *gorm.DB, where string, args ...any) (domain.Invitation, error) {
	var m invitation
	err := q.Where(where, args...).Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Invitation{}, domain.ErrInvitationNotFound
	case err != nil:
		return domain.Invitation{}, errs.Wrap(errs.Internal, "load invitation", err)
	}
	return m.toDomain()
}

func (r *Postgres) SaveInvitation(ctx context.Context, tx *gorm.DB, inv domain.Invitation) error {
	m := invitationFromDomain(inv)
	if err := tx.WithContext(ctx).Model(&invitation{}).Where("id = ?", m.ID).
		Updates(map[string]any{
			"accepted_at":      m.AcceptedAt,
			"accepted_user_id": m.AcceptedUserID,
			"revoked_at":       m.RevokedAt,
		}).Error; err != nil {
		return errs.Wrap(errs.Internal, "save invitation", err)
	}
	return nil
}

func invitationFromDomain(i domain.Invitation) invitation {
	return invitation{
		ID: i.ID, TenantID: i.TenantID, Email: i.Email, Name: i.Name,
		RoleIDs: formatInt64Array(i.RoleIDs), TokenHash: i.TokenHash, InvitedBy: nullable(i.InvitedBy),
		ExpiresAt: i.ExpiresAt, AcceptedAt: i.AcceptedAt, AcceptedUserID: nullable(i.AcceptedUserID),
		RevokedAt: i.RevokedAt, CreatedAt: i.CreatedAt,
	}
}

func (m invitation) toDomain() (domain.Invitation, error) {
	roles, err := parseInt64Array(m.RoleIDs)
	if err != nil {
		return domain.Invitation{}, errs.Wrap(errs.Internal, "decode invitation roles", err)
	}
	return domain.Invitation{
		ID: m.ID, TenantID: m.TenantID, Email: m.Email, Name: m.Name, RoleIDs: roles,
		TokenHash: m.TokenHash, InvitedBy: deref(m.InvitedBy), ExpiresAt: m.ExpiresAt.UTC(),
		AcceptedAt: m.AcceptedAt, AcceptedUserID: deref(m.AcceptedUserID), RevokedAt: m.RevokedAt,
		CreatedAt: m.CreatedAt.UTC(),
	}, nil
}
