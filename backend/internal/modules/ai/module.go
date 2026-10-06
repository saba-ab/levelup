// Package ai drafts badges, levels, missions, rewards, rules and segments
// with Claude for the portal AI Hub. Drafts are validated server-side and
// returned, never persisted: the portal creates them through the owning
// modules' normal endpoints. The module stores only per-tenant daily usage.
// Public surface: this file, config.go and contracts/.
package ai

import (
	"context"
	"encoding/json"
	"io/fs"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"

	"levelup/internal/modules/ai/contracts"
	"levelup/internal/modules/ai/internal/app"
	"levelup/internal/modules/ai/internal/llm"
	"levelup/internal/modules/ai/internal/repo"
	"levelup/internal/modules/ai/internal/transport"
	"levelup/internal/modules/ai/migrations"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/platform/modkit"
	"levelup/internal/shared/errs"
)

type Module struct {
	svc  *app.Service
	deps modkit.Deps
}

// New wires the module. Constructors do no I/O. ai reads no other module:
// tenant context (event types, badges, ...) comes from the client.
func New(d modkit.Deps, cfg Config) *Module {
	gen := llm.New(llm.Options{
		APIKey:          cfg.AnthropicAPIKey,
		BaseURL:         cfg.AnthropicBaseURL,
		Model:           cfg.Model,
		MaxTokens:       cfg.maxTokens(),
		Effort:          cfg.Effort,
		RefusalFallback: cfg.RefusalFallback,
		Timeout:         cfg.Timeout,
		MaxRetries:      cfg.MaxRetries,
	})
	svc := app.NewService(repo.NewPostgres(d.DB), gen, d.Authz, d.DB, d.Clock, d.Log, app.Settings{
		Enabled:    strings.TrimSpace(cfg.AnthropicAPIKey) != "",
		Model:      cfg.Model,
		DailyLimit: cfg.DailyLimit,
	})
	return &Module{svc: svc, deps: d}
}

func (m *Module) Name() string { return contracts.Module }

func (m *Module) Migrations() fs.FS { return migrations.FS }

// GoMigrations satisfies postgres.GoMigrator: permission catalogue + grants.
func (m *Module) GoMigrations() []*goose.Migration { return migrations.Go() }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc, m.deps.Validate).Mount(r)
}

func (m *Module) Subscriptions() []bus.Subscription {
	return []bus.Subscription{
		{Topic: identitycontracts.TopicTenantDeleted, Group: contracts.Module, Handler: m.onTenantDeleted},
	}
}

func (m *Module) onTenantDeleted(ctx context.Context, e bus.Envelope) error {
	var ev identitycontracts.TenantDeletedV1
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return errs.Wrap(errs.Invalid, "undecodable tenant.deleted.v1", err)
	}
	return m.svc.PurgeTenant(ctx, ev.TenantID)
}

func (m *Module) Jobs() []jobs.Job { return nil }

func (m *Module) Health(ctx context.Context) error {
	sqlDB, err := m.deps.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (m *Module) Permissions() []authz.Permission { return contracts.AllPermissions }

var _ modkit.Module = (*Module)(nil)
