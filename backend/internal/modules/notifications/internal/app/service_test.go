package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	badgescontracts "levelup/internal/modules/badges/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	missionscontracts "levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/domain"
	"levelup/internal/modules/notifications/internal/ports"
	playercontracts "levelup/internal/modules/player/contracts"
	pointscontracts "levelup/internal/modules/points/contracts"
	progressioncontracts "levelup/internal/modules/progression/contracts"
	rewardscontracts "levelup/internal/modules/rewards/contracts"
	streakscontracts "levelup/internal/modules/streaks/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/mail"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

const (
	tenantA = "0199a000-0000-7000-8000-00000000000a"
	tenantB = "0199a000-0000-7000-8000-00000000000b"
	player1 = "0199a000-0000-7000-8000-000000000001"
	player2 = "0199a000-0000-7000-8000-000000000002"
	player3 = "0199a000-0000-7000-8000-000000000003"
	ghost   = "0199a000-0000-7000-8000-0000000000ff"
	adminID = "0199a000-0000-7000-8000-0000000000ad"
)

var t0 = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

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
	mailer  *flakyMailer
	clk     *clock.Fake
}

func newHarness(t *testing.T, enf authz.Enforcer) *harness {
	t.Helper()
	h := &harness{
		repo: newFakeRepo(),
		ob:   &fakeOutbox{},
		players: &fakePlayers{players: map[string]ports.PlayerSnapshot{
			player1: {ID: player1, TenantID: tenantA, DisplayName: "Nino <b>", Email: "nino@example.com", ExternalID: "ext-1", Active: true},
			player2: {ID: player2, TenantID: tenantA, DisplayName: "Giorgi", Active: true},
			player3: {ID: player3, TenantID: tenantA, DisplayName: "Off", Email: "off@example.com", Active: false},
		}},
		mailer: &flakyMailer{},
		clk:    clock.NewFake(t0),
	}
	h.svc = NewService(h.repo, h.players, h.mailer, h.ob, enf, nil, h.clk, nil, Options{EmailMaxAttempts: 3})
	h.svc.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error { return fn(&gorm.DB{}) }
	return h
}

func asTenant(tenantID string) context.Context {
	return authz.Into(context.Background(), authz.Principal{UserID: adminID, TenantID: tenantID})
}

func (h *harness) template(t *testing.T, trigger string, channels []string, title, body string) domain.Template {
	t.Helper()
	tpl, err := h.svc.CreateTemplate(asTenant(tenantA), domain.NewTemplateParams{
		Name: "tpl " + id.NewID(), Trigger: trigger, Channels: channels, TitleTemplate: title, BodyTemplate: body, Active: true,
	})
	require.NoError(t, err)
	return tpl
}

func (h *harness) enableEmail(t *testing.T) {
	t.Helper()
	on := true
	_, err := h.svc.UpdateChannels(asTenant(tenantA), ChannelsPatch{EmailEnabled: &on})
	require.NoError(t, err)
}

func badgeFact(playerID string) Fact {
	return Fact{TenantID: tenantA, PlayerID: playerID, Data: domain.Data{Badge: domain.BadgeData{ID: id.NewID(), Name: "starter", Tier: "gold"}}}
}

func jobBody(t *testing.T, cmd any) []byte {
	t.Helper()
	payload, err := json.Marshal(cmd)
	require.NoError(t, err)
	body, err := json.Marshal(bus.Envelope{EventID: id.NewID(), Topic: contracts.Topic(contracts.JobSendEmail), Payload: payload})
	require.NoError(t, err)
	return body
}

func (h *harness) sendJobs(t *testing.T) []contracts.SendEmailCmdV1 {
	t.Helper()
	var out []contracts.SendEmailCmdV1
	for _, p := range h.ob.byTopic(contracts.Topic(contracts.JobSendEmail)) {
		out = append(out, p.(contracts.SendEmailCmdV1))
	}
	return out
}

