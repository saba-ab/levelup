package repo

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/shared/errs"
)

// apiKey → identity_svc.api_keys. RoleIDs travels as Postgres' text form of
// a bigint[] ("{4,6}"): under the PgBouncer-safe simple protocol parameters
// are untyped text, which Postgres casts to the column type.
type apiKey struct {
	ID         string `gorm:"primaryKey;type:uuid"`
	TenantID   string `gorm:"type:uuid"`
	Name       string
	Prefix     string
	SecretHash string
	RoleIDs    string  `gorm:"column:role_ids;type:bigint[]"`
	CreatedBy  *string `gorm:"type:uuid"`
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
}

var _ app.APIKeyRepository = (*Postgres)(nil)

func (r *Postgres) CreateAPIKey(ctx context.Context, tx *gorm.DB, k domain.APIKey) error {
	m := apiKey{
		ID: k.ID, TenantID: k.TenantID, Name: k.Name, Prefix: k.Prefix, SecretHash: k.SecretHash,
		RoleIDs: formatInt64Array(k.RoleIDs), CreatedAt: k.CreatedAt, ExpiresAt: k.ExpiresAt,
	}
	if k.CreatedBy != "" {
		m.CreatedBy = &k.CreatedBy
	}
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		return errs.Wrap(errs.Internal, "insert api key", err)
	}
	return nil
}

func (r *Postgres) APIKeysInTenant(ctx context.Context, tenantID string, page app.Page) ([]domain.APIKey, error) {
	var ms []apiKey
	if err := keyset(r.db.WithContext(ctx).Where("tenant_id = ?", tenantID), page).Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list api keys", err)
	}
	out := make([]domain.APIKey, 0, len(ms))
	for _, m := range ms {
		k, err := m.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

func (r *Postgres) APIKeyInTenantForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.APIKey, error) {
	var m apiKey
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND id = ?", tenantID, id).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.APIKey{}, domain.ErrAPIKeyNotFound
	}
	if err != nil {
		return domain.APIKey{}, errs.Wrap(errs.Internal, "load api key", err)
	}
	return m.toDomain()
}

// APIKeyByPrefix is the authentication lookup. The tenant must still be
// active and not deleted; a key of a suspended tenant authenticates nothing.
func (r *Postgres) APIKeyByPrefix(ctx context.Context, prefix string) (domain.APIKey, error) {
	var m apiKey
	err := r.db.WithContext(ctx).
		Where("prefix = ?", prefix).
		Where("EXISTS (SELECT 1 FROM identity_svc.tenants t WHERE t.id = api_keys.tenant_id AND t.active AND t.deleted_at IS NULL)").
		Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.APIKey{}, domain.ErrAPIKeyNotFound
	}
	if err != nil {
		return domain.APIKey{}, errs.Wrap(errs.Internal, "load api key by prefix", err)
	}
	return m.toDomain()
}

func (r *Postgres) RevokeAPIKey(ctx context.Context, tx *gorm.DB, id string, at time.Time) error {
	if err := tx.WithContext(ctx).Model(&apiKey{}).Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", at).Error; err != nil {
		return errs.Wrap(errs.Internal, "revoke api key", err)
	}
	return nil
}

// TouchAPIKey records use at most once per window: a hot key costs one
// write every few minutes, not one per request.
func (r *Postgres) TouchAPIKey(ctx context.Context, id string, at time.Time, window time.Duration) error {
	if err := r.db.WithContext(ctx).Model(&apiKey{}).
		Where("id = ? AND (last_used_at IS NULL OR last_used_at < ?)", id, at.Add(-window)).
		Update("last_used_at", at).Error; err != nil {
		return errs.Wrap(errs.Internal, "touch api key", err)
	}
	return nil
}

func (m apiKey) toDomain() (domain.APIKey, error) {
	roles, err := parseInt64Array(m.RoleIDs)
	if err != nil {
		return domain.APIKey{}, errs.Wrap(errs.Internal, "decode api key roles", err)
	}
	k := domain.APIKey{
		ID: m.ID, TenantID: m.TenantID, Name: m.Name, Prefix: m.Prefix, SecretHash: m.SecretHash,
		RoleIDs: roles, CreatedAt: m.CreatedAt, LastUsedAt: m.LastUsedAt, ExpiresAt: m.ExpiresAt, RevokedAt: m.RevokedAt,
	}
	if m.CreatedBy != nil {
		k.CreatedBy = *m.CreatedBy
	}
	return k, nil
}

func formatInt64Array(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func parseInt64Array(s string) ([]int64, error) {
	s = strings.Trim(s, "{}")
	if s == "" {
		return []int64{}, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int64, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
