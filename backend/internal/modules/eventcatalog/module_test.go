package eventcatalog

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/eventcatalog/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/modkit"
	"levelup/internal/shared/errs"
)

func TestSubscribesToTenantDeletedWithOwnGroup(t *testing.T) {
	m := New(modkit.Deps{}, Config{})
	subs := m.Subscriptions()
	require.Len(t, subs, 1)
	require.Equal(t, identitycontracts.TopicTenantDeleted, subs[0].Topic)
	require.Equal(t, "eventcatalog", subs[0].Group)
	require.Equal(t, "eventcatalog", m.Name())
	require.Equal(t, contracts.AllPermissions, m.Permissions())
	require.NotNil(t, m.Reader())
}

// Malformed tenant.deleted.v1 payloads can never succeed: errs.Invalid makes
// the consumer loop dead-letter them instead of climbing the retry ladder.
func TestTenantDeletedMalformedPayloadIsInvalid(t *testing.T) {
	m := New(modkit.Deps{}, Config{})
	handler := m.Subscriptions()[0].Handler

	err := handler(context.Background(), bus.Envelope{EventID: "e1", Payload: json.RawMessage(`{not json`)})
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	err = handler(context.Background(), bus.Envelope{EventID: "e2", Payload: json.RawMessage(`{"tenant_id":""}`)})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}
