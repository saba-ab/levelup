package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/activity/contracts"
	"levelup/internal/modules/activity/internal/app"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/modules/activity/internal/ports"
	"levelup/internal/modules/activity/internal/testfakes"
	rulescontracts "levelup/internal/modules/rules/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

const (
	tenantA  = "0198d000-0000-7000-8000-00000000000a"
	tenantB  = "0198d000-0000-7000-8000-00000000000b"
	playerA1 = "0198d000-0000-7000-8000-0000000000a1"
)

var start = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

type harness struct {
	svc        *app.Service
	repo       *testfakes.Repo
	ob         *testfakes.Outbox
	players    *testfakes.Players
	eventTypes *testfakes.EventTypes
	clock      *clock.Fake
}

func allPerms() testfakes.AllowKeys {
	return testfakes.AllowKeys{
		contracts.PermIngest.Key():  true,
		contracts.PermView.Key():    true,
		contracts.PermViewAny.Key(): true,
	}
}

func defaultSettings() app.Settings {
	return app.Settings{
		RequireKnownEventType: true,
		Limits: domain.Limits{
			MaxAge:          30 * 24 * time.Hour,
			MaxFutureSkew:   5 * time.Minute,
			MaxPayloadBytes: 32 << 10,
		},
		StuckAfter:     5 * time.Minute,
		MaxRepublishes: 3,
		SweepBatch:     100,
	}
}

func newHarness(t *testing.T, enf authz.Enforcer, mutate ...func(*app.Settings)) *harness {
	t.Helper()
	cfg := defaultSettings()
	for _, m := range mutate {
		m(&cfg)
	}
	h := &harness{
		repo: testfakes.NewRepo(),
		ob:   &testfakes.Outbox{},
		players: &testfakes.Players{ByExt: map[string]ports.PlayerSnapshot{
			"ext-1": {ID: playerA1, ExternalID: "ext-1", Active: true},
		}},
		eventTypes: &testfakes.EventTypes{Types: map[string]ports.EventTypeSnapshot{
			"purchase_completed": {Slug: "purchase_completed", Active: true},
			"login":              {Slug: "login", Active: true},
			"retired":            {Slug: "retired", Active: false},
		}},
		clock: clock.NewFake(start),
	}
	h.svc = app.NewService(h.repo, h.players, h.eventTypes, h.ob, enf, nil, h.clock, nil, cfg, nil,
		app.WithTx(testfakes.PassThroughTx))
	return h
}

func as(tenantID string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "u1", TenantID: tenantID, RoleIDs: []int64{6}})
}

func cmd(eventID string) app.IngestCmd {
	return app.IngestCmd{
		EventID:          eventID,
		EventType:        "purchase_completed",
		PlayerExternalID: "ext-1",
		Properties:       map[string]any{"amount": 42.5},
	}
}

func received(t *testing.T, p testfakes.Published) contracts.ReceivedV1 {
	t.Helper()
	require.Equal(t, contracts.TopicReceived, p.Topic)
	ev, ok := p.Payload.(contracts.ReceivedV1)
	require.True(t, ok)
	return ev
}

// --- ingest ------------------------------------------------------------

func TestIngestStoresAndPublishesOnce(t *testing.T) {
	h := newHarness(t, allPerms())
	occurred := start.Add(-time.Hour)
	c := cmd("evt-1")
	c.OccurredAt = &occurred
	c.Context = map[string]any{"ip": "1.2.3.4"}

	res, err := h.svc.Ingest(as(tenantA), c)
	require.NoError(t, err)
	require.False(t, res.Duplicate)
	require.Equal(t, domain.StatusPending, res.Activity.Status)
	require.Equal(t, playerA1, res.Activity.PlayerID, "existing player resolved at ingest")
	require.Equal(t, 1, h.repo.Count())

	require.Len(t, h.ob.Published, 1)
	ev := received(t, h.ob.Published[0])
	require.Equal(t, res.Activity.ID, ev.ActivityID)
	require.Equal(t, tenantA, ev.TenantID, "tenant comes from the principal")
	require.Equal(t, "evt-1", ev.EventID)
	require.Equal(t, "purchase_completed", ev.EventType)
	require.Equal(t, "ext-1", ev.PlayerExternalID)
	require.Equal(t, playerA1, ev.PlayerID)
	require.Equal(t, occurred, ev.OccurredAt)
	require.Equal(t, start, ev.ReceivedAt)
	require.Equal(t, 0, ev.CausationDepth)
	require.Equal(t, 42.5, ev.Properties["amount"])
	require.False(t, ev.AutoCreatePlayer)
}

