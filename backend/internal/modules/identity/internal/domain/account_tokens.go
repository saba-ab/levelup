package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"levelup/internal/shared/id"
)

// Account tokens are the single-use secrets mailed to a person: password
// reset, invitation and email verification links. 32 random bytes (256
// bits) are sent as base64url; only a SHA-256 of the token is stored, so a
// database leak does not leak usable links. That entropy makes a fast hash
// appropriate, exactly like API keys.
const accountTokenBytes = 32

// NewAccountToken returns the plaintext token (mailed once, never stored)
// and its hash.
func NewAccountToken() (plain, hash string, err error) {
	buf := make([]byte, accountTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	return plain, HashAccountToken(plain), nil
}

// HashAccountToken is the lookup key of a presented token.
func HashAccountToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// WellFormedAccountToken rejects anything not shaped like a token, so
// garbage never reaches the database.
func WellFormedAccountToken(plain string) bool {
	if len(plain) != base64.RawURLEncoding.EncodedLen(accountTokenBytes) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(plain)
	return err == nil
}

// PasswordReset is one emailed reset link. Email is the address the link
// went to: a reset requested before an email change does not survive it.
type PasswordReset struct {
	ID        string
	UserID    string
	Email     string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func NewPasswordReset(userID, email, tokenHash string, ttl time.Duration, now time.Time) PasswordReset {
	return PasswordReset{
		ID: id.NewID(), UserID: userID, Email: email, TokenHash: tokenHash,
		ExpiresAt: now.Add(ttl), CreatedAt: now,
	}
}

// Usable reports an unused, unexpired reset.
func (r PasswordReset) Usable(now time.Time) bool {
	return r.UsedAt == nil && now.Before(r.ExpiresAt)
}

// EmailVerification is one emailed verification link for Email.
type EmailVerification struct {
	ID        string
	UserID    string
	Email     string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func NewEmailVerification(userID, email, tokenHash string, ttl time.Duration, now time.Time) EmailVerification {
	return EmailVerification{
		ID: id.NewID(), UserID: userID, Email: email, TokenHash: tokenHash,
		ExpiresAt: now.Add(ttl), CreatedAt: now,
	}
}

func (v EmailVerification) Usable(now time.Time) bool {
	return v.UsedAt == nil && now.Before(v.ExpiresAt)
}

// InvitationStatus is derived, never stored.
type InvitationStatus string

const (
	InvitationPending  InvitationStatus = "pending"
	InvitationExpired  InvitationStatus = "expired"
	InvitationAccepted InvitationStatus = "accepted"
	InvitationRevoked  InvitationStatus = "revoked"
)

// Invitation lets a person join TenantID with RoleIDs by choosing their own
// password. Email is normalised; at most one invitation per (tenant, email)
// is open at a time (a re-invite revokes the previous one).
type Invitation struct {
	ID             string
	TenantID       string
	Email          string
	Name           string // optional suggestion for the accept form
	RoleIDs        []int64
	TokenHash      string
	InvitedBy      string
	ExpiresAt      time.Time
	AcceptedAt     *time.Time
	AcceptedUserID string
	RevokedAt      *time.Time
	CreatedAt      time.Time
}

// NewInvitation validates email and name; roles are checked by the service
// (they depend on the inviter).
func NewInvitation(tenantID, email, name, invitedBy, tokenHash string, roleIDs []int64, ttl time.Duration, now time.Time) (Invitation, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return Invitation{}, err
	}
	name = strings.TrimSpace(name)
	if name != "" {
		if name, err = cleanName(name); err != nil {
			return Invitation{}, err
		}
	}
	return Invitation{
		ID: id.NewID(), TenantID: tenantID, Email: email, Name: name,
		RoleIDs: NormalizeRoleIDs(roleIDs), TokenHash: tokenHash, InvitedBy: invitedBy,
		ExpiresAt: now.Add(ttl), CreatedAt: now,
	}, nil
}

func (i Invitation) Status(now time.Time) InvitationStatus {
	switch {
	case i.AcceptedAt != nil:
		return InvitationAccepted
	case i.RevokedAt != nil:
		return InvitationRevoked
	case !now.Before(i.ExpiresAt):
		return InvitationExpired
	}
	return InvitationPending
}

// Revoke is idempotent; an accepted invitation cannot be revoked.
func (i *Invitation) Revoke(now time.Time) (bool, error) {
	switch {
	case i.AcceptedAt != nil:
		return false, ErrInvitationAccepted
	case i.RevokedAt != nil:
		return false, nil
	}
	i.RevokedAt = &now
	return true, nil
}

// Accept marks a pending invitation as used by userID.
func (i *Invitation) Accept(userID string, now time.Time) error {
	if i.Status(now) != InvitationPending {
		return ErrInvalidInvitationToken
	}
	i.AcceptedAt, i.AcceptedUserID = &now, userID
	return nil
}
