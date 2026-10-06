package leaderboards

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/leaderboards/internal/domain"
	"levelup/internal/platform/bus"
)

func TestActivityFactKeysOnTenantEventIDAndOccurredAt(t *testing.T) {
	occurred := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	env := bus.Envelope{EventID: "env-1", Topic: activitycontracts.TopicReceived, OccurredAt: occurred.Add(time.Hour)}
	ev := activitycontracts.ReceivedV1{
		TenantID: "t1", EventID: "order-42", EventType: "purchase", PlayerID: "p1",
		Properties: map[string]any{"amount": 9.0}, OccurredAt: occurred, ReceivedAt: occurred.Add(time.Minute),
	}
	f := activityFact(env, ev)
	require.Equal(t, "activity:t1:order-42", f.EventID, "a re-published copy with a new envelope id still applies once")
	require.Equal(t, domain.FactActivity, f.Kind)
	require.Equal(t, occurred, f.At, "the period comes from occurred_at")
	require.Equal(t, "purchase", f.EventType)
	require.Equal(t, 9.0, f.Properties["amount"])

	ev.EventID, ev.OccurredAt = "", time.Time{}
	f = activityFact(env, ev)
	require.Equal(t, "env-1", f.EventID)
	require.Equal(t, occurred.Add(time.Minute), f.At, "falls back to received_at")
}
