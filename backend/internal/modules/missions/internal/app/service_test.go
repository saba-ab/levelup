package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	badgescontracts "levelup/internal/modules/badges/contracts"
	"levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/missions/internal/domain"
	"levelup/internal/modules/missions/internal/ports"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/shared/effect"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

const (
	tenantA  = "0198d000-0000-7000-8000-00000000000a"
	tenantB  = "0198d000-0000-7000-8000-00000000000b"
	player1  = "0198d000-0000-7000-8000-000000000101"
	player2  = "0198d000-0000-7000-8000-000000000102"
	inactive = "0198d000-0000-7000-8000-000000000103"
	unknown  = "0198d000-0000-7000-8000-000000000199"
	badgeID  = "0198d000-0000-7000-8000-0000000000bb"
)

var start = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

type harness struct {
	svc   *Service
	repo  *fakeRepo
	ob    *fakeOutbox
	clock *clock.Fake
	ctx   context.Context
}

func newHarness(t *testing.T, enf authz.Enforcer) *harness {
	t.Helper()
	if enf == nil {
		enf = allowAll()
	}
	repo, ob := newFakeRepo(), &fakeOutbox{}
	c := clock.NewFake(start)
	players := &fakePlayers{players: map[string]ports.PlayerSnapshot{
		player1:  {ID: player1, TenantID: tenantA, ExternalID: "ext-1", Active: true},
		player2:  {ID: player2, TenantID: tenantA, ExternalID: "ext-2", Active: true},
		inactive: {ID: inactive, TenantID: tenantA, ExternalID: "ext-inactive", Active: false},
	}}
	svc := NewService(repo, players, ob, enf, nil, c, 2)
	svc.tx = func(_ context.Context, fn func(tx *gorm.DB) error) error { return fn(nil) }
	ctx := authz.Into(context.Background(), authz.Principal{UserID: "admin-1", TenantID: tenantA, RoleIDs: []int64{2}})
	return &harness{svc: svc, repo: repo, ob: ob, clock: c, ctx: ctx}
}

func (h *harness) mission(t *testing.T, mod func(*CreateMissionCmd)) domain.Mission {
	t.Helper()
	cmd := CreateMissionCmd{
		Name: "Buy things", Type: contracts.TypeRepeating, Status: contracts.MissionActive,
		Target: 3, PointsReward: 100, XPReward: 50, BadgeRewardID: badgeID,
	}
	if mod != nil {
		mod(&cmd)
	}
	m, err := h.svc.CreateMission(h.ctx, cmd)
	require.NoError(t, err)
	h.ob.published = nil
	return m
}

func progressCmd(key, missionID, playerID string, inc int64) contracts.ProgressCmdV1 {
	return contracts.ProgressCmdV1{
		IdempotencyKey: key, TenantID: tenantA, PlayerID: playerID, MissionID: missionID, Increment: inc,
		Source: effect.Source{Kind: effect.SourceRule, ID: "exec-1", ActivityID: "act-1"}, OccurredAt: start,
	}
}

func (h *harness) attempts(missionID, playerID string) []domain.Attempt {
	var out []domain.Attempt
	for _, a := range h.repo.attempts {
		if a.MissionID == missionID && a.PlayerID == playerID {
			out = append(out, a)
		}
	}
	return out
}

func requireCode(t *testing.T, err error, kind errs.Kind, code string) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, kind, errs.KindOf(err), err.Error())
	require.Equal(t, code, errs.CodeOf(err))
}

// ---- CRUD ----

func TestCreateMissionStampsTenantAndPublishes(t *testing.T) {
	h := newHarness(t, nil)
	m, err := h.svc.CreateMission(h.ctx, CreateMissionCmd{Name: "Daily Login", Type: contracts.TypeDaily, Target: 1})
	require.NoError(t, err)
	require.Equal(t, tenantA, m.TenantID)
	require.Equal(t, "daily-login", m.Slug)
	require.Equal(t, contracts.MissionDraft, m.Status)
	require.Equal(t, []string{contracts.TopicMissionCreated}, h.ob.topics())

	_, err = h.svc.CreateMission(h.ctx, CreateMissionCmd{Name: "Daily Login", Type: contracts.TypeDaily, Target: 1})
	requireCode(t, err, errs.AlreadyExists, "slug_taken")
}

