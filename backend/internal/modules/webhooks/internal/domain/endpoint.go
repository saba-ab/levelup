package domain

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"levelup/internal/shared/errs"
	"levelup/internal/shared/id"
)

// SecretPrefix marks webhook signing secrets.
const SecretPrefix = "whsec_"

// MaxEndpointsPerTenant bounds the per-event fan-out.
const MaxEndpointsPerTenant = 25

const maxDescriptionLength = 1000

// Endpoint is a tenant's registered receiver. Secret is stored as-is and is
// only ever shown to clients on create and rotate.
type Endpoint struct {
	ID                  string
	TenantID            string
	URL                 string
	Description         string
	EventTypes          []string
	Secret              string
	Active              bool
	ConsecutiveFailures int
	DisabledReason      string
	DisabledAt          *time.Time
	Version             int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type NewEndpointInput struct {
	URL         string
	Description string
	EventTypes  []string
	Active      bool
}

// EndpointPatch is a partial update; nil fields stay untouched.
type EndpointPatch struct {
	URL         *string
	Description *string
	EventTypes  *[]string
	Active      *bool
}

func NewEndpoint(tenantID string, in NewEndpointInput, policy URLPolicy, now time.Time) (Endpoint, error) {
	if tenantID == "" {
		return Endpoint{}, ErrNoTenant
	}
	u, err := policy.Validate(in.URL)
	if err != nil {
		return Endpoint{}, err
	}
	if len([]rune(in.Description)) > maxDescriptionLength {
		return Endpoint{}, ErrDescriptionTooLong
	}
	types, err := NormalizeEventTypes(in.EventTypes)
	if err != nil {
		return Endpoint{}, err
	}
	secret, err := NewSecret()
	if err != nil {
		return Endpoint{}, err
	}
	return Endpoint{
		ID:          id.NewID(),
		TenantID:    tenantID,
		URL:         u,
		Description: in.Description,
		EventTypes:  types,
		Secret:      secret,
		Active:      in.Active,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Apply validates and applies a patch. Re-activating an endpoint clears its
// failure streak and disabled reason.
func (e *Endpoint) Apply(p EndpointPatch, policy URLPolicy, now time.Time) error {
	next := *e
	if p.URL != nil {
		u, err := policy.Validate(*p.URL)
		if err != nil {
			return err
		}
		next.URL = u
	}
	if p.Description != nil {
		if len([]rune(*p.Description)) > maxDescriptionLength {
			return ErrDescriptionTooLong
		}
		next.Description = *p.Description
	}
	if p.EventTypes != nil {
		types, err := NormalizeEventTypes(*p.EventTypes)
		if err != nil {
			return err
		}
		next.EventTypes = types
	}
	if p.Active != nil {
		if *p.Active && !e.Active {
			next.ConsecutiveFailures = 0
			next.DisabledReason = ""
			next.DisabledAt = nil
		}
		next.Active = *p.Active
	}
	next.UpdatedAt = now
	*e = next
	return nil
}

// RotateSecret replaces the signing secret.
func (e *Endpoint) RotateSecret(now time.Time) error {
	s, err := NewSecret()
	if err != nil {
		return err
	}
	e.Secret = s
	e.UpdatedAt = now
	return nil
}

// Subscribes reports whether the endpoint wants event.
func (e Endpoint) Subscribes(event string) bool {
	return Subscribes(e.EventTypes, event)
}

// NewSecret returns "whsec_" + 192 random bits in hex.
func NewSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", errs.Wrap(errs.Internal, "generate webhook secret", err)
	}
	return SecretPrefix + hex.EncodeToString(b), nil
}
