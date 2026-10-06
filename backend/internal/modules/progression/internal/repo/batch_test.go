package repo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/progression/internal/app"
	"levelup/internal/modules/progression/internal/domain"
	"levelup/internal/modules/progression/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// knownPlayers resolves only the listed (tenant, player) pairs.
type knownPlayers map[string]string // player → tenant

func (k knownPlayers) ByID(_ context.Context, tenantID, playerID string) (ports.PlayerSnapshot, error) {
	if k[playerID] != tenantID {
		return ports.PlayerSnapshot{}, errs.New(errs.NotFound, "player not found")
	}
	return ports.PlayerSnapshot{ID: playerID, TenantID: tenantID, Active: true}, nil
}

func (k knownPlayers) ByIDs(ctx context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	out := map[string]ports.PlayerSnapshot{}
	for _, pid := range ids {
		if p, err := k.ByID(ctx, tenantID, pid); err == nil {
			out[pid] = p
		}
	}
	return out, nil
}

type allowAll struct{}

func (allowAll) Authorize(context.Context, authz.Principal, authz.Permission, any) error { return nil }

func TestBatchProgressAgainstPostgres(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	rich, fresh, foreign := id.NewID(), id.NewID(), id.NewID()

	inTx(t, db, func(tx *gorm.DB) error {
		for i, xp := range []int64{0, 100, 250} {
			if err := r.CreateLevel(ctx, tx, mustLevel(t, tenant, i+1, xp)); err != nil {
				return err
			}
		}
		p := domain.NewProgress(tenant, rich, now)
		if err := r.EnsureProgress(ctx, tx, p); err != nil {
			return err
		}
		p.TotalXP = 175
		return r.SaveProgress(ctx, tx, p)
	})

	players := knownPlayers{rich: tenant, fresh: tenant, foreign: id.NewID()}
	svc := app.NewService(r, players, nil, allowAll{}, db, clock.System(), nil, app.Options{})
	caller := authz.Into(ctx, authz.Principal{UserID: id.NewID(), TenantID: tenant})

	views, err := svc.BatchProgress(caller, []string{fresh, foreign, rich})
	require.NoError(t, err)
	require.Len(t, views, 2)
	require.Equal(t, fresh, views[0].PlayerID)
	require.Equal(t, int64(0), views[0].TotalXP)
	require.Equal(t, 1, views[0].Current.Number)
	require.Equal(t, rich, views[1].PlayerID)
	require.Equal(t, int64(175), views[1].TotalXP)
	require.Equal(t, 2, views[1].Current.Number)
	require.Equal(t, int64(75), *views[1].XPToNext)

	single, err := svc.GetProgress(caller, rich)
	require.NoError(t, err)
	require.Equal(t, single, views[1])
}
