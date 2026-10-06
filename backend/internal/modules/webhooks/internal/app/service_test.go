package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	badgescontracts "levelup/internal/modules/badges/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/modules/webhooks/contracts"
	"levelup/internal/modules/webhooks/internal/domain"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/errs"
)

const (
	tenantA = "tenant-a"
	tenantB = "tenant-b"
)

var jobDeliver = contracts.Topic(contracts.JobDeliver)

type harness struct {
	svc    *Service
	repo   *fakeRepo
	ob     *fakeOutbox
	sender *fakeSender
	clock  *clock.Fake
}

func allPerms() allowKeys {
	a := allowKeys{}
	for _, p := range contracts.AllPermissions {
		a[p.Key()] = true
	}
	return a
}

func viewOnly() allowKeys { return allowKeys{contracts.PermView.Key(): true} }

func newHarness(t *testing.T, enf authz.Enforcer) *harness {
	t.Helper()
	h := &harness{
		repo:   newFakeRepo(),
		ob:     &fakeOutbox{},
		sender: &fakeSender{},
		clock:  clock.NewFake(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)),
	}
	h.svc = NewService(h.repo, h.sender, h.ob, enf, nil, h.clock, Options{
		Policy:               domain.URLPolicy{},
		Retry:                domain.RetryPolicy{LadderAttempts: 4, MaxAttempts: 6},
		StaleAfter:           10 * time.Minute,
		DisableAfterFailures: 3,
		Lease:                15 * time.Second,
	})
	h.svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
	return h
}

func ctxFor(tenant string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: "user-1", TenantID: tenant, RoleIDs: []int64{2}})
}

func (h *harness) endpoint(t *testing.T, tenant string, types ...string) domain.Endpoint {
	t.Helper()
	e, err := h.svc.CreateEndpoint(ctxFor(tenant), domain.NewEndpointInput{
		URL: "https://hooks.example.com/" + tenant, EventTypes: types, Active: true,
	})
	require.NoError(t, err)
	return e
}

func fact(topic, eventID, tenant string) Fact {
	body, _ := json.Marshal(map[string]any{"tenant_id": tenant, "player_id": "p1"})
	return Fact{Topic: topic, EventID: eventID, OccurredAt: time.Date(2026, 10, 6, 11, 0, 0, 0, time.UTC), Payload: body}
}

func lastCmd(t *testing.T, ob *fakeOutbox) contracts.DeliverCmdV1 {
	t.Helper()
	for i := len(ob.published) - 1; i >= 0; i-- {
		if ob.published[i].topic == jobDeliver {
			return ob.published[i].payload.(contracts.DeliverCmdV1)
		}
	}
	t.Fatal("no deliver command published")
	return contracts.DeliverCmdV1{}
}

// ---- endpoints ----

func TestCreateEndpointValidatesAndReturnsSecretOnce(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "badges.awarded")
	require.Contains(t, e.Secret, domain.SecretPrefix)
	require.Equal(t, tenantA, e.TenantID, "tenant stamped from the principal")

	_, err := h.svc.CreateEndpoint(ctxFor(tenantA), domain.NewEndpointInput{URL: "https://169.254.169.254/", EventTypes: []string{"*"}})
	require.ErrorIs(t, err, domain.ErrForbiddenHost)
	_, err = h.svc.CreateEndpoint(ctxFor(tenantA), domain.NewEndpointInput{URL: "https://x.example", EventTypes: []string{"nope"}})
	require.Equal(t, domain.CodeUnknownEventType, errs.CodeOf(err))

	rotated, err := h.svc.RotateSecret(ctxFor(tenantA), e.ID)
	require.NoError(t, err)
	require.NotEqual(t, e.Secret, rotated.Secret)
	stored, _ := h.repo.EndpointByID(context.Background(), tenantA, e.ID)
	require.Equal(t, rotated.Secret, stored.Secret)
}

func TestEndpointLimitPerTenant(t *testing.T) {
	h := newHarness(t, allPerms())
	for range domain.MaxEndpointsPerTenant {
		h.endpoint(t, tenantA, "*")
	}
	_, err := h.svc.CreateEndpoint(ctxFor(tenantA), domain.NewEndpointInput{URL: "https://x.example", EventTypes: []string{"*"}})
	require.ErrorIs(t, err, domain.ErrEndpointLimit)
	h.endpoint(t, tenantB, "*") // other tenants are unaffected
}