func TestCreateMissionDeniedWithoutPermission(t *testing.T) {
	h := newHarness(t, allowKeys{contracts.PermViewAny.Key(): true})
	_, err := h.svc.CreateMission(h.ctx, CreateMissionCmd{Name: "X", Type: contracts.TypeDaily, Target: 1})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Empty(t, h.ob.published)
	require.Empty(t, h.repo.missions)
}

func TestNoTenantPrincipalIsDenied(t *testing.T) {
	h := newHarness(t, nil)
	ctx := authz.Into(context.Background(), authz.Principal{UserID: "platform"})
	_, err := h.svc.ListMissions(ctx, MissionFilter{}, "", 0)
	require.ErrorIs(t, err, authz.ErrNoTenant)
}

func TestCrossTenantMissionIsNotFound(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)
	ctxB := authz.Into(context.Background(), authz.Principal{UserID: "b", TenantID: tenantB})

	_, err := h.svc.GetMission(ctxB, m.ID)
	requireCode(t, err, errs.NotFound, "mission_not_found")
	_, err = h.svc.UpdateMission(ctxB, m.ID, UpdateMissionCmd{Name: new("hijack")})
	requireCode(t, err, errs.NotFound, "mission_not_found")
	requireCode(t, h.svc.DeleteMission(ctxB, m.ID), errs.NotFound, "mission_not_found")
	_, err = h.svc.ManualProgress(ctxB, m.ID, player1, 1, "")
	requireCode(t, err, errs.NotFound, "mission_not_found")
	_, err = h.svc.StartMission(ctxB, m.ID, player1)
	requireCode(t, err, errs.NotFound, "mission_not_found")
}

func TestUpdateMissionPartialAndTransitions(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, func(c *CreateMissionCmd) { c.Status = contracts.MissionDraft; c.Description = "keep me" })

	got, err := h.svc.UpdateMission(h.ctx, m.ID, UpdateMissionCmd{Name: new("Renamed"), Status: new(contracts.MissionActive)})
	require.NoError(t, err)
	require.Equal(t, "Renamed", got.Name)
	require.Equal(t, "keep me", got.Description, "omitted fields stay untouched")
	require.Equal(t, contracts.MissionActive, got.Status)
	require.Equal(t, []string{contracts.TopicMissionUpdated}, h.ob.topics())

	_, err = h.svc.UpdateMission(h.ctx, m.ID, UpdateMissionCmd{Status: new(contracts.MissionDraft)})
	requireCode(t, err, errs.Conflict, "invalid_status_transition")

	_, err = h.svc.UpdateMission(h.ctx, m.ID, UpdateMissionCmd{Type: new(contracts.TypeDaily)})
	requireCode(t, err, errs.Conflict, "mission_type_immutable")

	got, err = h.svc.UpdateMission(h.ctx, m.ID, UpdateMissionCmd{BadgeRewardID: new(""), MaxCompletionsPerPlayer: new(0)})
	require.NoError(t, err)
	require.Empty(t, got.BadgeRewardID)
	require.Nil(t, got.MaxCompletionsPerPlayer)
}

func TestDeleteMissionHidesIt(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)
	require.NoError(t, h.svc.DeleteMission(h.ctx, m.ID))
	require.Equal(t, []string{contracts.TopicMissionDeleted}, h.ob.topics())
	_, err := h.svc.GetMission(h.ctx, m.ID)
	requireCode(t, err, errs.NotFound, "mission_not_found")
}