func TestIngestDuplicateReturnsStoredRowWithoutPublishing(t *testing.T) {
	h := newHarness(t, allPerms())
	first, err := h.svc.Ingest(as(tenantA), cmd("evt-1"))
	require.NoError(t, err)

	h.clock.Advance(time.Minute)
	again := cmd("evt-1")
	again.Properties = map[string]any{"amount": 1}
	second, err := h.svc.Ingest(as(tenantA), again)
	require.NoError(t, err)
	require.True(t, second.Duplicate)
	require.Equal(t, first.Activity.ID, second.Activity.ID, "the stored activity is returned")
	require.Equal(t, 42.5, second.Activity.Properties["amount"], "stored body wins")
	require.Equal(t, 1, h.repo.Count())
	require.Len(t, h.ob.Published, 1, "a duplicate publishes nothing")
}

func TestSameEventIDInAnotherTenantIsNotADuplicate(t *testing.T) {
	h := newHarness(t, allPerms())
	_, err := h.svc.Ingest(as(tenantA), cmd("evt-1"))
	require.NoError(t, err)
	res, err := h.svc.Ingest(as(tenantB), cmd("evt-1"))
	require.NoError(t, err)
	require.False(t, res.Duplicate)
	require.Equal(t, 2, h.repo.Count())
	require.Len(t, h.ob.Published, 2)
}

func TestIngestRejectsTimestampsOutsideWindow(t *testing.T) {
	cases := map[string]struct {
		at   time.Time
		code string
	}{
		"future": {start.Add(6 * time.Minute), "occurred_at_in_future"},
		"old":    {start.Add(-31 * 24 * time.Hour), "occurred_at_too_old"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, allPerms())
			c := cmd("evt-1")
			c.OccurredAt = &tc.at
			_, err := h.svc.Ingest(as(tenantA), c)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
			require.Equal(t, tc.code, errs.CodeOf(err))
			require.Zero(t, h.repo.Count())
			require.Empty(t, h.ob.Published)
		})
	}
}

func TestIngestRejectsOversizedProperties(t *testing.T) {
	h := newHarness(t, allPerms(), func(s *app.Settings) { s.Limits.MaxPayloadBytes = 16 })
	c := cmd("evt-1")
	c.Properties = map[string]any{"blob": "0123456789abcdef"}
	_, err := h.svc.Ingest(as(tenantA), c)
	require.Equal(t, "properties_too_large", errs.CodeOf(err))
}

func TestIngestUnknownEventType(t *testing.T) {
	h := newHarness(t, allPerms())
	c := cmd("evt-1")
	c.EventType = "never_defined"
	_, err := h.svc.Ingest(as(tenantA), c)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	require.Equal(t, "unknown_event_type", errs.CodeOf(err))
	require.Zero(t, h.repo.Count())
	require.Empty(t, h.ob.Published)
}

func TestIngestInactiveEventType(t *testing.T) {
	h := newHarness(t, allPerms())
	c := cmd("evt-1")
	c.EventType = "retired"
	_, err := h.svc.Ingest(as(tenantA), c)
	require.Equal(t, "inactive_event_type", errs.CodeOf(err))
}

func TestIngestUnknownEventTypeAllowedWhenNotStrict(t *testing.T) {
	h := newHarness(t, allPerms(), func(s *app.Settings) { s.RequireKnownEventType = false })
	h.eventTypes.Err = errors.New("must not be called")
	c := cmd("evt-1")
	c.EventType = "never_defined"
	_, err := h.svc.Ingest(as(tenantA), c)
	require.NoError(t, err)
	require.Zero(t, h.eventTypes.Calls, "lenient mode skips the catalogue lookup")
	require.Len(t, h.ob.Published, 1)
}

func TestIngestUnknownPlayerIsAcceptedUnresolved(t *testing.T) {
	h := newHarness(t, allPerms())
	c := cmd("evt-1")
	c.PlayerExternalID = "nobody"
	res, err := h.svc.Ingest(as(tenantA), c)
	require.NoError(t, err)
	require.Empty(t, res.Activity.PlayerID)
	ev := received(t, h.ob.Published[0])
	require.Empty(t, ev.PlayerID)
	require.False(t, ev.AutoCreatePlayer, "players are not auto-created by default; rules rejects them")
}

