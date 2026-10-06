package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	activitycontracts "levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/player/contracts"
	"levelup/internal/modules/player/internal/app"
	"levelup/internal/modules/player/internal/app/apptest"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

func flagged(ext string) activitycontracts.ReceivedV1 {
	return activitycontracts.ReceivedV1{
		ActivityID:       id.NewID(),
		TenantID:         tenantA,
		EventID:          "evt-" + ext,
		EventType:        "login",
		PlayerExternalID: ext,
		AutoCreatePlayer: true,
	}
}

func TestAutoCreateCreatesSystemPlayerAndPublishesInTx(t *testing.T) {
	f := newFixture(t, apptest.AllowKeys{}) // system path: no principal, no permission
	ev := flagged(" new-1 ")

	created, err := f.svc.AutoCreateFromActivity(context.Background(), ev)
	require.NoError(t, err)
	require.True(t, created)
	require.Len(t, f.repo.Rows, 1)

	var pl = f.repo.Rows[id.Derive("player.autocreate", tenantA, ev.ActivityID)]
	require.Equal(t, tenantA, pl.TenantID)
	require.Equal(t, "new-1", pl.ExternalID)
	require.Equal(t, "new-1", pl.DisplayName, "display name defaults to the external id")
	require.Empty(t, pl.Email)
	require.Empty(t, pl.CreatedBy, "created_by is null: the system created it")
	require.True(t, pl.Active)

	require.Len(t, f.ob.Published, 1)
	got := f.ob.Published[0]
	require.Equal(t, contracts.TopicPlayerCreated, got.Topic)
	require.True(t, got.InTx)
	require.Equal(t, contracts.PlayerCreatedV1{
		PlayerID: pl.ID, TenantID: tenantA, ExternalID: "new-1", DisplayName: "new-1", At: pl.CreatedAt,
		AutoCreated: true, SourceActivityID: ev.ActivityID,
	}, got.Payload)

	require.Len(t, f.repo.Evictions, 1, "evicts the tombstone activity's lookup may have cached")
	require.False(t, f.repo.Evictions[0].InTx)
	require.Equal(t, []app.Ref{{ID: pl.ID, ExternalID: "new-1"}}, f.repo.Evictions[0].Refs)
}

func TestAutoCreateRedeliveryCreatesNothing(t *testing.T) {
	f := newFixture(t, allPerms)
	ev := flagged("again")
	_, err := f.svc.AutoCreateFromActivity(context.Background(), ev)
	require.NoError(t, err)

	created, err := f.svc.AutoCreateFromActivity(context.Background(), ev)
	require.NoError(t, err)
	require.False(t, created)
	require.Len(t, f.repo.Rows, 1)
	require.Len(t, f.ob.Published, 1, "one row, one publish")
}

func TestAutoCreateRedeliveryAfterDeleteDoesNotResurrect(t *testing.T) {
	f := newFixture(t, allPerms)
	ev := flagged("gone")
	_, err := f.svc.AutoCreateFromActivity(context.Background(), ev)
	require.NoError(t, err)
	pl := f.repo.Rows[id.Derive("player.autocreate", tenantA, ev.ActivityID)]
	require.NoError(t, f.svc.Delete(asTenant(tenantA), pl.ID))

	created, err := f.svc.AutoCreateFromActivity(context.Background(), ev)
	require.NoError(t, err)
	require.False(t, created, "the derived id collides with the soft-deleted row")

	// A NEW activity for that external id does create a fresh player.
	created, err = f.svc.AutoCreateFromActivity(context.Background(), flagged("gone"))
	require.NoError(t, err)
	require.True(t, created)
}

func TestAutoCreateSkipsExistingAndUnflagged(t *testing.T) {
	f := newFixture(t, allPerms)
	existing := f.create(t, asTenant(tenantA), "known")
	published := len(f.ob.Published)

	created, err := f.svc.AutoCreateFromActivity(context.Background(), flagged("known"))
	require.NoError(t, err)
	require.False(t, created)

	ev := flagged("unflagged")
	ev.AutoCreatePlayer = false
	created, err = f.svc.AutoCreateFromActivity(context.Background(), ev)
	require.NoError(t, err)
	require.False(t, created)

	require.Len(t, f.repo.Rows, 1)
	require.Equal(t, existing.ID, f.repo.Rows[existing.ID].ID)
	require.Len(t, f.ob.Published, published)
}

