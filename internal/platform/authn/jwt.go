// Package authn issues and verifies identity (PRD §5): JWT access tokens,
// opaque rotating refresh tokens on redis-core, and the middleware that puts
// the Principal into context. Authorization is authz's job, not this one's.
package authn

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"myapp/internal/platform/authz"
	"myapp/internal/platform/clock"
	"myapp/internal/shared/errs"
	"myapp/internal/shared/id"
)

// Distinct verification failures (R18): the log line for a forged signature
// and the one for an expired token must never look the same.
var (
	ErrExpired      = errs.New(errs.Unauthenticated, "token expired")
	ErrMalformed    = errs.New(errs.Unauthenticated, "token malformed")
	ErrBadSignature = errs.New(errs.Unauthenticated, "token signature invalid")
	ErrDenied       = errs.New(errs.Unauthenticated, "token revoked")
)

type Issuer struct {
	secret    []byte
	accessTTL time.Duration
	clock     clock.Clock
}

func NewIssuer(secret string, accessTTL time.Duration, c clock.Clock) *Issuer {
	return &Issuer{secret: []byte(secret), accessTTL: accessTTL, clock: c}
}

type claims struct {
	RoleIDs []int64 `json:"rids,omitempty"`
	jwt.RegisteredClaims
}

// IssueAccess mints an HS256 access token. jti is returned so logout can
// deny-list exactly this token for its remaining TTL (PRD §7.8).
func (i *Issuer) IssueAccess(userID string, roleIDs []int64) (token, jti string, err error) {
	now := i.clock.Now()
	jti = id.NewID()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		RoleIDs: roleIDs,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.accessTTL)),
		},
	})
	signed, err := t.SignedString(i.secret)
	if err != nil {
		return "", "", errs.Wrap(errs.Internal, "sign token", err)
	}
	return signed, jti, nil
}

func (i *Issuer) AccessTTL() time.Duration { return i.accessTTL }

// Verify parses and validates, mapping the library's failure modes onto the
// package's distinct sentinels.
func (i *Issuer) Verify(token string) (authz.Principal, string, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return i.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	switch {
	case err == nil:
	case errors.Is(err, jwt.ErrTokenExpired):
		return authz.Principal{}, "", ErrExpired
	case errors.Is(err, jwt.ErrSignatureInvalid):
		return authz.Principal{}, "", ErrBadSignature
	default:
		return authz.Principal{}, "", ErrMalformed
	}
	return authz.Principal{UserID: c.Subject, RoleIDs: c.RoleIDs}, c.ID, nil
}