func TestIngestUnknownPlayerFlaggedWhenAutoCreateOn(t *testing.T) {
	h := newHarness(t, allPerms(), func(s *app.Settings) { s.AutoCreatePlayers = true })
	c := cmd("evt-1")
	c.PlayerExternalID = "nobody"
	_, err := h.svc.Ingest(as(tenantA), c)
	require.NoError(t, err)
	require.True(t, received(t, h.ob.Published[0]).AutoCreatePlayer)

	_, err = h.svc.Ingest(as(tenantA), cmd("evt-2"))
	require.NoError(t, err)
	require.False(t, received(t, h.ob.Published[1]).AutoCreatePlayer, "known players are never flagged")
}

func TestIngestPortFailureFailsWholeRequest(t *testing.T) {
	h := newHarness(t, allPerms())
	h.players.Err = errs.New(errs.Unavailable, "player down")
	_, err := h.svc.Ingest(as(tenantA), cmd("evt-1"))
	require.Equal(t, errs.Unavailable, errs.KindOf(err))
	require.Zero(t, h.repo.Count())
	require.Empty(t, h.ob.Published)
}

func TestIngestAuthz(t *testing.T) {
	t.Run("no principal", func(t *testing.T) {
		h := newHarness(t, allPerms())
		_, err := h.svc.Ingest(context.Background(), cmd("evt-1"))
		require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
	})
	t.Run("no tenant", func(t *testing.T) {
		h := newHarness(t, allPerms())
		_, err := h.svc.Ingest(as(""), cmd("evt-1"))
		require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	})
	t.Run("missing permission", func(t *testing.T) {
		h := newHarness(t, testfakes.AllowKeys{contracts.PermView.Key(): true})
		_, err := h.svc.Ingest(as(tenantA), cmd("evt-1"))
		require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
		require.Zero(t, h.repo.Count())
		require.Empty(t, h.ob.Published)
	})
}

// --- batch ---------------------------------------------------------------

func TestIngestBatchPartialFailures(t *testing.T) {
	h := newHarness(t, allPerms())
	prior, err := h.svc.Ingest(as(tenantA), cmd("seen-before"))
	require.NoError(t, err)

	future := start.Add(time.Hour)
	futureCmd := cmd("evt-future")
	futureCmd.OccurredAt = &future
	unknownType := cmd("evt-unknown")
	unknownType.EventType = "never_defined"

	items, err := h.svc.IngestBatch(as(tenantA), []app.IngestCmd{
		cmd("evt-ok"),        // 0 accepted
		futureCmd,            // 1 rejected: future
		unknownType,          // 2 rejected: unknown type
		cmd("seen-before"),   // 3 duplicate of an earlier request
		cmd("evt-ok"),        // 4 duplicate inside this batch
		{EventType: "login"}, // 5 rejected: missing ids
	})
	require.NoError(t, err)
	require.Len(t, items, 6)

	require.NoError(t, items[0].Err)
	require.False(t, items[0].Result.Duplicate)

	require.Equal(t, "occurred_at_in_future", errs.CodeOf(items[1].Err))
	require.Equal(t, "unknown_event_type", errs.CodeOf(items[2].Err))

	require.NoError(t, items[3].Err)
	require.True(t, items[3].Result.Duplicate)
	require.Equal(t, prior.Activity.ID, items[3].Result.Activity.ID)

	require.NoError(t, items[4].Err)
	require.True(t, items[4].Result.Duplicate)
	require.Equal(t, items[0].Result.Activity.ID, items[4].Result.Activity.ID)

	require.Equal(t, "invalid_event_id", errs.CodeOf(items[5].Err))

	require.Equal(t, 2, h.repo.Count())
	require.Len(t, h.ob.Published, 2, "one publish for the prior ingest, one for evt-ok")
	require.Equal(t, 2, h.eventTypes.Calls, "one batched lookup per request (prior ingest + this batch)")
}

func TestIngestBatchSizeLimits(t *testing.T) {
	h := newHarness(t, allPerms())
	_, err := h.svc.IngestBatch(as(tenantA), nil)
	require.Equal(t, "batch_empty", errs.CodeOf(err))

	big := make([]app.IngestCmd, app.MaxBatchItems+1)
	for i := range big {
		big[i] = cmd(id.NewID())
	}
	_, err = h.svc.IngestBatch(as(tenantA), big)
	require.Equal(t, "batch_too_large", errs.CodeOf(err))

	_, err = h.svc.IngestBatch(as(tenantA), big[:app.MaxBatchItems])
	require.NoError(t, err)
	require.Len(t, h.ob.Published, app.MaxBatchItems)
}

