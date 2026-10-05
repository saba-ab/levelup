package app

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authn"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/errs"
)

// CurrentTenant returns the caller's tenant.
func (s *Service) CurrentTenant(ctx context.Context) (domain.Tenant, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Tenant{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermTenantView, nil); err != nil {
		return domain.Tenant{}, err
	}
	return s.repo.TenantByID(ctx, p.TenantID)
}

// UpdateCurrentTenant changes name, timezone (IANA) and settings of the
// caller's own tenant only (fixes doc 02 §10 bug 6). The slug is stable.
func (s *Service) UpdateCurrentTenant(ctx context.Context, c domain.TenantChanges) (domain.Tenant, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Tenant{}, err
	}
	if err := requireHuman(p); err != nil {
		return domain.Tenant{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermTenantUpdate, nil); err != nil {
		return domain.Tenant{}, err
	}
	now := s.clock.Now()
	var out domain.Tenant
	err = s.tx(ctx, func(tx *gorm.DB) error {
		t, err := s.repo.TenantByIDForUpdate(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		changed, err := t.Apply(c, now)
		if err != nil {
			return err
		}
		out = t
		if !changed {
			return nil
		}
		return s.saveTenantAndPublishUpdate(ctx, tx, &out, now)
	})
	if err != nil {
		return domain.Tenant{}, err
	}
	return out, nil
}

// DeleteCurrentTenant soft-deletes the caller's tenant. Owner only. Every
// module purges its rows on tenant.deleted.v1; members' sessions are revoked
// after commit and login fails from now on.
func (s *Service) DeleteCurrentTenant(ctx context.Context) error {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return err
	}
	if err := requireHuman(p); err != nil {
		return err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermTenantDelete, nil); err != nil {
		return err
	}
	now := s.clock.Now()
	err = s.tx(ctx, func(tx *gorm.DB) error {
		t, err := s.repo.TenantByIDForUpdate(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		if !t.OwnedBy(p.UserID) {
			return domain.ErrNotTenantOwner
		}
		t.SoftDelete(now)
		if err := s.repo.SaveTenant(ctx, tx, t); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicTenantDeleted, contracts.TenantDeletedV1{
			TenantID: t.ID, At: now,
		})
	})
	if err != nil {
		return err
	}
	if jti, ok := authn.JTIFrom(ctx); ok && jti != "" && s.refresh != nil && s.issuer != nil {
		_ = s.refresh.Deny(ctx, jti, s.issuer.AccessTTL())
	}
	s.revokeTenantSessions(ctx, p.TenantID)
	return nil
}

// requirePlatform admits only principals WITHOUT a tenant holding
// platform_tenants_manage.
func (s *Service) requirePlatform(ctx context.Context) (authz.Principal, error) {
	p, ok := authz.From(ctx)
	if !ok {
		return authz.Principal{}, errs.New(errs.Unauthenticated, "authentication required")
	}
	if p.TenantID != "" {
		return authz.Principal{}, domain.ErrPlatformOnly
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermPlatformTenantsManage, nil); err != nil {
		return authz.Principal{}, err
	}
	return p, nil
}

// PlatformListTenants pages every non-deleted tenant.
func (s *Service) PlatformListTenants(ctx context.Context, limit int, cursor string) ([]domain.Tenant, string, error) {
	if _, err := s.requirePlatform(ctx); err != nil {
		return nil, "", err
	}
	page, err := pageFrom(limit, cursor)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.repo.ListTenants(ctx, page)
	if err != nil {
		return nil, "", err
	}
	out, next := trimPage(rows, page.Limit, func(t domain.Tenant) (time.Time, string) { return t.CreatedAt, t.ID })
	return out, next, nil
}

// PlatformSetTenantActive activates or deactivates a tenant. Deactivation
// revokes every member's refresh tokens after commit.
func (s *Service) PlatformSetTenantActive(ctx context.Context, id string, active bool) (domain.Tenant, error) {
	if _, err := s.requirePlatform(ctx); err != nil {
		return domain.Tenant{}, err
	}
	now := s.clock.Now()
	var (
		out     domain.Tenant
		changed bool
	)
	err := s.tx(ctx, func(tx *gorm.DB) error {
		t, err := s.repo.TenantByIDForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		out = t
		if changed = out.SetActive(active, now); !changed {
			return nil
		}
		return s.saveTenantAndPublishUpdate(ctx, tx, &out, now)
	})
	if err != nil {
		return domain.Tenant{}, err
	}
	if changed && !active {
		s.revokeTenantSessions(ctx, id)
	}
	return out, nil
}

func (s *Service) saveTenantAndPublishUpdate(ctx context.Context, tx *gorm.DB, t *domain.Tenant, now time.Time) error {
	if err := s.repo.SaveTenant(ctx, tx, *t); err != nil {
		return err
	}
	t.Version++
	return s.outbox.Publish(ctx, tx, contracts.TopicTenantUpdated, contracts.TenantUpdatedV1{
		TenantID: t.ID, Name: t.Name, Slug: t.Slug, Active: t.IsActive(), Timezone: t.Timezone, At: now,
	})
}

func (s *Service) revokeTenantSessions(ctx context.Context, tenantID string) {
	ids, err := s.repo.UserIDsInTenant(ctx, tenantID)
	if err != nil {
		s.log.Warn("list tenant members for session revoke failed",
			zap.String("tenant_id", tenantID), zap.Error(err))
		return
	}
	s.revokeSessions(ctx, ids...)
}

// TenantsByIDs implements contracts.TenantReader. Unknown, malformed and
// soft-deleted ids are simply absent from the result.
func (s *Service) TenantsByIDs(ctx context.Context, ids []string) ([]contracts.TenantSnapshot, error) {
	valid := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, raw := range ids {
		u, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		key := u.String()
		if !seen[key] {
			seen[key] = true
			valid = append(valid, key)
		}
	}
	if len(valid) == 0 {
		return []contracts.TenantSnapshot{}, nil
	}
	rows, err := s.repo.TenantsByIDs(ctx, valid)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.TenantSnapshot, 0, len(rows))
	for _, t := range rows {
		out = append(out, contracts.TenantSnapshot{
			ID: t.ID, Name: t.Name, Slug: t.Slug, Active: t.IsActive(), Timezone: t.Timezone,
		})
	}
	return out, nil
}
