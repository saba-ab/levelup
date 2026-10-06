package transport

import (
	"time"

	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/app"
	"levelup/internal/modules/notifications/internal/domain"
)

// CreateTemplateReq is POST /notifications/templates. is_active defaults to
// true. title_template/body_template are Go text/template sources over the
// documented field set ({{.Player.DisplayName}}, {{.Badge.Name}}, ...).
type CreateTemplateReq struct {
	Name          string   `json:"name"           validate:"required,max=255"`
	Trigger       string   `json:"trigger"        validate:"required,oneof=badges.awarded progression.level_reached missions.completed streaks.milestone_reached streaks.broken rewards.claimed points.credited"`
	Channels      []string `json:"channels"       validate:"required,min=1,max=2,dive,oneof=in_app email"`
	TitleTemplate string   `json:"title_template" validate:"required,max=500"`
	BodyTemplate  string   `json:"body_template"  validate:"max=10000"`
	IsActive      *bool    `json:"is_active"`
}

func (r CreateTemplateReq) params() domain.NewTemplateParams {
	active := true
	if r.IsActive != nil {
		active = *r.IsActive
	}
	return domain.NewTemplateParams{
		Name:          r.Name,
		Trigger:       r.Trigger,
		Channels:      r.Channels,
		TitleTemplate: r.TitleTemplate,
		BodyTemplate:  r.BodyTemplate,
		Active:        active,
	}
}

// UpdateTemplateReq is PATCH /notifications/templates/{id}: omitted fields
// stay untouched.
type UpdateTemplateReq struct {
	Name          *string   `json:"name"           validate:"omitempty,max=255"`
	Trigger       *string   `json:"trigger"        validate:"omitempty,oneof=badges.awarded progression.level_reached missions.completed streaks.milestone_reached streaks.broken rewards.claimed points.credited"`
	Channels      *[]string `json:"channels"       validate:"omitempty,min=1,max=2,dive,oneof=in_app email"`
	TitleTemplate *string   `json:"title_template" validate:"omitempty,max=500"`
	BodyTemplate  *string   `json:"body_template"  validate:"omitempty,max=10000"`
	IsActive      *bool     `json:"is_active"`
}

func (r UpdateTemplateReq) patch() domain.Patch {
	return domain.Patch{
		Name:          r.Name,
		Trigger:       r.Trigger,
		Channels:      r.Channels,
		TitleTemplate: r.TitleTemplate,
		BodyTemplate:  r.BodyTemplate,
		Active:        r.IsActive,
	}
}

