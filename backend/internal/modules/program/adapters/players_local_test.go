package adapters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/shared/errs"
)

type stubPlayers struct {
	rows []playercontracts.PlayerSnapshot
}

func (s stubPlayers) PlayersByIDs(_ context.Context, _ string, ids []string) ([]playercontracts.PlayerSnapshot, error) {
	var out []playercontracts.PlayerSnapshot
	for _, r := range s.rows {
		for _, id := range ids {
			if r.ID == id {
				out = append(out, r)
			}
		}
	}
	return out, nil
}

func (s stubPlayers) PlayersByExternalIDs(context.Context, string, []string) ([]playercontracts.PlayerSnapshot, error) {
	return nil, nil
}

func TestLocalPlayers(t *testing.T) {
	l := NewLocalPlayers(stubPlayers{rows: []playercontracts.PlayerSnapshot{
		{ID: "p1", TenantID: "t1", ExternalID: "e1", DisplayName: "One", Active: true},
		{ID: "p2", TenantID: "t2", ExternalID: "e2", Active: true},
	}})
	ctx := context.Background()

	snap, err := l.ByID(ctx, "t1", "p1")
	require.NoError(t, err)
	require.Equal(t, "e1", snap.ExternalID)
	require.True(t, snap.Active)

	_, err = l.ByID(ctx, "t1", "missing")
	require.Equal(t, errs.NotFound, errs.KindOf(err))

	_, err = l.ByID(ctx, "t1", "p2")
	require.Equal(t, errs.NotFound, errs.KindOf(err), "another tenant's player never leaks")

	got, err := l.ByIDs(ctx, "t1", []string{"p1", "p2", "missing"})
	require.NoError(t, err)
	require.Len(t, got, 1)

	empty, err := l.ByIDs(ctx, "t1", nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
