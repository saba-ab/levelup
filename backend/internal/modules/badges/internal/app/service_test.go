package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/badges/internal/domain"
	"levelup/internal/modules/badges/internal/ports"
	identitycontracts "levelup/internal/modules/identity/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

const (
	tenantA = "0199a000-0000-7000-8000-00000000000a"
	tenantB = "0199a000-0000-7000-8000-00000000000b"
	player1 = "0199a000-0000-7000-8000-000000000001"
	player2 = "0199a000-0000-7000-8000-000000000002"
	ghost   = "0199a000-0000-7000-8000-0000000000ff"
	adminID = "0199a000-0000-7000-8000-0000000000ad"
)

var t0 = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

var allPerms = func() allowKeys {
	a := allowKeys{}
	for _, p := range contracts.AllPermissions {
		a[p.Key()] = true
	}
	return a
}()

type harness struct {
	svc     *Service
	repo    *fakeRepo
	ob      *fakeOutbox
	players *fakePlayers
	clk     *clock.Fake
	txs     int
	// publishTx maps every published record index to the tx it rode.
	publishTx []*gorm.DB
}

// txOutbox records the tx handle each publish rode, proving "same tx".
type txOutbox struct {
	h *harness
}

func (o txOutbox) Publish(ctx context.Context, tx *gorm.DB, topic string, payload any) error {
	o.h.publishTx = append(o.h.publishTx, tx)
	return o.h.ob.Publish(ctx, tx, topic, payload)
}

func newHarness(t *testing.T, enf authz.Enforcer) *harness {
	t.Helper()
	h := &harness{
		repo: newFakeRepo(),
		ob:   &fakeOutbox{},
		players: &fakePlayers{players: map[string]ports.PlayerSnapshot{
			player1: {ID: player1, TenantID: tenantA, Active: true},
			player2: {ID: player2, TenantID: tenantA, Active: false},
		}},
		clk: clock.NewFake(t0),
	}
	h.svc = NewService(h.repo, h.players, txOutbox{h}, enf, nil, h.clk, nil, nil)
	// Pass-through runner handing each transaction a distinct handle.
	h.svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error {
		h.txs++
		return fn(&gorm.DB{})
	}
	return h
}

func ctxAs(tenantID string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: adminID, TenantID: tenantID, RoleIDs: []int64{2}})
}

func intp(v int) *int { return &v }

func (h *harness) seedBadge(t *testing.T, mut func(*domain.NewBadgeParams)) domain.Badge {
	t.Helper()
	p := domain.NewBadgeParams{TenantID: tenantA, Name: "Badge " + id.NewID(), Tier: "bronze", Category: "achievement", Active: true}
	if mut != nil {
		mut(&p)
	}
	b, err := domain.NewBadge(p, h.clk.Now())
	require.NoError(t, err)
	require.NoError(t, h.repo.CreateBadge(context.Background(), nil, b))
	h.clk.Advance(time.Second)
	return b
}

func jobBody(t *testing.T, cmd contracts.AwardCmdV1) []byte {
	t.Helper()
	payload, err := json.Marshal(cmd)
	require.NoError(t, err)
	body, err := json.Marshal(bus.Envelope{EventID: id.NewID(), Topic: contracts.Topic(contracts.JobAward), OccurredAt: t0, Payload: payload})
	require.NoError(t, err)
	return body
}

func awardCmd(badgeID, playerID, key string) contracts.AwardCmdV1 {
	return contracts.AwardCmdV1{
		IdempotencyKey: key,
		TenantID:       tenantA,
		PlayerID:       playerID,
		BadgeID:        badgeID,
		Source:         effect.Source{Kind: effect.SourceRule, ID: "effect-" + key, ActivityID: "act-1"},
		OccurredAt:     t0,
	}
}

