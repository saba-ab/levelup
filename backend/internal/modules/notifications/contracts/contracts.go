// Package contracts is notifications' public surface: player-facing
// notifications (in-app feed and email) rendered from tenant templates when a
// gamification fact happens.
package contracts

import "levelup/internal/platform/authz"

const Module = "notifications"

// JobSendEmail delivers one email notification row. It travels through the
// outbox as topic Topic(JobSendEmail) (R46); only notifications publishes it.
const JobSendEmail = "notifications.send_email"

// JobSweepStaleEmail is the cron job failing email rows stuck in pending
// (the ladder parked their job, or a worker died mid-send).
const JobSweepStaleEmail = "notifications.sweep_stale_email"

func Topic(job string) string { return "job." + job }

// SendEmailCmdV1 is the payload of job.notifications.send_email.
type SendEmailCmdV1 struct {
	TenantID       string `json:"tenant_id"`
	NotificationID string `json:"notification_id"`
}

// Triggers: the facts a template can react to (topic without the version).
const (
	TriggerBadgeAwarded     = "badges.awarded"
	TriggerLevelReached     = "progression.level_reached"
	TriggerMissionCompleted = "missions.completed"
	TriggerStreakMilestone  = "streaks.milestone_reached"
	TriggerStreakBroken     = "streaks.broken"
	TriggerRewardClaimed    = "rewards.claimed"
	TriggerPointsCredited   = "points.credited"
)

var AllTriggers = []string{
	TriggerBadgeAwarded, TriggerLevelReached, TriggerMissionCompleted, TriggerStreakMilestone,
	TriggerStreakBroken, TriggerRewardClaimed, TriggerPointsCredited,
}

// Channels. in_app is always available; email is enabled per tenant.
const (
	ChannelInApp = "in_app"
	ChannelEmail = "email"
)

// Notification statuses.
const (
	StatusPending   = "pending"   // email queued for the send job
	StatusDelivered = "delivered" // in_app immediately; email after the mailer accepted it
	StatusFailed    = "failed"    // render error, permanent mail error, or retries exhausted
	StatusSkipped   = "skipped"   // email disabled for the tenant, or the player has no email
)

// Skip / failure reasons stored on the row.
const (
	ReasonEmailDisabled  = "email_disabled"
	ReasonNoEmail        = "no_email"
	ReasonPlayerNotFound = "player_not_found"
	ReasonRenderError    = "render_error"
	ReasonMailRejected   = "mail_rejected"
	ReasonRetriesExhaust = "retries_exhausted"
	ReasonStale          = "stale"
)

// Problem codes (ADR-0016).
const (
	CodeTemplateNotFound     = "notification_template_not_found"
	CodeTemplateInvalid      = "notification_template_invalid"
	CodeTemplateNameTaken    = "notification_template_name_taken"
	CodeNotificationNotFound = "notification_not_found"
	CodePlayerNotFound       = "player_not_found"
	CodeVersionConflict      = "version_conflict"
)

var (
	PermViewAny        = authz.Permission{Module: Module, Action: "view_any"}
	PermView           = authz.Permission{Module: Module, Action: "view"}
	PermCreate         = authz.Permission{Module: Module, Action: "create"}          // admin roles
	PermUpdate         = authz.Permission{Module: Module, Action: "update"}          // admin roles
	PermDelete         = authz.Permission{Module: Module, Action: "delete"}          // admin roles
	PermChannelsManage = authz.Permission{Module: Module, Action: "channels_manage"} // admin roles
	PermFeedView       = authz.Permission{Module: Module, Action: "feed_view"}
	PermFeedMarkRead   = authz.Permission{Module: Module, Action: "feed_mark_read"}
)

var AllPermissions = []authz.Permission{
	PermViewAny, PermView, PermCreate, PermUpdate, PermDelete, PermChannelsManage, PermFeedView, PermFeedMarkRead,
}
