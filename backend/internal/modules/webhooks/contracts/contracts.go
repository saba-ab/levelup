// Package contracts is webhooks' public surface. Tenants register HTTPS
// endpoints; facts published by other modules are fanned out to them as
// signed POST requests.
package contracts

import (
	"time"

	"levelup/internal/platform/authz"
)

const Module = "webhooks"

// TopicEndpointDisabled is published when an endpoint is switched off after
// too many consecutive failed deliveries.
const TopicEndpointDisabled = "webhooks.endpoint_disabled.v1"

// JobDeliver performs one delivery attempt. It travels through the outbox
// as topic "job.webhooks.deliver" (R46).
const JobDeliver = "webhooks.deliver"

// JobRetrySweep is the reconciling cron (Schedule set in Jobs()).
const JobRetrySweep = "webhooks.retry_sweep"

func Topic(job string) string { return "job." + job }

// DisabledReasonConsecutiveFailures is EndpointDisabledV1.Reason when the
// failure threshold tripped.
const DisabledReasonConsecutiveFailures = "consecutive_failures"

// DeliverCmdV1 asks the worker to attempt one delivery. BaseAttempt is the
// delivery's cycle attempt count when the command was enqueued: the job
// gives up (marks the delivery failed) once it has tried LadderAttempts
// times since BaseAttempt, so a delivery is final when the retry ladder ends.
type DeliverCmdV1 struct {
	TenantID    string `json:"tenant_id"`
	DeliveryID  string `json:"delivery_id"`
	BaseAttempt int    `json:"base_attempt"`
}

type EndpointDisabledV1 struct {
	TenantID            string    `json:"tenant_id"`
	EndpointID          string    `json:"endpoint_id"`
	URL                 string    `json:"url"`
	Reason              string    `json:"reason"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	At                  time.Time `json:"at"`
}

var (
	PermView   = authz.Permission{Module: Module, Action: "view"}
	PermManage = authz.Permission{Module: Module, Action: "manage"} // admin roles
)

var AllPermissions = []authz.Permission{PermView, PermManage}
