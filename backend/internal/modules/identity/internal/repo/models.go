// Package repo implements identity's persistence. Models are unexported and
// map to domain types; table names derive from struct names (no
// TableName()): tenant → identity_svc.tenants, user → users, userRole →
// user_roles.
package repo

import (
	"encoding/json"
	"time"

	"levelup/internal/modules/identity/internal/domain"
)

type tenant struct {
	ID          string `gorm:"primaryKey;type:uuid"`
	LegacyID    *int64
	Name        string
	Slug        string
	OwnerUserID *string `gorm:"type:uuid"`
	Active      bool
	Timezone    string
	Settings    string `gorm:"type:jsonb"`
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

type user struct {
	ID                string `gorm:"primaryKey;type:uuid"`
	LegacyID          *int64
	TenantID          *string `gorm:"type:uuid"`
	Name              string
	Email             string
	PasswordHash      string
	Active            bool
	EmailVerifiedAt   *time.Time
	PasswordChangedAt *time.Time
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type userRole struct {
	UserID    string `gorm:"primaryKey;type:uuid"`
	RoleID    int64  `gorm:"primaryKey"`
	GrantedAt time.Time
}

func tenantFromDomain(t domain.Tenant) (tenant, error) {
	settings := t.Settings
	if settings == nil {
		settings = map[string]any{}
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return tenant{}, err
	}
	return tenant{
		ID:          t.ID,
		Name:        t.Name,
		Slug:        t.Slug,
		OwnerUserID: nullable(t.OwnerUserID),
		Active:      t.Active,
		Timezone:    t.Timezone,
		Settings:    string(raw),
		Version:     t.Version,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		DeletedAt:   t.DeletedAt,
	}, nil
}

func (m tenant) toDomain() domain.Tenant {
	settings := map[string]any{}
	if m.Settings != "" {
		_ = json.Unmarshal([]byte(m.Settings), &settings)
	}
	return domain.Tenant{
		ID:          m.ID,
		Name:        m.Name,
		Slug:        m.Slug,
		OwnerUserID: deref(m.OwnerUserID),
		Active:      m.Active,
		Timezone:    m.Timezone,
		Settings:    settings,
		Version:     m.Version,
		CreatedAt:   m.CreatedAt.UTC(),
		UpdatedAt:   m.UpdatedAt.UTC(),
		DeletedAt:   m.DeletedAt,
	}
}

func userFromDomain(u domain.User) user {
	return user{
		ID:                u.ID,
		TenantID:          nullable(u.TenantID),
		Name:              u.Name,
		Email:             u.Email,
		PasswordHash:      u.PasswordHash,
		Active:            u.Active,
		EmailVerifiedAt:   u.EmailVerifiedAt,
		PasswordChangedAt: u.PasswordChangedAt,
		Version:           u.Version,
		CreatedAt:         u.CreatedAt,
		UpdatedAt:         u.UpdatedAt,
	}
}

func (m user) toDomain(roleIDs []int64) domain.User {
	return domain.User{
		ID:                m.ID,
		TenantID:          deref(m.TenantID),
		Name:              m.Name,
		Email:             m.Email,
		PasswordHash:      m.PasswordHash,
		Active:            m.Active,
		EmailVerifiedAt:   m.EmailVerifiedAt,
		PasswordChangedAt: m.PasswordChangedAt,
		RoleIDs:           roleIDs,
		Version:           m.Version,
		CreatedAt:         m.CreatedAt.UTC(),
		UpdatedAt:         m.UpdatedAt.UTC(),
	}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
