package domain

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// Delivery statuses.
const (
	StatusPending   = "pending"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// ResponseSnippetLimit caps the stored response body.
const ResponseSnippetLimit = 1024

// Last-error texts recorded when a delivery never reached the receiver.
const (
	ErrTextEndpointInactive = "endpoint inactive or deleted"
	ErrTextAttemptsExceeded = "attempt budget exhausted"
)

// Delivery is one event sent (or to be sent) to one endpoint. The pair
// (EndpointID, EventID) is unique: a redelivered fact never fans out twice.
//
// Attempts counts every attempt ever made. CycleAttempts counts attempts
// since the delivery was (re)queued by a fan-out or a manual redeliver; the
// retry budget applies to it.
type Delivery struct {
	ID             string
	TenantID       string
	EndpointID     string
	EventID        string
	Event          string
	Payload        []byte
	Status         string
	Attempts       int
	CycleAttempts  int
	ResponseStatus *int
	ResponseBody   string
	LatencyMS      *int64
	LastError      string
	LeaseUntil     *time.Time
	EnqueuedAt     time.Time
	LastAttemptAt  *time.Time
	DeliveredAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewDelivery(tenantID, endpointID, eventID, event string, payload []byte, now time.Time) Delivery {
	return Delivery{
		ID:         id.NewID(),
		TenantID:   tenantID,
		EndpointID: endpointID,
		EventID:    eventID,
		Event:      event,
		Payload:    payload,
		Status:     StatusPending,
		EnqueuedAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// RetryPolicy bounds attempts. LadderAttempts is how many times one queued
// job runs before the broker's retry ladder ends (first try + retry tiers);
// MaxAttempts caps a cycle across sweeps.
type RetryPolicy struct {
	LadderAttempts int
	MaxAttempts    int
}

// Final reports whether a failed attempt ends the delivery: the job's ladder
// is exhausted (so the broker would dead-letter next), or the cycle's budget
// is spent.
func (p RetryPolicy) Final(cycleAttempts, baseAttempt int) bool {
	return cycleAttempts-baseAttempt >= p.LadderAttempts || cycleAttempts >= p.MaxAttempts
}

// AttemptResult is the outcome of one POST.
type AttemptResult struct {
	StatusCode int // 0 when no response arrived
	LatencyMS  int64
	Body       string // response snippet, at most ResponseSnippetLimit bytes
	Err        string // transport error (timeout, refused, blocked destination)
}

// OK is a 2xx response.
func (r AttemptResult) OK() bool {
	return r.Err == "" && r.StatusCode >= 200 && r.StatusCode < 300
}

// Error describes a failed attempt.
func (r AttemptResult) Error() string {
	if r.Err != "" {
		return r.Err
	}
	return "receiver responded " + strconv.Itoa(r.StatusCode)
}

// Begin claims an attempt: the delivery must be pending and not leased by
// another in-flight attempt.
func (d *Delivery) Begin(now time.Time, lease time.Duration) error {
	if d.Status != StatusPending {
		return errs.New(errs.Conflict, "delivery is not pending")
	}
	if d.LeaseUntil != nil && d.LeaseUntil.After(now) {
		return ErrDeliveryInProgress
	}
	until := now.Add(lease)
	d.Attempts++
	d.CycleAttempts++
	d.LeaseUntil = &until
	d.LastAttemptAt = &now
	d.UpdatedAt = now
	return nil
}

// Settle records an attempt's outcome. A success is final; a failure is
// final only when final is set, otherwise the delivery stays pending.
func (d *Delivery) Settle(res AttemptResult, final bool, now time.Time) {
	d.record(res)
	d.LeaseUntil = nil
	d.UpdatedAt = now
	switch {
	case res.OK():
		d.Status = StatusSucceeded
		d.LastError = ""
		d.DeliveredAt = &now
	case final:
		d.Status = StatusFailed
		d.LastError = res.Error()
	default:
		d.LastError = res.Error()
	}
}

func (d *Delivery) record(res AttemptResult) {
	if res.StatusCode > 0 {
		code := res.StatusCode
		d.ResponseStatus = &code
	} else {
		d.ResponseStatus = nil
	}
	lat := res.LatencyMS
	d.LatencyMS = &lat
	d.ResponseBody = Snippet(res.Body)
}

// Fail ends a delivery without an attempt (endpoint gone, budget spent).
func (d *Delivery) Fail(reason string, now time.Time) {
	d.Status = StatusFailed
	d.LastError = reason
	d.LeaseUntil = nil
	d.UpdatedAt = now
}

// Requeue re-arms a delivery for a fresh cycle (manual redeliver).
func (d *Delivery) Requeue(now time.Time) error {
	if d.Status == StatusPending && d.LeaseUntil != nil && d.LeaseUntil.After(now) {
		return ErrDeliveryInProgress
	}
	d.Status = StatusPending
	d.CycleAttempts = 0
	d.LeaseUntil = nil
	d.DeliveredAt = nil
	d.EnqueuedAt = now
	d.UpdatedAt = now
	return nil
}

// Snippet truncates a response body to ResponseSnippetLimit bytes of valid
// UTF-8.
func Snippet(s string) string {
	if len(s) > ResponseSnippetLimit {
		s = s[:ResponseSnippetLimit]
		for len(s) > 0 && !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "�")
}

// Envelope is the JSON body POSTed to receivers.
type Envelope struct {
	Event      string          `json:"event"`
	EventID    string          `json:"event_id"`
	OccurredAt time.Time       `json:"occurred_at"`
	TenantID   string          `json:"tenant_id"`
	Data       json.RawMessage `json:"data"`
}

// BuildPayload renders the request body: the webhook envelope with the
// fact's own JSON under "data".
func BuildPayload(event, eventID, tenantID string, occurredAt time.Time, data json.RawMessage) ([]byte, error) {
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	if !json.Valid(data) {
		return nil, errs.New(errs.Invalid, "fact payload is not valid JSON")
	}
	b, err := json.Marshal(Envelope{Event: event, EventID: eventID, OccurredAt: occurredAt.UTC(), TenantID: tenantID, Data: data})
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "marshal webhook payload", err)
	}
	return b, nil
}
