package player

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	activitycontracts "levelup/internal/modules/activity/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/platform/bus"
	"levelup/internal/shared/errs"
)

type recordingPurger struct{ tenants []string }

func (r *recordingPurger) PurgeTenant(_ context.Context, tenantID string) (int, error) {
	r.tenants = append(r.tenants, tenantID)
	return 0, nil
}

func TestTenantDeletedHandlerDecodesAndPurges(t *testing.T) {
	p := &recordingPurger{}
	body, err := json.Marshal(identitycontracts.TenantDeletedV1{TenantID: "t-1"})
	require.NoError(t, err)

	require.NoError(t, handleTenantDeleted(context.Background(), p, bus.Envelope{EventID: "e1", Payload: body}))
	require.NoError(t, handleTenantDeleted(context.Background(), p, bus.Envelope{EventID: "e1", Payload: body}))
	require.Equal(t, []string{"t-1", "t-1"}, p.tenants, "redelivery is handed to the idempotent purge")
}

func TestTenantDeletedHandlerDeadLettersGarbage(t *testing.T) {
	err := handleTenantDeleted(context.Background(), &recordingPurger{}, bus.Envelope{Payload: []byte("{not json")})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestSubscribesToTenantDeletedAsPlayerGroup(t *testing.T) {
	m := &Module{}
	subs := m.Subscriptions()
	require.Len(t, subs, 1)
	require.Equal(t, identitycontracts.TopicTenantDeleted, subs[0].Topic)
	require.Equal(t, "player", subs[0].Group)
}

type recordingCreator struct {
	events []activitycontracts.ReceivedV1
}

func (r *recordingCreator) AutoCreateFromActivity(_ context.Context, ev activitycontracts.ReceivedV1) (bool, error) {
	r.events = append(r.events, ev)
	return true, nil
}

func TestActivityReceivedHandlerDecodesAndDelegates(t *testing.T) {
	c := &recordingCreator{}
	body, err := json.Marshal(activitycontracts.ReceivedV1{TenantID: "t-1", PlayerExternalID: "ext", AutoCreatePlayer: true})
	require.NoError(t, err)
	require.NoError(t, handleActivityReceived(context.Background(), c, bus.Envelope{EventID: "e1", Payload: body}))
	require.Len(t, c.events, 1)
	require.True(t, c.events[0].AutoCreatePlayer)
	require.Equal(t, "ext", c.events[0].PlayerExternalID)

	err = handleActivityReceived(context.Background(), c, bus.Envelope{Payload: []byte("{nope")})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestActivitySubscriptionIsConfigGated(t *testing.T) {
	m := &Module{cfg: Config{AutoCreateFromActivities: true}}
	subs := m.Subscriptions()
	require.Len(t, subs, 2)
	require.Equal(t, activitycontracts.TopicReceived, subs[1].Topic)
	require.Equal(t, "player", subs[1].Group)
}