func rejectedReasons(ob *fakeOutbox) []string {
	var out []string
	for _, p := range ob.byTopic(contracts.TopicAwardRejected) {
		out = append(out, p.(contracts.AwardRejectedV1).Reason)
	}
	return out
}

// ---- job: badges.award ----

func TestAwardJobAppliesAndIssuesPointsCreditInSameTx(t *testing.T) {
	h := newHarness(t, allowKeys{})
	b := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Tier = "gold" }) // 50 points

	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "k1"))))

	require.Equal(t, []string{contracts.TopicAwarded, pointscontracts.Topic(pointscontracts.JobCredit)}, h.ob.topics())
	require.Len(t, h.publishTx, 2)
	require.Same(t, h.publishTx[0], h.publishTx[1], "the credit command rides the award's transaction")

	awarded := h.ob.byTopic(contracts.TopicAwarded)[0].(contracts.AwardedV1)
	require.Equal(t, tenantA, awarded.TenantID)
	require.Equal(t, 1, awarded.EarnedCount)
	require.True(t, awarded.IsFirstEarn)
	require.EqualValues(t, 50, awarded.PointsValue)
	require.Equal(t, b.Slug, awarded.BadgeSlug)
	require.Equal(t, "k1", awarded.IdempotencyKey)

	credit := h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobCredit))[0].(pointscontracts.CreditCmdV1)
	require.Equal(t, id.Derive("badge_award", awarded.AwardID, "points"), credit.IdempotencyKey, "deterministic key")
	require.Equal(t, PointsCreditKey(awarded.AwardID), credit.IdempotencyKey)
	require.Equal(t, pointscontracts.KindBonus, credit.Kind)
	require.EqualValues(t, 50, credit.Amount)
	require.Equal(t, tenantA, credit.TenantID)
	require.Equal(t, player1, credit.PlayerID)
	require.Equal(t, effect.Source{Kind: effect.SourceBadge, ID: awarded.AwardID, ActivityID: "act-1"}, credit.Source)

	pb, found, _ := h.repo.PlayerBadge(context.Background(), tenantA, player1, b.ID)
	require.True(t, found)
	require.Equal(t, 1, pb.EarnedCount)
	applied := h.repo.appliedAwards()
	require.Len(t, applied, 1)
	require.Equal(t, pb.ID, applied[0].PlayerBadgeID)
}

func TestAwardJobRedeliveryIsNoOp(t *testing.T) {
	h := newHarness(t, allowKeys{})
	b := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Stackable = true })
	body := jobBody(t, awardCmd(b.ID, player1, "same-key"))

	require.NoError(t, h.svc.HandleAwardJob(context.Background(), body))
	callsAfterFirst := h.players.calls
	require.NoError(t, h.svc.HandleAwardJob(context.Background(), body))
	// A NEW envelope (different event id) with the same command key too.
	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "same-key"))))

	require.Len(t, h.repo.appliedAwards(), 1, "one ledger row")
	require.Len(t, h.ob.byTopic(contracts.TopicAwarded), 1, "one publish")
	require.Len(t, h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobCredit)), 1, "credited once")
	pb, _, _ := h.repo.PlayerBadge(context.Background(), tenantA, player1, b.ID)
	require.Equal(t, 1, pb.EarnedCount, "a stackable badge must not stack on redelivery")
	require.Equal(t, callsAfterFirst, h.players.calls, "redelivery short-circuits before the player lookup")
}

