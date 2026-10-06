package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/shared/errs"
)

var t0 = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func validParams() NewTemplateParams {
	return NewTemplateParams{
		TenantID:      "t1",
		Name:          "Badge earned",
		Trigger:       contracts.TriggerBadgeAwarded,
		Channels:      []string{"in_app", "email"},
		TitleTemplate: "Congrats {{.Player.DisplayName}}!",
		BodyTemplate:  "You earned {{.Badge.Name}} ({{.Badge.Tier}}).",
		Active:        true,
	}
}

func TestNewTemplateValidation(t *testing.T) {
	cases := map[string]struct {
		mut   func(*NewTemplateParams)
		field string
	}{
		"missing name":       {func(p *NewTemplateParams) { p.Name = "  " }, "name"},
		"unknown trigger":    {func(p *NewTemplateParams) { p.Trigger = "points.debited" }, "trigger"},
		"no channels":        {func(p *NewTemplateParams) { p.Channels = nil }, "channels"},
		"bad channel":        {func(p *NewTemplateParams) { p.Channels = []string{"sms"} }, "channels"},
		"missing title":      {func(p *NewTemplateParams) { p.TitleTemplate = "" }, "title_template"},
		"parse error":        {func(p *NewTemplateParams) { p.BodyTemplate = "{{.Player.DisplayName" }, "body_template"},
		"unknown field":      {func(p *NewTemplateParams) { p.BodyTemplate = "{{.Player.Email}}" }, "body_template"},
		"unknown section":    {func(p *NewTemplateParams) { p.TitleTemplate = "{{.Wallet.Balance}}" }, "title_template"},
		"range refused":      {func(p *NewTemplateParams) { p.BodyTemplate = "{{range 1000000000}}x{{end}}" }, "body_template"},
		"define refused":     {func(p *NewTemplateParams) { p.BodyTemplate = `{{define "x"}}{{template "x"}}{{end}}` }, "body_template"},
		"template refused":   {func(p *NewTemplateParams) { p.BodyTemplate = `{{template "n"}}` }, "body_template"},
		"printf refused":     {func(p *NewTemplateParams) { p.BodyTemplate = `{{printf "%099999999d" 1}}` }, "body_template"},
		"call in if refused": {func(p *NewTemplateParams) { p.BodyTemplate = `{{if call .Player.DisplayName}}y{{end}}` }, "body_template"},
		"title too long":     {func(p *NewTemplateParams) { p.TitleTemplate = strings.Repeat("a", MaxTitleSource+1) }, "title_template"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := validParams()
			tc.mut(&p)
			_, err := NewTemplate(p, t0, time.Second)
			require.Error(t, err)
			require.Equal(t, errs.Invalid, errs.KindOf(err))
			require.Equal(t, contracts.CodeTemplateInvalid, errs.CodeOf(err))
			require.Contains(t, errs.FieldsOf(err), tc.field)
		})
	}
}

func TestNewTemplateNormalizesChannelsAndAllowsSafeBuiltins(t *testing.T) {
	p := validParams()
	p.Channels = []string{" In_App ", "in_app", "email"}
	p.BodyTemplate = `{{if gt .Badge.PointsValue 0}}+{{.Badge.PointsValue}} pts{{else}}no points{{end}} {{len .Badge.Slug}} {{print .Level.Number}}{{with .Reward.Code}} code {{.}}{{end}}`
	tpl, err := NewTemplate(p, t0, time.Second)
	require.NoError(t, err)
	require.Equal(t, []string{"in_app", "email"}, tpl.Channels)
	require.True(t, tpl.HasChannel("email"))
	require.Equal(t, t0, tpl.CreatedAt)
}

func TestApplyRevalidatesAndKeepsOldStateOnError(t *testing.T) {
	tpl, err := NewTemplate(validParams(), t0, time.Second)
	require.NoError(t, err)
	bad := "{{.Nope}}"
	err = tpl.Apply(Patch{TitleTemplate: &bad}, t0.Add(time.Minute), time.Second)
	require.Error(t, err)
	require.Equal(t, "Congrats {{.Player.DisplayName}}!", tpl.TitleTemplate, "a rejected patch changes nothing")
	require.Equal(t, t0, tpl.UpdatedAt)

	name, off := "Renamed", false
	require.NoError(t, tpl.Apply(Patch{Name: &name, Active: &off}, t0.Add(time.Minute), time.Second))
	require.Equal(t, "Renamed", tpl.Name)
	require.False(t, tpl.Active)
	require.Equal(t, t0.Add(time.Minute), tpl.UpdatedAt)
}

