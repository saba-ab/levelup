package app

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"levelup/internal/modules/eventcatalog/contracts"
	"levelup/internal/modules/eventcatalog/internal/domain"
	"levelup/internal/shared/errs"
)

var _ contracts.Reader = (*Service)(nil)

// EventTypesBySlugs implements contracts.Reader: the event types tenantID
// sees for the given slugs, a tenant row shadowing a global one. Unknown
// slugs are simply absent; inactive types are returned (Active=false) so the
// caller decides. Results follow the order of slugs, duplicates collapsed.
// An empty tenantID resolves against the global catalogue only.
func (s *Service) EventTypesBySlugs(ctx context.Context, tenantID string, slugs []string) ([]contracts.EventTypeSnapshot, error) {
	wanted := make([]string, 0, len(slugs))
	seen := make(map[string]bool, len(slugs))
	for _, slug := range slugs {
		slug = strings.TrimSpace(slug)
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		wanted = append(wanted, slug)
	}
	if tenantID != "" {
		if _, err := uuid.Parse(tenantID); err != nil {
			return nil, errs.Wrap(errs.Invalid, "tenant id must be a uuid", err)
		}
	}
	if len(wanted) == 0 {
		return []contracts.EventTypeSnapshot{}, nil
	}
	if len(wanted) > MaxReaderSlugs {
		return nil, errs.New(errs.Invalid, fmt.Sprintf("at most %d slugs per call", MaxReaderSlugs))
	}

	rows, err := s.repo.TypesBySlugs(ctx, Scope{TenantID: tenantID}, wanted)
	if err != nil {
		return nil, err
	}
	resolved := domain.ResolveBySlug(tenantID, rows)

	out := make([]contracts.EventTypeSnapshot, 0, len(resolved))
	for _, slug := range wanted {
		et, ok := resolved[slug]
		if !ok {
			continue
		}
		out = append(out, contracts.EventTypeSnapshot{
			ID:             et.ID,
			TenantID:       et.TenantID,
			Slug:           et.Slug,
			Name:           et.Name,
			Active:         et.Active,
			PropertySchema: maps.Clone(et.PropertySchema),
		})
	}
	return out, nil
}

// PurgeTenant handles identity's tenant.deleted.v1: every row the tenant
// owns is hard-deleted (the Laravel FKs cascaded). Global rows are never
// touched. Idempotent: a redelivery deletes nothing and publishes nothing.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	if _, err := uuid.Parse(tenantID); err != nil {
		// errs.Invalid: the payload will never succeed; park it, don't retry.
		return errs.Wrap(errs.Invalid, "tenant.deleted.v1 needs a uuid tenant_id", err)
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, tenantID)
	})
}
