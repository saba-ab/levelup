package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
)

func TestNewAPIKeyFormatAndParse(t *testing.T) {
	now := time.Now()
	k, plain, err := domain.NewAPIKey("id", "tenant", "  Backend  ", "user", nil, nil, now)
	require.NoError(t, err)
	require.Equal(t, "Backend", k.Name)
	require.Equal(t, domain.DefaultAPIKeyRoles, k.RoleIDs)
	require.Len(t, plain, len("lvl_live_")+10+1+32)

	prefix, ok := domain.ParseAPIKey(plain)
	require.True(t, ok)
	require.Equal(t, k.Prefix, prefix)
	require.True(t, k.MatchesAPIKey(plain))
	require.False(t, k.MatchesAPIKey(plain+"x"))

	_, other, err := domain.NewAPIKey("id2", "tenant", "x", "user", nil, nil, now)
	require.NoError(t, err)
	require.NotEqual(t, plain, other, "keys are random")
}

func TestParseAPIKeyRejectsMalformed(t *testing.T) {
	for _, raw := range []string{"", "lvl_live_", "lvl_test_abcdefghij_" + strings.Repeat("a", 32),
		"lvl_live_abcdefghij-" + strings.Repeat("a", 32), "lvl_live_ABCDEFGHIJ_" + strings.Repeat("a", 32),
		"lvl_live_abcdefghij_" + strings.Repeat("a", 31), "lvl_live_abcdefghi1_" + strings.Repeat("a", 32)} {
		_, ok := domain.ParseAPIKey(raw)
		require.False(t, ok, raw)
	}
}

func TestAPIKeyValidationAndUsability(t *testing.T) {
	now := time.Now()
	_, _, err := domain.NewAPIKey("id", "t", " ", "u", nil, nil, now)
	require.ErrorIs(t, err, domain.ErrInvalidAPIKeyName)
	_, _, err = domain.NewAPIKey("id", "t", "k", "u", []int64{contracts.RoleOwner}, nil, now)
	require.ErrorIs(t, err, domain.ErrAPIKeyRoleNotAllowed)
	past := now.Add(-time.Second)
	_, _, err = domain.NewAPIKey("id", "t", "k", "u", nil, &past, now)
	require.ErrorIs(t, err, domain.ErrAPIKeyExpiryInPast)

	exp := now.Add(time.Hour)
	k, _, err := domain.NewAPIKey("id", "t", "k", "u", []int64{contracts.RoleAdmin, contracts.RoleAdmin}, &exp, now)
	require.NoError(t, err)
	require.Equal(t, []int64{contracts.RoleAdmin}, k.RoleIDs)
	require.True(t, k.Usable(now))
	require.False(t, k.Usable(exp))
	k.RevokedAt = &now
	require.False(t, k.Usable(now))
}
