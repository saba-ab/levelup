package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"strings"
	"time"

	"levelup/internal/modules/identity/contracts"
)

// API keys let a tenant's backend call the API without a person's session
// (ADR-0017). Format: lvl_live_<prefix>_<secret>. The prefix (10 chars) is
// stored in clear for lookup and display; only a SHA-256 of the whole key is
// stored, so a database leak does not leak usable keys. 160 bits of secret
// entropy make a fast hash appropriate (bcrypt would only add latency to
// every request).
const (
	apiKeyScheme    = "lvl_live_"
	apiKeyPrefixLen = 10
	apiKeySecretLen = 32 // base32 chars = 160 bits
	maxAPIKeyName   = 100
)

// APIKeyRoles are the roles a key may hold: operating roles, never owner or
// super admin (a key cannot be "the owner" of anything).
var APIKeyRoles = []int64{contracts.RoleAdmin, contracts.RoleProgramManager, contracts.RoleDeveloper}

// DefaultAPIKeyRoles is what a key gets when none are requested.
var DefaultAPIKeyRoles = []int64{contracts.RoleDeveloper}

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

type APIKey struct {
	ID         string
	TenantID   string
	Name       string
	Prefix     string
	SecretHash string
	RoleIDs    []int64
	CreatedBy  string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}

// Usable reports whether the key may authenticate at now.
func (k APIKey) Usable(now time.Time) bool {
	return k.RevokedAt == nil && (k.ExpiresAt == nil || now.Before(*k.ExpiresAt))
}

// NewAPIKey validates and mints a key; the plaintext is returned exactly
// once and never stored.
func NewAPIKey(id, tenantID, name, createdBy string, roleIDs []int64, expiresAt *time.Time, now time.Time) (APIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxAPIKeyName {
		return APIKey{}, "", ErrInvalidAPIKeyName
	}
	if len(roleIDs) == 0 {
		roleIDs = DefaultAPIKeyRoles
	}
	roleIDs = NormalizeRoleIDs(roleIDs)
	for _, r := range roleIDs {
		if !containsRole(APIKeyRoles, r) {
			return APIKey{}, "", ErrAPIKeyRoleNotAllowed
		}
	}
	if expiresAt != nil && !expiresAt.After(now) {
		return APIKey{}, "", ErrAPIKeyExpiryInPast
	}
	prefix, err := randomBase32(apiKeyPrefixLen)
	if err != nil {
		return APIKey{}, "", err
	}
	secret, err := randomBase32(apiKeySecretLen)
	if err != nil {
		return APIKey{}, "", err
	}
	plain := apiKeyScheme + prefix + "_" + secret
	return APIKey{
		ID: id, TenantID: tenantID, Name: name, Prefix: prefix, SecretHash: HashAPIKey(plain),
		RoleIDs: roleIDs, CreatedBy: createdBy, CreatedAt: now, ExpiresAt: expiresAt,
	}, plain, nil
}

// ParseAPIKey extracts the lookup prefix from a presented key. ok is false
// for anything not shaped like a key, so garbage never reaches the database.
func ParseAPIKey(raw string) (prefix string, ok bool) {
	rest, found := strings.CutPrefix(raw, apiKeyScheme)
	if !found || len(rest) != apiKeyPrefixLen+1+apiKeySecretLen || rest[apiKeyPrefixLen] != '_' {
		return "", false
	}
	prefix = rest[:apiKeyPrefixLen]
	for _, c := range rest {
		if c != '_' && (c < 'a' || c > 'z') && (c < '2' || c > '7') {
			return "", false
		}
	}
	return prefix, true
}

func HashAPIKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// MatchesAPIKey compares in constant time.
func (k APIKey) MatchesAPIKey(plain string) bool {
	return subtle.ConstantTimeCompare([]byte(HashAPIKey(plain)), []byte(k.SecretHash)) == 1
}

func randomBase32(n int) (string, error) {
	buf := make([]byte, n) // 5 bits used per char: n bytes is plenty
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return strings.ToLower(b32.EncodeToString(buf))[:n], nil
}

func containsRole(set []int64, id int64) bool {
	for _, r := range set {
		if r == id {
			return true
		}
	}
	return false
}
