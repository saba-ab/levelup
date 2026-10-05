package activity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/activity/contracts"
	badgescontracts "levelup/internal/modules/badges/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	missionscontracts "levelup/internal/modules/missions/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	rulescontracts "levelup/internal/modules/rules/contracts"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/modkit"
	"levelup/internal/shared/errs"
)

func topics(subs []bus.Subscription) []string {
	out := make([]string, len(subs))
	for i, s := range subs {
		out[i] = s.Topic
	}
	return out
}

func TestSubscriptionsDependOnInternalTriggers(t *testing.T) {
	off := New(modkit.Deps{}, Config{}, nil, nil)
	require.ElementsMatch(t, []string{rulescontracts.TopicDecisionMade, identitycontracts.TopicTenantDeleted}, topics(off.Subscriptions()))
	for _, s := range off.Subscriptions() {
		require.Equal(t, contracts.Module, s.Group)
	}

	on := New(modkit.Deps{}, Config{InternalTriggers: true}, nil, nil)
	require.ElementsMatch(t, []string{
		rulescontracts.TopicDecisionMade, identitycontracts.TopicTenantDeleted,
		progressioncontracts.TopicLevelReached, badgescontracts.TopicAwarded, missionscontracts.TopicCompleted,
	}, topics(on.Subscriptions()))
}

func TestUndecodablePayloadsAreInvalid(t *testing.T) {
	m := New(modkit.Deps{}, Config{InternalTriggers: true}, nil, nil)
	for _, s := range m.Subscriptions() {
		err := s.Handler(context.Background(), bus.Envelope{EventID: "e1", Topic: s.Topic, Payload: []byte(`{"tenant_id": 42}`)})
		require.Equal(t, errs.Invalid, errs.KindOf(err), s.Topic)
	}
}

func TestStuckSweepJobDeclared(t *testing.T) {
	jobs := New(modkit.Deps{}, Config{}, nil, nil).Jobs()
	require.Len(t, jobs, 1)
	require.Equal(t, "activity.stuck_sweep", jobs[0].Name)
	require.Equal(t, "* * * * *", jobs[0].Schedule)
}

func TestConfigDefaults(t *testing.T) {
	c := Config{}.withDefaults()
	require.Equal(t, 32<<10, c.MaxPayloadBytes)
	require.Equal(t, 5, c.MaxRepublishes)
	require.Positive(t, c.MaxAge)
	require.Positive(t, c.StuckAfter)
}