func TestListMissionsPaginates(t *testing.T) {
	h := newHarness(t, nil)
	for i := range 3 {
		h.mission(t, func(c *CreateMissionCmd) { c.Name = "M" + string(rune('a'+i)) })
		h.clock.Advance(time.Second)
	}
	page, err := h.svc.ListMissions(h.ctx, MissionFilter{}, "", 2)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.Equal(t, "mc", page.Items[0].Slug, "newest first")
	require.NotEmpty(t, page.NextCursor)

	page, err = h.svc.ListMissions(h.ctx, MissionFilter{}, page.NextCursor, 2)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Empty(t, page.NextCursor)

	_, err = h.svc.ListMissions(h.ctx, MissionFilter{}, "garbage!", 2)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

// ---- progress job (flow F4) ----

func TestProgressJobCompletesExactlyOnceWithDeterministicRewardCommands(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)

	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("k1", m.ID, player1, 2)))
	require.Equal(t, []string{contracts.TopicStarted, contracts.TopicProgressUpdated}, h.ob.topics())
	// The rules module settles its progress_mission effect by this key.
	updated := h.ob.byTopic(contracts.TopicProgressUpdated)
	require.Len(t, updated, 1)
	require.Equal(t, "k1", updated[0].(contracts.AttemptV1).IdempotencyKey)

	h.ob.published = nil
	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("k2", m.ID, player1, 5)))
	require.Equal(t, []string{
		contracts.TopicProgressUpdated,
		contracts.TopicCompleted,
		pointscontracts.Topic(pointscontracts.JobCredit),
		progressioncontracts.Topic(progressioncontracts.JobGrantXP),
		badgescontracts.Topic(badgescontracts.JobAward),
	}, h.ob.topics())

	attempts := h.attempts(m.ID, player1)
	require.Len(t, attempts, 1)
	a := attempts[0]
	require.Equal(t, contracts.AttemptCompleted, a.Status)
	require.Equal(t, int64(3), a.Progress, "capped at target")

	src := effect.Source{Kind: effect.SourceMission, ID: a.ID, ActivityID: "act-1"}
	credit := h.ob.byTopic("job.points.credit")[0].(pointscontracts.CreditCmdV1)
	require.Equal(t, id.Derive("mission_completion", a.ID, "points"), credit.IdempotencyKey)
	require.Equal(t, int64(100), credit.Amount)
	require.Equal(t, pointscontracts.KindReward, credit.Kind)
	require.Equal(t, tenantA, credit.TenantID)
	require.Equal(t, player1, credit.PlayerID)
	require.Equal(t, src, credit.Source)

	xp := h.ob.byTopic("job.progression.grant_xp")[0].(progressioncontracts.GrantXPCmdV1)
	require.Equal(t, id.Derive("mission_completion", a.ID, "xp"), xp.IdempotencyKey)
	require.Equal(t, int64(50), xp.Amount)
	require.Equal(t, src, xp.Source)

	badge := h.ob.byTopic("job.badges.award")[0].(badgescontracts.AwardCmdV1)
	require.Equal(t, id.Derive("mission_completion", a.ID, "badge"), badge.IdempotencyKey)
	require.Equal(t, badgeID, badge.BadgeID)
	require.Equal(t, src, badge.Source)

	done := h.ob.byTopic(contracts.TopicCompleted)[0].(contracts.CompletedV1)
	require.Equal(t, a.ID, done.AttemptID)
	require.Equal(t, m.Slug, done.MissionSlug)
}

func TestProgressJobRedeliveryIsNoOp(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, func(c *CreateMissionCmd) { c.Target = 2 })
	cmd := progressCmd("same-key", m.ID, player1, 2)

	require.NoError(t, h.svc.HandleProgressJob(h.ctx, cmd))
	published := len(h.ob.published)
	require.Len(t, h.ob.byTopic(contracts.TopicCompleted), 1)

	for range 3 {
		require.NoError(t, h.svc.HandleProgressJob(h.ctx, cmd))
	}
	require.Len(t, h.ob.published, published, "redelivery publishes nothing")
	require.Len(t, h.attempts(m.ID, player1), 1, "redelivery starts no new attempt of a repeating mission")
	require.Len(t, h.repo.events, 1)
}

func TestProgressAfterCompletionNeverCompletesAgain(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, func(c *CreateMissionCmd) { c.Type = contracts.TypeOneTime; c.Target = 1 })

	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("a", m.ID, player1, 1)))
	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("b", m.ID, player1, 1)))
	require.Len(t, h.ob.byTopic(contracts.TopicCompleted), 1)
	require.Len(t, h.ob.byTopic("job.points.credit"), 1)

	rejected := h.ob.byTopic(contracts.TopicProgressRejected)
	require.Len(t, rejected, 1)
	r := rejected[0].(contracts.ProgressRejectedV1)
	require.Equal(t, effect.ReasonLimitReached, r.Reason)
	require.Equal(t, "b", r.IdempotencyKey)
}