func byChannel(rows []domain.Notification, ch string) []domain.Notification {
	var out []domain.Notification
	for _, n := range rows {
		if n.Channel == ch {
			out = append(out, n)
		}
	}
	return out
}

// ---- templates ----

func TestCreateTemplateStampsTenantAndRejectsBadInput(t *testing.T) {
	h := newHarness(t, allPerms)
	tpl := h.template(t, contracts.TriggerBadgeAwarded, []string{"in_app"}, "Hi {{.Player.DisplayName}}", "")
	require.Equal(t, tenantA, tpl.TenantID)
	require.Equal(t, adminID, tpl.CreatedBy)

	_, err := h.svc.CreateTemplate(asTenant(tenantA), domain.NewTemplateParams{
		Name: tpl.Name, Trigger: contracts.TriggerBadgeAwarded, Channels: []string{"in_app"}, TitleTemplate: "x", Active: true})
	require.Equal(t, contracts.CodeTemplateNameTaken, errs.CodeOf(err))

	_, err = h.svc.CreateTemplate(asTenant(tenantA), domain.NewTemplateParams{
		Name: "bad", Trigger: contracts.TriggerBadgeAwarded, Channels: []string{"in_app"}, TitleTemplate: "{{.Player.Email}}"})
	require.Equal(t, contracts.CodeTemplateInvalid, errs.CodeOf(err))
}

func TestTemplateAuthzAndTenancy(t *testing.T) {
	member := allowKeys{contracts.PermViewAny.Key(): true, contracts.PermView.Key(): true}
	h := newHarness(t, allPerms)
	tpl := h.template(t, contracts.TriggerBadgeAwarded, []string{"in_app"}, "Hi", "")

	hm := newHarness(t, member)
	hm.repo = h.repo
	hm.svc.repo = h.repo
	_, err := hm.svc.CreateTemplate(asTenant(tenantA), domain.NewTemplateParams{Name: "x"})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	name := "y"
	_, err = hm.svc.UpdateTemplate(asTenant(tenantA), tpl.ID, domain.Patch{Name: &name})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	require.Equal(t, errs.PermissionDenied, errs.KindOf(hm.svc.DeleteTemplate(asTenant(tenantA), tpl.ID)))
	on := true
	_, err = hm.svc.UpdateChannels(asTenant(tenantA), ChannelsPatch{EmailEnabled: &on})
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
	_, err = hm.svc.GetTemplate(asTenant(tenantA), tpl.ID)
	require.NoError(t, err)

	_, err = h.svc.GetTemplate(asTenant(tenantB), tpl.ID)
	require.ErrorIs(t, err, domain.ErrTemplateNotFound, "another tenant's template is 404")
	_, err = h.svc.GetTemplate(context.Background(), tpl.ID)
	require.Equal(t, errs.Unauthenticated, errs.KindOf(err), "no principal")
}

func TestUpdateListDeleteTemplate(t *testing.T) {
	h := newHarness(t, allPerms)
	tpl := h.template(t, contracts.TriggerBadgeAwarded, []string{"in_app"}, "Hi", "")
	h.clk.Advance(time.Second)
	h.template(t, contracts.TriggerLevelReached, []string{"in_app"}, "Level {{.Level.Number}}", "")

	off := false
	upd, err := h.svc.UpdateTemplate(asTenant(tenantA), tpl.ID, domain.Patch{Active: &off})
	require.NoError(t, err)
	require.False(t, upd.Active)
	require.Equal(t, 1, upd.Version)

	rows, more, err := h.svc.ListTemplates(asTenant(tenantA), TemplateFilter{Limit: 1})
	require.NoError(t, err)
	require.True(t, more)
	require.Equal(t, contracts.TriggerLevelReached, rows[0].Trigger, "newest first")
	rows, _, err = h.svc.ListTemplates(asTenant(tenantA), TemplateFilter{Active: &off})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	require.NoError(t, h.svc.DeleteTemplate(asTenant(tenantA), tpl.ID))
	_, err = h.svc.GetTemplate(asTenant(tenantA), tpl.ID)
	require.ErrorIs(t, err, domain.ErrTemplateNotFound)
}

