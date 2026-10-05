package authz_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

func TestRequireTenant(t *testing.T) {
	_, err := authz.RequireTenant(context.Background())
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))

	_, err = authz.RequireTenant(authz.Into(context.Background(), authz.Principal{UserID: "u"}))
	require.ErrorIs(t, err, authz.ErrNoTenant)

	p, err := authz.RequireTenant(authz.Into(context.Background(), authz.Principal{UserID: "u", TenantID: "t"}))
	require.NoError(t, err)
	require.Equal(t, "t", p.TenantID)
}
