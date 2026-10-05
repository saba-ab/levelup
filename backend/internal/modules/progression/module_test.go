package progression

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/progression/contracts"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/modkit"
	"levelup/internal/shared/errs"
)

func TestModuleDeclarations(t *testing.T) {
	m := New(modkit.Deps{}, Config{ReconcileSchedule: "0 * * * *"}, nil)
	require.Equal(t, "progression", m.Name())
	require.Equal(t, contracts.AllPermissions, m.Permissions())
	require.NotNil(t, m.Reader())
	require.Len(t, m.GoMigrations(), 2)

	subs := m.Subscriptions()
	require.Len(t, subs, 1)
	require.Equal(t, "tenant.deleted.v1", subs[0].Topic)
	require.Equal(t, "progression", subs[0].Group)

	byName := map[string]string{}
	for _, j := range m.Jobs() {
		byName[j.Name] = j.Schedule
	}
	require.Equal(t, map[string]string{"progression.grant_xp": "", "progression.reconcile": "0 * * * *"}, byName)
}

func TestMalformedPayloadsAreInvalid(t *testing.T) {
	m := New(modkit.Deps{Clock: clock.System()}, Config{}, nil)

	err := m.onTenantDeleted(context.Background(), bus.Envelope{Payload: json.RawMessage(`{`)})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	err = m.onTenantDeleted(context.Background(), bus.Envelope{Payload: json.RawMessage(`{}`)})
	require.Equal(t, errs.Invalid, errs.KindOf(err))

	for _, j := range m.Jobs() {
		if j.Name != contracts.JobGrantXP {
			continue
		}
		require.Equal(t, errs.Invalid, errs.KindOf(j.Run(context.Background(), []byte("not json"))))
		env, _ := json.Marshal(bus.Envelope{Payload: json.RawMessage(`{"tenant_id":"t1","player_id":"p1","amount":0,"idempotency_key":"k"}`)})
		require.Equal(t, errs.Invalid, errs.KindOf(j.Run(context.Background(), env)), "zero amount dead-letters")
	}
}