func TestPreview(t *testing.T) {
	h := newHarness(t, allPerms)
	tpl := h.template(t, contracts.TriggerBadgeAwarded, []string{"email"}, "Hi {{.Player.DisplayName}}", "You got {{.Badge.Tier}}")
	p, err := h.svc.PreviewTemplate(asTenant(tenantA), tpl.ID, "")
	require.NoError(t, err)
	require.Equal(t, "Hi Alex", p.Rendered.Title)
	require.Equal(t, "You got gold", p.Rendered.Body)

	p, err = h.svc.PreviewTemplate(asTenant(tenantA), tpl.ID, player1)
	require.NoError(t, err)
	require.Equal(t, "Hi Nino <b>", p.Rendered.Title)
	require.Contains(t, p.HTML, "Hi Nino &lt;b&gt;", "email HTML is escaped")

	_, err = h.svc.PreviewTemplate(asTenant(tenantA), tpl.ID, ghost)
	require.ErrorIs(t, err, domain.ErrPlayerNotFound)
}

func TestChannelsDefaultAndUpdate(t *testing.T) {
	h := newHarness(t, allPerms)
	cs, err := h.svc.GetChannels(asTenant(tenantA))
	require.NoError(t, err)
	require.False(t, cs.EmailEnabled, "email is opt-in")
	name := "Acme Rewards"
	on := true
	cs, err = h.svc.UpdateChannels(asTenant(tenantA), ChannelsPatch{EmailEnabled: &on, EmailFromName: &name})
	require.NoError(t, err)
	require.True(t, cs.EmailEnabled)
	off := false
	cs, err = h.svc.UpdateChannels(asTenant(tenantA), ChannelsPatch{EmailEnabled: &off})
	require.NoError(t, err)
	require.False(t, cs.EmailEnabled)
	require.Equal(t, "Acme Rewards", cs.EmailFromName, "omitted fields stay")
}

// ---- fan-out ----

func TestFanOutIsIdempotentUnderRedelivery(t *testing.T) {
	h := newHarness(t, allPerms)
	h.enableEmail(t)
	h.template(t, contracts.TriggerBadgeAwarded, []string{"in_app", "email"}, "Congrats {{.Player.DisplayName}}", "You earned {{.Badge.Name}}")

	fact := badgeFact(player1)
	eventID := id.NewID()
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, eventID, fact))
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, eventID, fact), "redelivery")

	rows := h.repo.all()
	require.Len(t, rows, 2, "one row per channel, not per delivery")
	inApp := byChannel(rows, contracts.ChannelInApp)[0]
	require.Equal(t, contracts.StatusDelivered, inApp.Status)
	require.NotNil(t, inApp.DeliveredAt)
	require.Equal(t, "Congrats Nino <b>", inApp.Title)
	require.Equal(t, "You earned starter", inApp.Body)
	email := byChannel(rows, contracts.ChannelEmail)[0]
	require.Equal(t, contracts.StatusPending, email.Status)
	require.Equal(t, []contracts.SendEmailCmdV1{{TenantID: tenantA, NotificationID: email.ID}}, h.sendJobs(t), "one job, published once")

	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), fact))
	require.Len(t, h.repo.all(), 4, "a new fact is new notifications")
	require.Len(t, h.sendJobs(t), 2)
}

