package adapters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	playercontracts "levelup/internal/modules/player/contracts"
)

type stubPlayers struct {
	rows []playercontracts.PlayerSnapshot
}

func (s stubPlayers) PlayersByIDs(context.Context, string, []string) ([]playercontracts.PlayerSnapshot, error) {
	return s.rows, nil
}

func (s stubPlayers) PlayersByExternalIDs(context.Context, string, []string) ([]playercontracts.PlayerSnapshot, error) {
	return nil, nil
}

func TestLocalPlayersMapsAndDropsForeignTenants(t *testing.T) {
	a := NewLocalPlayers(stubPlayers{rows: []playercontracts.PlayerSnapshot{
		{ID: "p1", TenantID: "t1", Active: true},
		{ID: "p2", TenantID: "t2", Active: true},
	}})
	got, err := a.ByIDs(context.Background(), "t1", []string{"p1", "p2"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.True(t, got["p1"].Active)

	_, found, err := a.ByID(context.Background(), "t1", "p2")
	require.NoError(t, err)
	require.False(t, found)
}
