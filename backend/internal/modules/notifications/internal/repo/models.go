// Package repo implements notifications' persistence. Models are
// unexported; table names derive from struct names under the
// notifications_svc. prefix (NO TableName() overrides).
package repo

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"levelup/internal/modules/notifications/internal/domain"
)

// textArray maps a Postgres TEXT[] of simple identifiers (channel names).
// Values are written as an array literal; elements never contain commas,
// quotes or braces (the domain restricts them to in_app/email).
type textArray []string

func (a textArray) Value() (driver.Value, error) {
	for _, s := range a {
		if strings.ContainsAny(s, `{}," \`) || s == "" {
			return nil, fmt.Errorf("textArray: unsupported element %q", s)
		}
	}
	return "{" + strings.Join(a, ",") + "}", nil
}

func (a *textArray) Scan(src any) error {
	var raw string
	switch v := src.(type) {
	case nil:
		*a = nil
		return nil
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("textArray: cannot scan %T", src)
	}
	raw = strings.TrimSuffix(strings.TrimPrefix(raw, "{"), "}")
	if raw == "" {
		*a = textArray{}
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make(textArray, len(parts))
	for i, p := range parts {
		out[i] = strings.Trim(p, `"`)
	}
	*a = out
	return nil
}

// notificationTemplate → notifications_svc.notification_templates.
type notificationTemplate struct {
	ID            string    `gorm:"primaryKey;type:uuid"`
	TenantID      string    `gorm:"type:uuid;not null"`
	Name          string    `gorm:"not null"`
	Trigger       string    `gorm:"not null"`
	Channels      textArray `gorm:"type:text[];not null"`
	TitleTemplate string    `gorm:"not null"`
	BodyTemplate  string    `gorm:"not null"`
	IsActive      bool      `gorm:"not null"`
	CreatedBy     *string   `gorm:"type:uuid"`
	Version       int       `gorm:"not null"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

func templateFromDomain(t domain.Template) notificationTemplate {
	return notificationTemplate{
		ID:            t.ID,
		TenantID:      t.TenantID,
		Name:          t.Name,
		Trigger:       t.Trigger,
		Channels:      textArray(t.Channels),
		TitleTemplate: t.TitleTemplate,
		BodyTemplate:  t.BodyTemplate,
		IsActive:      t.Active,
		CreatedBy:     nullable(t.CreatedBy),
		Version:       t.Version,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
		DeletedAt:     t.DeletedAt,
	}
}

func (m notificationTemplate) toDomain() domain.Template {
	t := domain.Template{
		ID:            m.ID,
		TenantID:      m.TenantID,
		Name:          m.Name,
		Trigger:       m.Trigger,
		Channels:      []string(m.Channels),
		TitleTemplate: m.TitleTemplate,
		BodyTemplate:  m.BodyTemplate,
		Active:        m.IsActive,
		Version:       m.Version,
		CreatedAt:     m.CreatedAt.UTC(),
		UpdatedAt:     m.UpdatedAt.UTC(),
		DeletedAt:     utcPtr(m.DeletedAt),
	}
	if m.CreatedBy != nil {
		t.CreatedBy = *m.CreatedBy
	}
	return t
}

// notification → notifications_svc.notifications.
type notification struct {
	ID          string `gorm:"primaryKey;type:uuid"`
	TenantID    string `gorm:"type:uuid;not null"`
	TemplateID  string `gorm:"type:uuid;not null"`
	PlayerID    string `gorm:"type:uuid;not null"`
	EventID     string `gorm:"not null"`
	Trigger     string `gorm:"not null"`
	Channel     string `gorm:"not null"`
	Status      string `gorm:"not null"`
	Title       string `gorm:"not null"`
	Body        string `gorm:"not null"`
	Reason      string `gorm:"not null"`
	Attempts    int    `gorm:"not null"`
	LastError   string `gorm:"not null"`
	LeaseUntil  *time.Time
	DeliveredAt *time.Time
	ReadAt      *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func notificationFromDomain(n domain.Notification) notification {
	return notification{
		ID:          n.ID,
		TenantID:    n.TenantID,
		TemplateID:  n.TemplateID,
		PlayerID:    n.PlayerID,
		EventID:     n.EventID,
		Trigger:     n.Trigger,
		Channel:     n.Channel,
		Status:      n.Status,
		Title:       n.Title,
		Body:        n.Body,
		Reason:      n.Reason,
		Attempts:    n.Attempts,
		LastError:   n.LastError,
		LeaseUntil:  n.LeaseUntil,
		DeliveredAt: n.DeliveredAt,
		ReadAt:      n.ReadAt,
		CreatedAt:   n.CreatedAt,
		UpdatedAt:   n.UpdatedAt,
	}
}

func (m notification) toDomain() domain.Notification {
	return domain.Notification{
		ID:          m.ID,
		TenantID:    m.TenantID,
		TemplateID:  m.TemplateID,
		PlayerID:    m.PlayerID,
		EventID:     m.EventID,
		Trigger:     m.Trigger,
		Channel:     m.Channel,
		Status:      m.Status,
		Title:       m.Title,
		Body:        m.Body,
		Reason:      m.Reason,
		Attempts:    m.Attempts,
		LastError:   m.LastError,
		LeaseUntil:  utcPtr(m.LeaseUntil),
		DeliveredAt: utcPtr(m.DeliveredAt),
		ReadAt:      utcPtr(m.ReadAt),
		CreatedAt:   m.CreatedAt.UTC(),
		UpdatedAt:   m.UpdatedAt.UTC(),
	}
}

// channelSetting → notifications_svc.channel_settings.
type channelSetting struct {
	TenantID      string `gorm:"primaryKey;type:uuid"`
	EmailEnabled  bool   `gorm:"not null"`
	EmailFromName string `gorm:"not null"`
	UpdatedAt     time.Time
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