func TestAuthzMembersViewAdminsManage(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")

	h.svc.authz = viewOnly()
	_, err := h.svc.GetEndpoint(ctxFor(tenantA), e.ID)
	require.NoError(t, err)
	_, _, err = h.svc.ListDeliveries(ctxFor(tenantA), DeliveryFilter{}, "", 0)
	require.NoError(t, err)
	_, err = h.svc.EventTypes(ctxFor(tenantA))
	require.NoError(t, err)

	_, err = h.svc.CreateEndpoint(ctxFor(tenantA), domain.NewEndpointInput{URL: "https://x.example", EventTypes: []string{"*"}})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = h.svc.RotateSecret(ctxFor(tenantA), e.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = h.svc.SendTest(ctxFor(tenantA), e.ID)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(h.svc.DeleteEndpoint(ctxFor(tenantA), e.ID)))

	h.svc.authz = allowKeys{}
	_, err = h.svc.EventTypes(ctxFor(tenantA))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	h.svc.authz = allPerms()
	noTenant := authz.Into(context.Background(), authz.Principal{UserID: "platform-admin", RoleIDs: []int64{1}})
	_, _, err = h.svc.ListDeliveries(noTenant, DeliveryFilter{}, "", 0)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err), "a principal without a tenant is refused")
}

func TestCrossTenantIsNotFound(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")
	require.NoError(t, h.svc.HandleFact(context.Background(), fact(badgescontracts.TopicAwarded, "ev1", tenantA)))
	d := h.repo.deliveriesFor(e.ID)[0]

	_, err := h.svc.GetEndpoint(ctxFor(tenantB), e.ID)
	require.ErrorIs(t, err, domain.ErrEndpointNotFound)
	on := false
	_, err = h.svc.UpdateEndpoint(ctxFor(tenantB), e.ID, domain.EndpointPatch{Active: &on})
	require.ErrorIs(t, err, domain.ErrEndpointNotFound)
	require.ErrorIs(t, h.svc.DeleteEndpoint(ctxFor(tenantB), e.ID), domain.ErrEndpointNotFound)
	_, err = h.svc.SendTest(ctxFor(tenantB), e.ID)
	require.ErrorIs(t, err, domain.ErrEndpointNotFound)
	_, err = h.svc.GetDelivery(ctxFor(tenantB), d.ID)
	require.ErrorIs(t, err, domain.ErrDeliveryNotFound)
	_, err = h.svc.Redeliver(ctxFor(tenantB), d.ID)
	require.ErrorIs(t, err, domain.ErrDeliveryNotFound)
	rows, _, err := h.svc.ListDeliveries(ctxFor(tenantB), DeliveryFilter{}, "", 0)
	require.NoError(t, err)
	require.Empty(t, rows)
}

// ---- fan-out ----

func TestFanOutMatchesSubscriptionsAndIsIdempotent(t *testing.T) {
	h := newHarness(t, allPerms())
	badges := h.endpoint(t, tenantA, "badges.awarded")
	all := h.endpoint(t, tenantA, "*")
	points := h.endpoint(t, tenantA, "points.credited")
	off := h.endpoint(t, tenantA, "*")
	no := false
	_, err := h.svc.UpdateEndpoint(ctxFor(tenantA), off.ID, domain.EndpointPatch{Active: &no})
	require.NoError(t, err)
	other := h.endpoint(t, tenantB, "*")
	h.ob.reset()

	f := fact(badgescontracts.TopicAwarded, "env-1", tenantA)
	require.NoError(t, h.svc.HandleFact(context.Background(), f))
	require.Len(t, h.repo.deliveriesFor(badges.ID), 1)
	require.Len(t, h.repo.deliveriesFor(all.ID), 1)
	require.Empty(t, h.repo.deliveriesFor(points.ID), "not subscribed")
	require.Empty(t, h.repo.deliveriesFor(off.ID), "inactive")
	require.Empty(t, h.repo.deliveriesFor(other.ID), "other tenant")
	require.Equal(t, 2, h.ob.count(jobDeliver))

	d := h.repo.deliveriesFor(badges.ID)[0]
	require.Equal(t, "badges.awarded", d.Event)
	require.Equal(t, "env-1", d.EventID)
	var body domain.Envelope
	require.NoError(t, json.Unmarshal(d.Payload, &body))
	require.Equal(t, "badges.awarded", body.Event)
	require.Equal(t, "env-1", body.EventID)
	require.Equal(t, tenantA, body.TenantID)
	require.JSONEq(t, `{"tenant_id":"tenant-a","player_id":"p1"}`, string(body.Data))
	cmd := lastCmd(t, h.ob)
	require.Equal(t, tenantA, cmd.TenantID)
	require.Zero(t, cmd.BaseAttempt)

	// Redelivery of the same envelope: no new rows, no new jobs.
	require.NoError(t, h.svc.HandleFact(context.Background(), f))
	require.Len(t, h.repo.deliveriesFor(badges.ID), 1)
	require.Equal(t, 2, h.ob.count(jobDeliver))

	// A different fact fans out again.
	require.NoError(t, h.svc.HandleFact(context.Background(), fact(pointscontracts.TopicCredited, "env-2", tenantA)))
	require.Len(t, h.repo.deliveriesFor(points.ID), 1)
	require.Len(t, h.repo.deliveriesFor(all.ID), 2)
	require.Equal(t, 4, h.ob.count(jobDeliver))
}