func TestRepeatingRestartKeepsNoOldProgressAndRespectsMax(t *testing.T) {
	h := newHarness(t, nil)
	two := 2
	m := h.mission(t, func(c *CreateMissionCmd) { c.Target = 3; c.MaxCompletionsPerPlayer = &two })

	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("1", m.ID, player1, 3)))
	h.clock.Advance(time.Minute)
	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("2", m.ID, player1, 1)))

	var open domain.Attempt
	for _, a := range h.attempts(m.ID, player1) {
		if a.Status == contracts.AttemptInProgress {
			open = a
		}
	}
	require.Equal(t, int64(1), open.Progress, "restart begins at zero, not at the old progress (Laravel B3)")

	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("3", m.ID, player1, 2)))
	require.Len(t, h.ob.byTopic(contracts.TopicCompleted), 2)

	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("4", m.ID, player1, 1)))
	require.Len(t, h.ob.byTopic(contracts.TopicCompleted), 2, "max_completions_per_player reached")
	r := h.ob.byTopic(contracts.TopicProgressRejected)[0].(contracts.ProgressRejectedV1)
	require.Equal(t, effect.ReasonLimitReached, r.Reason)

	counts, err := h.svc.CompletedCounts(h.ctx, tenantA, []string{player1, player2})
	require.NoError(t, err)
	require.Equal(t, map[string]int{player1: 2}, counts)
}

func TestDailyMissionNewAttemptPerPeriod(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, func(c *CreateMissionCmd) { c.Type = contracts.TypeDaily; c.Target = 1 })

	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("d1", m.ID, player1, 1)))
	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("d1-again", m.ID, player1, 1)))
	require.Len(t, h.ob.byTopic(contracts.TopicCompleted), 1, "once per day")
	require.Len(t, h.ob.byTopic(contracts.TopicProgressRejected), 1)

	h.clock.Advance(24 * time.Hour)
	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("d2", m.ID, player1, 1)))
	require.Len(t, h.ob.byTopic(contracts.TopicCompleted), 2, "a new day opens a new attempt")

	keys := map[string]bool{}
	for _, a := range h.attempts(m.ID, player1) {
		keys[a.PeriodKey] = true
		require.Equal(t, contracts.AttemptCompleted, a.Status)
	}
	require.Equal(t, map[string]bool{"2026-10-07": true, "2026-10-08": true}, keys)
}

func TestProgressNoRewardCommandsWhenNoneConfigured(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, func(c *CreateMissionCmd) { c.Target = 1; c.PointsReward = 0; c.XPReward = 0; c.BadgeRewardID = "" })
	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("x", m.ID, player1, 1)))
	require.Equal(t, []string{contracts.TopicStarted, contracts.TopicProgressUpdated, contracts.TopicCompleted}, h.ob.topics())
}

func TestProgressJobRejections(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(h *harness, m domain.Mission)
		player string
		reason string
	}{
		{"player not found", nil, unknown, effect.ReasonPlayerNotFound},
		{"player inactive", nil, inactive, effect.ReasonPlayerInactive},
		{"mission paused", func(h *harness, m domain.Mission) {
			_, err := h.svc.UpdateMission(h.ctx, m.ID, UpdateMissionCmd{Status: new(contracts.MissionPaused)})
			require.NoError(t, err)
		}, player1, effect.ReasonTargetInactive},
		{"mission out of window", func(h *harness, m domain.Mission) {
			end := start.Add(time.Hour)
			_, err := h.svc.UpdateMission(h.ctx, m.ID, UpdateMissionCmd{EndsAt: &end})
			require.NoError(t, err)
			h.clock.Advance(2 * time.Hour)
		}, player1, effect.ReasonTargetInactive},
		{"mission deleted", func(h *harness, m domain.Mission) {
			require.NoError(t, h.svc.DeleteMission(h.ctx, m.ID))
		}, player1, effect.ReasonTargetNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, nil)
			m := h.mission(t, nil)
			if tc.setup != nil {
				tc.setup(h, m)
			}
			h.ob.published = nil
			cmd := progressCmd("rej", m.ID, tc.player, 1)
			require.NoError(t, h.svc.HandleProgressJob(h.ctx, cmd), "a rejection is a result, never an error")
			require.Equal(t, []string{contracts.TopicProgressRejected}, h.ob.topics())
			require.Equal(t, tc.reason, h.ob.published[0].payload.(contracts.ProgressRejectedV1).Reason)
			require.Empty(t, h.attempts(m.ID, tc.player))

			// Redelivered rejection: recorded once, published once.
			require.NoError(t, h.svc.HandleProgressJob(h.ctx, cmd))
			require.Len(t, h.ob.published, 1)
			ev, ok, err := h.repo.ProgressEventByKey(h.ctx, tenantA, "rej")
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, domain.ProgressRejected, ev.Status)
			require.Equal(t, tc.reason, ev.Reason)
		})
	}
}