func TestAwardJobAlreadyEarnedIsRejectionNotError(t *testing.T) {
	h := newHarness(t, allowKeys{})
	b := h.seedBadge(t, nil) // non-stackable, 10 points

	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "run-1"))))
	// Later rule runs keep deciding the same badge: they must be acked,
	// not fail forever (Laravel raised and rolled the whole batch back).
	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "run-2"))))
	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "run-3"))))

	require.Equal(t, []string{effect.ReasonAlreadyEarned, effect.ReasonAlreadyEarned}, rejectedReasons(h.ob))
	require.Len(t, h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobCredit)), 1, "rejections never credit")
	pb, _, _ := h.repo.PlayerBadge(context.Background(), tenantA, player1, b.ID)
	require.Equal(t, 1, pb.EarnedCount)

	rej, found, _ := h.repo.AwardByKey(context.Background(), tenantA, "run-2")
	require.True(t, found)
	require.Equal(t, domain.AwardRejected, rej.Status)
	require.Equal(t, effect.ReasonAlreadyEarned, rej.Reason)

	// A redelivered rejected command does not publish a second rejection.
	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "run-2"))))
	require.Len(t, rejectedReasons(h.ob), 2)
}

func TestAwardJobStackableUpToMaxAwards(t *testing.T) {
	h := newHarness(t, allowKeys{})
	b := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Stackable = true; p.MaxAwards = intp(2) })

	for _, key := range []string{"a", "b", "c", "d"} {
		require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, key))))
	}

	pb, _, _ := h.repo.PlayerBadge(context.Background(), tenantA, player1, b.ID)
	require.Equal(t, 2, pb.EarnedCount)
	awarded := h.ob.byTopic(contracts.TopicAwarded)
	require.Len(t, awarded, 2)
	require.True(t, awarded[0].(contracts.AwardedV1).IsFirstEarn)
	require.False(t, awarded[1].(contracts.AwardedV1).IsFirstEarn)
	require.Equal(t, 2, awarded[1].(contracts.AwardedV1).EarnedCount)
	require.Equal(t, []string{effect.ReasonLimitReached, effect.ReasonLimitReached}, rejectedReasons(h.ob))
	require.Len(t, h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobCredit)), 2, "each applied stack credits")
}

func TestAwardJobStackableUnlimited(t *testing.T) {
	h := newHarness(t, allowKeys{})
	b := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Stackable = true })
	for i := range 5 {
		require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, id.Derive("k", string(rune('a'+i)))))))
	}
	pb, _, _ := h.repo.PlayerBadge(context.Background(), tenantA, player1, b.ID)
	require.Equal(t, 5, pb.EarnedCount)
	require.Empty(t, rejectedReasons(h.ob))
}

func TestAwardJobZeroPointsIssuesNoCredit(t *testing.T) {
	h := newHarness(t, allowKeys{})
	zero := int64(0)
	b := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.PointsValue = &zero })
	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "k"))))
	require.Equal(t, []string{contracts.TopicAwarded}, h.ob.topics())
}

func TestAwardJobRejections(t *testing.T) {
	cases := map[string]struct {
		setup  func(h *harness) (badgeID, playerID string)
		reason string
	}{
		"inactive badge": {func(h *harness) (string, string) {
			return h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Active = false }).ID, player1
		}, effect.ReasonTargetInactive},
		"unknown badge": {func(h *harness) (string, string) { return ghost, player1 }, effect.ReasonTargetNotFound},
		"other tenant's badge": {func(h *harness) (string, string) {
			return h.seedBadge(t, func(p *domain.NewBadgeParams) { p.TenantID = tenantB }).ID, player1
		}, effect.ReasonTargetNotFound},
		"deleted badge": {func(h *harness) (string, string) {
			b := h.seedBadge(t, nil)
			require.NoError(t, h.svc.DeleteBadge(ctxAs(tenantA), b.ID))
			return b.ID, player1
		}, effect.ReasonTargetNotFound},
		"unknown player":  {func(h *harness) (string, string) { return h.seedBadge(t, nil).ID, ghost }, effect.ReasonPlayerNotFound},
		"inactive player": {func(h *harness) (string, string) { return h.seedBadge(t, nil).ID, player2 }, effect.ReasonPlayerInactive},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, allPerms)
			badgeID, playerID := tc.setup(h)
			require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(badgeID, playerID, "k"))))

			require.Equal(t, []string{tc.reason}, rejectedReasons(h.ob))
			require.Empty(t, h.ob.byTopic(contracts.TopicAwarded))
			require.Empty(t, h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobCredit)))
			_, found, _ := h.repo.PlayerBadge(context.Background(), tenantA, playerID, badgeID)
			require.False(t, found, "a rejected first award leaves no holding behind")

			rej := h.ob.byTopic(contracts.TopicAwardRejected)[0].(contracts.AwardRejectedV1)
			require.Equal(t, tenantA, rej.TenantID)
			require.Equal(t, "k", rej.IdempotencyKey)
		})
	}
}