func TestFanOutRejectsUndeliverableFacts(t *testing.T) {
	h := newHarness(t, allPerms())
	err := h.svc.HandleFact(context.Background(), fact("points.credit_rejected.v1", "e", tenantA))
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	err = h.svc.HandleFact(context.Background(), Fact{Topic: badgescontracts.TopicAwarded, EventID: "e", Payload: []byte(`{}`)})
	require.Equal(t, errs.Invalid, errs.KindOf(err), "missing tenant_id")
	err = h.svc.HandleFact(context.Background(), Fact{Topic: badgescontracts.TopicAwarded, EventID: "e", Payload: []byte(`nope`)})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	err = h.svc.HandleFact(context.Background(), fact(badgescontracts.TopicAwarded, "", tenantA))
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	// No endpoints: nothing to do.
	require.NoError(t, h.svc.HandleFact(context.Background(), fact(badgescontracts.TopicAwarded, "e", tenantA)))
	require.Empty(t, h.ob.published)
}

// ---- delivery job ----

func (h *harness) fannedOut(t *testing.T, e domain.Endpoint, eventID string) (domain.Delivery, contracts.DeliverCmdV1) {
	t.Helper()
	require.NoError(t, h.svc.HandleFact(context.Background(), fact(badgescontracts.TopicAwarded, eventID, e.TenantID)))
	for _, d := range h.repo.deliveriesFor(e.ID) {
		if d.EventID == eventID {
			return d, lastCmd(t, h.ob)
		}
	}
	t.Fatal("no delivery")
	return domain.Delivery{}, contracts.DeliverCmdV1{}
}

func (h *harness) delivery(id string) domain.Delivery {
	d, _ := h.repo.DeliveryByID(context.Background(), tenantA, id)
	return d
}

func TestDeliverSuccessSignsAndRecords(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")
	d, cmd := h.fannedOut(t, e, "ev1")
	h.sender.results = []domain.AttemptResult{{StatusCode: 202, LatencyMS: 42, Body: "ok"}}

	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	got := h.delivery(d.ID)
	require.Equal(t, domain.StatusSucceeded, got.Status)
	require.Equal(t, 1, got.Attempts)
	require.Equal(t, 202, *got.ResponseStatus)
	require.Equal(t, int64(42), *got.LatencyMS)
	require.Equal(t, "ok", got.ResponseBody)
	require.NotNil(t, got.DeliveredAt)

	req := h.sender.requests[0]
	require.Equal(t, e.URL, req.URL)
	require.Equal(t, e.Secret, req.Secret)
	require.Equal(t, d.ID, req.DeliveryID)
	require.Equal(t, "badges.awarded", req.Event)
	require.Equal(t, d.Payload, req.Body)

	// Redelivered job after success: no second POST.
	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	require.Equal(t, 1, h.sender.sent())
}