func TestFanOutSkipsAndFailures(t *testing.T) {
	h := newHarness(t, allPerms)
	h.template(t, contracts.TriggerBadgeAwarded, []string{"in_app", "email"}, "Hi", "")

	// Email disabled (default): skipped, no job.
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player1)))
	email := byChannel(h.repo.all(), contracts.ChannelEmail)[0]
	require.Equal(t, contracts.StatusSkipped, email.Status)
	require.Equal(t, contracts.ReasonEmailDisabled, email.Reason)
	require.Empty(t, h.sendJobs(t))

	// Player without email: skipped no_email.
	h.enableEmail(t)
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player2)))
	var noEmail []domain.Notification
	for _, n := range byChannel(h.repo.all(), contracts.ChannelEmail) {
		if n.PlayerID == player2 {
			noEmail = append(noEmail, n)
		}
	}
	require.Len(t, noEmail, 1)
	require.Equal(t, contracts.ReasonNoEmail, noEmail[0].Reason)
	require.Empty(t, h.sendJobs(t))

	// Inactive and unknown players get nothing.
	before := len(h.repo.all())
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player3)))
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(ghost)))
	require.Len(t, h.repo.all(), before)

	// No template for the trigger: nothing, and the player is not even read.
	h.players.err = errors.New("player module down")
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerStreakBroken, id.NewID(), badgeFact(player1)))
	// With templates, a transient player error is returned for the retry ladder.
	err := h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player1))
	require.Error(t, err)
	h.players.err = nil

	// Malformed fact → Invalid (DLQ).
	err = h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, "", Fact{TenantID: "nope", PlayerID: player1})
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}

func TestFanOutRecordsRenderErrorsAsFailed(t *testing.T) {
	h := newHarness(t, allPerms)
	h.enableEmail(t)
	// Bypass validation: a template that cannot execute (stored before a
	// field was removed, say).
	bad := domain.Template{ID: id.NewID(), TenantID: tenantA, Name: "broken", Trigger: contracts.TriggerBadgeAwarded,
		Channels: []string{"in_app", "email"}, TitleTemplate: "{{.Gone}}", Active: true, CreatedAt: t0, UpdatedAt: t0}
	require.NoError(t, h.repo.CreateTemplate(context.Background(), nil, bad))
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player1)))
	rows := h.repo.all()
	require.Len(t, rows, 2)
	for _, n := range rows {
		require.Equal(t, contracts.StatusFailed, n.Status)
		require.Equal(t, contracts.ReasonRenderError, n.Reason)
	}
	require.Empty(t, h.sendJobs(t))
}

