package domain

import (
	"net/mail"
	"slices"
	"strings"
	"time"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/shared/id"
)

// User is a human operator of a tenant (players are a separate module). A
// user belongs to exactly one tenant; TenantID == "" marks a platform user.
type User struct {
	ID                string
	TenantID          string
	Name              string
	Email             string
	PasswordHash      string
	Active            bool
	EmailVerifiedAt   *time.Time
	PasswordChangedAt *time.Time
	RoleIDs           []int64
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// NewUser normalises the email (trimmed, lower-cased: the unique index is
// case-insensitive, doc 02 §10 bug 20) and validates the name.
func NewUser(tenantID, name, email, passwordHash string, roleIDs []int64, now time.Time) (User, error) {
	name, err := cleanName(name)
	if err != nil {
		return User{}, err
	}
	email, err = NormalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	return User{
		ID:           id.NewID(),
		TenantID:     tenantID,
		Name:         name,
		Email:        email,
		PasswordHash: passwordHash,
		Active:       true,
		RoleIDs:      NormalizeRoleIDs(roleIDs),
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// NormalizeEmail trims, lower-cases and checks the address shape.
func NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > maxNameLen {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// NameFromEmail is the Laravel register default: the email's local part.
func NameFromEmail(email string) string {
	local, _, _ := strings.Cut(strings.TrimSpace(email), "@")
	if local == "" {
		return "user"
	}
	return local
}

func (u User) IsPlatform() bool { return u.TenantID == "" }

func (u User) HasRole(roleID int64) bool { return slices.Contains(u.RoleIDs, roleID) }

func (u User) IsOwnerRole() bool { return u.HasRole(contracts.RoleOwner) }

// Rename returns whether the name changed.
func (u *User) Rename(name string, now time.Time) (bool, error) {
	name, err := cleanName(name)
	if err != nil {
		return false, err
	}
	if name == u.Name {
		return false, nil
	}
	u.Name, u.UpdatedAt = name, now
	return true, nil
}

// ChangeEmail resets verification on a real change (doc 02 §10 bug 4).
func (u *User) ChangeEmail(email string, now time.Time) (bool, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return false, err
	}
	if email == u.Email {
		return false, nil
	}
	u.Email, u.EmailVerifiedAt, u.UpdatedAt = email, nil, now
	return true, nil
}

func (u *User) SetPasswordHash(hash string, now time.Time) {
	u.PasswordHash = hash
	u.PasswordChangedAt = &now
	u.UpdatedAt = now
}

func (u *User) SetActive(active bool, now time.Time) bool {
	if u.Active == active {
		return false
	}
	u.Active, u.UpdatedAt = active, now
	return true
}

// ReplaceRoles returns whether the set changed.
func (u *User) ReplaceRoles(roleIDs []int64, now time.Time) bool {
	next := NormalizeRoleIDs(roleIDs)
	if slices.Equal(next, NormalizeRoleIDs(u.RoleIDs)) {
		return false
	}
	u.RoleIDs, u.UpdatedAt = next, now
	return true
}
