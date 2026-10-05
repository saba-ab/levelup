package adapters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	playercontracts "levelup/internal/modules/player/contracts"
	"levelup/internal/shared/errs"
)

type stubPlayers struct {
	got   []playercontracts.PlayerSnapshot
	calls int
}

func (s *stubPlayers) PlayersByIDs(_ context.Context, _ string, _ []string) ([]playercontracts.PlayerSnapshot, error) {
	s.calls++
	return s.got, nil
}

func (s *stubPlayers) PlayersByExternalIDs(context.Context, string, []string) ([]playercontracts.PlayerSnapshot, error) {
	return nil, nil
}

func TestLocalPlayersMapsAndFilters(t *testing.T) {
	stub := &stubPlayers{got: []playercontracts.PlayerSnapshot{
		{ID: "p1", TenantID: "t1", Active: true},
		{ID: "p2", TenantID: "t1", Active: false},
		{ID: "leak", TenantID: "t2", Active: true},
	}}
	a := NewLocalPlayers(stub)

	got, err := a.ByIDs(context.Background(), "t1", []string{"p1", "p2", "leak"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.True(t, got["p1"].Active)
	require.False(t, got["p2"].Active)
	_, leaked := got["leak"]
	require.False(t, leaked, "another tenant's player never crosses the port")

	_, err = a.ByID(context.Background(), "t1", "missing")
	require.Equal(t, errs.NotFound, errs.KindOf(err))

	empty, err := a.ByIDs(context.Background(), "t1", nil)
	require.NoError(t, err)
	require.Empty(t, empty)
	require.Equal(t, 2, stub.calls, "an empty batch makes no call")
}