func TestEveryTriggerTopicDecodesIntoTemplateData(t *testing.T) {
	cases := map[string]struct {
		payload any
		title   string
		want    string
	}{
		contracts.TriggerBadgeAwarded: {badgescontracts.AwardedV1{TenantID: tenantA, PlayerID: player1, BadgeID: id.NewID(),
			BadgeSlug: "explorer", Tier: "gold", EarnedCount: 2, PointsValue: 50}, "{{.Badge.Name}} x{{.Badge.EarnedCount}} +{{.Badge.PointsValue}}", "explorer x2 +50"},
		contracts.TriggerLevelReached: {progressioncontracts.LevelReachedV1{TenantID: tenantA, PlayerID: player1, LevelNumber: 7,
			LevelName: "Pro"}, "Level {{.Level.Number}} {{.Level.Name}}", "Level 7 Pro"},
		contracts.TriggerMissionCompleted: {missionscontracts.CompletedV1{TenantID: tenantA, PlayerID: player1, MissionSlug: "daily",
			XPReward: 30}, "{{.Mission.Name}} {{.Mission.XPReward}}", "daily 30"},
		contracts.TriggerStreakMilestone: {streakscontracts.MilestoneReachedV1{TenantID: tenantA, PlayerID: player1, Milestone: 14,
			BonusPoints: 140}, "{{.Streak.Milestone}} days +{{.Streak.BonusPoints}}", "14 days +140"},
		contracts.TriggerStreakBroken: {streakscontracts.BrokenV1{TenantID: tenantA, PlayerID: player1, FinalCount: 9},
			"lost {{.Streak.FinalCount}}", "lost 9"},
		contracts.TriggerRewardClaimed: {rewardscontracts.ClaimV1{TenantID: tenantA, PlayerID: player1, RewardSlug: "coffee",
			Code: "C-1", PointsCost: 500}, "{{.Reward.Name}} {{.Reward.Code}} -{{.Reward.PointsCost}}", "coffee C-1 -500"},
		contracts.TriggerPointsCredited: {pointscontracts.LedgerMovedV1{TenantID: tenantA, PlayerID: player1, Amount: 25,
			BalanceAfter: 125}, "+{{.Points.Amount}} = {{.Points.Balance}} ({{.Trigger}})", "+25 = 125 (points.credited)"},
	}
	topics := TriggerTopics()
	require.Len(t, topics, len(contracts.AllTriggers))
	for _, tt := range topics {
		t.Run(tt.Trigger, func(t *testing.T) {
			tc, ok := cases[tt.Trigger]
			require.True(t, ok)
			h := newHarness(t, allPerms)
			h.template(t, tt.Trigger, []string{"in_app"}, tc.title+" {{.Player.DisplayName}}", "")
			raw, err := json.Marshal(tc.payload)
			require.NoError(t, err)
			require.NoError(t, h.svc.Handler(tt)(context.Background(), bus.Envelope{EventID: id.NewID(), Topic: tt.Topic, Payload: raw}))
			rows := h.repo.all()
			require.Len(t, rows, 1)
			require.Equal(t, tc.want+" Nino <b>", rows[0].Title)
			require.Equal(t, tt.Trigger, rows[0].Trigger)

			err = h.svc.Handler(tt)(context.Background(), bus.Envelope{EventID: id.NewID(), Topic: tt.Topic, Payload: []byte("{")})
			require.Equal(t, errs.Invalid, errs.KindOf(err))
		})
	}
}

// ---- email job ----

func (h *harness) pendingEmail(t *testing.T) contracts.SendEmailCmdV1 {
	t.Helper()
	h.enableEmail(t)
	h.template(t, contracts.TriggerBadgeAwarded, []string{"email"}, "Congrats {{.Player.DisplayName}}", "You earned {{.Badge.Name}}\n\nSee you")
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player1)))
	jobs := h.sendJobs(t)
	require.Len(t, jobs, 1)
	return jobs[0]
}

func (h *harness) row(t *testing.T, notificationID string) domain.Notification {
	t.Helper()
	n, ok, err := h.repo.NotificationByID(context.Background(), tenantA, notificationID)
	require.NoError(t, err)
	require.True(t, ok)
	return n
}

func TestSendEmailDeliversOnceWithRecorder(t *testing.T) {
	h := newHarness(t, allPerms)
	cmd := h.pendingEmail(t)

	require.NoError(t, h.svc.HandleSendEmail(context.Background(), jobBody(t, cmd)))
	require.Len(t, h.mailer.Sent, 1)
	msg := h.mailer.Sent[0]
	require.Equal(t, "nino@example.com", msg.To)
	require.Equal(t, "Congrats Nino <b>", msg.Subject)
	require.Equal(t, "You earned starter\n\nSee you", msg.Text)
	require.Contains(t, msg.HTML, "Congrats Nino &lt;b&gt;")
	require.NotContains(t, msg.HTML, "<b>")

	n := h.row(t, cmd.NotificationID)
	require.Equal(t, contracts.StatusDelivered, n.Status)
	require.Equal(t, 1, n.Attempts)
	require.Nil(t, n.LeaseUntil)

	require.NoError(t, h.svc.HandleSendEmail(context.Background(), jobBody(t, cmd)), "redelivery")
	require.Len(t, h.mailer.Sent, 1, "no second email")
}

