// Package domain holds identity's entities and invariants: tenants, users,
// the role catalogue and the password policy. No gorm, json or validate
// tags here (R17).
package domain

import "levelup/internal/shared/errs"

func coded(kind errs.Kind, msg, code string) error {
	return errs.WithCode(errs.New(kind, msg), code)
}

var (
	// ErrBadCredentials is the ONLY failure login and refresh report for an
	// unknown email, a wrong password, an inactive user or an inactive tenant,
	// so the response never tells an attacker which one it was.
	ErrBadCredentials = coded(errs.Unauthenticated, "invalid credentials", "invalid_credentials")

	ErrEmailTaken     = coded(errs.AlreadyExists, "email already taken", "email_taken")
	ErrSlugTaken      = coded(errs.AlreadyExists, "tenant slug already taken", "slug_taken")
	ErrUserNotFound   = coded(errs.NotFound, "user not found", "user_not_found")
	ErrTenantNotFound = coded(errs.NotFound, "tenant not found", "tenant_not_found")

	ErrInvalidName     = coded(errs.Invalid, "name must be 1 to 255 characters", "invalid_name")
	ErrInvalidEmail    = coded(errs.Invalid, "email is not valid", "invalid_email")
	ErrInvalidTimezone = coded(errs.Invalid, "timezone must be an IANA zone name", "invalid_timezone")

	ErrPasswordTooShort        = coded(errs.Invalid, "password must be at least 8 bytes", "password_too_short")
	ErrPasswordTooLong         = coded(errs.Invalid, "password must be at most 72 bytes", "password_too_long")
	ErrCurrentPasswordRequired = coded(errs.Invalid, "current_password is required to change your own password", "current_password_required")
	ErrCurrentPasswordWrong    = coded(errs.Invalid, "current_password does not match", "current_password_invalid")

	ErrPlatformRoleForbidden = coded(errs.PermissionDenied, "the platform admin role cannot be granted to tenant users", "cannot_assign_platform_role")
	ErrUnknownRole           = coded(errs.Invalid, "unknown role id", "unknown_role")
	ErrRoleEscalation        = coded(errs.PermissionDenied, "cannot grant a role above your own", "role_escalation")
	ErrInsufficientRank      = coded(errs.PermissionDenied, "target user outranks you", "insufficient_rank")

	ErrOwnerCannotBeDeleted     = coded(errs.Conflict, "the tenant owner cannot be deleted", "owner_cannot_be_deleted")
	ErrOwnerCannotBeDeactivated = coded(errs.Conflict, "the tenant owner cannot be deactivated", "owner_cannot_be_deactivated")
	ErrOwnerRoleRequired        = coded(errs.Conflict, "the tenant owner must keep the owner role", "owner_role_required")
	ErrCannotDeactivateSelf     = coded(errs.Conflict, "you cannot deactivate yourself", "cannot_deactivate_self")
	ErrNotTenantOwner           = coded(errs.PermissionDenied, "only the tenant owner may do this", "not_tenant_owner")
	ErrPlatformOnly             = coded(errs.PermissionDenied, "platform-level principals only", "platform_only")
	ErrSignupDisabled           = coded(errs.PermissionDenied, "self-signup is disabled", "signup_disabled")

	ErrVersionConflict = coded(errs.Conflict, "modified concurrently, retry", "version_conflict")
)