func TestProgressJobMalformedIsInvalid(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)
	bad := []contracts.ProgressCmdV1{
		progressCmd("", m.ID, player1, 1),
		progressCmd("k", m.ID, player1, 0),
		progressCmd("k", "not-a-uuid", player1, 1),
		progressCmd("k", m.ID, "nope", 1),
	}
	for _, cmd := range bad {
		require.Equal(t, errs.Invalid, errs.KindOf(h.svc.HandleProgressJob(h.ctx, cmd)))
	}
	require.Empty(t, h.ob.published)
}

func TestProgressJobTransientPlayerErrorRetries(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)
	h.svc.players = &fakePlayers{err: errs.New(errs.Unavailable, "player down")}
	err := h.svc.HandleProgressJob(h.ctx, progressCmd("k", m.ID, player1, 1))
	require.Equal(t, errs.Unavailable, errs.KindOf(err))
	require.Empty(t, h.repo.events, "nothing recorded: the retry must be able to apply it")
}

// ---- HTTP progress / start / complete ----

func TestManualProgressUsesNamespacedIdempotencyKey(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, func(c *CreateMissionCmd) { c.Target = 10 })

	res, err := h.svc.ManualProgress(h.ctx, m.ID, player1, 4, "client-key")
	require.NoError(t, err)
	require.Equal(t, OutcomeApplied, res.Outcome)
	require.Equal(t, int64(4), res.Attempt.Progress)

	ev, ok, err := h.repo.ProgressEventByKey(h.ctx, tenantA, "manual:client-key")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, effect.SourceManual, ev.SourceKind)
	require.Equal(t, "admin-1", ev.SourceID)

	again, err := h.svc.ManualProgress(h.ctx, m.ID, player1, 4, "client-key")
	require.NoError(t, err)
	require.Equal(t, OutcomeDuplicate, again.Outcome)
	require.Equal(t, int64(4), again.Attempt.Progress, "replayed, not counted twice")

	_, err = h.svc.ManualProgress(h.ctx, m.ID, player1, 1, "")
	require.NoError(t, err)
	_, err = h.svc.ManualProgress(h.ctx, m.ID, player1, 1, "")
	require.NoError(t, err)
	require.Len(t, h.repo.events, 3, "no key → every call counts")
}

func TestManualProgressRejectionsAreCodedErrors(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)
	_, err := h.svc.ManualProgress(h.ctx, m.ID, unknown, 1, "")
	requireCode(t, err, errs.NotFound, "player_not_found")
	_, err = h.svc.ManualProgress(h.ctx, m.ID, inactive, 1, "")
	requireCode(t, err, errs.Conflict, "player_inactive")

	_, err = h.svc.UpdateMission(h.ctx, m.ID, UpdateMissionCmd{Status: new(contracts.MissionPaused)})
	require.NoError(t, err)
	_, err = h.svc.ManualProgress(h.ctx, m.ID, player1, 1, "k")
	requireCode(t, err, errs.Conflict, "mission_not_available")
	_, err = h.svc.ManualProgress(h.ctx, m.ID, player1, 1, "k")
	requireCode(t, err, errs.Conflict, "mission_not_available")

	h2 := newHarness(t, allowKeys{contracts.PermViewAny.Key(): true})
	_, err = h2.svc.ManualProgress(h2.ctx, m.ID, player1, 1, "")
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err), "progress is authorized (Laravel B5)")
}