func TestDeliverRetriesThenFailsAfterLadder(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")
	d, cmd := h.fannedOut(t, e, "ev1")
	h.sender.results = []domain.AttemptResult{{StatusCode: 503, LatencyMS: 5, Body: "down"}}

	for i := 1; i <= 3; i++ {
		err := h.svc.HandleDeliver(context.Background(), cmd)
		require.Equal(t, errs.Unavailable, errs.KindOf(err), "attempt %d retries", i)
		require.Equal(t, domain.CodeDeliveryFailed, errs.CodeOf(err))
		got := h.delivery(d.ID)
		require.Equal(t, domain.StatusPending, got.Status)
		require.Equal(t, i, got.Attempts)
		require.Nil(t, got.LeaseUntil, "lease released after settle")
	}
	// Fourth run is the last rung: final failure, nil (no DLQ).
	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	got := h.delivery(d.ID)
	require.Equal(t, domain.StatusFailed, got.Status)
	require.Equal(t, 4, got.Attempts)
	require.Equal(t, "receiver responded 503", got.LastError)
	ep, _ := h.repo.EndpointByID(context.Background(), tenantA, e.ID)
	require.Equal(t, 1, ep.ConsecutiveFailures, "one failed delivery")

	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd), "settled: no-op")
	require.Equal(t, 4, h.sender.sent())
}

func TestDeliverTimeoutIsRetryable(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")
	d, cmd := h.fannedOut(t, e, "ev1")
	h.sender.results = []domain.AttemptResult{{Err: "timeout", LatencyMS: 10000}, {StatusCode: 200}}

	require.Equal(t, errs.Unavailable, errs.KindOf(h.svc.HandleDeliver(context.Background(), cmd)))
	require.Equal(t, "timeout", h.delivery(d.ID).LastError)
	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	require.Equal(t, domain.StatusSucceeded, h.delivery(d.ID).Status)
	require.Equal(t, 2, h.delivery(d.ID).Attempts)
}

func TestDeliverSkipsWhileLeased(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")
	d, cmd := h.fannedOut(t, e, "ev1")
	until := h.clock.Now().Add(10 * time.Second)
	d.LeaseUntil = &until
	require.NoError(t, h.repo.SaveDelivery(context.Background(), nil, d))

	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	require.Zero(t, h.sender.sent(), "another attempt is in flight")
}

func TestDeliverToDeletedEndpointFailsWithoutPosting(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")
	d, cmd := h.fannedOut(t, e, "ev1")
	require.NoError(t, h.svc.DeleteEndpoint(ctxFor(tenantA), e.ID))

	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	require.Zero(t, h.sender.sent())
	got := h.delivery(d.ID)
	require.Equal(t, domain.StatusFailed, got.Status)
	require.Equal(t, domain.ErrTextEndpointInactive, got.LastError)

	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.HandleDeliver(context.Background(), contracts.DeliverCmdV1{})))
	require.NoError(t, h.svc.HandleDeliver(context.Background(), contracts.DeliverCmdV1{TenantID: tenantA, DeliveryID: "gone"}), "purged row: no-op")
}

func TestConsecutiveFailuresDisableEndpointOnce(t *testing.T) {
	h := newHarness(t, allPerms()) // threshold 3
	h.svc.opts.Retry.LadderAttempts = 1
	e := h.endpoint(t, tenantA, "*")
	h.sender.results = []domain.AttemptResult{{StatusCode: 500}}

	// A success in between resets the streak.
	_, cmd := h.fannedOut(t, e, "ev0")
	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	h.sender.results = []domain.AttemptResult{{StatusCode: 200}}
	_, cmd = h.fannedOut(t, e, "ev-ok")
	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	ep, _ := h.repo.EndpointByID(context.Background(), tenantA, e.ID)
	require.Zero(t, ep.ConsecutiveFailures)

	h.sender.results = []domain.AttemptResult{{StatusCode: 500}}
	for i := 1; i <= 3; i++ {
		_, cmd := h.fannedOut(t, e, "ev"+string(rune('0'+i)))
		require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd), "ladder of 1: final at once")
	}
	ep, _ = h.repo.EndpointByID(context.Background(), tenantA, e.ID)
	require.False(t, ep.Active)
	require.Equal(t, contracts.DisabledReasonConsecutiveFailures, ep.DisabledReason)
	require.Equal(t, 1, h.ob.count(contracts.TopicEndpointDisabled))
	ev := h.ob.published[len(h.ob.published)-1].payload.(contracts.EndpointDisabledV1)
	require.Equal(t, e.ID, ev.EndpointID)
	require.Equal(t, 3, ev.ConsecutiveFailures)

	// Disabled: new facts no longer fan out to it, sweep does not re-publish.
	require.NoError(t, h.svc.HandleFact(context.Background(), fact(badgescontracts.TopicAwarded, "ev-late", tenantA)))
	require.Len(t, h.repo.deliveriesFor(e.ID), 5)
	_, err := h.svc.RetrySweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, h.ob.count(contracts.TopicEndpointDisabled))

	// Re-enabling clears the streak.
	on := true
	ep, err = h.svc.UpdateEndpoint(ctxFor(tenantA), e.ID, domain.EndpointPatch{Active: &on})
	require.NoError(t, err)
	stored, _ := h.repo.EndpointByID(context.Background(), tenantA, e.ID)
	require.True(t, stored.Active)
	require.Zero(t, stored.ConsecutiveFailures)
	require.Empty(t, stored.DisabledReason)
}