// --- reads -----------------------------------------------------------------

func TestGetIsTenantScoped(t *testing.T) {
	h := newHarness(t, allPerms())
	res, err := h.svc.Ingest(as(tenantA), cmd("evt-1"))
	require.NoError(t, err)

	got, err := h.svc.Get(as(tenantA), res.Activity.ID)
	require.NoError(t, err)
	require.Equal(t, res.Activity.ID, got.ID)

	_, err = h.svc.Get(as(tenantB), res.Activity.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err), "another tenant's activity is a 404")
	require.Equal(t, "activity_not_found", errs.CodeOf(err))

	_, err = h.svc.Get(as(tenantA), "not-a-uuid")
	require.Equal(t, errs.NotFound, errs.KindOf(err))
}

func TestGetRequiresViewPermission(t *testing.T) {
	h := newHarness(t, testfakes.AllowKeys{contracts.PermIngest.Key(): true})
	res, err := h.svc.Ingest(as(tenantA), cmd("evt-1"))
	require.NoError(t, err)
	_, err = h.svc.Get(as(tenantA), res.Activity.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = h.svc.List(as(tenantA), app.ListFilter{}, "", 0)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestListPagesAndFilters(t *testing.T) {
	h := newHarness(t, allPerms())
	for i := range 5 {
		c := cmd(id.NewID())
		if i%2 == 0 {
			c.EventType = "login"
		}
		_, err := h.svc.Ingest(as(tenantA), c)
		require.NoError(t, err)
		h.clock.Advance(time.Second)
	}
	_, err := h.svc.Ingest(as(tenantB), cmd("other-tenant"))
	require.NoError(t, err)

	page1, err := h.svc.List(as(tenantA), app.ListFilter{}, "", 2)
	require.NoError(t, err)
	require.Len(t, page1.Items, 2)
	require.NotEmpty(t, page1.NextCursor)
	require.True(t, page1.Items[0].CreatedAt.After(page1.Items[1].CreatedAt), "newest first")

	page2, err := h.svc.List(as(tenantA), app.ListFilter{}, page1.NextCursor, 2)
	require.NoError(t, err)
	require.Len(t, page2.Items, 2)
	page3, err := h.svc.List(as(tenantA), app.ListFilter{}, page2.NextCursor, 2)
	require.NoError(t, err)
	require.Len(t, page3.Items, 1)
	require.Empty(t, page3.NextCursor, "last page has no cursor")

	logins, err := h.svc.List(as(tenantA), app.ListFilter{EventType: "login"}, "", 0)
	require.NoError(t, err)
	require.Len(t, logins.Items, 3)

	pending, err := h.svc.List(as(tenantA), app.ListFilter{Status: domain.StatusPending, PlayerExternalID: "ext-1"}, "", 0)
	require.NoError(t, err)
	require.Len(t, pending.Items, 5)

	_, err = h.svc.List(as(tenantA), app.ListFilter{Status: "done"}, "", 0)
	require.Equal(t, "invalid_status", errs.CodeOf(err))

	_, err = h.svc.List(as(tenantA), app.ListFilter{}, "garbage!", 0)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

// --- decision projection -----------------------------------------------------

func decision(a domain.Activity, outcome string) rulescontracts.DecisionMadeV1 {
	return rulescontracts.DecisionMadeV1{
		DecisionID: id.Derive("decision", a.ID),
		ActivityID: a.ID,
		TenantID:   a.TenantID,
		PlayerID:   playerA1,
		EventType:  a.EventType,
		Outcome:    outcome,
		At:         start.Add(time.Second),
	}
}

func TestApplyDecisionProjectsAndIsIdempotent(t *testing.T) {
	h := newHarness(t, allPerms())
	c := cmd("evt-1")
	c.PlayerExternalID = "late-player"
	res, err := h.svc.Ingest(as(tenantA), c)
	require.NoError(t, err)
	published := len(h.ob.Published)

	ev := decision(res.Activity, rulescontracts.OutcomeMatched)
	require.NoError(t, h.svc.ApplyDecision(context.Background(), ev))
	got, _ := h.repo.Get(res.Activity.ID)
	require.Equal(t, domain.StatusDecided, got.Status)
	require.Equal(t, ev.DecisionID, got.DecisionID)
	require.Equal(t, rulescontracts.OutcomeMatched, got.Outcome)
	require.Equal(t, playerA1, got.PlayerID, "rules' resolution back-fills an unresolved player")
	require.Equal(t, ev.At, got.DecidedAt)

	// Redelivery of the same decision changes nothing.
	h.clock.Advance(time.Minute)
	require.NoError(t, h.svc.ApplyDecision(context.Background(), ev))
	again, _ := h.repo.Get(res.Activity.ID)
	require.Equal(t, got, again)

	// A different, late decision for a settled activity is ignored too.
	other := ev
	other.DecisionID = id.NewID()
	other.Outcome = rulescontracts.OutcomeRejected
	require.NoError(t, h.svc.ApplyDecision(context.Background(), other))
	still, _ := h.repo.Get(res.Activity.ID)
	require.Equal(t, got, still)

	require.Len(t, h.ob.Published, published, "the projection publishes nothing")
}

func TestApplyDecisionRejectedOutcome(t *testing.T) {
	h := newHarness(t, allPerms())
	res, err := h.svc.Ingest(as(tenantA), cmd("evt-1"))
	require.NoError(t, err)

	ev := decision(res.Activity, rulescontracts.OutcomeRejected)
	ev.Reason = "player_not_found"
	require.NoError(t, h.svc.ApplyDecision(context.Background(), ev))
	got, _ := h.repo.Get(res.Activity.ID)
	require.Equal(t, domain.StatusRejected, got.Status)
	require.Equal(t, "player_not_found", got.Reason)
}

func TestApplyDecisionOutcomesOtherThanRejectedAreDecided(t *testing.T) {
	for _, outcome := range []string{rulescontracts.OutcomeNoMatch, rulescontracts.OutcomeLimitReached} {
		h := newHarness(t, allPerms())
		res, err := h.svc.Ingest(as(tenantA), cmd("evt-1"))
		require.NoError(t, err)
		require.NoError(t, h.svc.ApplyDecision(context.Background(), decision(res.Activity, outcome)))
		got, _ := h.repo.Get(res.Activity.ID)
		require.Equal(t, domain.StatusDecided, got.Status, outcome)
	}
}

func TestApplyDecisionCrossTenantPayloadDoesNotTouchRow(t *testing.T) {
	h := newHarness(t, allPerms())
	res, err := h.svc.Ingest(as(tenantA), cmd("evt-1"))
	require.NoError(t, err)
	ev := decision(res.Activity, rulescontracts.OutcomeMatched)
	ev.TenantID = tenantB
	require.NoError(t, h.svc.ApplyDecision(context.Background(), ev))
	got, _ := h.repo.Get(res.Activity.ID)
	require.Equal(t, domain.StatusPending, got.Status)
}

func TestApplyDecisionMalformedIsInvalid(t *testing.T) {
	h := newHarness(t, allPerms())
	err := h.svc.ApplyDecision(context.Background(), rulescontracts.DecisionMadeV1{ActivityID: "x", TenantID: tenantA, DecisionID: id.NewID()})
	require.Equal(t, errs.Invalid, errs.KindOf(err), "malformed payloads go straight to the DLQ")
}

func TestApplyDecisionForUnknownActivityIsNoop(t *testing.T) {
	h := newHarness(t, allPerms())
	err := h.svc.ApplyDecision(context.Background(), rulescontracts.DecisionMadeV1{
		ActivityID: id.NewID(), TenantID: tenantA, DecisionID: id.NewID(), Outcome: rulescontracts.OutcomeMatched,
	})
	require.NoError(t, err)
}

// --- internal triggers -----------------------------------------------------

func trigger(sourceEventID string) app.InternalTrigger {
	return app.InternalTrigger{
		TenantID:      tenantA,
		PlayerID:      playerA1,
		EventType:     contracts.EventTypeLevelUp,
		SourceTopic:   "progression.level_reached.v1",
		SourceEventID: sourceEventID,
		Properties:    map[string]any{"level_number": 3},
		OccurredAt:    start,
	}
}

func TestRecordInternalIsIdempotentOnSourceEvent(t *testing.T) {
	h := newHarness(t, allPerms())
	src := id.NewID()
	require.NoError(t, h.svc.RecordInternal(context.Background(), trigger(src)))
	require.NoError(t, h.svc.RecordInternal(context.Background(), trigger(src)))

	require.Equal(t, 1, h.repo.Count())
	require.Len(t, h.ob.Published, 1)
	ev := received(t, h.ob.Published[0])
	require.Equal(t, "sys:"+src, ev.EventID)
	require.Equal(t, contracts.EventTypeLevelUp, ev.EventType)
	require.Equal(t, 1, ev.CausationDepth)
	require.Equal(t, src, ev.SourceEventID)
	require.Equal(t, playerA1, ev.PlayerID)
	require.Equal(t, "ext-1", ev.PlayerExternalID, "external id resolved through the port")
}

func TestRecordInternalSkipsEventTypeStrictness(t *testing.T) {
	h := newHarness(t, allPerms())
	h.eventTypes.Err = errors.New("must not be called")
	require.NoError(t, h.svc.RecordInternal(context.Background(), trigger(id.NewID())))
}

func TestRecordInternalDepthGrowsAlongChain(t *testing.T) {
	h := newHarness(t, allPerms())
	require.NoError(t, h.svc.RecordInternal(context.Background(), trigger(id.NewID())))
	parent := received(t, h.ob.Published[0])

	child := trigger(id.NewID())
	child.EventType = contracts.EventTypeBadgeEarned
	child.ParentActivityID = parent.ActivityID
	require.NoError(t, h.svc.RecordInternal(context.Background(), child))
	require.Equal(t, 2, received(t, h.ob.Published[1]).CausationDepth)

	orphan := trigger(id.NewID())
	orphan.ParentActivityID = id.NewID()
	require.NoError(t, h.svc.RecordInternal(context.Background(), orphan))
	require.Equal(t, 1, received(t, h.ob.Published[2]).CausationDepth, "unknown parent counts as depth 0")
}

func TestRecordInternalMalformed(t *testing.T) {
	h := newHarness(t, allPerms())
	tr := trigger("")
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.RecordInternal(context.Background(), tr)))
	tr = trigger(id.NewID())
	tr.PlayerID = ""
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.RecordInternal(context.Background(), tr)))
}

