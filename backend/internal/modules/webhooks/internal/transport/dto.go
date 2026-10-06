package transport

import (
	"encoding/json"
	"time"

	"levelup/internal/modules/webhooks/internal/domain"
)

// CreateEndpointReq registers an endpoint. event_types is a subset of
// GET /webhooks/event-types names, or ["*"] for all.
type CreateEndpointReq struct {
	URL         string   `json:"url" validate:"required,max=2048"`
	Description string   `json:"description" validate:"max=1000"`
	EventTypes  []string `json:"event_types" validate:"required,min=1,max=64,dive,required,max=100"`
	IsActive    *bool    `json:"is_active"`
}

// UpdateEndpointReq is a partial update: omitted fields stay untouched.
// Setting is_active=true on a disabled endpoint clears its failure streak.
type UpdateEndpointReq struct {
	URL         *string   `json:"url" validate:"omitempty,min=1,max=2048"`
	Description *string   `json:"description" validate:"omitempty,max=1000"`
	EventTypes  *[]string `json:"event_types" validate:"omitempty,min=1,max=64,dive,required,max=100"`
	IsActive    *bool     `json:"is_active"`
}

// EndpointResp never carries the secret.
type EndpointResp struct {
	ID                  string     `json:"id"`
	TenantID            string     `json:"tenant_id"`
	URL                 string     `json:"url"`
	Description         string     `json:"description"`
	EventTypes          []string   `json:"event_types"`
	IsActive            bool       `json:"is_active"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	DisabledReason      string     `json:"disabled_reason"`
	DisabledAt          *time.Time `json:"disabled_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// EndpointWithSecretResp is returned by create and rotate-secret only: the
// one time the signing secret is shown.
type EndpointWithSecretResp struct {
	EndpointResp
	Secret string `json:"secret"`
}

type EndpointListResp struct {
	Data       []EndpointResp `json:"data"`
	NextCursor string         `json:"next_cursor"`
}

// DeliveryResp is one delivery. payload (the exact request body) is
// included on GET /webhooks/deliveries/{id}, redeliver and test, and
// omitted from lists.
type DeliveryResp struct {
	ID             string          `json:"id"`
	EndpointID     string          `json:"endpoint_id"`
	Event          string          `json:"event"`
	EventID        string          `json:"event_id"`
	Status         string          `json:"status" enums:"pending,succeeded,failed"`
	Attempts       int             `json:"attempts"`
	ResponseStatus *int            `json:"response_status"`
	ResponseBody   string          `json:"response_body"`
	LatencyMS      *int64          `json:"latency_ms"`
	LastError      string          `json:"last_error"`
	LastAttemptAt  *time.Time      `json:"last_attempt_at"`
	DeliveredAt    *time.Time      `json:"delivered_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Payload        json.RawMessage `json:"payload,omitempty" swaggertype:"object"`
}

type DeliveryListResp struct {
	Data       []DeliveryResp `json:"data"`
	NextCursor string         `json:"next_cursor"`
}

type EventTypeResp struct {
	Event       string `json:"event"`
	Description string `json:"description"`
}

type EventTypeListResp struct {
	Data []EventTypeResp `json:"data"`
}

func toEndpointResp(e domain.Endpoint) EndpointResp {
	types := e.EventTypes
	if types == nil {
		types = []string{}
	}
	return EndpointResp{
		ID:                  e.ID,
		TenantID:            e.TenantID,
		URL:                 e.URL,
		Description:         e.Description,
		EventTypes:          types,
		IsActive:            e.Active,
		ConsecutiveFailures: e.ConsecutiveFailures,
		DisabledReason:      e.DisabledReason,
		DisabledAt:          e.DisabledAt,
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
	}
}

func toEndpointWithSecret(e domain.Endpoint) EndpointWithSecretResp {
	return EndpointWithSecretResp{EndpointResp: toEndpointResp(e), Secret: e.Secret}
}

func toDeliveryResp(d domain.Delivery, withPayload bool) DeliveryResp {
	out := DeliveryResp{
		ID:             d.ID,
		EndpointID:     d.EndpointID,
		Event:          d.Event,
		EventID:        d.EventID,
		Status:         d.Status,
		Attempts:       d.Attempts,
		ResponseStatus: d.ResponseStatus,
		ResponseBody:   d.ResponseBody,
		LatencyMS:      d.LatencyMS,
		LastError:      d.LastError,
		LastAttemptAt:  d.LastAttemptAt,
		DeliveredAt:    d.DeliveredAt,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
	if withPayload && json.Valid(d.Payload) {
		out.Payload = json.RawMessage(d.Payload)
	}
	return out
}
