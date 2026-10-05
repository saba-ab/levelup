package app

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"

	"levelup/internal/config"
	"levelup/internal/platform/authn"
	"levelup/internal/platform/authz"
	authzmigrations "levelup/internal/platform/authz/migrations"
	"levelup/internal/platform/httpx"
	"levelup/internal/platform/idempotency"
	idmigrations "levelup/internal/platform/idempotency/migrations"
	"levelup/internal/platform/modkit"
	outboxmigrations "levelup/internal/platform/outbox/migrations"
	"levelup/internal/platform/postgres"
)

// App owns boot order and graceful shutdown (PRD §5 lifecycle.go).
type App struct {
	P       *Platform
	Modules []modkit.Module
	cleanup func()
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	p, cleanup, err := InitializePlatform(ctx, cfg)
	if err != nil {
		return nil, err
	}
	modules, err := buildModules(p, cfg)
	if err != nil {
		cleanup()
		return nil, err
	}
	return &App{P: p, Modules: modules, cleanup: cleanup}, nil
}

func (a *App) Close() {
	if a.cleanup != nil {
		a.cleanup()
	}
}

// Router mounts platform middleware, /health, and every enabled module under
// /api/v1. The router each module receives is already scoped (PRD §6).
func (a *App) Router() chi.Router {
	r := chi.NewRouter()
	// CORS first: a browser preflight must not be rate limited, logged as an
	// auth failure, or claimed by the idempotency store.
	r.Use(httpx.CORS(a.P.Cfg.HTTP.CORSAllowedOrigins))
	r.Use(httpx.BaseMiddleware(a.P.Tel.Log, a.P.Tel.Tracer, a.P.Tel.Registry)...)
	// Token PARSING is global (never rejects); rejection is per-route-group
	// via httpx.RequireAuth inside modules.
	r.Use(authn.Middleware(a.P.Authn.Issuer, a.P.Authn.Refresh, a.P.Tel.Log))
	// After authn so limits key on the principal when there is one (R38).
	r.Use(httpx.RateLimit(a.P.Limiter, a.P.Cfg.HTTP.RateLimitPerMinute, a.P.Tel.Log))
	// Idempotency-Key replay protection on non-GET (R11).
	r.Use(idempotency.Middleware(a.P.Idem, a.P.Tel.Log))
	r.Get("/health", a.handleHealth)
	r.Route("/api/v1", func(api chi.Router) {
		for _, m := range a.Modules {
			m.RegisterHTTP(api)
		}
	})
	return r
}

// handleHealth reports per-module readiness (R2): only enabled modules
// appear; any failing module degrades the whole answer to 503.
func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	type payload struct {
		Status  string            `json:"status"`
		Modules map[string]string `json:"modules"`
	}
	out := payload{Status: "ok", Modules: map[string]string{}}
	for _, m := range a.Modules {
		if err := m.Health(ctx); err != nil {
			out.Status = "degraded"
			out.Modules[m.Name()] = err.Error()
			continue
		}
		out.Modules[m.Name()] = "ok"
	}
	status := http.StatusOK
	if out.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	httpx.JSON(w, status, out)
}

// RunAPI serves the public and admin listeners until ctx is cancelled, then
// drains. Boot order: Starters → listeners. Shutdown order: listeners →
// Stoppers (reverse registration) → platform cleanup (reverse boot, R16).
func (a *App) RunAPI(ctx context.Context) error {
	log := a.P.Tel.Log

	for _, m := range a.Modules {
		if s, ok := m.(modkit.Starter); ok {
			if err := s.Start(ctx); err != nil {
				return err
			}
		}
	}

	pub := httpx.NewServer(a.P.Cfg.HTTP.Addr, a.Router())
	adm := httpx.NewServer(a.P.Cfg.HTTP.AdminAddr, a.P.Tel.AdminHandler())

	errCh := make(chan error, 2)
	go func() { errCh <- pub.ListenAndServe() }()
	go func() { errCh <- adm.ListenAndServe() }()
	log.Info("serving",
		zap.String("addr", a.P.Cfg.HTTP.Addr),
		zap.String("admin", a.P.Cfg.HTTP.AdminAddr),
		zap.Int("modules", len(a.Modules)))

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	drain, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpx.Shutdown(drain, pub)
	_ = httpx.Shutdown(drain, adm)

	for i := len(a.Modules) - 1; i >= 0; i-- {
		if s, ok := a.Modules[i].(modkit.Stopper); ok {
			_ = s.Stop(drain)
		}
	}
	log.Info("drained, exiting")
	return nil
}

// platformSchemas are platform-owned migrations applied before any module
// (R3 ordering). Uniform rule: every schema is <name>_svc with its own goose
// version table.
var platformSchemas = []struct {
	Name string
	FS   fs.FS
}{
	{"outbox", outboxmigrations.FS},
	{"authz", authzmigrations.FS},
	{"idempotency", idmigrations.FS},
}

// VerifyAuthz runs the boot-time catalogue drift check (R19). Serving
// binaries call it after New; cmd/migrate does not (on a first boot the
// policy table does not exist until it creates it).
func (a *App) VerifyAuthz() error {
	cat, ok := a.P.Authz.(authz.Cataloguer)
	if !ok {
		return nil // test doubles carry no catalogue
	}
	var declared []authz.Permission
	for _, m := range a.Modules {
		declared = append(declared, m.Permissions()...)
	}
	return authz.VerifyCatalogue(cat, declared)
}

// Migrate applies platform schemas then every enabled module's migrations in
// registry order (R3), and provisions the per-module DB roles. Module roles
// additionally get INSERT on the outbox: writing the event inside the owning
// transaction is the whole design (PRD §4 P3), so the outbox table is the
// one deliberate platform exception to schema privacy.
func Migrate(ctx context.Context, cfg config.Config) error {
	app, err := New(ctx, cfg)
	if err != nil {
		return err
	}
	defer app.Close()

	dsn := cfg.MigrateDSN()
	for _, p := range platformSchemas {
		if err := postgres.Apply(ctx, dsn, p.Name, p.FS); err != nil {
			return err
		}
		app.P.Tel.Log.Info("migrated platform schema", zap.String("schema", p.Name))
	}
	for _, m := range app.Modules {
		fsys := m.Migrations()
		if fsys == nil {
			continue
		}
		var goMigrations []*goose.Migration
		if gm, ok := m.(postgres.GoMigrator); ok {
			goMigrations = gm.GoMigrations()
		}
		if err := postgres.Apply(ctx, dsn, m.Name(), fsys, goMigrations...); err != nil {
			return err
		}
		if err := postgres.CreateModuleRole(ctx, dsn, m.Name()); err != nil {
			return err
		}
		if err := postgres.GrantOutboxWrite(ctx, dsn, m.Name()); err != nil {
			return err
		}
		app.P.Tel.Log.Info("migrated", zap.String("module", m.Name()))
	}
	return nil
}
