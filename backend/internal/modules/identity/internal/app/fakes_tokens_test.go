package app

import (
	"context"
	"slices"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/identity/internal/domain"
)

// fakeRepo also implements AccountTokenRepository, so the fake transaction
// rolls tokens back together with users.

func (f *fakeRepo) UserByIDForUpdate(ctx context.Context, _ *gorm.DB, id string) (domain.User, error) {
	u, err := f.UserByID(ctx, id)
	u.RoleIDs = slices.Clone(u.RoleIDs)
	return u, err
}

func (f *fakeRepo) ReplacePasswordReset(_ context.Context, _ *gorm.DB, r domain.PasswordReset) error {
	for id, x := range f.resets {
		if x.UserID == r.UserID {
			delete(f.resets, id)
		}
	}
	f.resets[r.ID] = r
	return nil
}

func (f *fakeRepo) PasswordResetByHash(_ context.Context, hash string) (domain.PasswordReset, error) {
	for _, r := range f.resets {
		if r.TokenHash == hash {
			return r, nil
		}
	}
	return domain.PasswordReset{}, domain.ErrInvalidResetToken
}

func (f *fakeRepo) PasswordResetByHashForUpdate(ctx context.Context, _ *gorm.DB, hash string) (domain.PasswordReset, error) {
	return f.PasswordResetByHash(ctx, hash)
}

func (f *fakeRepo) MarkPasswordResetUsed(_ context.Context, _ *gorm.DB, id string, at time.Time) error {
	r := f.resets[id]
	if r.UsedAt == nil {
		r.UsedAt = &at
	}
	f.resets[id] = r
	return nil
}

func (f *fakeRepo) ReplaceEmailVerification(_ context.Context, _ *gorm.DB, v domain.EmailVerification) error {
	for id, x := range f.verifs {
		if x.UserID == v.UserID {
			delete(f.verifs, id)
		}
	}
	f.verifs[v.ID] = v
	return nil
}

func (f *fakeRepo) EmailVerificationByHashForUpdate(_ context.Context, _ *gorm.DB, hash string) (domain.EmailVerification, error) {
	for _, v := range f.verifs {
		if v.TokenHash == hash {
			return v, nil
		}
	}
	return domain.EmailVerification{}, domain.ErrInvalidVerificationToken
}

func (f *fakeRepo) MarkEmailVerificationUsed(_ context.Context, _ *gorm.DB, id string, at time.Time) error {
	v := f.verifs[id]
	if v.UsedAt == nil {
		v.UsedAt = &at
	}
	f.verifs[id] = v
	return nil
}

func (f *fakeRepo) CreateInvitation(_ context.Context, _ *gorm.DB, inv domain.Invitation) error {
	for _, x := range f.invites {
		if x.TenantID == inv.TenantID && x.Email == inv.Email && x.AcceptedAt == nil && x.RevokedAt == nil {
			return domain.ErrVersionConflict
		}
	}
	f.invites[inv.ID] = inv
	return nil
}

func (f *fakeRepo) RevokeOpenInvitations(_ context.Context, _ *gorm.DB, tenantID, email string, at time.Time) error {
	for id, x := range f.invites {
		if x.TenantID == tenantID && x.Email == email && x.AcceptedAt == nil && x.RevokedAt == nil {
			x.RevokedAt = &at
			f.invites[id] = x
		}
	}
	return nil
}

func (f *fakeRepo) OpenInvitations(_ context.Context, tenantID string, page Page) ([]domain.Invitation, error) {
	var all []domain.Invitation
	for _, x := range f.invites {
		if x.TenantID == tenantID && x.AcceptedAt == nil && x.RevokedAt == nil {
			all = append(all, x)
		}
	}
	return pageOf(all, page, func(i domain.Invitation) (time.Time, string) { return i.CreatedAt, i.ID }), nil
}

func (f *fakeRepo) InvitationInTenantForUpdate(_ context.Context, _ *gorm.DB, tenantID, id string) (domain.Invitation, error) {
	x, ok := f.invites[id]
	if !ok || x.TenantID != tenantID {
		return domain.Invitation{}, domain.ErrInvitationNotFound
	}
	return x, nil
}

func (f *fakeRepo) InvitationByHash(_ context.Context, hash string) (domain.Invitation, error) {
	for _, x := range f.invites {
		if x.TokenHash == hash {
			return x, nil
		}
	}
	return domain.Invitation{}, domain.ErrInvitationNotFound
}

func (f *fakeRepo) InvitationByHashForUpdate(ctx context.Context, _ *gorm.DB, hash string) (domain.Invitation, error) {
	return f.InvitationByHash(ctx, hash)
}

func (f *fakeRepo) SaveInvitation(_ context.Context, _ *gorm.DB, inv domain.Invitation) error {
	f.invites[inv.ID] = inv
	return nil
}

// fakeThrottle counts per key and ignores the window unless reset; fail
// makes it return an error (the service must fail open).
type fakeThrottle struct {
	counts map[string]int
	fail   error
}

func newFakeThrottle() *fakeThrottle { return &fakeThrottle{counts: map[string]int{}} }

func (f *fakeThrottle) Allow(_ context.Context, key string, limit int, _ time.Duration) (bool, error) {
	if f.fail != nil {
		return false, f.fail
	}
	f.counts[key]++
	return f.counts[key] <= limit, nil
}
