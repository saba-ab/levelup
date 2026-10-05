package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"myapp/internal/config"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	// Clear every key this package reads: `task` loads .env via dotenv, so
	// an ambient DB_MIGRATE_DSN (or any other) would otherwise make these
	// assertions depend on the developer's local file.
	for _, k := range []string{
		"APP_ENV", "MODULES_ENABLED",
		"HTTP_ADDR", "HTTP_ADMIN_ADDR",
		"DB_WRITER_DSN", "DB_READER_DSN", "DB_MIGRATE_DSN", "DB_MAX_CONNS",
		"JWT_SECRET", "JWT_ACCESS_TTL", "JWT_REFRESH_TTL",
		"REDIS_CORE_ADDR", "REDIS_CACHE_ADDR",
		"RABBIT_URL", "RABBIT_MGMT_URL", "RABBIT_MGMT_USER", "RABBIT_MGMT_PASS",
		"OUTBOX_MAX_PUBLISH_ATTEMPTS",
	} {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
	for k, v := range map[string]string{
		"APP_ENV":          "test",
		"MODULES_ENABLED":  "billing,orders",
		"DB_WRITER_DSN":    "postgres://app:app@localhost:6432/app?sslmode=disable",
		"JWT_SECRET":       "0123456789abcdef0123456789abcdef",
		"REDIS_CORE_ADDR":  "localhost:6380",
		"REDIS_CACHE_ADDR": "localhost:6381",
		"RABBIT_URL":       "amqp://guest:guest@localhost:5672/",
	} {
		t.Setenv(k, v)
	}
}

func TestLoadHappyPath(t *testing.T) {
	setValidEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, []string{"billing", "orders"}, cfg.ModulesEnabled)
	require.Equal(t, int32(8), cfg.DB.MaxConns, "default applies")
	require.Equal(t, ":8080", cfg.HTTP.Addr, "default applies")
	// Migrations fall back to the writer DSN when unset.
	require.Equal(t, cfg.DB.WriterDSN, cfg.MigrateDSN())
}

func TestLoadFailsNamingMissingKey(t *testing.T) {
	setValidEnv(t)
	t.Setenv("DB_WRITER_DSN", "")

	_, err := config.Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "DB_WRITER_DSN",
		"boot failure must name the env key (R15)")
}

// A template boots with no business modules: an empty MODULES_ENABLED is
// a valid, empty registry, not a misconfiguration (R2).
func TestEmptyModulesEnabledIsAnEmptyRegistry(t *testing.T) {
	setValidEnv(t)
	t.Setenv("MODULES_ENABLED", "")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Empty(t, cfg.ModulesEnabled)
}

func TestModulesEnabledIgnoresBlankEntries(t *testing.T) {
	setValidEnv(t)
	t.Setenv("MODULES_ENABLED", "billing, ,orders,")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, []string{"billing", "orders"}, cfg.ModulesEnabled)
}

func TestLoadFailsOnShortJWTSecret(t *testing.T) {
	setValidEnv(t)
	t.Setenv("JWT_SECRET", "too-short")

	_, err := config.Load()
	require.Error(t, err)
}

// R23: the dispatcher's attempt cap is configuration, with a default that
// spans a few minutes of backoff before a row is parked.
func TestOutboxMaxPublishAttemptsDefaultsToTen(t *testing.T) {
	setValidEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, 10, cfg.Outbox.MaxPublishAttempts)
}

func TestOutboxMaxPublishAttemptsRejectsZero(t *testing.T) {
	setValidEnv(t)
	t.Setenv("OUTBOX_MAX_PUBLISH_ATTEMPTS", "0")

	_, err := config.Load()
	require.Error(t, err, "zero attempts would park every nacked row on first refusal")
}

func TestLoadFailsOnMalformedDuration(t *testing.T) {
	setValidEnv(t)
	t.Setenv("JWT_ACCESS_TTL", "fifteen minutes")

	_, err := config.Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), "JWT_ACCESS_TTL")
}