func TestRenderUsesDataAndZeroValuesForOtherSections(t *testing.T) {
	tpl, err := NewTemplate(validParams(), t0, time.Second)
	require.NoError(t, err)
	tpl.TitleTemplate = "Hi {{.Player.DisplayName}}\n  you   leveled {{.Level.Number}}"
	r, err := Render(tpl, Data{Player: PlayerData{DisplayName: "Nino"}, Badge: BadgeData{Name: "starter", Tier: "gold"}}, time.Second)
	require.NoError(t, err)
	require.Equal(t, "Hi Nino you leveled 0", r.Title, "title collapsed to one line; unset section renders zero")
	require.Equal(t, "You earned starter (gold).", r.Body)
}

func TestRenderOutputCap(t *testing.T) {
	c, err := Compile(strings.Repeat("{{.Player.DisplayName}}", 10))
	require.NoError(t, err)
	_, err = c.Execute(Data{Player: PlayerData{DisplayName: strings.Repeat("x", 100)}}, time.Second, 500)
	require.ErrorIs(t, err, ErrRenderTooLarge)
}

func TestRenderTimeout(t *testing.T) {
	// A deadline that has already passed: the select must not wait forever.
	c, err := Compile(strings.Repeat("{{.Player.DisplayName}}", 2000))
	require.NoError(t, err)
	_, err = c.Execute(Data{Player: PlayerData{DisplayName: strings.Repeat("y", 30)}}, time.Nanosecond, MaxRenderOutput*4)
	if err != nil {
		require.ErrorIs(t, err, ErrRenderTimeout)
	}
}

func TestEmailHTMLEscapes(t *testing.T) {
	out := EmailHTML(Rendered{Title: `<script>alert(1)</script>`, Body: "Hi <b>Ana</b> & co\nline two\n\nnew para"})
	require.NotContains(t, out, "<script>")
	require.Contains(t, out, "&lt;script&gt;")
	require.Contains(t, out, "Hi &lt;b&gt;Ana&lt;/b&gt; &amp; co<br>line two")
	require.Contains(t, out, "<p>new para</p>")
}

func TestNotificationTransitions(t *testing.T) {
	lease := t0.Add(time.Minute)
	n := Notification{Status: contracts.StatusPending, LeaseUntil: &lease}
	require.True(t, n.Pending())
	n.Retry(strings.Repeat("e", 2000), t0)
	require.True(t, n.Pending())
	require.Nil(t, n.LeaseUntil)
	require.Len(t, n.LastError, 1000)
	n.Deliver(t0)
	require.Equal(t, contracts.StatusDelivered, n.Status)
	require.Empty(t, n.LastError)
	require.NotNil(t, n.DeliveredAt)

	s := Notification{}
	s.Skip(contracts.ReasonNoEmail, t0)
	require.Equal(t, contracts.StatusSkipped, s.Status)
	f := Notification{}
	f.Fail(contracts.ReasonMailRejected, "550", t0)
	require.Equal(t, contracts.StatusFailed, f.Status)
	require.Equal(t, "550", f.LastError)
}

func TestAggregate(t *testing.T) {
	s := Aggregate([]StatsRow{
		{TemplateID: "a", Channel: "in_app", Status: "delivered", Count: 4, Read: 1},
		{TemplateID: "a", Channel: "email", Status: "delivered", Count: 2},
		{TemplateID: "a", Channel: "email", Status: "skipped", Count: 3},
		{TemplateID: "b", Channel: "email", Status: "failed", Count: 1},
		{TemplateID: "b", Channel: "email", Status: "pending", Count: 1},
	})
	require.Equal(t, int64(8), s.Sent, "skipped rows are not sent")
	require.Equal(t, int64(6), s.Delivered)
	require.Equal(t, int64(1), s.Failed)
	require.Equal(t, int64(1), s.Pending)
	require.Equal(t, int64(3), s.Skipped)
	require.Equal(t, int64(4), s.ByChannel["email"].Sent)
	require.Equal(t, int64(6), s.ByTemplate["a"].Sent)
	require.InDelta(t, 0.25, s.OpenRate, 1e-9)
	require.Zero(t, Aggregate(nil).OpenRate)
}
