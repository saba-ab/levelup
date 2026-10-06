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
		{ID: "p1", TenantID: "t1", DisplayName: "Ana", Email: "ana@example.com", ExternalID: "ext-1", Active: true},
		{ID: "p2", TenantID: "t2", Active: true},
	}})
	got, err := a.ByIDs(context.Background(), "t1", []string{"p1", "p2"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Ana", got["p1"].DisplayName)
	require.Equal(t, "ana@example.com", got["p1"].Email)
	require.Equal(t, "ext-1", got["p1"].ExternalID)

	_, found, err := a.ByID(context.Background(), "t1", "p2")
	require.NoError(t, err)
	require.False(t, found)

	empty, err := a.ByIDs(context.Background(), "t1", nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
