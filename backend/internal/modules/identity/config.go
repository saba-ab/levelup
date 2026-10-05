package identity

// Config is embedded by the composition root with envPrefix "IDENTITY_".
// Access and refresh TTLs are platform settings (AUTH_ACCESS_TTL /
// AUTH_REFRESH_TTL), not module ones.
type Config struct {
	// BcryptCost for new and upgraded hashes. 12 matches the Laravel hashes
	// being migrated; weaker hashes are upgraded on the next login.
	BcryptCost int `env:"BCRYPT_COST" envDefault:"12"`
	// AllowSelfSignup gates POST /auth/register.
	AllowSelfSignup bool `env:"ALLOW_SELF_SIGNUP" envDefault:"true"`
}
