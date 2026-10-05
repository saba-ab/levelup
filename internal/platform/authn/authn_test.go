package authn_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"myapp/internal/platform/authn"
	"myapp/internal/platform/clock"
	"myapp/internal/platform/redis"
	"myapp/internal/platform/redis/redistest"
	"myapp/internal/shared/errs"
)

const secret = "0123456789abcdef0123456789abcdef"

func issuer(c clock.Clock) *authn.Issuer {
	return authn.NewIssuer(secret, 15*time.Minute, c)
}

func TestIssueAndVerifyRoundTrip(t *testing.T) {
	iss := issuer(clock.System())
	token, jti, err := iss.IssueAccess("user-1", []int64{1, 7})
	require.NoError(t, err)
	require.NotEmpty(t, jti)

	p, gotJTI, err := iss.Verify(token)
	require.NoError(t, err)
	require.Equal(t, "user-1", p.UserID)
	require.Equal(t, []int64{1, 7}, p.RoleIDs)
	require.Equal(t, jti, gotJTI)
}

// R18: expired, malformed and wrong-signature tokens are DISTINCT errors so
// the logs can tell an attack from clock skew from a bug.
func TestVerifyDistinguishesFailureModes(t *testing.T) {
	past := clock.NewFake(time.Now().Add(-2 * time.Hour))
	expired, _, err := issuer(past).IssueAccess("user-1", nil)
	require.NoError(t, err)

	iss := issuer(clock.System())

	_, _, err = iss.Verify(expired)
	require.ErrorIs(t, err, authn.ErrExpired)
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))

	_, _, err = iss.Verify("not-a-jwt-at-all")
	require.ErrorIs(t, err, authn.ErrMalformed)
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))

	other := authn.NewIssuer("ffffffffffffffffffffffffffffffff", 15*time.Minute, clock.System())
	forged, _, err := other.IssueAccess("user-1", nil)
	require.NoError(t, err)
	_, _, err = iss.Verify(forged)
	require.ErrorIs(t, err, authn.ErrBadSignature)
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
}

func store(t *testing.T) *authn.RefreshStore {
	t.Helper()
	return authn.NewRefreshStore(redis.NewCore(redistest.Addr(t)), 720*time.Hour, zap.NewNop())
}

func TestRefreshRotation(t *testing.T) {
	s := store(t)
	ctx := context.Background()

	tok, err := s.Issue(ctx, "u1")
	require.NoError(t, err)

	tok2, err := s.Rotate(ctx, tok)
	require.NoError(t, err)
	require.NotEqual(t, tok, tok2)

	// The used token is gone: rotation is GETDEL-atomic (PRD §7.8.2).
	_, err = s.Rotate(ctx, tok)
	require.ErrorIs(t, err, authn.ErrTokenReused)
}

// PRD §7.8.2: a replayed refresh token means it leaked — revoke the family.
func TestReuseRevokesWholeFamily(t *testing.T) {
	s := store(t)
	ctx := context.Background()

	first, err := s.Issue(ctx, "u2")
	require.NoError(t, err)
	second, err := s.Rotate(ctx, first)
	require.NoError(t, err)

	_, err = s.Rotate(ctx, first) // replayed old token
	require.ErrorIs(t, err, authn.ErrTokenReused)

	_, err = s.Rotate(ctx, second)
	require.Error(t, err, "reuse must revoke every token for the user")
}

func TestRevokeAll(t *testing.T) {
	s := store(t)
	ctx := context.Background()

	a, err := s.Issue(ctx, "u3")
	require.NoError(t, err)
	b, err := s.Issue(ctx, "u3")
	require.NoError(t, err)

	require.NoError(t, s.RevokeAll(ctx, "u3"))
	_, err = s.Rotate(ctx, a)
	require.Error(t, err)
	_, err = s.Rotate(ctx, b)
	require.Error(t, err)
}

func TestDenyList(t *testing.T) {
	s := store(t)
	ctx := context.Background()

	require.False(t, s.IsDenied(ctx, "jti-1"))
	require.NoError(t, s.Deny(ctx, "jti-1", time.Minute))
	require.True(t, s.IsDenied(ctx, "jti-1"))
}

func TestMalformedRefreshToken(t *testing.T) {
	s := store(t)
	_, err := s.Rotate(context.Background(), "garbage-token")
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
}
