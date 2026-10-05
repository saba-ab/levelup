package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/id"
)

// The role_ids text form and the active-tenant join only prove themselves
// against real Postgres through the PgBouncer-safe exec mode.
func TestAPIKeyPersistenceRoundTrip(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tn, u := newTenantWithOwner(t, "keys-"+id.NewID()[24:]+"@example.com")
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		if err := r.CreateTenant(ctx, tx, tn); err != nil {
			return err
		}
		return r.CreateUser(ctx, tx, u)
	}))

	now := time.Now().UTC().Truncate(time.Microsecond)
	k, plain, err := domain.NewAPIKey(id.NewID(), tn.ID, "Backend", u.ID,
		[]int64{contracts.RoleDeveloper, contracts.RoleAdmin}, nil, now)
	require.NoError(t, err)
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateAPIKey(ctx, tx, k) }))

	got, err := r.APIKeyByPrefix(ctx, k.Prefix)
	require.NoError(t, err)
	require.Equal(t, []int64{contracts.RoleAdmin, contracts.RoleDeveloper}, got.RoleIDs)
	require.True(t, got.MatchesAPIKey(plain))
	require.Equal(t, u.ID, got.CreatedBy)

	list, err := r.APIKeysInTenant(ctx, tn.ID, app.Page{Limit: 10})
	require.NoError(t, err)
	require.Len(t, list, 1)

	// last_used_at is written at most once per window.
	require.NoError(t, r.TouchAPIKey(ctx, k.ID, now, 5*time.Minute))
	require.NoError(t, r.TouchAPIKey(ctx, k.ID, now.Add(time.Minute), 5*time.Minute))
	got, err = r.APIKeyByPrefix(ctx, k.Prefix)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	require.True(t, got.LastUsedAt.Equal(now))

	// Revocation and tenant suspension.
	require.NoError(t, postgres.InTx(ctx, db, func(tx *gorm.DB) error {
		locked, err := r.APIKeyInTenantForUpdate(ctx, tx, tn.ID, k.ID)
		require.NoError(t, err)
		return r.RevokeAPIKey(ctx, tx, locked.ID, now)
	}))
	got, err = r.APIKeyByPrefix(ctx, k.Prefix)
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)

	require.NoError(t, db.Exec("UPDATE identity_svc.tenants SET active = false WHERE id = ?", tn.ID).Error)
	_, err = r.APIKeyByPrefix(ctx, k.Prefix)
	require.ErrorIs(t, err, domain.ErrAPIKeyNotFound)

	_, err = r.APIKeyInTenantForUpdate(ctx, db, id.NewID(), k.ID)
	require.ErrorIs(t, err, domain.ErrAPIKeyNotFound, "another tenant cannot load the key")
}