func TestStartMission(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)

	a, err := h.svc.StartMission(h.ctx, m.ID, player1)
	require.NoError(t, err)
	require.Equal(t, contracts.AttemptInProgress, a.Status)
	require.Equal(t, int64(0), a.Progress)
	require.Equal(t, int64(3), a.Target)
	require.Equal(t, []string{contracts.TopicStarted}, h.ob.topics())

	_, err = h.svc.StartMission(h.ctx, m.ID, player1)
	requireCode(t, err, errs.AlreadyExists, "mission_already_started")
	_, err = h.svc.StartMission(h.ctx, m.ID, unknown)
	requireCode(t, err, errs.NotFound, "player_not_found")

	draft := h.mission(t, func(c *CreateMissionCmd) { c.Name = "draft"; c.Status = contracts.MissionDraft })
	_, err = h.svc.StartMission(h.ctx, draft.ID, player1)
	requireCode(t, err, errs.Conflict, "mission_not_available")

	h3 := newHarness(t, allowKeys{})
	_, err = h3.svc.StartMission(h3.ctx, m.ID, player1)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestCompleteMission(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)

	_, err := h.svc.CompleteMission(h.ctx, m.ID, player1)
	requireCode(t, err, errs.NotFound, "mission_not_started") // Laravel B1 gave a 500

	_, err = h.svc.ManualProgress(h.ctx, m.ID, player1, 2, "")
	require.NoError(t, err)
	_, err = h.svc.CompleteMission(h.ctx, m.ID, player1)
	requireCode(t, err, errs.Conflict, "mission_not_completed")

	_, err = h.svc.ManualProgress(h.ctx, m.ID, player1, 1, "")
	require.NoError(t, err)
	require.Len(t, h.ob.byTopic(contracts.TopicCompleted), 1)

	// Repeated complete: same attempt back, nothing re-published (B2, R61).
	before := len(h.ob.published)
	for range 3 {
		a, err := h.svc.CompleteMission(h.ctx, m.ID, player1)
		require.NoError(t, err)
		require.Equal(t, contracts.AttemptCompleted, a.Status)
	}
	require.Len(t, h.ob.published, before)
	require.Len(t, h.ob.byTopic("job.points.credit"), 1)
}

func TestCompleteMissionExplicitPathPublishesRewards(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)
	a, err := h.svc.StartMission(h.ctx, m.ID, player1)
	require.NoError(t, err)
	// Reaching the target through the domain without auto-complete is not
	// possible via the API; seed the attempt at its target to exercise the
	// explicit transition.
	stored := h.repo.attempts[a.ID]
	stored.Progress = stored.Target
	h.repo.attempts[a.ID] = stored
	h.ob.published = nil

	got, err := h.svc.CompleteMission(h.ctx, m.ID, player1)
	require.NoError(t, err)
	require.Equal(t, contracts.AttemptCompleted, got.Status)
	require.Equal(t, []string{
		contracts.TopicCompleted,
		pointscontracts.Topic(pointscontracts.JobCredit),
		progressioncontracts.Topic(progressioncontracts.JobGrantXP),
		badgescontracts.Topic(badgescontracts.JobAward),
	}, h.ob.topics())
	credit := h.ob.byTopic("job.points.credit")[0].(pointscontracts.CreditCmdV1)
	require.Equal(t, id.Derive("mission_completion", a.ID, "points"), credit.IdempotencyKey)
}

// ---- lists ----

func TestPlayerMissionsListIncludesMissionSummary(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)
	_, err := h.svc.StartMission(h.ctx, m.ID, player1)
	require.NoError(t, err)

	page, err := h.svc.ListPlayerAttempts(h.ctx, player1, AttemptFilter{}, "", 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.True(t, page.Items[0].HasMission)
	require.Equal(t, m.Slug, page.Items[0].Mission.Slug)

	_, err = h.svc.ListPlayerAttempts(h.ctx, unknown, AttemptFilter{}, "", 0)
	requireCode(t, err, errs.NotFound, "player_not_found")

	attempts, err := h.svc.ListMissionAttempts(h.ctx, m.ID, AttemptFilter{}, "", 0)
	require.NoError(t, err)
	require.Len(t, attempts.Items, 1)
}

// ---- sweep ----

