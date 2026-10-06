// Package repo implements webhooks' persistence. Models stay unexported; no
// TableName() overrides: tables derive from struct names under the module
// prefix (webhooks_svc.<plural>).
package repo

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"levelup/internal/modules/webhooks/internal/domain"
)

// endpoint → webhooks_svc.endpoints
type endpoint struct {
	ID                  string `gorm:"primaryKey;type:uuid"`
	TenantID            string `gorm:"type:uuid"`
	URL                 string `gorm:"column:url"`
	Description         string
	EventTypes          textArray `gorm:"type:text[]"`
	Secret              string
	IsActive            bool
	ConsecutiveFailures int
	DisabledReason      string
	DisabledAt          *time.Time
	Version             int
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
}

func (m endpoint) toDomain() domain.Endpoint {
	return domain.Endpoint{
		ID:                  m.ID,
		TenantID:            m.TenantID,
		URL:                 m.URL,
		Description:         m.Description,
		EventTypes:          []string(m.EventTypes),
		Secret:              m.Secret,
		Active:              m.IsActive,
		ConsecutiveFailures: m.ConsecutiveFailures,
		DisabledReason:      m.DisabledReason,
		DisabledAt:          utcPtr(m.DisabledAt),
		Version:             m.Version,
		CreatedAt:           m.CreatedAt.UTC(),
		UpdatedAt:           m.UpdatedAt.UTC(),
	}
}

func endpointFromDomain(e domain.Endpoint) endpoint {
	return endpoint{
		ID:                  e.ID,
		TenantID:            e.TenantID,
		URL:                 e.URL,
		Description:         e.Description,
		EventTypes:          textArray(e.EventTypes),
		Secret:              e.Secret,
		IsActive:            e.Active,
		ConsecutiveFailures: e.ConsecutiveFailures,
		DisabledReason:      e.DisabledReason,
		DisabledAt:          e.DisabledAt,
		Version:             e.Version,
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
	}
}

// delivery → webhooks_svc.deliveries
type delivery struct {
	ID             string `gorm:"primaryKey;type:uuid"`
	TenantID       string `gorm:"type:uuid"`
	EndpointID     string `gorm:"type:uuid"`
	EventID        string
	Event          string
	Payload        string
	Status         string
	Attempts       int
	CycleAttempts  int
	ResponseStatus *int
	ResponseBody   string
	LatencyMs      *int64
	LastError      string
	LeaseUntil     *time.Time
	EnqueuedAt     time.Time
	LastAttemptAt  *time.Time
	DeliveredAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (m delivery) toDomain() domain.Delivery {
	return domain.Delivery{
		ID:             m.ID,
		TenantID:       m.TenantID,
		EndpointID:     m.EndpointID,
		EventID:        m.EventID,
		Event:          m.Event,
		Payload:        []byte(m.Payload),
		Status:         m.Status,
		Attempts:       m.Attempts,
		CycleAttempts:  m.CycleAttempts,
		ResponseStatus: m.ResponseStatus,
		ResponseBody:   m.ResponseBody,
		LatencyMS:      m.LatencyMs,
		LastError:      m.LastError,
		LeaseUntil:     utcPtr(m.LeaseUntil),
		EnqueuedAt:     m.EnqueuedAt.UTC(),
		LastAttemptAt:  utcPtr(m.LastAttemptAt),
		DeliveredAt:    utcPtr(m.DeliveredAt),
		CreatedAt:      m.CreatedAt.UTC(),
		UpdatedAt:      m.UpdatedAt.UTC(),
	}
}

func deliveryFromDomain(d domain.Delivery) delivery {
	return delivery{
		ID:             d.ID,
		TenantID:       d.TenantID,
		EndpointID:     d.EndpointID,
		EventID:        d.EventID,
		Event:          d.Event,
		Payload:        string(d.Payload),
		Status:         d.Status,
		Attempts:       d.Attempts,
		CycleAttempts:  d.CycleAttempts,
		ResponseStatus: d.ResponseStatus,
		ResponseBody:   d.ResponseBody,
		LatencyMs:      d.LatencyMS,
		LastError:      d.LastError,
		LeaseUntil:     d.LeaseUntil,
		EnqueuedAt:     d.EnqueuedAt,
		LastAttemptAt:  d.LastAttemptAt,
		DeliveredAt:    d.DeliveredAt,
		CreatedAt:      d.CreatedAt,
		UpdatedAt:      d.UpdatedAt,
	}
}

// reconcileMarker → webhooks_svc.reconcile_markers
type reconcileMarker struct {
	Job     string `gorm:"primaryKey"`
	LastRun time.Time
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

// textArray maps a Postgres TEXT[] column. Event names are restricted to
// [a-z0-9._*] by the domain, but elements are quoted and escaped anyway.
type textArray []string

func (a textArray) Value() (driver.Value, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, s := range a {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		b.WriteString(strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String(), nil
}

func (a *textArray) Scan(src any) error {
	var raw string
	switch v := src.(type) {
	case nil:
		*a = textArray{}
		return nil
	case []byte:
		raw = string(v)
	case string:
		raw = v
	default:
		return fmt.Errorf("text[]: unsupported type %T", src)
	}
	out, err := parseTextArray(raw)
	if err != nil {
		return err
	}
	*a = out
	return nil
}

// parseTextArray parses a one-dimensional Postgres array literal.
func parseTextArray(raw string) (textArray, error) {
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return nil, fmt.Errorf("text[]: malformed literal %q", raw)
	}
	body := raw[1 : len(raw)-1]
	out := textArray{}
	if body == "" {
		return out, nil
	}
	var cur strings.Builder
	quoted, inQuotes, escaped := false, false, false
	flush := func() {
		v := cur.String()
		if !quoted && v == "NULL" {
			v = ""
		}
		out = append(out, v)
		cur.Reset()
		quoted = false
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case escaped:
			cur.WriteByte(c)
			escaped = false
		case c == '\\':
			escaped = true
		case c == '"':
			inQuotes = !inQuotes
			quoted = true
		case c == ',' && !inQuotes:
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	if inQuotes || escaped {
		return nil, fmt.Errorf("text[]: malformed literal %q", raw)
	}
	flush()
	return out, nil
}
