package app

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// APIKeyRepository is the persistence API keys need (internal/repo).
type APIKeyRepository interface {
	CreateAPIKey(ctx context.Context, tx *gorm.DB, k domain.APIKey) error
	APIKeysInTenant(ctx context.Context, tenantID string, page Page) ([]domain.APIKey, error)
	APIKeyInTenantForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.APIKey, error)
	// APIKeyByPrefix returns only keys of active, non-deleted tenants.
	APIKeyByPrefix(ctx context.Context, prefix string) (domain.APIKey, error)
	RevokeAPIKey(ctx context.Context, tx *gorm.DB, id string, at time.Time) error
	TouchAPIKey(ctx context.Context, id string, at time.Time, window time.Duration) error
}

// KeyCache is the read-through cache verification uses (redis-cache via
// Deps.Cache). Unknown prefixes are negatively cached, so guessing keys
// does not reach Postgres on every attempt.
type KeyCache interface {
	GetOrLoad(ctx context.Context, key string, dst any, load func(context.Context) (any, error)) error
	Del(ctx context.Context, keys ...string) error
}

const touchWindow = 5 * time.Minute

// WithAPIKeys enables API keys; a service without them refuses every key.
func (s *Service) WithAPIKeys(repo APIKeyRepository, cache KeyCache) *Service {
	s.keys, s.keyCache = repo, cache
	return s
}

// requireHuman refuses API-key principals: a key with the admin role must
// still not be able to mint users or keys (privilege persistence after a
// leak), so administration needs a signed-in person.
func requireHuman(p authz.Principal) error {
	if p.IsAPIKey() {
		return domain.ErrHumanPrincipalRequired
	}
	return nil
}

func (s *Service) keyAdmin(ctx context.Context) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return p, err
	}
	if err := requireHuman(p); err != nil {
		return p, err
	}
	if s.keys == nil {
		return p, errs.New(errs.Unavailable, "api keys are not enabled")
	}
	return p, s.authz.Authorize(ctx, p, contracts.PermAPIKeysManage, nil)
}

type CreateAPIKeyCmd struct {
	Name      string
	RoleIDs   []int64
	ExpiresAt *time.Time
}

// CreateAPIKey mints a key; the plaintext is in the return value only.
func (s *Service) CreateAPIKey(ctx context.Context, cmd CreateAPIKeyCmd) (domain.APIKey, string, error) {
	p, err := s.keyAdmin(ctx)
	if err != nil {
		return domain.APIKey{}, "", err
	}
	roles := cmd.RoleIDs
	if len(roles) == 0 {
		roles = domain.DefaultAPIKeyRoles
	}
	if err := domain.CheckGrantable(p.RoleIDs, roles); err != nil {
		return domain.APIKey{}, "", err
	}
	k, plain, err := domain.NewAPIKey(id.NewID(), p.TenantID, cmd.Name, p.UserID, roles, cmd.ExpiresAt, s.clock.Now())
	if err != nil {
		return domain.APIKey{}, "", err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error { return s.keys.CreateAPIKey(ctx, tx, k) }); err != nil {
		return domain.APIKey{}, "", err
	}
	s.log.Info("api key created", zap.String("tenant_id", k.TenantID), zap.String("api_key_id", k.ID), zap.String("prefix", k.Prefix))
	return k, plain, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, limit int, cursor string) ([]domain.APIKey, string, error) {
	p, err := s.keyAdmin(ctx)
	if err != nil {
		return nil, "", err
	}
	page, err := pageFrom(limit, cursor)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.keys.APIKeysInTenant(ctx, p.TenantID, page)
	if err != nil {
		return nil, "", err
	}
	out, next := trimPage(rows, page.Limit, func(k domain.APIKey) (time.Time, string) { return k.CreatedAt, k.ID })
	return out, next, nil
}

// RevokeAPIKey is idempotent; the cache entry is evicted after commit, so
// the key stops working on the next request.
func (s *Service) RevokeAPIKey(ctx context.Context, keyID string) error {
	p, err := s.keyAdmin(ctx)
	if err != nil {
		return err
	}
	var prefix string
	err = s.tx(ctx, func(tx *gorm.DB) error {
		k, err := s.keys.APIKeyInTenantForUpdate(ctx, tx, p.TenantID, keyID)
		if err != nil {
			return err
		}
		prefix = k.Prefix
		if k.RevokedAt != nil {
			return nil
		}
		return s.keys.RevokeAPIKey(ctx, tx, k.ID, s.clock.Now())
	})
	if err != nil {
		return err
	}
	if s.keyCache != nil {
		_ = s.keyCache.Del(ctx, keyCacheKey(prefix))
	}
	s.log.Info("api key revoked", zap.String("tenant_id", p.TenantID), zap.String("api_key_id", keyID))
	return nil
}

// cachedKey is the cache form: the hash only, never the secret.
type cachedKey struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	SecretHash string     `json:"secret_hash"`
	RoleIDs    []int64    `json:"role_ids"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func keyCacheKey(prefix string) string { return "apikey:" + prefix }

// VerifyAPIKey implements authn.KeyVerifier (wired through the module).
// Every failure is the same ErrInvalidAPIKey: the caller learns nothing
// about which part was wrong.
func (s *Service) VerifyAPIKey(ctx context.Context, raw string) (authz.Principal, error) {
	if s.keys == nil {
		return authz.Principal{}, domain.ErrInvalidAPIKey
	}
	prefix, ok := domain.ParseAPIKey(raw)
	if !ok {
		return authz.Principal{}, domain.ErrInvalidAPIKey
	}
	load := func(ctx context.Context) (any, error) {
		k, err := s.keys.APIKeyByPrefix(ctx, prefix)
		if err != nil {
			return nil, err
		}
		return cachedKey{ID: k.ID, TenantID: k.TenantID, SecretHash: k.SecretHash, RoleIDs: k.RoleIDs, ExpiresAt: k.ExpiresAt, RevokedAt: k.RevokedAt}, nil
	}
	var c cachedKey
	var err error
	if s.keyCache != nil {
		err = s.keyCache.GetOrLoad(ctx, keyCacheKey(prefix), &c, load)
	} else {
		var v any
		if v, err = load(ctx); err == nil {
			c = v.(cachedKey)
		}
	}
	if err != nil {
		if errs.KindOf(err) != errs.NotFound {
			s.log.Error("api key lookup failed", zap.Error(err))
		}
		return authz.Principal{}, domain.ErrInvalidAPIKey
	}
	k := domain.APIKey{ID: c.ID, SecretHash: c.SecretHash, ExpiresAt: c.ExpiresAt, RevokedAt: c.RevokedAt}
	now := s.clock.Now()
	if !k.MatchesAPIKey(raw) || !k.Usable(now) {
		return authz.Principal{}, domain.ErrInvalidAPIKey
	}
	if err := s.keys.TouchAPIKey(ctx, c.ID, now, touchWindow); err != nil {
		s.log.Warn("api key last_used_at update failed", zap.Error(err))
	}
	return authz.Principal{UserID: c.ID, TenantID: c.TenantID, RoleIDs: c.RoleIDs, APIKeyID: c.ID}, nil
}
