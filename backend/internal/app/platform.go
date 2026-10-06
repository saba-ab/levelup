// Package app is the composition root — the only place that knows every
// module exists (PRD §5). Wire assembles the Platform (infrastructure);
// module construction and selection stay hand-written (PRD §7.2).
package app

import (
	"context"
	"time"

	redis_rate "github.com/go-redis/redis_rate/v10"
	"gorm.io/gorm"

	"levelup/internal/config"
	"levelup/internal/platform/authn"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/idempotency"
	"levelup/internal/platform/modkit"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/redis"
	"levelup/internal/platform/telemetry"
	"levelup/internal/shared/validate"
)

const defaultCacheTTL = 5 * time.Minute

// Platform is everything below the modules: infrastructure only, zero
// business logic. Built by Wire (wire.go / wire_gen.go).
type Platform struct {
	Cfg        config.Config
	Tel        *telemetry.Telemetry
	DB         *postgres.DB
	Gorm       *gorm.DB
	RedisCore  redis.Core
	RedisCache redis.Cache
	Bus        bus.Bus
	Outbox     outbox.Store
	Clock      clock.Clock
	Validate   *validate.Validator
	Authz      authz.Enforcer
	Authn      *authn.Auth
	Idem       *idempotency.Store
	Limiter    *redis_rate.Limiter
}

// DepsFor mints one module's dependency bundle (PRD §7.2: "roughly ten lines
// and worth more than any Wire provider in this repo"). Everything scoping a
// module — schema-pinned GORM session, prefix-bound cache, tagged logger —
// happens here and only here.
func (p *Platform) DepsFor(module string) modkit.Deps {
	log := telemetry.ModuleLogger(p.Tel.Log, module)
	return modkit.Deps{
		DB:       postgres.NewModuleDB(p.Gorm, module),
		SQL:      p.DB,
		Cache:    redis.NewModuleCache(p.RedisCache, module, defaultCacheTTL, log),
		Redis:    p.RedisCore,
		Bus:      p.Bus,
		Outbox:   p.Outbox,
		Clock:    p.Clock,
		Log:      log,
		Validate: p.Validate,
		Authz:    p.Authz,
		Metrics:  p.Tel.Registry,
		Tracer:   p.Tel.Tracer,
	}
}

// Providers adapting config → platform primitives. Platform packages never
// import config (that would cycle through module config embeds and couple
// infrastructure to application shape); the adaptation happens here.

func provideTelemetry(cfg config.Config) (*telemetry.Telemetry, func(), error) {
	return telemetry.New(cfg.Env)
}

func provideDB(ctx context.Context, cfg config.Config) (*postgres.DB, func(), error) {
	return postgres.NewFromDSNs(ctx, cfg.DB.WriterDSN, cfg.DB.ReaderDSN, cfg.DB.MaxConns)
}

func provideRedisCore(cfg config.Config) redis.Core { return redis.NewCore(cfg.Redis.CoreAddr) }

func provideRedisCache(cfg config.Config) redis.Cache { return redis.NewCache(cfg.Redis.CacheAddr) }

func provideGorm(db *postgres.DB, tel *telemetry.Telemetry) (*gorm.DB, error) {
	return db.GormBase(tel.Log)
}

func provideBus() bus.Bus { return bus.NewInProcess() }

func provideOutbox(c clock.Clock) outbox.Store { return outbox.NewPostgres(c) }

func provideAuthn(cfg config.Config, core redis.Core, c clock.Clock, tel *telemetry.Telemetry) *authn.Auth {
	return authn.New(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL, core, c, tel.Log)
}

func provideIdempotency(db *postgres.DB) *idempotency.Store {
	return idempotency.NewStore(db.Writer())
}

func provideLimiter(core redis.Core) *redis_rate.Limiter { return redis.NewLimiter(core) }

func provideClock() clock.Clock { return clock.System() }

func provideValidator() *validate.Validator { return validate.New() }

// provideEnforcer builds the casbin enforcer over authz_svc with the
// redis-core watcher (PRD §7.6). Policy storage is platform-owned; no
// module ever sees casbin (depguard enforces it).
func provideEnforcer(g *gorm.DB, core redis.Core, tel *telemetry.Telemetry) (authz.Enforcer, func(), error) {
	enf, cleanup, err := authz.NewCasbin(postgres.NewModuleDB(g, "authz"), core.UniversalClient, tel.Log)
	if err != nil {
		return nil, nil, err
	}
	return enf, cleanup, nil
}
