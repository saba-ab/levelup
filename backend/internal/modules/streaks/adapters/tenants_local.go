package adapters

import (
	"context"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/streaks/internal/ports"
)

// LocalTenants reads tenants in-process through identity's TenantReader.
type LocalTenants struct {
	tenants identitycontracts.TenantReader
}

func NewLocalTenants(tenants identitycontracts.TenantReader) *LocalTenants {
	return &LocalTenants{tenants: tenants}
}

var _ ports.TenantReader = (*LocalTenants)(nil)

func (l *LocalTenants) TenantsByIDs(ctx context.Context, ids []string) (map[string]ports.TenantSnapshot, error) {
	out := make(map[string]ports.TenantSnapshot, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	got, err := l.tenants.TenantsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, t := range got {
		out[t.ID] = ports.TenantSnapshot{ID: t.ID, Timezone: t.Timezone}
	}
	return out, nil
}