func TestSendEmailTransientErrorsClimbTheLadderThenFail(t *testing.T) {
	h := newHarness(t, allPerms)
	cmd := h.pendingEmail(t)
	h.mailer.failures, h.mailer.err = 1, errs.New(errs.Unavailable, "smtp down")

	err := h.svc.HandleSendEmail(context.Background(), jobBody(t, cmd))
	require.Equal(t, errs.Unavailable, errs.KindOf(err), "transient: retry")
	n := h.row(t, cmd.NotificationID)
	require.Equal(t, contracts.StatusPending, n.Status)
	require.Equal(t, 1, n.Attempts)
	require.Contains(t, n.LastError, "smtp down")
	require.Nil(t, n.LeaseUntil, "lease released for the retry")

	require.NoError(t, h.svc.HandleSendEmail(context.Background(), jobBody(t, cmd)))
	require.Equal(t, contracts.StatusDelivered, h.row(t, cmd.NotificationID).Status)

	// Retries exhausted (EmailMaxAttempts = 3 in the harness).
	h2 := newHarness(t, allPerms)
	cmd2 := h2.pendingEmail(t)
	h2.mailer.failures, h2.mailer.err = 100, errors.New("connection reset")
	for range 2 {
		require.Error(t, h2.svc.HandleSendEmail(context.Background(), jobBody(t, cmd2)))
	}
	require.NoError(t, h2.svc.HandleSendEmail(context.Background(), jobBody(t, cmd2)), "final attempt settles and acks")
	n = h2.row(t, cmd2.NotificationID)
	require.Equal(t, contracts.StatusFailed, n.Status)
	require.Equal(t, contracts.ReasonRetriesExhaust, n.Reason)
	require.Equal(t, 3, n.Attempts)
	require.NoError(t, h2.svc.HandleSendEmail(context.Background(), jobBody(t, cmd2)))
	require.Equal(t, 3, h2.mailer.calls, "a settled row is never sent again")
}

func TestSendEmailPermanentErrorFails(t *testing.T) {
	h := newHarness(t, allPerms)
	cmd := h.pendingEmail(t)
	h.mailer.failures, h.mailer.err = 1, errs.New(errs.Invalid, "recipient rejected")
	require.NoError(t, h.svc.HandleSendEmail(context.Background(), jobBody(t, cmd)))
	n := h.row(t, cmd.NotificationID)
	require.Equal(t, contracts.StatusFailed, n.Status)
	require.Equal(t, contracts.ReasonMailRejected, n.Reason)
}

func TestSendEmailSkipsAndGuards(t *testing.T) {
	h := newHarness(t, allPerms)
	cmd := h.pendingEmail(t)

	// The player removed their email after fan-out.
	p := h.players.players[player1]
	p.Email = ""
	h.players.players[player1] = p
	require.NoError(t, h.svc.HandleSendEmail(context.Background(), jobBody(t, cmd)))
	require.Equal(t, contracts.ReasonNoEmail, h.row(t, cmd.NotificationID).Reason)
	require.Empty(t, h.mailer.Sent)

	// Email disabled after fan-out.
	h2 := newHarness(t, allPerms)
	cmd2 := h2.pendingEmail(t)
	off := false
	_, err := h2.svc.UpdateChannels(asTenant(tenantA), ChannelsPatch{EmailEnabled: &off})
	require.NoError(t, err)
	require.NoError(t, h2.svc.HandleSendEmail(context.Background(), jobBody(t, cmd2)))
	require.Equal(t, contracts.StatusSkipped, h2.row(t, cmd2.NotificationID).Status)

	// A row leased by another worker: retry later, do not send twice.
	h3 := newHarness(t, allPerms)
	cmd3 := h3.pendingEmail(t)
	_, claimed, err := h3.repo.ClaimEmail(context.Background(), nil, tenantA, cmd3.NotificationID, t0, t0.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, claimed)
	err = h3.svc.HandleSendEmail(context.Background(), jobBody(t, cmd3))
	require.Equal(t, errs.Unavailable, errs.KindOf(err))
	require.Empty(t, h3.mailer.Sent)

	// Malformed and unknown commands.
	err = h.svc.HandleSendEmail(context.Background(), []byte("nope"))
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	err = h.svc.HandleSendEmail(context.Background(), jobBody(t, contracts.SendEmailCmdV1{TenantID: tenantA, NotificationID: "x"}))
	require.Equal(t, errs.Invalid, errs.KindOf(err))
	require.NoError(t, h.svc.HandleSendEmail(context.Background(), jobBody(t, contracts.SendEmailCmdV1{TenantID: tenantA, NotificationID: ghost})),
		"a purged row acks")
	require.NoError(t, h.svc.HandleSendEmail(context.Background(), jobBody(t, contracts.SendEmailCmdV1{TenantID: tenantB, NotificationID: cmd.NotificationID})),
		"another tenant's row is invisible")
}

