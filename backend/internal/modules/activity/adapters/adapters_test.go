package adapters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	eventcatalogcontracts "levelup/internal/modules/eventcatalog/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
)

type stubPlayers struct {
	rows []playercontracts.PlayerSnapshot
}

func (s stubPlayers) PlayersByIDs(_ context.Context, _ string, ids []string) ([]playercontracts.PlayerSnapshot, error) {
	return s.filter(func(p playercontracts.PlayerSnapshot) string { return p.ID }, ids), nil
}

func (s stubPlayers) PlayersByExternalIDs(_ context.Context, _ string, ids []string) ([]playercontracts.PlayerSnapshot, error) {
	return s.filter(func(p playercontracts.PlayerSnapshot) string { return p.ExternalID }, ids), nil
}

func (s stubPlayers) filter(key func(playercontracts.PlayerSnapshot) string, ids []string) []playercontracts.PlayerSnapshot {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []playercontracts.PlayerSnapshot
	for _, p := range s.rows {
		if want[key(p)] {
			out = append(out, p)
		}
	}
	return out
}

func TestLocalPlayersKeysSnapshots(t *testing.T) {
	l := NewLocalPlayers(stubPlayers{rows: []playercontracts.PlayerSnapshot{
		{ID: "p1", ExternalID: "e1", Active: true},
		{ID: "p2", ExternalID: "e2"},
	}})
	ctx := context.Background()

	byExt, err := l.ByExternalIDs(ctx, "t", []string{"e1", "missing"})
	require.NoError(t, err)
	require.Len(t, byExt, 1)
	require.Equal(t, "p1", byExt["e1"].ID)
	require.True(t, byExt["e1"].Active)

	byID, err := l.ByIDs(ctx, "t", []string{"p2"})
	require.NoError(t, err)
	require.Equal(t, "e2", byID["p2"].ExternalID)

	empty, err := l.ByIDs(ctx, "t", nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

type stubTypes []eventcatalogcontracts.EventTypeSnapshot

func (s stubTypes) EventTypesBySlugs(context.Context, string, []string) ([]eventcatalogcontracts.EventTypeSnapshot, error) {
	return s, nil
}

func TestLocalEventTypesPrefersTenantRow(t *testing.T) {
	for _, order := range [][]eventcatalogcontracts.EventTypeSnapshot{
		{{Slug: "login", TenantID: "t", Active: false}, {Slug: "login", Active: true}},
		{{Slug: "login", Active: true}, {Slug: "login", TenantID: "t", Active: false}},
	} {
		got, err := NewLocalEventTypes(stubTypes(order)).BySlugs(context.Background(), "t", []string{"login"})
		require.NoError(t, err)
		require.False(t, got["login"].Active, "the tenant's own type shadows the global one")
	}
}