func TestAwardJobTransientPlayerErrorIsRetried(t *testing.T) {
	h := newHarness(t, allowKeys{})
	b := h.seedBadge(t, nil)
	h.players.err = errs.New(errs.Unavailable, "player service down")

	err := h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "k")))
	require.Equal(t, errs.Unavailable, errs.KindOf(err), "transient → retry ladder, not DLQ")
	require.Empty(t, h.ob.topics())
	_, found, _ := h.repo.AwardByKey(context.Background(), tenantA, "k")
	require.False(t, found, "nothing recorded, so the retry can still apply")
}

func TestAwardJobMalformedPayloadIsInvalid(t *testing.T) {
	h := newHarness(t, allowKeys{})
	good := awardCmd(ghost, player1, "k")
	cases := map[string][]byte{
		"not json": []byte("{"),
		"payload garbage": func() []byte {
			b, _ := json.Marshal(map[string]any{"event_id": "x", "payload": "nope"})
			return b
		}(),
		"missing key":   jobBody(t, func() contracts.AwardCmdV1 { c := good; c.IdempotencyKey = ""; return c }()),
		"bad tenant id": jobBody(t, func() contracts.AwardCmdV1 { c := good; c.TenantID = "acme"; return c }()),
		"bad player id": jobBody(t, func() contracts.AwardCmdV1 { c := good; c.PlayerID = ""; return c }()),
		"bad badge id":  jobBody(t, func() contracts.AwardCmdV1 { c := good; c.BadgeID = "42"; return c }()),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			err := h.svc.HandleAwardJob(context.Background(), body)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
		})
	}
	require.Empty(t, h.ob.topics())
}

// ---- HTTP award ----

func TestAwardManuallyHappyPathAndReplay(t *testing.T) {
	h := newHarness(t, allPerms)
	b := h.seedBadge(t, nil)

	out, err := h.svc.AwardManually(ctxAs(tenantA), b.ID, player1, "client-key")
	require.NoError(t, err)
	require.False(t, out.Replay)
	require.Equal(t, 1, out.PlayerBadge.EarnedCount)
	require.Equal(t, "manual:client-key", out.Award.IdempotencyKey)
	require.Equal(t, effect.Source{Kind: effect.SourceManual, ID: adminID}, out.Award.Source)
	require.Equal(t, adminID, out.Award.AwardedBy)

	again, err := h.svc.AwardManually(ctxAs(tenantA), b.ID, player1, "client-key")
	require.NoError(t, err, "same key replays the original outcome, not 409")
	require.True(t, again.Replay)
	require.Equal(t, out.Award.ID, again.Award.ID)
	require.Equal(t, 1, again.PlayerBadge.EarnedCount)
	require.Len(t, h.ob.byTopic(contracts.TopicAwarded), 1)
	require.Len(t, h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobCredit)), 1)
}

func TestAwardManuallyWithoutKeyUsesFreshKeys(t *testing.T) {
	h := newHarness(t, allPerms)
	b := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Stackable = true })
	first, err := h.svc.AwardManually(ctxAs(tenantA), b.ID, player1, "")
	require.NoError(t, err)
	second, err := h.svc.AwardManually(ctxAs(tenantA), b.ID, player1, "")
	require.NoError(t, err)
	require.NotEqual(t, first.Award.IdempotencyKey, second.Award.IdempotencyKey)
	require.Equal(t, 2, second.PlayerBadge.EarnedCount)
}

