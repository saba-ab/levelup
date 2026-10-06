package repo

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/redis/redistest"
	"levelup/internal/shared/id"
)

func seedTenantAndOwner(t *testing.T, r *Postgres, db *gorm.DB) (domain.Tenant, domain.User) {
	t.Helper()
	ctx := context.Background()
	tn, u := newTenantWithOwner(t, "tok-"+id.NewID()[24:]+"@example.com")
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		if err := r.CreateTenant(ctx, tx, tn); err != nil {
			return err
		}
		return r.CreateUser(ctx, tx, u)
	}))
	return tn, u
}

func TestPasswordResetReplaceLookupAndMarkUsed(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	_, u := seedTenantAndOwner(t, r, db)
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, h1, err := domain.NewAccountToken()
	require.NoError(t, err)
	_, h2, err := domain.NewAccountToken()
	require.NoError(t, err)
	first := domain.NewPasswordReset(u.ID, u.Email, h1, time.Hour, now)
	second := domain.NewPasswordReset(u.ID, u.Email, h2, time.Hour, now)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.ReplacePasswordReset(ctx, tx, first) }))
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.ReplacePasswordReset(ctx, tx, second) }))

	_, err = r.PasswordResetByHash(ctx, h1)
	require.ErrorIs(t, err, domain.ErrInvalidResetToken, "a newer reset deletes the older one")
	got, err := r.PasswordResetByHash(ctx, h2)
	require.NoError(t, err)
	require.Equal(t, second, got)

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		locked, err := r.PasswordResetByHashForUpdate(ctx, tx, h2)
		require.NoError(t, err)
		if err := r.MarkPasswordResetUsed(ctx, tx, locked.ID, now); err != nil {
			return err
		}
		lockedUser, err := r.UserByIDForUpdate(ctx, tx, u.ID)
		require.NoError(t, err)
		require.Equal(t, []int64{contracts.RoleOwner}, lockedUser.RoleIDs)
		return nil
	}))
	got, err = r.PasswordResetByHash(ctx, h2)
	require.NoError(t, err)
	require.False(t, got.Usable(now))

	// A malformed id never reaches Postgres.
	_, err = r.UserByIDForUpdate(ctx, db, "not-a-uuid")
	require.ErrorIs(t, err, domain.ErrUserNotFound)
}

func TestEmailVerificationReplaceAndMarkUsed(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	_, u := seedTenantAndOwner(t, r, db)
	now := time.Now().UTC().Truncate(time.Microsecond)

	_, h1, err := domain.NewAccountToken()
	require.NoError(t, err)
	_, h2, err := domain.NewAccountToken()
	require.NoError(t, err)
	for _, h := range []string{h1, h2} {
		v := domain.NewEmailVerification(u.ID, u.Email, h, time.Hour, now)
		require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.ReplaceEmailVerification(ctx, tx, v) }))
	}
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		_, err := r.EmailVerificationByHashForUpdate(ctx, tx, h1)
		return err
	})
	require.ErrorIs(t, err, domain.ErrInvalidVerificationToken)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		v, err := r.EmailVerificationByHashForUpdate(ctx, tx, h2)
		if err != nil {
			return err
		}
		require.Equal(t, u.Email, v.Email)
		return r.MarkEmailVerificationUsed(ctx, tx, v.ID, now)
	}))
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		v, err := r.EmailVerificationByHashForUpdate(ctx, tx, h2)
		require.NotNil(t, v.UsedAt)
		return err
	}))
}

// One open invitation per (tenant, email): the partial unique index, the
// revoke-then-insert re-invite, role_ids round trip and the open list.
func TestInvitationPersistence(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tn, owner := seedTenantAndOwner(t, r, db)
	other, _ := seedTenantAndOwner(t, r, db)
	now := time.Now().UTC().Truncate(time.Microsecond)

	newInv := func(tenantID string, at time.Time) domain.Invitation {
		_, h, err := domain.NewAccountToken()
		require.NoError(t, err)
		inv, err := domain.NewInvitation(tenantID, "Guest@Example.com", "Guest", owner.ID, h,
			[]int64{contracts.RoleDeveloper, contracts.RoleAdmin}, 7*24*time.Hour, at)
		require.NoError(t, err)
		return inv
	}
	first := newInv(tn.ID, now)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateInvitation(ctx, tx, first) }))

	dup := newInv(tn.ID, now.Add(time.Second))
	err := postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateInvitation(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrVersionConflict, "a second open invitation for the address is refused")

	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		if err := r.RevokeOpenInvitations(ctx, tx, tn.ID, dup.Email, now); err != nil {
			return err
		}
		return r.CreateInvitation(ctx, tx, dup)
	}))
	otherInv := newInv(other.ID, now)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateInvitation(ctx, tx, otherInv) }))

	open, err := r.OpenInvitations(ctx, tn.ID, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, open, 1)
	require.Equal(t, dup.ID, open[0].ID)
	require.Equal(t, []int64{contracts.RoleAdmin, contracts.RoleDeveloper}, open[0].RoleIDs)
	require.Equal(t, "guest@example.com", open[0].Email)
	require.Equal(t, owner.ID, open[0].InvitedBy)

	revoked, err := r.InvitationByHash(ctx, first.TokenHash)
	require.NoError(t, err)
	require.Equal(t, domain.InvitationRevoked, revoked.Status(now))

	// Tenant scoping on the admin lookup.
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		_, err := r.InvitationInTenantForUpdate(ctx, tx, tn.ID, otherInv.ID)
		return err
	})
	require.ErrorIs(t, err, domain.ErrInvitationNotFound)

	// Accept: the accepted row leaves the open list.
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		inv, err := r.InvitationByHashForUpdate(ctx, tx, dup.TokenHash)
		if err != nil {
			return err
		}
		require.NoError(t, inv.Accept(owner.ID, now))
		return r.SaveInvitation(ctx, tx, inv)
	}))
	open, err = r.OpenInvitations(ctx, tn.ID, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Empty(t, open)
	accepted, err := r.InvitationByHash(ctx, dup.TokenHash)
	require.NoError(t, err)
	require.Equal(t, owner.ID, accepted.AcceptedUserID)
	require.Equal(t, domain.InvitationAccepted, accepted.Status(now))
}

func TestThrottleFixedWindowOnRedis(t *testing.T) {
	rdb := goredis.NewClient(&goredis.Options{Addr: redistest.Addr(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	th := NewThrottle(rdb)
	key := "test:" + id.NewID()

	for i := range 3 {
		ok, err := th.Allow(ctx, key, 3, time.Hour)
		require.NoError(t, err)
		require.True(t, ok, "hit %d", i+1)
	}
	ok, err := th.Allow(ctx, key, 3, time.Hour)
	require.NoError(t, err)
	require.False(t, ok, "the fourth hit in the window is refused")

	ttl, err := rdb.TTL(ctx, "identity:throttle:"+key).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, 59*time.Minute, "the window is set once, by the first hit")

	other, err := th.Allow(ctx, key+"-other", 3, time.Hour)
	require.NoError(t, err)
	require.True(t, other)
}