// ---- sweep ----

func TestRetrySweepRequeuesStaleAndFailsSpent(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")
	lost, _ := h.fannedOut(t, e, "lost")   // job never ran
	spent, _ := h.fannedOut(t, e, "spent") // budget used up
	fresh, _ := h.fannedOut(t, e, "fresh")
	spent.CycleAttempts, spent.Attempts = 6, 6
	require.NoError(t, h.repo.SaveDelivery(context.Background(), nil, spent))

	h.clock.Advance(11 * time.Minute)
	f := fact(badgescontracts.TopicAwarded, "fresh2", tenantA)
	require.NoError(t, h.svc.HandleFact(context.Background(), f)) // enqueued now: not stale
	h.ob.reset()

	res, err := h.svc.RetrySweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, res.Requeued, "lost + fresh (11 min old) requeued; fresh2 is recent")
	require.Equal(t, 1, res.Failed)
	require.Equal(t, 2, h.ob.count(jobDeliver))
	require.Equal(t, domain.StatusFailed, h.delivery(spent.ID).Status)
	require.Equal(t, domain.ErrTextAttemptsExceeded, h.delivery(spent.ID).LastError)
	require.Equal(t, h.clock.Now(), h.delivery(lost.ID).EnqueuedAt)
	require.Equal(t, h.clock.Now(), h.delivery(fresh.ID).EnqueuedAt)
	require.Equal(t, h.clock.Now(), h.repo.markers[contracts.JobRetrySweep])

	// Requeued rows are not stale again on the next tick.
	h.ob.reset()
	res, err = h.svc.RetrySweep(context.Background())
	require.NoError(t, err)
	require.Zero(t, res.Requeued)
	require.Zero(t, h.ob.count(jobDeliver))
}

func TestRetrySweepRequeueGetsBoundedLadder(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")
	d, _ := h.fannedOut(t, e, "ev1")
	d.CycleAttempts, d.Attempts = 4, 4 // first ladder died (e.g. worker crash) before settling
	require.NoError(t, h.repo.SaveDelivery(context.Background(), nil, d))
	h.clock.Advance(11 * time.Minute)
	h.ob.reset()

	_, err := h.svc.RetrySweep(context.Background())
	require.NoError(t, err)
	cmd := lastCmd(t, h.ob)
	require.Equal(t, 4, cmd.BaseAttempt)

	h.sender.results = []domain.AttemptResult{{StatusCode: 500}}
	require.Equal(t, errs.Unavailable, errs.KindOf(h.svc.HandleDeliver(context.Background(), cmd)))
	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd), "6th attempt hits MaxAttempts")
	require.Equal(t, domain.StatusFailed, h.delivery(d.ID).Status)
	require.Equal(t, 6, h.delivery(d.ID).Attempts)
}

func TestRetrySweepDisablesEndpointsOverThreshold(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")
	ep := h.repo.endpoints[e.ID]
	ep.ConsecutiveFailures = 3
	h.repo.endpoints[e.ID] = ep

	res, err := h.svc.RetrySweep(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, res.Disabled)
	require.Equal(t, 1, h.ob.count(contracts.TopicEndpointDisabled))
}

// ---- redeliver & test ----

func TestRedeliverStartsFreshCycle(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "*")
	d, cmd := h.fannedOut(t, e, "ev1")
	h.sender.results = []domain.AttemptResult{{StatusCode: 500}}
	h.svc.opts.Retry.LadderAttempts = 1
	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	require.Equal(t, domain.StatusFailed, h.delivery(d.ID).Status)
	h.ob.reset()

	got, err := h.svc.Redeliver(ctxFor(tenantA), d.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusPending, got.Status)
	require.Zero(t, got.CycleAttempts)
	cmd = lastCmd(t, h.ob)
	require.Equal(t, d.ID, cmd.DeliveryID)
	require.Zero(t, cmd.BaseAttempt)

	h.sender.results = []domain.AttemptResult{{StatusCode: 200}}
	require.NoError(t, h.svc.HandleDeliver(context.Background(), cmd))
	final := h.delivery(d.ID)
	require.Equal(t, domain.StatusSucceeded, final.Status)
	require.Equal(t, 2, final.Attempts)
	require.Equal(t, d.Payload, h.sender.requests[1].Body, "same bytes resent")

	// Disabled endpoint: redeliver refused.
	off := false
	_, err = h.svc.UpdateEndpoint(ctxFor(tenantA), e.ID, domain.EndpointPatch{Active: &off})
	require.NoError(t, err)
	_, err = h.svc.Redeliver(ctxFor(tenantA), d.ID)
	require.ErrorIs(t, err, domain.ErrEndpointInactive)
}