type TemplateResp struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Trigger       string    `json:"trigger"`
	Channels      []string  `json:"channels"`
	TitleTemplate string    `json:"title_template"`
	BodyTemplate  string    `json:"body_template"`
	IsActive      bool      `json:"is_active"`
	CreatedBy     string    `json:"created_by,omitempty"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func toTemplateResp(t domain.Template) TemplateResp {
	ch := t.Channels
	if ch == nil {
		ch = []string{}
	}
	return TemplateResp{
		ID: t.ID, Name: t.Name, Trigger: t.Trigger, Channels: ch, TitleTemplate: t.TitleTemplate,
		BodyTemplate: t.BodyTemplate, IsActive: t.Active, CreatedBy: t.CreatedBy, Version: t.Version,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

type TemplateListResp struct {
	Data       []TemplateResp `json:"data"`
	NextCursor string         `json:"next_cursor"`
}

// PreviewReq is POST /notifications/templates/{id}/preview.
type PreviewReq struct {
	PlayerID string `json:"player_id" validate:"omitempty,uuid"`
}

type PreviewResp struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	HTML  string `json:"html"`
}

// ChannelsResp is GET/PATCH /notifications/channels.
type ChannelsResp struct {
	InApp     InAppChannelResp `json:"in_app"`
	Email     EmailChannelResp `json:"email"`
	UpdatedAt *time.Time       `json:"updated_at"`
}

type InAppChannelResp struct {
	Enabled bool `json:"enabled"` // always true
}

type EmailChannelResp struct {
	Enabled  bool   `json:"enabled"`
	FromName string `json:"from_name"`
}

func toChannelsResp(cs domain.ChannelSettings) ChannelsResp {
	out := ChannelsResp{InApp: InAppChannelResp{Enabled: true},
		Email: EmailChannelResp{Enabled: cs.EmailEnabled, FromName: cs.EmailFromName}}
	if !cs.UpdatedAt.IsZero() {
		at := cs.UpdatedAt
		out.UpdatedAt = &at
	}
	return out
}

// UpdateChannelsReq is PATCH /notifications/channels; in_app cannot be
// disabled.
type UpdateChannelsReq struct {
	Email *EmailChannelPatch `json:"email" validate:"required"`
}

type EmailChannelPatch struct {
	Enabled  *bool   `json:"enabled"`
	FromName *string `json:"from_name" validate:"omitempty,max=100,excludesall=<>\"\r\n"`
}

func (r UpdateChannelsReq) patch() app.ChannelsPatch {
	return app.ChannelsPatch{EmailEnabled: r.Email.Enabled, EmailFromName: r.Email.FromName}
}

// NotificationResp is a history row.
type NotificationResp struct {
	ID           string     `json:"id"`
	TemplateID   string     `json:"template_id"`
	TemplateName string     `json:"template_name"`
	PlayerID     string     `json:"player_id"`
	EventID      string     `json:"event_id"`
	Trigger      string     `json:"trigger"`
	Channel      string     `json:"channel"`
	Status       string     `json:"status"`
	Reason       string     `json:"reason,omitempty"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	Attempts     int        `json:"attempts"`
	LastError    string     `json:"last_error,omitempty"`
	DeliveredAt  *time.Time `json:"delivered_at"`
	ReadAt       *time.Time `json:"read_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func toNotificationResp(n domain.Notification, templateName string) NotificationResp {
	return NotificationResp{
		ID: n.ID, TemplateID: n.TemplateID, TemplateName: templateName, PlayerID: n.PlayerID, EventID: n.EventID,
		Trigger: n.Trigger, Channel: n.Channel, Status: n.Status, Reason: n.Reason, Title: n.Title, Body: n.Body,
		Attempts: n.Attempts, LastError: n.LastError, DeliveredAt: n.DeliveredAt, ReadAt: n.ReadAt,
		CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
	}
}

type HistoryListResp struct {
	Data       []NotificationResp `json:"data"`
	NextCursor string             `json:"next_cursor"`
}

// FeedItemResp is one in-app notification in a player's feed.
type FeedItemResp struct {
	ID         string     `json:"id"`
	TemplateID string     `json:"template_id"`
	Trigger    string     `json:"trigger"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Read       bool       `json:"read"`
	ReadAt     *time.Time `json:"read_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func toFeedItemResp(n domain.Notification) FeedItemResp {
	return FeedItemResp{ID: n.ID, TemplateID: n.TemplateID, Trigger: n.Trigger, Title: n.Title, Body: n.Body,
		Read: n.Read(), ReadAt: n.ReadAt, CreatedAt: n.CreatedAt}
}

type FeedResp struct {
	Data        []FeedItemResp `json:"data"`
	NextCursor  string         `json:"next_cursor"`
	UnreadCount int64          `json:"unread_count"`
}

type ReadAllResp struct {
	Updated int64 `json:"updated"`
}

// CountsResp is one stats bucket.
type CountsResp struct {
	Sent      int64 `json:"sent"`
	Delivered int64 `json:"delivered"`
	Failed    int64 `json:"failed"`
	Pending   int64 `json:"pending"`
	Skipped   int64 `json:"skipped"`
	Read      int64 `json:"read"`
}

func toCountsResp(c domain.Counts) CountsResp {
	return CountsResp{Sent: c.Sent, Delivered: c.Delivered, Failed: c.Failed, Pending: c.Pending, Skipped: c.Skipped, Read: c.Read}
}

type TemplateCountsResp struct {
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
	Trigger    string `json:"trigger"`
	CountsResp
}

// StatsResp is GET /notifications/stats. sent = every non-skipped
// notification; open_rate = read / delivered over in_app (0..1).
type StatsResp struct {
	CountsResp
	ByChannel  map[string]CountsResp `json:"by_channel"`
	ByTemplate []TemplateCountsResp  `json:"by_template"`
	OpenRate   float64               `json:"open_rate"`
}

func toStatsResp(s app.StatsResult) StatsResp {
	out := StatsResp{
		CountsResp: toCountsResp(s.Counts),
		ByChannel:  map[string]CountsResp{},
		ByTemplate: []TemplateCountsResp{},
		OpenRate:   s.OpenRate,
	}
	for _, ch := range []string{contracts.ChannelInApp, contracts.ChannelEmail} {
		out.ByChannel[ch] = toCountsResp(s.ByChannel[ch])
	}
	for tid, c := range s.ByTemplate {
		out.ByTemplate = append(out.ByTemplate, TemplateCountsResp{
			TemplateID: tid, Name: s.TemplateNames[tid], Trigger: s.TemplateTriggers[tid], CountsResp: toCountsResp(c),
		})
	}
	sortTemplateCounts(out.ByTemplate)
	return out
}
