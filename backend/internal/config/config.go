// Package config is the single translation from environment to typed values,
// validated at boot (PRD §7.3). A malformed value kills the process before the
// listener binds instead of surfacing as a zero value three days later.
package config

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"

	"levelup/internal/modules/activity"
	"levelup/internal/modules/badges"
	"levelup/internal/modules/eventcatalog"
	"levelup/internal/modules/identity"
	"levelup/internal/modules/leaderboards"
	"levelup/internal/modules/missions"
	"levelup/internal/modules/player"
	"levelup/internal/modules/points"
	"levelup/internal/modules/program"
	"levelup/internal/modules/progression"
	"levelup/internal/modules/rewards"
	"levelup/internal/modules/rules"
	"levelup/internal/modules/streaks"
	"levelup/internal/shared/validate"
)

type Config struct {
	Env string `env:"APP_ENV" envDefault:"local"`
	// ModulesEnabled selects and orders the registry (R2). Empty is a valid,
	// empty registry: the template boots with no business modules.
	ModulesEnabled []string `env:"MODULES_ENABLED" envSeparator:","`

	HTTP struct {
		Addr      string `env:"ADDR" envDefault:":8080"`
		AdminAddr string `env:"ADMIN_ADDR" envDefault:":8081"`
		// RateLimitPerMinute is per principal (or per IP when anonymous).
		// A hardcoded value is wrong here: the right number is a product
		// decision, and load tests need to raise it (R24 makes it
		// per-module later).
		RateLimitPerMinute int `env:"RATE_LIMIT_PER_MINUTE" envDefault:"600"`
	} `envPrefix:"HTTP_"`

	DB struct {
		WriterDSN string `env:"WRITER_DSN,required,notEmpty"`
		// ReaderDSN empty → reads fall back to the writer pool.
		ReaderDSN string `env:"READER_DSN"`
		// MigrateDSN must bypass PgBouncer: goose sets search_path, which is
		// unsafe through a transaction pooler. Empty → writer DSN.
		MigrateDSNRaw string `env:"MIGRATE_DSN"`
		MaxConns      int32  `env:"MAX_CONNS" envDefault:"8"`
	} `envPrefix:"DB_"`

	JWT struct {
		Secret     string        `env:"SECRET,required,notEmpty" validate:"min=32"`
		AccessTTL  time.Duration `env:"ACCESS_TTL" envDefault:"15m"`
		RefreshTTL time.Duration `env:"REFRESH_TTL" envDefault:"720h"`
	} `envPrefix:"JWT_"`

	Redis struct {
		CoreAddr  string `env:"CORE_ADDR,required,notEmpty"`
		CacheAddr string `env:"CACHE_ADDR,required,notEmpty"`
	} `envPrefix:"REDIS_"`

	Rabbit struct {
		URL      string `env:"URL,required,notEmpty"`
		MgmtURL  string `env:"MGMT_URL" envDefault:"http://localhost:15672"`
		MgmtUser string `env:"MGMT_USER" envDefault:"guest"`
		MgmtPass string `env:"MGMT_PASS" envDefault:"guest"`
	} `envPrefix:"RABBIT_"`

	Outbox struct {
		// MaxPublishAttempts caps broker nacks per row before the dispatcher
		// parks it in outbox_svc.dead_letters (R23). With the dispatcher's
		// backoff (1s doubling to 60s) ten attempts span about four minutes.
		MaxPublishAttempts int `env:"MAX_PUBLISH_ATTEMPTS" envDefault:"10" validate:"min=1"`
	} `envPrefix:"OUTBOX_"`

	// Module-owned config structs embed here (PRD §7.3): the module defines
	// the struct and its env tags; nothing in modkit ever learns a
	// module-specific key. One line per module, for example:
	//
	//	Billing billing.Config `envPrefix:"BILLING_"`
	Identity     identity.Config     `envPrefix:"IDENTITY_"`
	Player       player.Config       `envPrefix:"PLAYER_"`
	EventCatalog eventcatalog.Config `envPrefix:"EVENTCATALOG_"`
	Program      program.Config      `envPrefix:"PROGRAM_"`
	Points       points.Config       `envPrefix:"POINTS_"`
	Badges       badges.Config       `envPrefix:"BADGES_"`
	Progression  progression.Config  `envPrefix:"PROGRESSION_"`
	Streaks      streaks.Config      `envPrefix:"STREAKS_"`
	Missions     missions.Config     `envPrefix:"MISSIONS_"`
	Rewards      rewards.Config      `envPrefix:"REWARDS_"`
	Leaderboards leaderboards.Config `envPrefix:"LEADERBOARDS_"`
	Activity     activity.Config     `envPrefix:"ACTIVITY_"`
	Rules        rules.Config        `envPrefix:"RULES_"`
}

func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return c, annotateWithEnvKeys(err, reflect.TypeOf(c))
	}
	c.ModulesEnabled = compactNames(c.ModulesEnabled)
	if err := validate.New().Struct(c); err != nil {
		return c, err
	}
	return c, nil
}

// compactNames trims each entry and drops blanks, so "a, ,b," and "" mean
// what a human means by them rather than producing a module named "".
func compactNames(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// annotateWithEnvKeys rewrites env's parse errors — which name Go fields —
// so the message names the actual environment key (R15: boot failures name
// the key someone has to go fix).
func annotateWithEnvKeys(err error, t reflect.Type) error {
	msg := err.Error()
	var walk func(t reflect.Type, prefix string)
	keys := map[string]string{} // field name → env key
	walk = func(t reflect.Type, prefix string) {
		for f := range t.Fields() {
			ft := f.Type
			if ft.Kind() == reflect.Struct && ft != reflect.TypeFor[time.Time]() && f.Tag.Get("env") == "" {
				walk(ft, prefix+f.Tag.Get("envPrefix"))
				continue
			}
			if tag := f.Tag.Get("env"); tag != "" {
				key, _, _ := strings.Cut(tag, ",")
				if _, taken := keys[f.Name]; !taken {
					keys[f.Name] = prefix + key
				}
			}
		}
	}
	walk(t, "")
	for field, key := range keys {
		msg = strings.ReplaceAll(msg,
			fmt.Sprintf("on field %q", field),
			fmt.Sprintf("on %s (field %q)", key, field))
	}
	return fmt.Errorf("%s", msg)
}

// MigrateDSN returns the direct-to-Postgres DSN for goose.
func (c Config) MigrateDSN() string {
	if c.DB.MigrateDSNRaw != "" {
		return c.DB.MigrateDSNRaw
	}
	return c.DB.WriterDSN
}

// ReaderDSN falls back to the writer when no replica is configured.
func (c Config) ReaderDSN() string {
	if c.DB.ReaderDSN != "" {
		return c.DB.ReaderDSN
	}
	return c.DB.WriterDSN
}
