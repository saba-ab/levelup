package webhooks

import (
	"testing"

	"github.com/stretchr/testify/require"

	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/webhooks/contracts"
	"levelup/internal/platform/modkit"
)

func TestModuleDeclaresSubscriptionsJobsAndPermissions(t *testing.T) {
	m := New(modkit.Deps{}, Config{})
	require.Equal(t, "webhooks", m.Name())

	topics := map[string]bool{}
	for _, s := range m.Subscriptions() {
		require.Equal(t, contracts.Module, s.Group)
		require.NotNil(t, s.Handler)
		topics[s.Topic] = true
	}
	require.Len(t, topics, 15)
	for _, tp := range []string{
		identitycontracts.TopicTenantDeleted,
		"player.created.v1", "player.updated.v1", "points.credited.v1", "points.debited.v1",
		"badges.awarded.v1", "progression.level_reached.v1", "missions.completed.v1",
		"streaks.milestone_reached.v1", "streaks.broken.v1", "rewards.claimed.v1", "rewards.redeemed.v1",
		"rules.decision_made.v1", "leaderboards.period_closed.v1", "activity.received.v1",
	} {
		require.True(t, topics[tp], tp)
	}

	jobs := map[string]string{}
	for _, j := range m.Jobs() {
		jobs[j.Name] = j.Schedule
	}
	require.Equal(t, map[string]string{"webhooks.deliver": "", "webhooks.retry_sweep": "*/5 * * * *"}, jobs)
	require.ElementsMatch(t, contracts.AllPermissions, m.Permissions())
	require.Equal(t, "webhooks:manage", contracts.PermManage.Key())
	require.Equal(t, "webhooks:view", contracts.PermView.Key())
}