func TestAwardManuallyRejectionsMapToCodedErrors(t *testing.T) {
	h := newHarness(t, allPerms)
	once := h.seedBadge(t, nil)
	capped := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Stackable = true; p.MaxAwards = intp(1) })
	inactive := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Active = false })
	ctx := ctxAs(tenantA)

	_, err := h.svc.AwardManually(ctx, once.ID, player1, "")
	require.NoError(t, err)
	_, err = h.svc.AwardManually(ctx, once.ID, player1, "")
	require.Equal(t, errs.Conflict, errs.KindOf(err))
	require.Equal(t, "badge_already_earned", errs.CodeOf(err))

	_, err = h.svc.AwardManually(ctx, capped.ID, player1, "")
	require.NoError(t, err)
	_, err = h.svc.AwardManually(ctx, capped.ID, player1, "")
	require.Equal(t, "badge_max_awards_reached", errs.CodeOf(err))

	_, err = h.svc.AwardManually(ctx, inactive.ID, player1, "")
	require.Equal(t, "badge_inactive", errs.CodeOf(err))

	_, err = h.svc.AwardManually(ctx, once.ID, player2, "")
	require.Equal(t, errs.Conflict, errs.KindOf(err))
	require.Equal(t, "player_inactive", errs.CodeOf(err))

	require.Equal(t,
		[]string{effect.ReasonAlreadyEarned, effect.ReasonLimitReached, effect.ReasonTargetInactive, effect.ReasonPlayerInactive},
		rejectedReasons(h.ob), "HTTP rejections are recorded and published like job rejections")
}

func TestAwardManuallyNotFoundRecordsNothing(t *testing.T) {
	h := newHarness(t, allPerms)
	b := h.seedBadge(t, nil)
	foreign := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.TenantID = tenantB })

	_, err := h.svc.AwardManually(ctxAs(tenantA), foreign.ID, player1, "")
	require.Equal(t, errs.NotFound, errs.KindOf(err), "another tenant's badge is 404")
	require.Equal(t, "badge_not_found", errs.CodeOf(err))

	_, err = h.svc.AwardManually(ctxAs(tenantA), b.ID, ghost, "")
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	require.Equal(t, "player_not_found", errs.CodeOf(err))

	require.Empty(t, h.ob.topics())
	require.Empty(t, h.repo.awards)
}

func TestAwardManuallyAuthz(t *testing.T) {
	h := newHarness(t, allowKeys{contracts.PermView.Key(): true})
	b := h.seedBadge(t, nil)

	_, err := h.svc.AwardManually(ctxAs(tenantA), b.ID, player1, "")
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))

	_, err = h.svc.AwardManually(ctxAs(""), b.ID, player1, "")
	require.ErrorIs(t, err, authz.ErrNoTenant, "no tenant in the token → 403, never an unscoped write")

	_, err = h.svc.AwardManually(context.Background(), b.ID, player1, "")
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err))
	require.Empty(t, h.ob.topics())
}

// ---- revoke ----

