// Package domain holds webhooks' entities and invariants: endpoint
// validation (including the SSRF guard), the event catalogue, the delivery
// state machine and request signing. No framework tags.
package domain

import "levelup/internal/shared/errs"

// Problem codes (ADR-0016) clients may branch on.
const (
	CodeEndpointNotFound   = "webhook_endpoint_not_found"
	CodeDeliveryNotFound   = "webhook_delivery_not_found"
	CodeInvalidEndpoint    = "invalid_webhook_endpoint"
	CodeInsecureURL        = "webhook_url_insecure"
	CodeForbiddenHost      = "webhook_url_forbidden_host"
	CodeUnknownEventType   = "webhook_unknown_event_type"
	CodeEndpointLimit      = "webhook_endpoint_limit_reached"
	CodeEndpointInactive   = "webhook_endpoint_inactive"
	CodeDeliveryInProgress = "webhook_delivery_in_progress"
	CodeVersionConflict    = "webhook_endpoint_version_conflict"
	CodeDeliveryFailed     = "webhook_delivery_failed"
)

var (
	ErrEndpointNotFound   = errs.WithCode(errs.New(errs.NotFound, "webhook endpoint not found"), CodeEndpointNotFound)
	ErrDeliveryNotFound   = errs.WithCode(errs.New(errs.NotFound, "webhook delivery not found"), CodeDeliveryNotFound)
	ErrEndpointLimit      = errs.WithCode(errs.New(errs.Conflict, "the tenant already has the maximum number of webhook endpoints"), CodeEndpointLimit)
	ErrEndpointInactive   = errs.WithCode(errs.New(errs.Conflict, "webhook endpoint is not active"), CodeEndpointInactive)
	ErrDeliveryInProgress = errs.WithCode(errs.New(errs.Conflict, "delivery attempt in progress, retry shortly"), CodeDeliveryInProgress)
	ErrVersionConflict    = errs.WithCode(errs.New(errs.Conflict, "webhook endpoint modified concurrently, retry"), CodeVersionConflict)

	ErrURLRequired        = invalid("url is required")
	ErrURLTooLong         = invalid("url must be at most 2048 characters")
	ErrURLMalformed       = invalid("url must be an absolute URL with a host")
	ErrURLCredentials     = invalid("url must not carry user credentials")
	ErrURLFragment        = invalid("url must not carry a fragment")
	ErrInsecureURL        = errs.WithCode(errs.New(errs.Invalid, "url must use https"), CodeInsecureURL)
	ErrForbiddenHost      = errs.WithCode(errs.New(errs.Invalid, "url must not point at a private, loopback, link-local or reserved address"), CodeForbiddenHost)
	ErrDescriptionTooLong = invalid("description must be at most 1000 characters")
	ErrEventTypesRequired = invalid("event_types must name at least one event type or \"*\"")
	ErrWildcardNotAlone   = invalid("\"*\" must be the only entry of event_types")
	ErrTooManyEventTypes  = invalid("too many event_types")
	ErrNoTenant           = invalid("tenant id is required")
)

func invalid(msg string) error {
	return errs.WithCode(errs.New(errs.Invalid, msg), CodeInvalidEndpoint)
}

// UnknownEventType names the offending entry.
func UnknownEventType(name string) error {
	return errs.WithCode(errs.New(errs.Invalid, "unknown event type: "+name), CodeUnknownEventType)
}