// --- tenant purge ------------------------------------------------------------

func TestPurgeTenantIsIdempotentAndScoped(t *testing.T) {
	h := newHarness(t, allPerms())
	for _, e := range []string{"a1", "a2"} {
		_, err := h.svc.Ingest(as(tenantA), cmd(e))
		require.NoError(t, err)
	}
	_, err := h.svc.Ingest(as(tenantB), cmd("b1"))
	require.NoError(t, err)

	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))
	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))
	require.Equal(t, 1, h.repo.Count(), "only tenant B's activity survives")

	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.PurgeTenant(context.Background(), "")))
}

// --- stuck sweep -------------------------------------------------------------

func TestSweepStuckRepublishesWithCap(t *testing.T) {
	h := newHarness(t, allPerms())
	stuck, err := h.svc.Ingest(as(tenantA), cmd("stuck"))
	require.NoError(t, err)
	decided, err := h.svc.Ingest(as(tenantA), cmd("decided"))
	require.NoError(t, err)
	require.NoError(t, h.svc.ApplyDecision(context.Background(), decision(decided.Activity, rulescontracts.OutcomeMatched)))
	h.ob.Published = nil

	// Too young: nothing to do.
	h.clock.Advance(4 * time.Minute)
	require.NoError(t, h.svc.SweepStuck(context.Background()))
	require.Empty(t, h.ob.Published)

	// Pending past StuckAfter: re-published once, decided row untouched.
	h.clock.Advance(2 * time.Minute)
	require.NoError(t, h.svc.SweepStuck(context.Background()))
	require.Len(t, h.ob.Published, 1)
	ev := received(t, h.ob.Published[0])
	require.Equal(t, stuck.Activity.ID, ev.ActivityID, "the same activity id: rules dedupes on it")
	require.Equal(t, "stuck", ev.EventID)

	// The next tick right after does not re-publish again.
	h.clock.Advance(time.Minute)
	require.NoError(t, h.svc.SweepStuck(context.Background()))
	require.Len(t, h.ob.Published, 1)

	// Each StuckAfter window re-publishes again until the cap (3).
	for range 5 {
		h.clock.Advance(6 * time.Minute)
		require.NoError(t, h.svc.SweepStuck(context.Background()))
	}
	require.Len(t, h.ob.Published, 3, "capped at MaxRepublishes")
	got, _ := h.repo.Get(stuck.Activity.ID)
	require.Equal(t, 3, got.RepublishCount)
	require.Equal(t, domain.StatusPending, got.Status)

	require.Equal(t, h.clock.Now(), h.repo.Markers[app.StuckSweepJob], "successful run recorded")
}