func TestRevokeRemovesAllStacksWithoutPointsReversal(t *testing.T) {
	h := newHarness(t, allPerms)
	b := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Stackable = true })
	for _, k := range []string{"a", "b", "c"} {
		require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, k))))
	}
	before, _, _ := h.repo.PlayerBadge(context.Background(), tenantA, player1, b.ID)
	creditsBefore := len(h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobCredit)))

	require.NoError(t, h.svc.Revoke(ctxAs(tenantA), b.ID, player1))

	_, found, _ := h.repo.PlayerBadge(context.Background(), tenantA, player1, b.ID)
	require.False(t, found)
	require.Len(t, h.repo.revocations, 1)
	require.Equal(t, 3, h.repo.revocations[0].EarnedCountRemoved)
	require.Equal(t, adminID, h.repo.revocations[0].RevokedBy)
	rev := h.ob.byTopic(contracts.TopicRevoked)
	require.Len(t, rev, 1)
	require.Equal(t, 3, rev[0].(contracts.RevokedV1).EarnedCountRemoved)
	require.Equal(t, before.ID, rev[0].(contracts.RevokedV1).PlayerBadgeID)
	require.Len(t, h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobCredit)), creditsBefore,
		"points are not taken back (product decision)")
	require.Empty(t, h.ob.byTopic(pointscontracts.Topic(pointscontracts.JobDebit)))
	require.Len(t, h.repo.appliedAwards(), 3, "the award ledger is append-only")

	// Re-award after revoke starts a new holding generation at 1.
	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "d"))))
	after, found, _ := h.repo.PlayerBadge(context.Background(), tenantA, player1, b.ID)
	require.True(t, found)
	require.Equal(t, 1, after.EarnedCount)
	require.NotEqual(t, before.ID, after.ID)
}

func TestRevokeNotHeldIsNoOp(t *testing.T) {
	h := newHarness(t, allPerms)
	b := h.seedBadge(t, nil)
	require.NoError(t, h.svc.Revoke(ctxAs(tenantA), b.ID, player1))
	require.Empty(t, h.ob.topics())
	require.Empty(t, h.repo.revocations)
}

func TestRevokeAuthzAndNotFound(t *testing.T) {
	h := newHarness(t, allowKeys{contracts.PermAward.Key(): true})
	b := h.seedBadge(t, nil)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(h.svc.Revoke(ctxAs(tenantA), b.ID, player1)))

	h = newHarness(t, allPerms)
	foreign := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.TenantID = tenantB })
	require.Equal(t, errs.NotFound, errs.KindOf(h.svc.Revoke(ctxAs(tenantA), foreign.ID, player1)))
	own := h.seedBadge(t, nil)
	err := h.svc.Revoke(ctxAs(tenantA), own.ID, ghost)
	require.Equal(t, "player_not_found", errs.CodeOf(err))
}

// ---- catalogue ----

func TestCreateBadgeStampsTenantAndChecksSlug(t *testing.T) {
	h := newHarness(t, allPerms)
	b, err := h.svc.CreateBadge(ctxAs(tenantA), domain.NewBadgeParams{TenantID: tenantB, Name: "First Steps", Tier: "gold", Category: "achievement", Active: true})
	require.NoError(t, err)
	require.Equal(t, tenantA, b.TenantID, "tenant comes from the principal, never the body")
	require.EqualValues(t, 50, b.PointsValue)

	_, err = h.svc.CreateBadge(ctxAs(tenantA), domain.NewBadgeParams{Name: "First  Steps!", Tier: "gold", Category: "achievement"})
	require.Equal(t, errs.AlreadyExists, errs.KindOf(err))
	require.Equal(t, "badge_slug_taken", errs.CodeOf(err))

	_, err = h.svc.CreateBadge(ctxAs(tenantB), domain.NewBadgeParams{Name: "First Steps", Tier: "gold", Category: "achievement"})
	require.NoError(t, err, "slugs are unique per tenant only")
}

func TestCreateBadgeDeniedForMembers(t *testing.T) {
	member := allowKeys{contracts.PermView.Key(): true, contracts.PermViewAny.Key(): true, contracts.PermAward.Key(): true}
	h := newHarness(t, member)
	_, err := h.svc.CreateBadge(ctxAs(tenantA), domain.NewBadgeParams{Name: "x", Tier: "gold", Category: "skill"})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(h.svc.DeleteBadge(ctxAs(tenantA), ghost)))
	_, err = h.svc.UpdateBadge(ctxAs(tenantA), ghost, domain.Patch{})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestGetUpdateDeleteAreTenantScoped(t *testing.T) {
	h := newHarness(t, allPerms)
	foreign := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.TenantID = tenantB })

	_, err := h.svc.GetBadge(ctxAs(tenantA), foreign.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	name := "hijack"
	_, err = h.svc.UpdateBadge(ctxAs(tenantA), foreign.ID, domain.Patch{Name: &name})
	require.Equal(t, errs.NotFound, errs.KindOf(err))
	require.Equal(t, errs.NotFound, errs.KindOf(h.svc.DeleteBadge(ctxAs(tenantA), foreign.ID)))
}