func TestExpireSweep(t *testing.T) {
	h := newHarness(t, nil)
	end := start.Add(time.Hour)
	ending := h.mission(t, func(c *CreateMissionCmd) { c.Name = "ending"; c.EndsAt = &end })
	daily := h.mission(t, func(c *CreateMissionCmd) { c.Name = "daily"; c.Type = contracts.TypeDaily; c.Target = 5 })
	forever := h.mission(t, func(c *CreateMissionCmd) { c.Name = "forever" })

	_, err := h.svc.StartMission(h.ctx, ending.ID, player1)
	require.NoError(t, err)
	_, err = h.svc.StartMission(h.ctx, daily.ID, player1)
	require.NoError(t, err)
	keep, err := h.svc.StartMission(h.ctx, forever.ID, player1)
	require.NoError(t, err)
	h.ob.published = nil

	h.clock.Advance(13 * time.Hour) // past ending's ends_at and past the daily period
	require.NoError(t, h.svc.ExpireSweep(h.ctx))

	require.Len(t, h.ob.byTopic(contracts.TopicExpired), 1)
	require.Equal(t, ending.ID, h.ob.byTopic(contracts.TopicExpired)[0].(contracts.MissionChangedV1).MissionID)
	require.Equal(t, contracts.MissionExpired, h.repo.missions[ending.ID].Status)
	require.Len(t, h.ob.byTopic(contracts.TopicAttemptExpired), 2)
	require.Equal(t, contracts.AttemptInProgress, h.repo.attempts[keep.ID].Status)
	require.Equal(t, h.clock.Now(), h.repo.markers[contracts.JobExpireSweep])

	// Reconciling: a second run finds nothing more.
	h.ob.published = nil
	require.NoError(t, h.svc.ExpireSweep(h.ctx))
	require.Empty(t, h.ob.published)
}

func TestExpireSweepDrainsInBatches(t *testing.T) {
	h := newHarness(t, nil) // batch size 2
	end := start.Add(time.Minute)
	for i := range 5 {
		h.mission(t, func(c *CreateMissionCmd) { c.Name = "m" + string(rune('a'+i)); c.EndsAt = &end })
	}
	h.clock.Advance(time.Hour)
	require.NoError(t, h.svc.ExpireSweep(h.ctx))
	require.Len(t, h.ob.byTopic(contracts.TopicExpired), 5)
}

// ---- subscriptions ----

func TestTenantDeletedPurgesIdempotently(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, nil)
	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("k", m.ID, player1, 1)))

	require.NoError(t, h.svc.OnTenantDeleted(h.ctx, tenantA))
	require.NoError(t, h.svc.OnTenantDeleted(h.ctx, tenantA))
	require.Empty(t, h.repo.missions)
	require.Empty(t, h.repo.attempts)
	require.Empty(t, h.repo.events)
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.OnTenantDeleted(h.ctx, "")))
}

func TestPlayerDeletedAbandonsOpenAttemptsIdempotently(t *testing.T) {
	h := newHarness(t, nil)
	m := h.mission(t, func(c *CreateMissionCmd) { c.Target = 1 })
	other := h.mission(t, func(c *CreateMissionCmd) { c.Name = "other" })
	require.NoError(t, h.svc.HandleProgressJob(h.ctx, progressCmd("done", m.ID, player1, 1)))
	open, err := h.svc.StartMission(h.ctx, other.ID, player1)
	require.NoError(t, err)

	require.NoError(t, h.svc.OnPlayerDeleted(h.ctx, tenantA, player1))
	require.NoError(t, h.svc.OnPlayerDeleted(h.ctx, tenantA, player1))
	require.Equal(t, contracts.AttemptAbandoned, h.repo.attempts[open.ID].Status)
	counts, err := h.svc.CompletedCounts(h.ctx, tenantA, []string{player1})
	require.NoError(t, err)
	require.Equal(t, 1, counts[player1], "completed history is kept")
}

func TestRejectionErrorMapping(t *testing.T) {
	require.True(t, errors.Is(rejectionError(effect.ReasonTargetInactive), domain.ErrNotAvailable))
	require.True(t, errors.Is(rejectionError(effect.ReasonLimitReached), domain.ErrLimitReached))
}
