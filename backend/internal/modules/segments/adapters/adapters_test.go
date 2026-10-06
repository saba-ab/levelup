package adapters

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	activitycontracts "levelup/internal/modules/activity/contracts"
	badgescontracts "levelup/internal/modules/badges/contracts"
	playercontracts "levelup/internal/modules/player/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
)

type stubPlayers struct {
	out []playercontracts.PlayerSnapshot
}

func (s stubPlayers) PlayersByIDs(context.Context, string, []string) ([]playercontracts.PlayerSnapshot, error) {
	return s.out, nil
}

func (s stubPlayers) PlayersByExternalIDs(context.Context, string, []string) ([]playercontracts.PlayerSnapshot, error) {
	return nil, nil
}

type stubLister struct{ after string }

func (s *stubLister) ListPlayerIDs(_ context.Context, _, afterID string, _ int) ([]string, error) {
	s.after = afterID
	return []string{"p9"}, nil
}

func TestLocalPlayersDropsForeignTenantsAndDelegatesListing(t *testing.T) {
	lister := &stubLister{}
	l := NewLocalPlayers(stubPlayers{out: []playercontracts.PlayerSnapshot{
		{ID: "p1", TenantID: "t1", ExternalID: "e1", Active: true, Attributes: map[string]any{"a": 1}},
		{ID: "p2", TenantID: "t2"},
	}}, lister)
	got, err := l.PlayersByIDs(context.Background(), "t1", []string{"p1", "p2"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "e1", got["p1"].ExternalID)
	require.Equal(t, map[string]any{"a": 1}, got["p1"].Attributes)

	ids, err := l.ListPlayerIDs(context.Background(), "t1", "p5", 10)
	require.NoError(t, err)
	require.Equal(t, []string{"p9"}, ids)
	require.Equal(t, "p5", lister.after)
}

type stubBadges struct{}

func (stubBadges) BadgesByIDs(context.Context, string, []string) ([]badgescontracts.BadgeSnapshot, error) {
	return nil, nil
}

func (stubBadges) PlayerBadges(context.Context, string, []string) ([]badgescontracts.PlayerBadgeSnapshot, error) {
	return []badgescontracts.PlayerBadgeSnapshot{
		{BadgeID: "b1", PlayerID: "p1", EarnedCount: 2},
		{BadgeID: "b2", PlayerID: "p1", EarnedCount: 0}, // revoked
	}, nil
}

func TestLocalBadgesKeepsOnlyHeldBadges(t *testing.T) {
	got, err := NewLocalBadges(stubBadges{}).EarnedBadges(context.Background(), "t1", []string{"p1"})
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"p1": {"b1"}}, got)
}

type stubProgress struct{}

func (stubProgress) ProgressByPlayerIDs(context.Context, string, []string) ([]progressioncontracts.ProgressSnapshot, error) {
	return []progressioncontracts.ProgressSnapshot{{PlayerID: "p1", LevelNumber: 4}}, nil
}

type stubPoints struct{}

func (stubPoints) WalletsByPlayerIDs(context.Context, string, []string) ([]pointscontracts.WalletSnapshot, error) {
	return []pointscontracts.WalletSnapshot{
		{PlayerID: "p1", TenantID: "t1", Balance: 5, LifetimeEarned: 9},
		{PlayerID: "p2", TenantID: "t2", Balance: 100},
	}, nil
}

func (stubPoints) OutcomeByKey(context.Context, string, string) (pointscontracts.MoveOutcome, bool, error) {
	return pointscontracts.MoveOutcome{}, false, nil
}

type stubActivity struct{ calls *[]int }

func (s stubActivity) LastSeen(_ context.Context, _ string, ids []string) (map[string]time.Time, error) {
	if len(ids) > activitycontracts.MaxLastSeenIDs {
		return nil, errors.New("too many ids")
	}
	*s.calls = append(*s.calls, len(ids))
	out := map[string]time.Time{}
	for _, id := range ids {
		out[id] = time.Unix(0, 0)
	}
	return out, nil
}

func TestLocalReaders(t *testing.T) {
	ctx := context.Background()
	lv, err := NewLocalProgress(stubProgress{}).LevelsByPlayerIDs(ctx, "t1", []string{"p1"})
	require.NoError(t, err)
	require.Equal(t, map[string]int{"p1": 4}, lv)

	w, err := NewLocalWallets(stubPoints{}).WalletsByPlayerIDs(ctx, "t1", []string{"p1", "p2"})
	require.NoError(t, err)
	require.Len(t, w, 1)
	require.EqualValues(t, 9, w["p1"].LifetimeEarned)

	var calls []int
	ids := make([]string, 250)
	for i := range ids {
		ids[i] = fmt.Sprintf("p%d", i)
	}
	seen, err := NewLocalActivity(stubActivity{calls: &calls}).LastSeen(ctx, "t1", ids)
	require.NoError(t, err)
	require.Len(t, seen, 250)
	require.Equal(t, []int{100, 100, 50}, calls, "chunked to the provider's cap")
	empty, err := NewLocalActivity(stubActivity{calls: &calls}).LastSeen(ctx, "t1", nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