func TestUpdateBadgePartialAndDelete(t *testing.T) {
	h := newHarness(t, allPerms)
	b := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Stackable = true; p.MaxAwards = intp(3) })
	name := "Renamed"
	got, err := h.svc.UpdateBadge(ctxAs(tenantA), b.ID, domain.Patch{Name: &name, MaxAwardsSet: true})
	require.NoError(t, err)
	require.Equal(t, "Renamed", got.Name)
	require.Equal(t, b.Slug, got.Slug)
	require.Nil(t, got.MaxAwards)
	require.True(t, got.Stackable, "omitted fields stay untouched")
	require.Equal(t, 1, got.Version)

	require.NoError(t, h.svc.DeleteBadge(ctxAs(tenantA), b.ID))
	_, err = h.svc.GetBadge(ctxAs(tenantA), b.ID)
	require.Equal(t, errs.NotFound, errs.KindOf(err))
}

func TestListBadgesFiltersAndPages(t *testing.T) {
	h := newHarness(t, allPerms)
	for range 3 {
		h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Tier = "gold" })
	}
	h.seedBadge(t, func(p *domain.NewBadgeParams) { p.Tier = "silver"; p.Secret = true })
	h.seedBadge(t, func(p *domain.NewBadgeParams) { p.TenantID = tenantB; p.Tier = "gold" })

	all, more, err := h.svc.ListBadges(ctxAs(tenantA), BadgeFilter{})
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, all, 4, "other tenants excluded, secret included")

	page, more, err := h.svc.ListBadges(ctxAs(tenantA), BadgeFilter{Tier: "gold", Limit: 2})
	require.NoError(t, err)
	require.True(t, more)
	require.Len(t, page, 2)
	rest, more, err := h.svc.ListBadges(ctxAs(tenantA), BadgeFilter{Tier: "gold", Limit: 2,
		Before: page[1].CreatedAt, BeforeID: page[1].ID})
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, rest, 1)

	_, _, err = newHarness(t, allowKeys{}).svc.ListBadges(ctxAs(tenantA), BadgeFilter{})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestListPlayerBadges(t *testing.T) {
	h := newHarness(t, allPerms)
	b := h.seedBadge(t, nil)
	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "k"))))

	rows, more, err := h.svc.ListPlayerBadges(ctxAs(tenantA), player1, PageCursor{}, 0)
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Badge)
	require.Equal(t, b.ID, rows[0].Badge.ID)

	_, _, err = h.svc.ListPlayerBadges(ctxAs(tenantA), ghost, PageCursor{}, 0)
	require.Equal(t, "player_not_found", errs.CodeOf(err))
	_, _, err = h.svc.ListPlayerBadges(ctxAs(tenantB), player1, PageCursor{}, 0)
	require.Equal(t, errs.NotFound, errs.KindOf(err), "another tenant's player is 404")
}

// ---- reader ----