func TestSweepStaleEmailFailsOldPendingRows(t *testing.T) {
	h := newHarness(t, allPerms)
	cmd := h.pendingEmail(t)
	require.NoError(t, h.svc.SweepStaleEmail(context.Background()))
	require.Equal(t, contracts.StatusPending, h.row(t, cmd.NotificationID).Status, "young rows stay")
	h.clk.Advance(3 * time.Hour)
	require.NoError(t, h.svc.SweepStaleEmail(context.Background()))
	n := h.row(t, cmd.NotificationID)
	require.Equal(t, contracts.StatusFailed, n.Status)
	require.Equal(t, contracts.ReasonStale, n.Reason)
	require.NoError(t, h.svc.SweepStaleEmail(context.Background()), "idempotent")
}

// ---- feed, history, stats ----

func TestFeedReadState(t *testing.T) {
	h := newHarness(t, allPerms)
	h.enableEmail(t)
	h.template(t, contracts.TriggerBadgeAwarded, []string{"in_app", "email"}, "Badge {{.Badge.Name}}", "")
	for range 3 {
		h.clk.Advance(time.Second)
		require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player1)))
	}
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player2)))
	ctx := asTenant(tenantA)

	page, err := h.svc.PlayerFeed(ctx, player1, FeedFilter{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page.Items, 2, "in_app only, paged")
	require.True(t, page.More)
	require.Equal(t, int64(3), page.UnreadCount)
	for _, n := range page.Items {
		require.Equal(t, contracts.ChannelInApp, n.Channel)
	}

	h.clk.Advance(time.Minute)
	first, err := h.svc.MarkRead(ctx, player1, page.Items[0].ID)
	require.NoError(t, err)
	require.NotNil(t, first.ReadAt)
	readAt := *first.ReadAt
	h.clk.Advance(time.Minute)
	again, err := h.svc.MarkRead(ctx, player1, page.Items[0].ID)
	require.NoError(t, err)
	require.Equal(t, readAt, *again.ReadAt, "read_at keeps the first read")

	unread, err := h.svc.PlayerFeed(ctx, player1, FeedFilter{UnreadOnly: true})
	require.NoError(t, err)
	require.Len(t, unread.Items, 2)
	require.Equal(t, int64(2), unread.UnreadCount)

	n, err := h.svc.MarkAllRead(ctx, player1)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
	n, err = h.svc.MarkAllRead(ctx, player1)
	require.NoError(t, err)
	require.Zero(t, n)
	other, err := h.svc.PlayerFeed(ctx, player2, FeedFilter{UnreadOnly: true})
	require.NoError(t, err)
	require.Equal(t, int64(1), other.UnreadCount, "another player's feed is untouched")

	// Guards: other player's row, an email row, unknown player, another tenant.
	_, err = h.svc.MarkRead(ctx, player2, page.Items[0].ID)
	require.ErrorIs(t, err, domain.ErrNotificationNotFound)
	email := byChannel(h.repo.all(), contracts.ChannelEmail)[0]
	_, err = h.svc.MarkRead(ctx, email.PlayerID, email.ID)
	require.ErrorIs(t, err, domain.ErrNotificationNotFound)
	_, err = h.svc.PlayerFeed(ctx, ghost, FeedFilter{})
	require.ErrorIs(t, err, domain.ErrPlayerNotFound)
	_, err = h.svc.PlayerFeed(asTenant(tenantB), player1, FeedFilter{})
	require.ErrorIs(t, err, domain.ErrPlayerNotFound)

	viewOnly := newHarness(t, allowKeys{contracts.PermFeedView.Key(): true})
	_, err = viewOnly.svc.MarkAllRead(ctx, player1)
	require.Equal(t, errs.PermissionDenied, errs.KindOf(err))
}

