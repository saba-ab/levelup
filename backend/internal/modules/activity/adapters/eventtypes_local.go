package adapters

import (
	"context"

	"levelup/internal/modules/activity/internal/ports"
	eventcatalogcontracts "levelup/internal/modules/eventcatalog/contracts"
)

// LocalEventTypes serves activity's EventTypeReader from the in-process
// eventcatalog module. The provider already resolves tenant-over-global
// shadowing; a duplicate slug in its answer prefers the tenant's own row.
type LocalEventTypes struct {
	types eventcatalogcontracts.Reader
}

func NewLocalEventTypes(types eventcatalogcontracts.Reader) *LocalEventTypes {
	return &LocalEventTypes{types: types}
}

func (l *LocalEventTypes) BySlugs(ctx context.Context, tenantID string, slugs []string) (map[string]ports.EventTypeSnapshot, error) {
	out := map[string]ports.EventTypeSnapshot{}
	if len(slugs) == 0 {
		return out, nil
	}
	got, err := l.types.EventTypesBySlugs(ctx, tenantID, slugs)
	if err != nil {
		return nil, err
	}
	for _, t := range got {
		if _, seen := out[t.Slug]; seen && t.TenantID == "" {
			continue
		}
		out[t.Slug] = ports.EventTypeSnapshot{Slug: t.Slug, Active: t.Active}
	}
	return out, nil
}
