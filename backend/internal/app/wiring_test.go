package app

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"levelup/internal/config"
	"levelup/internal/platform/authn"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/redis"
	"levelup/internal/platform/telemetry"
	"levelup/internal/shared/validate"
)

// allModules is the production MODULES_ENABLED, in dependency order.
var allModules = []string{
	"identity", "player", "eventcatalog", "program", "points", "badges", "progression", "streaks", "missions", "rewards", "leaderboards", "activity", "rules", "segments", "analytics", "webhooks", "notifications", "ai",
}

// offlinePlatform builds a Platform whose clients never dial: constructors
// do no I/O (PRD §6), so the whole registry can be built and routed without
// Postgres, Redis or RabbitMQ.
func offlinePlatform(t *testing.T) (*Platform, config.Config) {
	t.Helper()
	t.Setenv("DB_WRITER_DSN", "postgres://x:x@127.0.0.1:1/x?sslmode=disable")
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("REDIS_CORE_ADDR", "127.0.0.1:1")
	t.Setenv("REDIS_CACHE_ADDR", "127.0.0.1:2")
	t.Setenv("RABBIT_URL", "amqp://127.0.0.1:1/")
	t.Setenv("MODULES_ENABLED", strings.Join(allModules, ","))
	cfg, err := config.Load()
	require.NoError(t, err)

	sqlDB, err := sql.Open("pgx", cfg.DB.WriterDSN) // lazy: no connection is made
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	g, err := gorm.Open(gormpg.New(gormpg.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)

	tel, cleanup, err := telemetry.New("test")
	require.NoError(t, err)
	t.Cleanup(cleanup)

	c := clock.System()
	core := redis.NewCore(cfg.Redis.CoreAddr)
	return &Platform{
		Cfg:        cfg,
		Tel:        tel,
		Gorm:       g,
		RedisCore:  core,
		RedisCache: redis.NewCache(cfg.Redis.CacheAddr),
		Bus:        bus.NewInProcess(),
		Outbox:     outbox.NewPostgres(c),
		Clock:      c,
		Validate:   validate.New(),
		Authz:      authz.AllowAll{},
		Authn:      authn.New(cfg.JWT.Secret, 15*time.Minute, time.Hour, core, c, tel.Log),
	}, cfg
}

// TestAllModulesWireAndRoute builds the full registry and mounts every
// module on one router: chi panics on conflicting mounts, so two modules
// claiming the same prefix fail here instead of at boot in production.
func TestAllModulesWireAndRoute(t *testing.T) {
	p, cfg := offlinePlatform(t)
	mods, err := buildModules(p, cfg)
	require.NoError(t, err)
	require.Len(t, mods, len(allModules))

	r := chi.NewRouter()
	require.NotPanics(t, func() {
		r.Route("/api/v1", func(api chi.Router) {
			for _, m := range mods {
				m.RegisterHTTP(api)
			}
		})
	})

	seen := map[string]bool{}
	require.NoError(t, chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		key := method + " " + strings.ReplaceAll(route, "/*", "")
		require.Falsef(t, seen[key], "route registered twice: %s", key)
		seen[key] = true
		return nil
	}))
	require.NotEmpty(t, seen)
}

// TestModuleDeclarationsAreUnique: job names and permission keys are global
// namespaces; a collision would silently route work to the wrong module.
func TestModuleDeclarationsAreUnique(t *testing.T) {
	p, cfg := offlinePlatform(t)
	mods, err := buildModules(p, cfg)
	require.NoError(t, err)

	jobsSeen, permsSeen := map[string]string{}, map[string]string{}
	for _, m := range mods {
		for _, j := range m.Jobs() {
			prev, dup := jobsSeen[j.Name]
			require.Falsef(t, dup, "job %q declared by %s and %s", j.Name, prev, m.Name())
			require.Truef(t, strings.HasPrefix(j.Name, m.Name()+"."), "job %q of %s must be prefixed with the module name", j.Name, m.Name())
			jobsSeen[j.Name] = m.Name()
		}
		for _, perm := range m.Permissions() {
			prev, dup := permsSeen[perm.Key()]
			require.Falsef(t, dup, "permission %q declared by %s and %s", perm.Key(), prev, m.Name())
			require.Equalf(t, m.Name(), perm.Module, "permission %q declared by %s", perm.Key(), m.Name())
			permsSeen[perm.Key()] = m.Name()
		}
		for _, s := range m.Subscriptions() {
			require.Equalf(t, m.Name(), s.Group, "subscription %s of %s uses group %q", s.Topic, m.Name(), s.Group)
			require.NotNil(t, s.Handler)
		}
	}
	names := make([]string, 0, len(jobsSeen))
	for n := range jobsSeen {
		names = append(names, n)
	}
	sort.Strings(names)
	t.Logf("jobs: %v", names)
}

// TestEveryRouteReachesItsHandler dispatches one request per registered
// route through the full router. Several modules hang routes under
// /players/{playerID}/... next to player's own /players mount; a 404 or 405
// here means a mount swallowed another module's route.
func TestEveryRouteReachesItsHandler(t *testing.T) {
	p, cfg := offlinePlatform(t)
	mods, err := buildModules(p, cfg)
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := authz.Into(req.Context(), authz.Principal{
				UserID: "0192a6d0-0000-7000-8000-000000000001", TenantID: "0192a6d0-0000-7000-8000-000000000002", RoleIDs: []int64{2},
			})
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Route("/api/v1", func(api chi.Router) {
		for _, m := range mods {
			m.RegisterHTTP(api)
		}
	})

	const uuid = "0192a6d0-0000-7000-8000-0000000000aa"
	param := regexp.MustCompile(`\{[^}]+\}`)
	require.NoError(t, chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		path := param.ReplaceAllString(strings.ReplaceAll(route, "/*", ""), uuid)
		path = strings.TrimSuffix(path, "/")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusMethodNotAllowed || (rec.Code == http.StatusNotFound && rec.Body.Len() == len("404 page not found\n")) {
			t.Errorf("%s %s (route %s) did not reach a handler: %d %s", method, path, route, rec.Code, rec.Body.String())
		}
		return nil
	}))
}
