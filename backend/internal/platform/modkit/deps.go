package modkit

import (
	goredis "github.com/redis/go-redis/v9"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/redis"
	"levelup/internal/shared/validate"
)

// Deps is the one bundle every module receives (PRD §6). Nothing
// module-specific leaks in: module settings are a typed struct owned by the
// module and passed to its constructor separately (PRD §7.3).
type Deps struct {
	// DB is ALREADY scoped to this module's schema via table prefix
	// (PRD §7.1): it cannot address another module's tables.
	DB *gorm.DB
	// SQL is the raw pgx pool pair for hot paths where generated SQL must
	// be read before it ships (PRD §7.1).
	SQL *postgres.DB
	// Cache is prefix-bound to this module (PRD §7.8.1, R42).
	Cache *redis.ModuleCache
	// Redis is redis-core, for module-owned ephemera with explicit TTLs.
	Redis goredis.UniversalClient

	Bus    bus.Bus
	Outbox outbox.Store

	Clock    clock.Clock
	Log      *zap.Logger // pre-tagged zap.String("module", name)
	Validate *validate.Validator
	Authz    authz.Enforcer

	Metrics *prometheus.Registry
	Tracer  trace.Tracer
}