func TestHistoryAndStats(t *testing.T) {
	h := newHarness(t, allPerms)
	h.enableEmail(t)
	tpl := h.template(t, contracts.TriggerBadgeAwarded, []string{"in_app", "email"}, "Badge", "")
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player1)))
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player2)))
	ctx := asTenant(tenantA)

	page, err := h.svc.History(ctx, HistoryFilter{Channel: contracts.ChannelEmail})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.Equal(t, tpl.Name, page.TemplateNames[tpl.ID])
	page, err = h.svc.History(ctx, HistoryFilter{Status: contracts.StatusSkipped, PlayerID: player2})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	page, err = h.svc.History(asTenant(tenantB), HistoryFilter{})
	require.NoError(t, err)
	require.Empty(t, page.Items, "tenant-scoped")

	feed, err := h.svc.PlayerFeed(ctx, player1, FeedFilter{})
	require.NoError(t, err)
	_, err = h.svc.MarkRead(ctx, player1, feed.Items[0].ID)
	require.NoError(t, err)

	s, err := h.svc.Stats(ctx, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Equal(t, int64(3), s.Sent, "2 in_app delivered + 1 email pending; the skipped email is not sent")
	require.Equal(t, int64(2), s.Delivered)
	require.Equal(t, int64(1), s.Pending)
	require.Equal(t, int64(1), s.Skipped)
	require.InDelta(t, 0.5, s.OpenRate, 1e-9)
	require.Equal(t, tpl.Name, s.TemplateNames[tpl.ID])
	require.Equal(t, int64(3), s.ByTemplate[tpl.ID].Sent)

	s, err = h.svc.Stats(ctx, t0.Add(time.Hour), time.Time{})
	require.NoError(t, err)
	require.Zero(t, s.Sent)
}

// ---- purge ----

func TestTenantAndPlayerPurgeAreIdempotent(t *testing.T) {
	h := newHarness(t, allPerms)
	h.template(t, contracts.TriggerBadgeAwarded, []string{"in_app"}, "Badge", "")
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player1)))
	require.NoError(t, h.svc.FanOut(context.Background(), contracts.TriggerBadgeAwarded, id.NewID(), badgeFact(player2)))

	raw, _ := json.Marshal(playercontracts.PlayerDeletedV1{TenantID: tenantA, PlayerID: player1})
	for range 2 {
		require.NoError(t, h.svc.OnPlayerDeleted(context.Background(), bus.Envelope{Payload: raw}))
	}
	require.Len(t, h.repo.all(), 1)

	raw, _ = json.Marshal(identitycontracts.TenantDeletedV1{TenantID: tenantA})
	for range 2 {
		require.NoError(t, h.svc.OnTenantDeleted(context.Background(), bus.Envelope{Payload: raw}))
	}
	require.Empty(t, h.repo.all())
	require.Empty(t, h.repo.templates)

	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.OnTenantDeleted(context.Background(), bus.Envelope{Payload: []byte(`{"tenant_id":"x"}`)})))
	require.Equal(t, errs.Invalid, errs.KindOf(h.svc.OnPlayerDeleted(context.Background(), bus.Envelope{Payload: []byte(`{`)})))
}

var _ mail.Mailer = (*flakyMailer)(nil)
