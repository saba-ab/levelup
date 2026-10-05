package adapters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	playercontracts "levelup/internal/modules/player/contracts"
)

type stubPlayers struct {
	got []playercontracts.PlayerSnapshot
}

func (s stubPlayers) PlayersByIDs(context.Context, string, []string) ([]playercontracts.PlayerSnapshot, error) {
	return s.got, nil
}

func (s stubPlayers) PlayersByExternalIDs(context.Context, string, []string) ([]playercontracts.PlayerSnapshot, error) {
	return nil, nil
}

func TestLocalPlayersMapsAndFiltersTenant(t *testing.T) {
	r := NewLocalPlayers(stubPlayers{got: []playercontracts.PlayerSnapshot{
		{ID: "p1", TenantID: "t1", ExternalID: "e1", DisplayName: "Ana", Active: true},
		{ID: "p2", TenantID: "t2", ExternalID: "e2"},
	}})
	got, err := r.ByIDs(context.Background(), "t1", []string{"p1", "p2"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Ana", got["p1"].DisplayName)
	require.Equal(t, "e1", got["p1"].ExternalID)

	empty, err := r.ByIDs(context.Background(), "t1", nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
