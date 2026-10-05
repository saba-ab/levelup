package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/shared/errs"
)

var now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func TestSlugify(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Acme Corp", "acme-corp"},
		{"  Hello,   World!! ", "hello-world"},
		{"Ünïcode Café", "n-code-caf"},
		{"---", ""},
		{"ABC123", "abc123"},
		{strings.Repeat("a", 100), strings.Repeat("a", 48)},
	} {
		require.Equal(t, tc.want, Slugify(tc.in), tc.in)
	}
	require.Equal(t, "tenant-x1y2z3", SlugFor("!!!", "x1y2z3"))
	require.Equal(t, "acme-abc123", SlugFor("Acme", "abc123"))
}

func TestValidateTimezone(t *testing.T) {
	for tz, ok := range map[string]bool{
		"UTC": true, "Asia/Tbilisi": true, "America/New_York": true,
		"": false, "Local": false, "Mars/Olympus": false, "+04:00": false,
	} {
		err := ValidateTimezone(tz)
		if ok {
			require.NoError(t, err, tz)
		} else {
			require.ErrorIs(t, err, ErrInvalidTimezone, tz)
		}
	}
}

func TestPasswordPolicyCountsBytes(t *testing.T) {
	require.ErrorIs(t, CheckPasswordPolicy("short"), ErrPasswordTooShort)
	require.NoError(t, CheckPasswordPolicy(strings.Repeat("a", 72)))
	require.ErrorIs(t, CheckPasswordPolicy(strings.Repeat("a", 73)), ErrPasswordTooLong)
	// 25 three-byte runes = 75 bytes: too long even though only 25 characters.
	require.ErrorIs(t, CheckPasswordPolicy(strings.Repeat("€", 25)), ErrPasswordTooLong)

	require.Len(t, LegacyTruncate(strings.Repeat("b", 100)), 72)
	require.Equal(t, []byte("abc"), LegacyTruncate("abc"))
}

func TestNewTenant(t *testing.T) {
	tn, err := NewTenant("  Acme  ", "", "abc123", now)
	require.NoError(t, err)
	require.Equal(t, "Acme", tn.Name)
	require.Equal(t, "acme-abc123", tn.Slug)
	require.Equal(t, "UTC", tn.Timezone)
	require.True(t, tn.IsActive())

	_, err = NewTenant("  ", "", "x", now)
	require.ErrorIs(t, err, ErrInvalidName)
	_, err = NewTenant("Acme", "Nowhere/City", "x", now)
	require.ErrorIs(t, err, ErrInvalidTimezone)
}

func TestTenantApply(t *testing.T) {
	tn, err := NewTenant("Acme", "UTC", "s", now)
	require.NoError(t, err)

	name, tz := "Acme", "UTC"
	changed, err := tn.Apply(TenantChanges{Name: &name, Timezone: &tz}, now)
	require.NoError(t, err)
	require.False(t, changed, "same values are not a change")

	tz = "Asia/Tbilisi"
	changed, err = tn.Apply(TenantChanges{Timezone: &tz, Settings: map[string]any{"a": 1.0}}, now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "Asia/Tbilisi", tn.Timezone)
	require.Equal(t, "acme-s", tn.Slug, "slug is stable across renames")

	bad := "Not/AZone"
	_, err = tn.Apply(TenantChanges{Timezone: &bad}, now)
	require.ErrorIs(t, err, ErrInvalidTimezone)

	tn.SoftDelete(now)
	require.False(t, tn.IsActive())
}

func TestNewUserNormalisesEmail(t *testing.T) {
	u, err := NewUser("t1", "Jane", "  Jane@Example.COM ", "h", []int64{4, 2, 4}, now)
	require.NoError(t, err)
	require.Equal(t, "jane@example.com", u.Email)
	require.Equal(t, []int64{2, 4}, u.RoleIDs)
	require.True(t, u.Active)

	for _, bad := range []string{"", "nope", "a@", "Jane <jane@example.com>"} {
		_, err := NewUser("t1", "Jane", bad, "h", nil, now)
		require.ErrorIs(t, err, ErrInvalidEmail, bad)
	}
	require.Equal(t, "jane", NameFromEmail("jane@example.com"))
}

func TestChangeEmailResetsVerification(t *testing.T) {
	u, err := NewUser("t1", "Jane", "jane@example.com", "h", nil, now)
	require.NoError(t, err)
	v := now
	u.EmailVerifiedAt = &v

	changed, err := u.ChangeEmail("JANE@example.com", now)
	require.NoError(t, err)
	require.False(t, changed)
	require.NotNil(t, u.EmailVerifiedAt)

	changed, err = u.ChangeEmail("new@example.com", now)
	require.NoError(t, err)
	require.True(t, changed)
	require.Nil(t, u.EmailVerifiedAt)
}

func TestCheckGrantable(t *testing.T) {
	owner := []int64{contracts.RoleOwner}
	admin := []int64{contracts.RoleAdmin}
	for _, tc := range []struct {
		name      string
		caller    []int64
		requested []int64
		want      error
	}{
		{"owner grants anything tenant", owner, []int64{2, 3, 4, 5, 6}, nil},
		{"admin grants admin and below", admin, []int64{4, 5, 6}, nil},
		{"admin cannot grant super admin", admin, []int64{3}, ErrRoleEscalation},
		{"admin cannot grant owner", admin, []int64{2}, ErrRoleEscalation},
		{"nobody grants platform admin", owner, []int64{contracts.RolePlatformAdmin}, ErrPlatformRoleForbidden},
		{"unknown role", owner, []int64{99}, ErrUnknownRole},
		{"no roles grants nothing", nil, []int64{6}, ErrRoleEscalation},
		{"empty request is fine", nil, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckGrantable(tc.caller, tc.requested)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.want)
		})
	}
	require.Equal(t, errs.PermissionDenied, errs.KindOf(ErrPlatformRoleForbidden))
	require.Equal(t, "cannot_assign_platform_role", errs.CodeOf(ErrPlatformRoleForbidden))
}

func TestCheckCanManage(t *testing.T) {
	require.NoError(t, CheckCanManage([]int64{2}, []int64{4}))
	require.NoError(t, CheckCanManage([]int64{4}, []int64{4}))
	require.NoError(t, CheckCanManage([]int64{4}, nil))
	require.ErrorIs(t, CheckCanManage([]int64{4}, []int64{2}), ErrInsufficientRank)
	require.ErrorIs(t, CheckCanManage(nil, []int64{6}), ErrInsufficientRank)
}