func TestReader(t *testing.T) {
	h := newHarness(t, allPerms)
	live := h.seedBadge(t, nil)
	gone := h.seedBadge(t, nil)
	foreign := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.TenantID = tenantB })
	require.NoError(t, h.svc.DeleteBadge(ctxAs(tenantA), gone.ID))

	snaps, err := h.svc.BadgesByIDs(context.Background(), tenantA, []string{live.ID, gone.ID, foreign.ID, ghost})
	require.NoError(t, err)
	require.Len(t, snaps, 1)
	require.Equal(t, live.ID, snaps[0].ID)
	require.Equal(t, live.PointsValue, snaps[0].PointsValue)

	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(live.ID, player1, "k"))))
	held, err := h.svc.PlayerBadges(context.Background(), tenantA, []string{player1, ghost})
	require.NoError(t, err)
	require.Len(t, held, 1)
	require.Equal(t, 1, held[0].EarnedCount)
	none, err := h.svc.PlayerBadges(context.Background(), tenantB, []string{player1})
	require.NoError(t, err)
	require.Empty(t, none)
}

// ---- subscriptions and crons ----

func tenantDeletedEnvelope(t *testing.T, tenantID string) bus.Envelope {
	t.Helper()
	payload, err := json.Marshal(identitycontracts.TenantDeletedV1{TenantID: tenantID, At: t0})
	require.NoError(t, err)
	return bus.Envelope{EventID: id.NewID(), Topic: identitycontracts.TopicTenantDeleted, Payload: payload}
}

func TestOnTenantDeletedPurgesIdempotently(t *testing.T) {
	h := newHarness(t, allPerms)
	b := h.seedBadge(t, nil)
	keep := h.seedBadge(t, func(p *domain.NewBadgeParams) { p.TenantID = tenantB })
	require.NoError(t, h.svc.HandleAwardJob(context.Background(), jobBody(t, awardCmd(b.ID, player1, "k"))))

	ev := tenantDeletedEnvelope(t, tenantA)
	require.NoError(t, h.svc.OnTenantDeleted(context.Background(), ev))
	require.NoError(t, h.svc.OnTenantDeleted(context.Background(), ev), "redelivery is a no-op")

	require.Len(t, h.repo.badges, 1)
	require.Contains(t, h.repo.badges, keep.ID)
	require.Empty(t, h.repo.holdings)
	require.Empty(t, h.repo.awards)

	err := h.svc.OnTenantDeleted(context.Background(), bus.Envelope{Payload: []byte("{")})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	err = h.svc.OnTenantDeleted(context.Background(), tenantDeletedEnvelope(t, ""))
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestReconcileSweepsFromMarkerAndCountsDrift(t *testing.T) {
	h := newHarness(t, allPerms)
	drift := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_drift"}, []string{"check"})
	h.svc.drift = drift
	last := t0.Add(-time.Hour)
	h.repo.lastRun = last
	h.repo.drift = []domain.Drift{
		{PlayerBadgeID: "pb1", EarnedCount: 2, AppliedAwards: 1, Stackable: true},
		{PlayerBadgeID: "pb2", EarnedCount: 3, AppliedAwards: 3, Stackable: true, MaxAwards: intp(2)},
	}

	require.NoError(t, h.svc.Reconcile(context.Background(), h.repo))
	require.Equal(t, last, h.repo.sweptSince)
	require.Equal(t, t0, h.repo.markedAt)
	require.InDelta(t, 1, counterValue(t, drift, domain.DriftCountMismatch), 0)
	require.InDelta(t, 1, counterValue(t, drift, domain.DriftOverMax), 0)
}

type failingReconciler struct{ fakeRepo }

func (f *failingReconciler) DriftSince(context.Context, time.Time) ([]domain.Drift, error) {
	return nil, errors.New("db down")
}

func TestReconcileFailureDoesNotAdvanceMarker(t *testing.T) {
	h := newHarness(t, allPerms)
	rec := &failingReconciler{fakeRepo: *newFakeRepo()}
	require.Error(t, h.svc.Reconcile(context.Background(), rec))
	require.True(t, rec.markedAt.IsZero(), "a failed sweep must be retried from the same marker")
}

func counterValue(t *testing.T, vec *prometheus.CounterVec, label string) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, vec.WithLabelValues(label).Write(&m))
	return m.GetCounter().GetValue()
}
