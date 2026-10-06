package identity

import "time"

// Config is embedded by the composition root with envPrefix "IDENTITY_".
// Access and refresh TTLs are platform settings (AUTH_ACCESS_TTL /
// AUTH_REFRESH_TTL), not module ones.
type Config struct {
	// BcryptCost for new and upgraded hashes. 12 matches the Laravel hashes
	// being migrated; weaker hashes are upgraded on the next login.
	BcryptCost int `env:"BCRYPT_COST" envDefault:"12"`
	// AllowSelfSignup gates POST /auth/register.
	AllowSelfSignup bool `env:"ALLOW_SELF_SIGNUP" envDefault:"true"`

	// PortalURL is the base of the links in account emails
	// (/reset-password, /accept-invite, /verify-email).
	PortalURL string `env:"PORTAL_URL" envDefault:"http://localhost:5173"`
	// Lifetimes of the emailed single-use tokens.
	PasswordResetTTL     time.Duration `env:"PASSWORD_RESET_TTL" envDefault:"1h"`
	InvitationTTL        time.Duration `env:"INVITATION_TTL" envDefault:"168h"`
	EmailVerificationTTL time.Duration `env:"EMAIL_VERIFICATION_TTL" envDefault:"48h"`
	// PasswordResetsPerHour caps forgot-password mails per address and
	// verification resends per user (fixed hour window on redis-core,
	// fails open).
	PasswordResetsPerHour int `env:"PASSWORD_RESETS_PER_HOUR" envDefault:"3"`
}