func TestAutoCreateTwoActivitiesForOneUnknownPlayerCreateOne(t *testing.T) {
	f := newFixture(t, allPerms)
	a, b := flagged("racer"), flagged("racer")
	_, err := f.svc.AutoCreateFromActivity(context.Background(), a)
	require.NoError(t, err)
	created, err := f.svc.AutoCreateFromActivity(context.Background(), b)
	require.NoError(t, err)
	require.False(t, created, "live (tenant, external_id) is unique")
	require.Len(t, f.repo.Rows, 1)
	require.Len(t, f.ob.Published, 1)
}

func TestAutoCreateSameExternalIDPerTenant(t *testing.T) {
	f := newFixture(t, allPerms)
	f.create(t, asTenant(tenantB), "shared")
	created, err := f.svc.AutoCreateFromActivity(context.Background(), flagged("shared"))
	require.NoError(t, err)
	require.True(t, created, "another tenant's player does not count")
}

func TestAutoCreateDisplayHints(t *testing.T) {
	cases := []struct {
		name      string
		props     map[string]any
		ctx       map[string]any
		wantName  string
		wantEmail string
	}{
		{name: "properties win", props: map[string]any{"player_display_name": "Neo", "player_email": "Neo@X.io"},
			ctx: map[string]any{"player_display_name": "Ctx"}, wantName: "Neo", wantEmail: "neo@x.io"},
		{name: "context fallback", ctx: map[string]any{"player_display_name": " Trinity ", "player_email": "t@x.io"},
			wantName: "Trinity", wantEmail: "t@x.io"},
		{name: "invalid email dropped", props: map[string]any{"player_display_name": "M", "player_email": "nope"},
			wantName: "M"},
		{name: "too long name falls back", props: map[string]any{"player_display_name": strings.Repeat("n", 300)},
			wantName: "hinted"},
		{name: "non-string hints ignored", props: map[string]any{"player_display_name": 42, "player_email": true},
			wantName: "hinted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, allPerms)
			ev := flagged("hinted")
			ev.Properties, ev.Context = tc.props, tc.ctx
			created, err := f.svc.AutoCreateFromActivity(context.Background(), ev)
			require.NoError(t, err)
			require.True(t, created)
			pl := f.repo.Rows[id.Derive("player.autocreate", tenantA, ev.ActivityID)]
			require.Equal(t, tc.wantName, pl.DisplayName)
			require.Equal(t, tc.wantEmail, pl.Email)
		})
	}
}

func TestAutoCreateMalformedIsInvalid(t *testing.T) {
	for name, mutate := range map[string]func(*activitycontracts.ReceivedV1){
		"no tenant":       func(e *activitycontracts.ReceivedV1) { e.TenantID = "" },
		"bad activity id": func(e *activitycontracts.ReceivedV1) { e.ActivityID = "x" },
		"blank external":  func(e *activitycontracts.ReceivedV1) { e.PlayerExternalID = "  " },
		"external too long": func(e *activitycontracts.ReceivedV1) {
			e.PlayerExternalID = strings.Repeat("x", 256)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, allPerms)
			ev := flagged("x")
			mutate(&ev)
			_, err := f.svc.AutoCreateFromActivity(context.Background(), ev)
			require.Equal(t, errs.Invalid, errs.KindOf(err), "parks in the DLQ at once")
			require.Empty(t, f.repo.Rows)
			require.Empty(t, f.ob.Published)
		})
	}
}

func TestAutoCreatePublishFailureFailsTheHandler(t *testing.T) {
	f := newFixture(t, allPerms)
	f.ob.Fail = errs.New(errs.Unavailable, "outbox down")
	_, err := f.svc.AutoCreateFromActivity(context.Background(), flagged("retry-me"))
	require.Equal(t, errs.Unavailable, errs.KindOf(err), "transient: the retry ladder redelivers")
}