func TestSendTestRecordsSynchronousResult(t *testing.T) {
	h := newHarness(t, allPerms())
	e := h.endpoint(t, tenantA, "badges.awarded")
	h.sender.results = []domain.AttemptResult{{StatusCode: 418, LatencyMS: 7, Body: "teapot"}}

	d, err := h.svc.SendTest(ctxFor(tenantA), e.ID)
	require.NoError(t, err)
	require.Equal(t, domain.TestEvent, d.Event)
	require.Equal(t, domain.StatusFailed, d.Status, "test deliveries are not retried")
	require.Equal(t, 418, *d.ResponseStatus)
	require.Equal(t, 1, d.Attempts)
	require.Zero(t, h.ob.count(jobDeliver), "no job for a test")
	stored := h.delivery(d.ID)
	require.Equal(t, domain.StatusFailed, stored.Status)

	var body domain.Envelope
	require.NoError(t, json.Unmarshal(h.sender.requests[0].Body, &body))
	require.Equal(t, domain.TestEvent, body.Event)
	require.Equal(t, d.EventID, body.EventID)
	require.Contains(t, string(body.Data), e.ID)

	h.sender.results = []domain.AttemptResult{{StatusCode: 200}}
	d, err = h.svc.SendTest(ctxFor(tenantA), e.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusSucceeded, d.Status)
	ep, _ := h.repo.EndpointByID(context.Background(), tenantA, e.ID)
	require.Zero(t, ep.ConsecutiveFailures, "tests never count toward disabling")
}

func TestListDeliveriesFiltersAndPages(t *testing.T) {
	h := newHarness(t, allPerms())
	e1 := h.endpoint(t, tenantA, "*")
	e2 := h.endpoint(t, tenantA, "points.credited")
	for i := range 3 {
		h.clock.Advance(time.Second)
		require.NoError(t, h.svc.HandleFact(context.Background(), fact(badgescontracts.TopicAwarded, "b"+string(rune('0'+i)), tenantA)))
	}
	h.clock.Advance(time.Second)
	require.NoError(t, h.svc.HandleFact(context.Background(), fact(pointscontracts.TopicCredited, "p0", tenantA)))

	rows, next, err := h.svc.ListDeliveries(ctxFor(tenantA), DeliveryFilter{EndpointID: e1.ID}, "", 2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NotEmpty(t, next)
	require.Equal(t, "points.credited", rows[0].Event, "newest first")
	rest, next, err := h.svc.ListDeliveries(ctxFor(tenantA), DeliveryFilter{EndpointID: e1.ID}, next, 2)
	require.NoError(t, err)
	require.Len(t, rest, 2)
	require.Empty(t, next)

	rows, _, err = h.svc.ListDeliveries(ctxFor(tenantA), DeliveryFilter{Event: "points.credited"}, "", 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	rows, _, err = h.svc.ListDeliveries(ctxFor(tenantA), DeliveryFilter{EndpointID: e2.ID, Status: domain.StatusSucceeded}, "", 0)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestPurgeTenant(t *testing.T) {
	h := newHarness(t, allPerms())
	a := h.endpoint(t, tenantA, "*")
	b := h.endpoint(t, tenantB, "*")
	h.fannedOut(t, a, "ev1")
	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA))
	require.NoError(t, h.svc.PurgeTenant(context.Background(), tenantA), "idempotent")
	require.Empty(t, h.repo.deliveriesFor(a.ID))
	_, err := h.repo.EndpointByID(context.Background(), tenantA, a.ID)
	require.ErrorIs(t, err, domain.ErrEndpointNotFound)
	_, err = h.repo.EndpointByID(context.Background(), tenantB, b.ID)
	require.NoError(t, err)
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.PurgeTenant(context.Background(), "")))
}
