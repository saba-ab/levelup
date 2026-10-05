package transport

import (
	"time"

	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/domain"
)

// Shape validation only; invariants live in the domain. Password max=72 is
// counted in bytes by the domain policy as well (the validator counts runes).

type RegisterReq struct {
	TenantName string `json:"tenant_name" validate:"required,max=255"`
	Name       string `json:"name"        validate:"omitempty,max=255"`
	Email      string `json:"email"       validate:"required,email,max=255"`
	Password   string `json:"password"    validate:"required,min=8,max=72"`
	// PasswordConfirmation is optional; when sent it must match.
	PasswordConfirmation string `json:"password_confirmation" validate:"omitempty,eqfield=Password"`
	Timezone             string `json:"timezone"              validate:"omitempty,max=64"`
}

type LoginReq struct {
	Email    string `json:"email"    validate:"required,max=255"`
	Password string `json:"password" validate:"required,max=4096"`
}

type RefreshReq struct {
	RefreshToken string `json:"refresh_token" validate:"required,max=1024"`
}

type CreateUserReq struct {
	Name                 string  `json:"name"                  validate:"required,max=255"`
	Email                string  `json:"email"                 validate:"required,email,max=255"`
	Password             string  `json:"password"              validate:"required,min=8,max=72"`
	PasswordConfirmation string  `json:"password_confirmation" validate:"omitempty,eqfield=Password"`
	RoleIDs              []int64 `json:"role_ids"              validate:"omitempty,max=10,dive,min=1"`
}

type UpdateUserReq struct {
	Name            *string `json:"name"             validate:"omitempty,min=1,max=255"`
	Email           *string `json:"email"            validate:"omitempty,email,max=255"`
	Password        *string `json:"password"         validate:"omitempty,min=8,max=72"`
	CurrentPassword *string `json:"current_password" validate:"omitempty,max=4096"`
	Active          *bool   `json:"active"`
}

type AssignRolesReq struct {
	RoleIDs []int64 `json:"role_ids" validate:"required,max=10,dive,min=1"`
}

type UpdateTenantReq struct {
	Name     *string        `json:"name"     validate:"omitempty,min=1,max=255"`
	Timezone *string        `json:"timezone" validate:"omitempty,min=1,max=64"`
	Settings map[string]any `json:"settings"`
}

type PlatformUpdateTenantReq struct {
	Active *bool `json:"active" validate:"required"`
}

type RoleResp struct {
	ID    int64  `json:"id"`
	Key   string `json:"key"`
	Label string `json:"label"`
}

type UserResp struct {
	ID              string     `json:"id"`
	TenantID        *string    `json:"tenant_id"`
	Name            string     `json:"name"`
	Email           string     `json:"email"`
	Active          bool       `json:"active"`
	EmailVerifiedAt *time.Time `json:"email_verified_at"`
	RoleIDs         []int64    `json:"role_ids"`
	Roles           []RoleResp `json:"roles"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type TenantResp struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Slug        string         `json:"slug"`
	OwnerUserID *string        `json:"owner_user_id"`
	Active      bool           `json:"active"`
	Timezone    string         `json:"timezone"`
	Settings    map[string]any `json:"settings"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// SessionResp is returned by register, login and refresh.
type SessionResp struct {
	User         UserResp    `json:"user"`
	Tenant       *TenantResp `json:"tenant"`
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    int64       `json:"expires_in"` // seconds
}

type MeResp struct {
	User   UserResp    `json:"user"`
	Tenant *TenantResp `json:"tenant"`
}

type UserListResp struct {
	Data       []UserResp `json:"data"`
	NextCursor string     `json:"next_cursor"`
}

type TenantListResp struct {
	Data       []TenantResp `json:"data"`
	NextCursor string       `json:"next_cursor"`
}

func toUserResp(u domain.User) UserResp {
	ids := u.RoleIDs
	if ids == nil {
		ids = []int64{}
	}
	roles := make([]RoleResp, 0, len(ids))
	for _, r := range domain.Roles(ids) {
		roles = append(roles, RoleResp{ID: r.ID, Key: r.Key, Label: r.Label})
	}
	var tid *string
	if u.TenantID != "" {
		v := u.TenantID
		tid = &v
	}
	return UserResp{
		ID: u.ID, TenantID: tid, Name: u.Name, Email: u.Email, Active: u.Active,
		EmailVerifiedAt: u.EmailVerifiedAt, RoleIDs: ids, Roles: roles,
		CreatedAt: u.CreatedAt.UTC(), UpdatedAt: u.UpdatedAt.UTC(),
	}
}

func toTenantResp(t domain.Tenant) TenantResp {
	var owner *string
	if t.OwnerUserID != "" {
		v := t.OwnerUserID
		owner = &v
	}
	settings := t.Settings
	if settings == nil {
		settings = map[string]any{}
	}
	return TenantResp{
		ID: t.ID, Name: t.Name, Slug: t.Slug, OwnerUserID: owner, Active: t.IsActive(),
		Timezone: t.Timezone, Settings: settings,
		CreatedAt: t.CreatedAt.UTC(), UpdatedAt: t.UpdatedAt.UTC(),
	}
}

func toTenantPtr(t *domain.Tenant) *TenantResp {
	if t == nil {
		return nil
	}
	r := toTenantResp(*t)
	return &r
}

func toSessionResp(s app.Session) SessionResp {
	return SessionResp{
		User:         toUserResp(s.User),
		Tenant:       toTenantPtr(s.Tenant),
		AccessToken:  s.AccessToken,
		RefreshToken: s.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.ExpiresIn,
	}
}
