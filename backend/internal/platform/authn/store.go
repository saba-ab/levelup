package authn

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"levelup/internal/platform/redis"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// ErrTokenReused is not noise: a refresh token used twice means it leaked,
// and the correct response is revoking every token for that user — which the
// index set makes possible without SCAN (PRD §7.8.2).
var ErrTokenReused = errs.New(errs.Unauthenticated, "refresh token replayed")

// RefreshStore lives on redis-core (noeviction + persistence): losing it
// logs everyone out — an accepted inconvenience, not data loss (PRD §7.8.2).
type RefreshStore struct {
	core goredis.UniversalClient
	ttl  time.Duration
	log  *zap.Logger
}

func NewRefreshStore(core redis.Core, ttl time.Duration, log *zap.Logger) *RefreshStore {
	return &RefreshStore{core: core.UniversalClient, ttl: ttl, log: log}
}

// Opaque refresh token: base64url("<userID>.<tokenID>"). The client cannot
// decompose it usefully; the server needs both halves for the keyed lookup.
func encodeToken(userID, tokenID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(userID + "." + tokenID))
}

func decodeToken(raw string) (userID, tokenID string, err error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", "", ErrMalformed
	}
	parts := strings.SplitN(string(b), ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", ErrMalformed
	}
	return parts[0], parts[1], nil
}

// DecodeUserID extracts the identity halves of an opaque refresh token —
// the credentials-owning module needs the user id after a rotation.
func DecodeUserID(raw string) (userID, tokenID string, err error) {
	return decodeToken(raw)
}

func rtKey(userID, tokenID string) string { return "rt:" + userID + ":" + tokenID }
func idxKey(userID string) string         { return "rt:idx:" + userID }

// Issue mints a refresh token and registers it in the user's index set.
func (s *RefreshStore) Issue(ctx context.Context, userID string) (string, error) {
	tokenID := id.NewID()
	pipe := s.core.TxPipeline()
	pipe.Set(ctx, rtKey(userID, tokenID), time.Now().UTC().Format(time.RFC3339), s.ttl)
	pipe.SAdd(ctx, idxKey(userID), tokenID)
	pipe.Expire(ctx, idxKey(userID), s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", errs.Wrap(errs.Unavailable, "issue refresh token", err)
	}
	return encodeToken(userID, tokenID), nil
}

// Rotate atomically consumes the presented token (GETDEL: a replayed token
// finds nothing) and issues a successor. Reuse revokes the whole family.
func (s *RefreshStore) Rotate(ctx context.Context, raw string) (string, error) {
	userID, tokenID, err := decodeToken(raw)
	if err != nil {
		return "", err
	}

	_, err = s.core.GetDel(ctx, rtKey(userID, tokenID)).Result()
	switch {
	case errors.Is(err, goredis.Nil):
		// Attack signal: revoke everything for this user (PRD §7.8.2).
		s.log.Warn("refresh token replay detected — revoking family", zap.String("user_id", userID))
		_ = s.RevokeAll(ctx, userID)
		return "", ErrTokenReused
	case err != nil:
		return "", errs.Wrap(errs.Unavailable, "rotate refresh token", err)
	}
	_ = s.core.SRem(ctx, idxKey(userID), tokenID).Err()
	return s.Issue(ctx, userID)
}

// RevokeAll deletes every refresh token the user holds — logout-everywhere
// without SCAN, which is banned in request paths (PRD §7.8.2).
func (s *RefreshStore) RevokeAll(ctx context.Context, userID string) error {
	ids, err := s.core.SMembers(ctx, idxKey(userID)).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return errs.Wrap(errs.Unavailable, "list refresh tokens", err)
	}
	keys := make([]string, 0, len(ids)+1)
	for _, tid := range ids {
		keys = append(keys, rtKey(userID, tid))
	}
	keys = append(keys, idxKey(userID))
	if err := s.core.Del(ctx, keys...).Err(); err != nil {
		return errs.Wrap(errs.Unavailable, "revoke refresh tokens", err)
	}
	return nil
}

// Deny blacklists an access token's jti for its remaining TTL (logout).
func (s *RefreshStore) Deny(ctx context.Context, jti string, remaining time.Duration) error {
	return s.core.Set(ctx, "jwt:deny:"+jti, 1, remaining).Err()
}

// IsDenied FAILS OPEN on backend errors: authentication still stands on the
// signature; the deny list only tightens logout latency.
func (s *RefreshStore) IsDenied(ctx context.Context, jti string) bool {
	n, err := s.core.Exists(ctx, "jwt:deny:"+jti).Result()
	if err != nil {
		s.log.Warn("deny-list check failed — allowing", zap.Error(err))
		return false
	}
	return n > 0
}
