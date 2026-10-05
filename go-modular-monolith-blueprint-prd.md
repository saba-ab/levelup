# PRD — Go Modular Monolith Blueprint

**Status:** Draft v1
**Owner:** Saba
**Type:** Internal engineering blueprint (reusable template repo)
**Target:** Single deployable Go binary set serving ~7k RPS peak, structured so modules can be extracted later without rewriting business logic.

---

## 1. Problem Statement

Go projects at this scale fail in one of two directions. Either they start as a flat `handlers/services/models` layout that becomes an untraceable dependency graph by month six, or they start as microservices and pay distributed-systems tax before there is a single paying user. Both failures are expensive and both are avoidable.

The cost of getting this wrong is not measured in refactor hours — it is measured in the moment a genuine scaling need arrives (one module needs GPUs, one team needs deploy independence) and extraction turns out to be a two-quarter project because of cross-module joins and shared transactions that nobody flagged at the time.

This blueprint defines a repository structure, a module contract, and a set of mechanically-enforced boundaries that make a Go monolith cheap to build now and cheap to split later — without paying for the split up front.

---

## 2. Goals

| # | Goal | How we know it succeeded |
|---|---|---|
| G1 | A new module can be scaffolded and wired in under 15 minutes | `task new-module NAME=x` produces a compiling, route-registered, test-covered module |
| G2 | Cross-module boundary violations fail CI, not code review | Compiler rejects illegal imports; `test/arch` + `depguard` catch the rest; zero boundary violations reachable on `main` |
| G3 | Extracting a module to a separate binary requires changing one adapter and one config value | Demonstrated in the blueprint by shipping one module with both a local and remote adapter behind the same interface |
| G4 | The blueprint sustains 7,000 RPS peak on commodity hardware with p99 < 200ms for cached reads | Load test in `test/load` against the reference module, results committed |
| G5 | Every module is independently disableable at runtime | Setting `MODULES_ENABLED=user,wallet` boots without billing; health endpoint reflects it |

---

## 3. Non-Goals

| Non-goal | Why |
|---|---|
| Microservice deployment topology | Deliberate. The blueprint makes extraction cheap; it does not perform it. Splitting without a measured driver multiplies availability failure and buys nothing at 7k RPS. |
| A DI framework (wire, fx, dig) | Manual wiring stays readable to ~30 modules and keeps the composition root greppable. Reconsider past 30. |
| Full hexagonal/clean-architecture layering in every module | Ceremony for thin domains. Layering is offered as an optional shape for rich modules only; CRUD modules stay flat. |
| gRPC / GraphQL transports in v1 | The module contract has a slot for additional transports. Wiring them is a follow-on. |
| Kafka, ClickHouse, CQRS read models | Not justified at this load. RabbitMQ is the broker (§7.9); `bus.Bus` stays an interface so a log-structured broker remains possible if per-partition ordering or replay ever becomes a requirement. |
| Multi-tenancy | Orthogonal concern. Blueprint must not *prevent* schema-per-tenant, but does not implement it. |

---

## 4. Core Principles

These are the load-bearing decisions. Everything downstream follows from them.

**P1 — Module privacy is compiler-enforced, not convention-enforced.**
Each module's implementation lives under `modules/<name>/internal/`. Go's `internal/` rule means no package outside `modules/<name>/` can import it. Not "should not" — *cannot compile*. This is the single highest-leverage structural decision in the blueprint and it costs nothing.

**P2 — Producers own event schemas; consumers own sync interfaces.**
- Outbound events live in `modules/<name>/contracts/`. One definition, imported by subscribers. Data only, zero dependencies.
- Inbound sync dependencies are declared by the *consumer* as a local interface in its own `ports.go`, with a local adapter that bridges to the provider. The consumer never imports the provider's service type.

This asymmetry is deliberate. Events have exactly one authoritative schema. Sync dependencies must be swappable for a network client without touching the consumer's logic.

**P3 — One transaction never spans two modules.**
Cross-module writes go through the outbox, which is the **only** publisher to RabbitMQ. A service that calls the broker directly has re-created the dual-write problem the outbox exists to solve: the row commits and the publish fails, or the publish succeeds and the transaction rolls back. Enforced by depguard — no module may import `platform/rabbit`.

**P4 — Schema per module.**
`user_svc`, `wallet_svc`. Each module's DB role sees only its own schema via `search_path`. Postgres rejects a cross-module join at runtime, so violations surface in integration tests. `pg_dump --schema=wallet_svc` is the extraction path.

**P5 — No foreign keys across module boundaries.**
`wallets.user_id` is a bare `uuid`. FKs cannot cross databases; having one now means dropping it under pressure later.

**P6 — Boring by default.**
`chi` over the stdlib router, `pgx` under GORM, `zap`, and nothing else that is not in §7. The stack is locked; every *addition* to it needs a written justification in `docs/adr/`. Where a chosen tool makes a §4 principle easy to violate — GORM most of all — the containment mechanism is mandatory and mechanically enforced, not left to review.

---

## 5. Repository Structure

```
myapp/
├── cmd/
│   ├── api/main.go                     # HTTP server binary
│   ├── worker/main.go                  # rabbit consumers: events + jobs
│   ├── dispatcher/main.go              # outbox → RabbitMQ publisher
│   ├── scheduler/main.go               # cron. DEPLOYED AT replicas=1 (§7.10)
│   ├── migrate/main.go                 # runs all module migrations in order
│   └── modgen/                         # scaffolds a new module (see §6.3)
│       ├── main.go
│       └── stubs/module/               # embed.FS — templates ship in the binary
│           ├── module.go.stub
│           ├── ports.go.stub
│           ├── contracts/
│           │   ├── events.go.stub
│           │   └── topics.go.stub
│           ├── internal/
│           │   ├── app/service.go.stub
│           │   ├── app/service_test.go.stub
│           │   ├── repo/postgres.go.stub
│           │   └── transport/http.go.stub
│           └── migrations/
│               ├── 0001_init.up.sql.stub
│               └── embed.go.stub
│
├── internal/
│   ├── app/                            # COMPOSITION ROOT — the only place that
│   │   ├── app.go                      #   knows every module exists
│   │   ├── wire.go                     # //go:build wireinject — platform graph
│   │   ├── wire_gen.go                 # generated; never hand-edited
│   │   ├── platform.go                 # Platform struct + DepsFor(module)
│   │   ├── registry.go                 # explicit []modkit.Module — HAND-WRITTEN
│   │   └── lifecycle.go                # boot order, graceful shutdown
│   │
│   ├── config/
│   │   ├── config.go                   # env → typed struct, validated at boot
│   │   └── config_test.go
│   │
│   ├── platform/                       # infrastructure. ZERO business logic.
│   │   ├── modkit/
│   │   │   ├── module.go               # the Module interface (see §6)
│   │   │   └── deps.go                 # what every module receives
│   │   ├── postgres/
│   │   │   ├── db.go                   # Writer/Reader pgx pools
│   │   │   ├── gorm.go                 # NewModuleDB — per-module table prefix
│   │   │   ├── tx.go                   # InTx helper, tx-aware ctx
│   │   │   └── migrate.go              # goose runner, per-schema version table
│   │   ├── authz/
│   │   │   ├── authz.go                # Permission, Principal, Enforcer iface
│   │   │   ├── casbin.go               # ONLY file importing casbin
│   │   │   ├── model.conf              # embedded RBAC model
│   │   │   └── watcher.go              # redis-watcher — multi-replica reload
│   │   ├── authn/                      # JWT issue/verify, Principal → ctx
│   │   ├── redis/
│   │   │   ├── client.go               # core + cache clients (§7.8)
│   │   │   ├── module.go               # ModuleCache — prefix-bound per module
│   │   │   ├── lock.go                 # SETNX + compare-and-delete release
│   │   │   └── ratelimit.go            # redis_rate GCRA limiter
│   │   ├── bus/                        # EVENTS — fanout, many consumers
│   │   │   ├── bus.go                  # Publish/Subscribe interface
│   │   │   ├── rabbit.go               # topic exchange `events`, quorum queues
│   │   │   ├── inprocess.go            # test double only — NOT a prod path
│   │   │   └── envelope.go             # event_id, topic, trace ctx, payload
│   │   ├── jobs/                       # WORK — one consumer, retries, priority
│   │   │   ├── queue.go                # Enqueue/Consume interface
│   │   │   ├── rabbit.go               # direct exchange `jobs`, x-max-priority
│   │   │   ├── retry.go                # DLX + TTL backoff, bounded → DLQ
│   │   │   └── worker.go               # prefetch, panic recovery
│   │   ├── scheduler/
│   │   │   └── cron.go                 # robfig/cron v3 — ENQUEUES, never executes
│   │   ├── rabbit/
│   │   │   ├── conn.go                 # connection + channel pool, reconnect
│   │   │   └── topology.go             # exchanges, queues, bindings, DLX
│   │   ├── outbox/
│   │   │   ├── store.go                # Publish(ctx, tx, topic, payload)
│   │   │   └── dispatcher.go           # SKIP LOCKED poller → publisher confirms
│   │   ├── httpx/
│   │   │   ├── server.go               # timeouts, graceful shutdown
│   │   │   ├── middleware.go           # reqid, recover, logging, otel, ratelimit
│   │   │   ├── render.go               # JSON encode, problem+json errors
│   │   │   └── errors.go               # errs.Kind → HTTP status mapping
│   │   ├── idempotency/                # Idempotency-Key store + middleware
│   │   ├── telemetry/                  # zap, otel tracer, prom registry, pprof
│   │   └── clock/                      # injectable time source
│   │
│   ├── modules/
│   │   ├── user/
│   │   │   ├── contracts/              # ═══ PUBLIC. Importable by anyone. ═══
│   │   │   │   ├── events.go           #   UserRegisteredV1, UserUpdatedV1
│   │   │   │   ├── topics.go           #   TopicUserRegistered = "user.registered.v1"
│   │   │   │   ├── permissions.go      #   AllPermissions — module-owned (§7.6)
│   │   │   │   └── types.go            #   UserID, Status — data only
│   │   │   ├── internal/               # ═══ COMPILER-PRIVATE to user/ ═══
│   │   │   │   ├── domain/
│   │   │   │   │   ├── user.go         #   entity, invariants, value objects
│   │   │   │   │   └── errors.go
│   │   │   │   ├── app/
│   │   │   │   │   ├── service.go      #   use cases
│   │   │   │   │   └── service_test.go #   unit, fake repo, no DB
│   │   │   │   ├── repo/
│   │   │   │   │   ├── repository.go   #   interface (consumed by app)
│   │   │   │   │   ├── model.go        #   UNEXPORTED gorm models + mappers
│   │   │   │   │   ├── gorm.go         #   implementation
│   │   │   │   │   └── cached.go       #   decorator: singleflight + jitter
│   │   │   │   └── transport/
│   │   │   │       ├── http.go         #   handlers
│   │   │   │       └── dto.go          #   request/response, validation tags
│   │   │   ├── migrations/
│   │   │   │   ├── 0001_init.sql       #   goose SQL migration
│   │   │   │   ├── 0003_seed_perms.go  #   goose GO migration — typed seeding
│   │   │   │   └── embed.go            #   //go:embed *.sql
│   │   │   ├── config.go               # ═══ PUBLIC. env-tagged Config struct ═══
│   │   │   └── module.go               # ═══ PUBLIC. Constructor + Module impl ═══
│   │   │
│   │   ├── wallet/
│   │   │   ├── contracts/
│   │   │   ├── internal/
│   │   │   ├── ports.go                # ═══ what wallet NEEDS from others ═══
│   │   │   ├── adapters/
│   │   │   │   ├── user_local.go       #   in-process bridge → user.Service
│   │   │   │   └── user_remote.go      #   HTTP client + breaker + cache
│   │   │   ├── migrations/
│   │   │   └── module.go
│   │   │
│   │   └── notification/               # thin module: flat internal/, no domain/
│   │       ├── contracts/
│   │       ├── internal/
│   │       │   ├── service.go
│   │       │   ├── repo.go
│   │       │   └── http.go
│   │       └── module.go
│   │
│   └── shared/                         # cross-cutting, dependency-free
│       ├── errs/                       # Kind enum, wrapping, no HTTP knowledge
│       ├── id/                         # UUIDv7 wrapper
│       ├── money/                      # int64 minor units, never float
│       ├── pagination/                 # cursor encode/decode
│       └── validate/
│
├── test/
│   ├── arch/arch_test.go               # import-graph assertions
│   ├── integration/                    # testcontainers: real PG + Redis
│   ├── e2e/                            # full binary, black-box HTTP
│   ├── load/                           # k6 scripts + committed results
│   └── fixtures/
│
├── docs/
│   ├── adr/                            # 0001-modular-monolith.md, ...
│   ├── modules.md                      # auto-generated dependency graph
│   └── runbook.md
│
├── deploy/
│   ├── docker/
│   ├── k8s/
│   └── compose.yaml
│
├── tools/
│   └── go.mod                          # dev-time deps only — kept OUT of the
│                                       #   service module graph (see §6.3)
├── .golangci.yml                       # depguard rules are the enforcement
├── .mockery.yaml                       # ports only — see §7.7
├── Taskfile.yml                        # task runner — see Appendix
└── go.mod
```

### Structural notes

- **No `pkg/`.** Nothing here is a published library. `internal/` prevents accidental external imports.
- **Thin vs rich modules.** `notification` shows the thin shape: flat files under `internal/`, no `domain/app/repo/transport` split. Use the layered shape only when a module has real invariants worth isolating. Splitting a 200-line CRUD module into four packages is ceremony.
- **`module.go` at module root is the entire public surface** alongside `contracts/`. It exports a constructor and the `Module` implementation. Nothing else.

---

## 6. The Module Contract

Every module implements one interface. The composition root knows nothing about any module except this.

```go
// internal/platform/modkit/module.go
package modkit

type Module interface {
	// Name is the module's stable identifier. Used for config keys,
	// schema name, metrics labels, and health reporting.
	Name() string

	// Migrations returns this module's embedded SQL, applied into
	// schema <Name>_svc. Nil is valid for modules with no storage.
	Migrations() fs.FS

	// RegisterHTTP mounts this module's routes. The router passed in is
	// already scoped and has platform middleware applied.
	RegisterHTTP(r chi.Router)

	// Subscriptions declares the events this module consumes. The
	// composition root wires these to the bus; the module never
	// touches transport.
	Subscriptions() []bus.Subscription

	// Jobs declares background work: crons and queue consumers.
	Jobs() []jobs.Job

	// Health reports module-level readiness (its own deps only).
	Health(ctx context.Context) error

	// Permissions returns every permission this module defines. The
	// composition root aggregates these into the global catalogue and
	// fails the boot if a role in the DB grants a permission no enabled
	// module declares. See §7.6.
	Permissions() []authz.Permission
}

// Optional lifecycle hooks. Type-assert in the composition root.
type Starter interface{ Start(ctx context.Context) error }
type Stopper interface{ Stop(ctx context.Context) error }
```

Every module receives the same dependency bundle. Nothing module-specific leaks in.

```go
// internal/platform/modkit/deps.go
type Deps struct {
	DB       *gorm.DB          // ALREADY scoped to this module's schema (§7.2)
	SQL      *postgres.DB      // raw pgx pool: .Writer() / .Reader() for hot paths
	Cache    *redis.ModuleCache // prefix-bound to this module (§7.8.1)
	Redis    redis.UniversalClient // redis-core, for module-owned ephemera
	Bus      bus.Bus
	Outbox   outbox.Store
	Clock    clock.Clock
	Log      *zap.Logger       // pre-tagged zap.String("module", name)
	Validate *validate.Validator
	Authz    authz.Enforcer    // casbin behind an interface (§7.6)
	Metrics  *prometheus.Registry
	Tracer   trace.Tracer
}
```

`Deps` is assembled by Wire (§7.4). `DB` is a per-module `*gorm.DB` session carrying that module's table prefix — a module physically cannot address another module's tables through it. Module-specific settings are a separate typed struct owned by the module, not a field here (§7.3).

### Registry — the one place modules are listed

```go
// internal/app/registry.go
func buildModules(d modkit.Deps, cfg config.Config) ([]modkit.Module, error) {
	all := map[string]func() modkit.Module{
		"user":         func() modkit.Module { return user.New(d) },
		"wallet":       func() modkit.Module { return wallet.New(d) },
		"notification": func() modkit.Module { return notification.New(d) },
	}

	var out []modkit.Module
	for _, name := range cfg.ModulesEnabled { // MODULES_ENABLED=user,wallet
		ctor, ok := all[name]
		if !ok {
			return nil, fmt.Errorf("unknown module %q", name)
		}
		out = append(out, ctor())
	}
	return out, nil
}
```

Runtime-disableable modules are not a gimmick. They force honest boundaries: if disabling `billing` breaks `user`, the coupling is real and you have found it in a test rather than during an extraction.

### Module implementation

```go
// internal/modules/wallet/module.go
package wallet

type Module struct {
	svc  *app.Service
	deps modkit.Deps
}

func New(d modkit.Deps) *Module {
	repo := repo.NewCached(
		repo.NewPostgres(d.DB),
		d.Redis, 5*time.Minute,
	)

	// Local adapter today. Swap this ONE line for adapters.NewRemoteUserReader
	// and the module is extracted. Nothing below changes.
	users := adapters.NewLocalUserReader(d)

	return &Module{svc: app.NewService(repo, users, d.Outbox, d.Clock), deps: d}
}

func (m *Module) Name() string      { return "wallet" }
func (m *Module) Migrations() fs.FS { return migrations.FS }

func (m *Module) RegisterHTTP(r chi.Router) {
	transport.NewHandler(m.svc).Mount(r)
}

func (m *Module) Subscriptions() []bus.Subscription {
	return []bus.Subscription{{
		Topic:   usercontracts.TopicUserRegistered,
		Group:   "wallet",
		Handler: m.onUserRegistered, // idempotent by construction
	}}
}

func (m *Module) Jobs() []jobs.Job {
	return []jobs.Job{{Name: "wallet.reconcile", Schedule: "*/15 * * * *", Run: m.svc.Reconcile}}
}

func (m *Module) Health(ctx context.Context) error { return m.deps.DB.Ping(ctx) }
```

### Consumer-defined port

```go
// internal/modules/wallet/ports.go
package wallet

// UserReader is what wallet needs. It does not know user.Service exists.
type UserReader interface {
	ByID(ctx context.Context, id string) (UserSnapshot, error)
	ByIDs(ctx context.Context, ids []string) (map[string]UserSnapshot, error) // batch: prevents N+1 after extraction
}

// wallet's own projection of a user. NOT user's entity type.
type UserSnapshot struct {
	ID       string
	Status   string
	Currency string
}
```

Ship the batch method before you need it. Its absence is what turns a `JOIN` into 50 HTTP calls on extraction day.

---

### 6.3 Scaffolding — `cmd/modgen` on go-stub

**Decision: use [`github.com/binafy/go-stub`](https://github.com/binafy/go-stub) as the template engine behind `modgen`.** Rationale and trade-off in ADR-0007.

The scaffolder's job is narrow: render a stub tree into `internal/modules/<name>/`, substituting placeholders in *file names* as well as content, gofmt the result, and refuse to clobber. go-stub's `GenerateDirFS` does exactly this in one call, reads templates from any `io/fs.FS` (so stubs ship embedded in the binary), and defaults to not overwriting existing files.

```go
// cmd/modgen/main.go
package main

//go:embed stubs
var stubs embed.FS

func main() {
	name := flag.String("name", "", "module name, e.g. wallet")
	flag.Parse()
	if *name == "" {
		log.Fatal("-name required")
	}

	dst := filepath.Join("internal", "modules", stub.ToSnake(*name))
	if _, err := os.Stat(dst); err == nil {
		log.Fatalf("module %s already exists", *name)
	}

	err := stub.GenerateDirFS(stubs, "stubs/module", dst,
		stub.WithReplaces(map[string]any{
			"PACKAGE": stub.ToSnake(*name),            // wallet
			"NAME":    stub.ToPascal(*name),           // Wallet
			"SCHEMA":  stub.ToSnake(*name) + "_svc",   // wallet_svc
			"TOPIC":   stub.ToSnake(*name),            // wallet.created.v1
			"MODULE":  modulePath(),                   // parsed from go.mod
		}),
		stub.WithTrimSuffix(".stub"),
		stub.WithDelimiters("<<", ">>"),
		stub.WithStrict(),
		stub.WithFormat(),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("created %s\nadd to internal/app/registry.go:\n  %q: func() modkit.Module { return %s.New(d) },\n",
		dst, stub.ToSnake(*name), stub.ToSnake(*name))
}
```

Four options here are load-bearing and must not be dropped:

| Option | Why it is mandatory |
|---|---|
| `WithStrict()` | Default behaviour leaves unknown keys untouched rather than erroring. For a generator that is wrong: a stub typo (`<< NAEM >>`) would silently emit a file with literal delimiters, and `WithFormat()` then fails with a confusing parse error instead of naming the missing key. Strict mode returns `ErrMissingKeys` listing exactly what was unresolved. |
| `WithDelimiters("<<", ">>")` | go-stub's default `{{ }}` collides with Go `text/template`. The first module that scaffolds an email or HTML template needs literal `{{ }}` to survive rendering. Cheaper to decide now than to discover mid-sprint. |
| `WithFormat()` | Generated code must be gofmt-clean or `task lint` fails immediately after scaffolding, which trains people to ignore lint. |
| *(no `WithForce`)* | Default no-overwrite is correct. `modgen` is a tool someone will eventually run twice by accident; the explicit `os.Stat` guard above makes the failure message readable. |

**Dependency isolation.** go-stub is a dev-time tool and must never appear in the service module graph. Declare it in `tools/go.mod` (or via the Go 1.24+ `tool` directive) so `go mod tidy` in the main module never pulls it, and so nothing in `cmd/api` or `cmd/worker` can transitively import it. Because it is zero-dependency, vendoring it outright is an acceptable fallback if the dependency itself is unwanted.

**Acknowledged risk.** go-stub is a small single-maintainer project with no tagged releases. Code hygiene is good (pure stdlib, ~97% coverage, race-checked across platforms) and the blast radius is bounded: it runs only at development time, is not in the serving path, and the stdlib replacement — `fs.WalkDir` + `text/template` + `go/format` — is roughly 80 lines. What that replacement would cost is re-implementing the case helpers (`ToSnake`/`ToPascal`/`ToCamel`, with acronym boundaries handled: `HTTPServer` → `http_server`), the write policies, and the strict-mode error contract. Reassess only if the project goes unmaintained past a Go release that breaks it.

---

## 7. Technology Stack (locked)

| Concern | Tool | Notes |
|---|---|---|
| HTTP router | `go-chi/chi/v5` | Grouped middleware + `Mount` — the module contract depends on both. Resolves §13 Q1. |
| ORM / data access | `gorm.io/gorm` + `pgx` stdlib driver | Constrained by §7.2. Raw pgx pool stays available for hot paths. |
| DI | `google/wire` | Builds `Deps` at compile time. Module *selection* stays runtime (§7.4). |
| Migrations | `pressly/goose/v3` | Per-module version table; Go migrations for typed seeding. |
| Validation | `go-playground/validator/v10` | Transport layer only (§7.5). |
| Config | `caarlos0/env/v11` | Typed, validated at boot. Resolves §13 Q3. |
| Logging | `uber-go/zap` | JSON in prod, pre-tagged per module. |
| Authn | `golang-jwt/jwt/v5` | Access + refresh. |
| Authz | `casbin/v2` + `casbin/gorm-adapter/v3` | Behind `authz.Enforcer` (§7.6). |
| Cache / sessions / limits | `redis/go-redis/v9` + `go-redis/redis_rate/v10` | Two instances, different eviction policies (§7.8). |
| Broker | RabbitMQ via `rabbitmq/amqp091-go` | Events *and* jobs, two separate abstractions (§7.9). |
| Scheduling | `robfig/cron/v3` | Singleton `cmd/scheduler`; enqueues, never executes (§7.10). |
| Tracing / metrics | OpenTelemetry + `prometheus/client_golang` | Trace context crosses the outbox envelope (§7.11). |
| API docs | `swaggo/swag` + Bruno collections | Code-first, drift-checked against the router (§7.12). |
| Testing | `testify` · `mockery` · `testcontainers-go` · `k6` | §7.7. |
| Lint | `golangci-lint` | depguard carries the boundary rules (§9). |
| Hot reload | `air` | Dev only. |
| Task runner | `go-task` | §15. |
| Scaffolding | `binafy/go-stub` | §6.3, `tools/go.mod`. |

The next seven subsections are not library documentation. They cover only the places where a chosen tool collides with a principle from §4, and how that collision is resolved.

---

### 7.1 GORM is the highest-risk choice here — read this before writing a repository

GORM makes P1–P5 easy to violate in a single line. `db.Preload("User")` from inside `wallet` is idiomatic GORM and is exactly the cross-module join that makes extraction a two-quarter project. Three mechanisms contain it; all three are mandatory.

**Mechanism 1 — per-module table prefix.** Each module receives a `*gorm.DB` whose naming strategy is pinned to its own schema.

```go
// internal/platform/postgres/gorm.go
func NewModuleDB(base *gorm.DB, module string) *gorm.DB {
	db, _ := gorm.Open(postgres.New(postgres.Config{
		Conn: base.ConnPool, // shared *sql.DB — one pool, many sessions
	}), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   module + "_svc.",   // wallet_svc.wallets
			SingularTable: false,
		},
		PrepareStmt:            false, // MANDATORY behind PgBouncer (§15)
		SkipDefaultTransaction: true,  // we manage transactions explicitly
		Logger:                 gormzap.New(log, gormzap.WithSlowThreshold(200*time.Millisecond)),
	})
	return db
}
```

A `Preload("User")` now resolves to `wallet_svc.users`, which does not exist. The violation fails at runtime in the first integration test rather than silently working until extraction day.

**Mechanism 2 — GORM models never leave `internal/repo/`.**

```go
// modules/wallet/internal/repo/model.go
type walletModel struct {                     // unexported. non-negotiable.
	ID        string `gorm:"primaryKey;type:uuid"`
	UserID    string `gorm:"type:uuid;index"`  // NO gorm foreign key tag (P5)
	BalanceMinor int64
	Version   int    `gorm:"default:0"`
}

func (m walletModel) toDomain() domain.Wallet { ... }
func fromDomain(w domain.Wallet) walletModel  { ... }
```

The domain entity has no `gorm` tags and no `gorm.Model` embed. If `domain.Wallet` carries persistence tags, the domain depends on the ORM, `NewWallet()` stops being the only way to construct a valid wallet, and the mapping you avoided returns as a whole class of "how did an invalid entity get into the database" bugs.

**Pragmatic carve-out:** for a genuinely thin CRUD module (`notification`), letting the GORM model *be* the module's type is acceptable — provided it stays unexported and never crosses into `contracts/`. Mapping layers on a 200-line module are the ceremony §3 rejects. The rule is about escape, not about layer count.

**Mechanism 3 — `AutoMigrate` is banned.** goose owns schema. An arch test asserts the string `AutoMigrate` appears nowhere under `internal/`. Two systems writing DDL against one database produces drift that only shows up in production.

**Where GORM is the wrong tool and pgx is used directly:** the outbox `SKIP LOCKED` poller, the cached read path in hot repositories, and any query where the generated SQL needs to be read before it ships. `Deps.SQL` exists for this. GORM earns its place on CRUD, associations within a module, and pessimistic locking:

```go
tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&w, "id = ?", id)
```

---

### 7.2 Wire builds infrastructure; the registry stays runtime

These two pull in opposite directions and the split has to be explicit. Wire is compile-time; `MODULES_ENABLED` (R2) is runtime. Attempting to express module selection in Wire produces either a generated file that has to change whenever config changes, or the loss of R2.

**Resolution: Wire owns everything below `Deps`. Module construction and selection stay hand-written.**

```go
//go:build wireinject

// internal/app/wire.go
func InitializePlatform(ctx context.Context, cfg config.Config) (*Platform, func(), error) {
	wire.Build(
		postgres.ProviderSet,   // pool, gorm base, tx manager
		redis.ProviderSet,
		telemetry.ProviderSet,  // zap, otel, prometheus
		bus.ProviderSet,
		outbox.ProviderSet,
		authz.ProviderSet,      // casbin enforcer + gorm adapter
		validate.ProviderSet,
		clock.ProviderSet,
		wire.Struct(new(Platform), "*"),
	)
	return nil, nil, nil
}
```

```go
// internal/app/registry.go — hand-written, deliberately
func buildModules(p *Platform, cfg config.Config) ([]modkit.Module, error) {
	all := map[string]func() modkit.Module{
		"user":   func() modkit.Module { return user.New(p.DepsFor("user"), cfg.User) },
		"wallet": func() modkit.Module { return wallet.New(p.DepsFor("wallet"), cfg.Wallet) },
	}
	// ... filter by cfg.ModulesEnabled, as R2
}
```

`p.DepsFor(name)` is where the per-module `*gorm.DB` session and pre-tagged zap logger are minted. It is roughly ten lines and it is worth more than any Wire provider in this repo.

**Honest note:** with `Deps` as a single struct, Wire's value here is modest — it is wiring maybe fifteen infrastructure constructors with a shutdown chain. It earns its place mainly through the `func()` cleanup composition, which is easy to get wrong by hand. If the generated file starts causing more friction than the manual constructor it replaced, deleting Wire is a contained change: `wire_gen.go` becomes `platform.go` and nothing else moves.

---

### 7.3 Config — typed per module, validated once

```go
// internal/config/config.go
type Config struct {
	Env            string   `env:"APP_ENV" envDefault:"local"`
	ModulesEnabled []string `env:"MODULES_ENABLED,required" envSeparator:","`

	DB struct {
		WriterDSN string `env:"WRITER_DSN,required"`
		ReaderDSN string `env:"READER_DSN"`
		MaxConns  int32  `env:"MAX_CONNS" envDefault:"8"`
	} `envPrefix:"DB_"`

	JWT struct {
		Secret     string        `env:"SECRET,required" validate:"min=32"`
		AccessTTL  time.Duration `env:"ACCESS_TTL" envDefault:"15m"`
		RefreshTTL time.Duration `env:"REFRESH_TTL" envDefault:"720h"`
	} `envPrefix:"JWT_"`

	Wallet wallet.Config // module owns its own struct
	User   user.Config
}

func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return c, err
	}
	return c, validate.Struct(c) // same validator instance as transport
}
```

**This resolves §13 Q3 in favour of typed structs over `map[string]string`.** The module defines `wallet.Config` with its own env tags; `config` embeds it. Nothing in `modkit` learns a module-specific key, and a malformed value kills the boot instead of surfacing as a zero value three days later.

Viper was rejected: multi-source precedence resolution nobody remembers, `interface{}` returns, and a large transitive tree, in exchange for capabilities a 12-factor service does not use.

---

### 7.4 Migrations — goose, with typed seeding

Per-module version table, embedded FS, applied in registry order:

```go
// internal/platform/postgres/migrate.go
func Apply(ctx context.Context, db *sql.DB, m modkit.Module) error {
	schema := m.Name() + "_svc"
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS `+pq.QuoteIdentifier(schema)); err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, m.Migrations(),
		goose.WithTableName(schema+".goose_db_version"),
	)
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	return err
}
```

The reason goose over golang-migrate is **Go migrations**, and the best use of them is seeding the permission catalogue from typed constants rather than hand-copied SQL strings:

```go
// modules/wallet/migrations/0003_seed_permissions.go
func init() {
	goose.AddMigrationContext(upSeedPerms, downSeedPerms)
}

func upSeedPerms(ctx context.Context, tx *sql.Tx) error {
	for _, p := range contracts.AllPermissions { // the same vars the code enforces
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO authz_svc.permissions (key, module) VALUES ($1,$2)
			 ON CONFLICT (key) DO NOTHING`, p.Key(), p.Module); err != nil {
			return err
		}
	}
	return nil
}
```

A permission can now never be granted in the database under a name the code does not define — the seed and the enforcement read the same Go constant.

---

### 7.5 Validation — transport only

```go
// modules/wallet/internal/transport/dto.go
type WithdrawReq struct {
	Amount   int64  `json:"amount"   validate:"required,gt=0"`
	Currency string `json:"currency" validate:"required,iso4217"`
}
```

**Tags validate shape. Constructors validate invariants.** `required,gt=0` is a transport concern. "A withdrawal may not exceed the daily limit for this account tier" is `domain.Wallet.Withdraw()` returning a typed error. Domain structs carry no `validate` tags, for the same reason they carry no `gorm` tags: a domain that depends on a transport library can be constructed invalid by any test that skips the validator.

`shared/validate` wraps the singleton, registers custom rules (`iso4217`, `ulid`), and maps `validator.ValidationErrors` into `errs.KindInvalid` so `httpx` renders problem+json field errors without importing the validator.

---

### 7.6 Authorization — casbin behind an interface, permissions owned by modules

Casbin's native model is string-based, which conflicts with role-by-ID and with module-owned permission catalogues. Three constraints reconcile it.

**Constraint 1 — no module imports casbin.** `platform/authz` is the only package that does.

```go
// internal/platform/authz/authz.go
type Permission struct{ Module, Action string }
func (p Permission) Key() string { return p.Module + ":" + p.Action }

type Principal struct {
	UserID   string
	RoleIDs  []int64   // IDs, never role name strings
	TenantID string
}

type Enforcer interface {
	Authorize(ctx context.Context, p Principal, perm Permission, resource any) error
}
```

The casbin subject is `role:{id}`, not a role name. Renaming a role in the admin UI must not silently change who can withdraw money.

**Constraint 2 — modules declare their own permissions.**

```go
// modules/wallet/contracts/permissions.go
var (
	PermWithdraw = authz.Permission{Module: "wallet", Action: "withdraw"}
	PermViewAny  = authz.Permission{Module: "wallet", Action: "view_any"}

	AllPermissions = []authz.Permission{PermWithdraw, PermViewAny}
)
```

`Module.Permissions()` returns this slice. At boot the registry aggregates all of them and fails if `casbin_rule` grants a permission no enabled module declares. Silent authorization drift after a rename or deletion is a nasty bug class and this catches it at startup rather than at the first 403 nobody reports.

**Constraint 3 — resource-scoped checks live in the service, not middleware.** Middleware can answer "may this role ever withdraw." It cannot answer "may this user withdraw from *this* wallet," because that needs the loaded entity.

```go
func (s *Service) Withdraw(ctx context.Context, cmd WithdrawCmd) error {
	w, err := s.repo.ByID(ctx, cmd.WalletID)
	if err != nil {
		return err
	}
	if err := s.authz.Authorize(ctx, authz.From(ctx), contracts.PermWithdraw, w); err != nil {
		return err // → errs.KindForbidden → 403
	}
	return s.withdraw(ctx, w, cmd.Amount)
}
```

**gorm-adapter operational requirements — both mandatory:**

- **Policy storage is its own schema.** `casbin_rule` lives in `authz_svc`, owned by `platform/authz`, not by any module. It is the one table outside the per-module schema rule, and the exception is deliberate: authz is platform, not a module.
- **Multi-replica policy staleness.** Casbin caches policy in memory. Twenty pods with a GORM adapter means a policy change in the admin UI reaches one pod. Wire a watcher (`casbin/redis-watcher/v2` — Redis is already a dependency) so `LoadPolicy` fires across all replicas on change. Without this, revoking someone's access appears to work and does not. This is the single most likely production incident in the authz stack.

---

### 7.7 Testing

| Layer | Tool | Rule |
|---|---|---|
| Assertions | `testify/require` | `require`, not `assert` — a failed `assert` continues and buries the real failure under cascading noise |
| Mocks | `mockery` (`.mockery.yaml`) | Generate for **ports only** (`wallet.UserReader`, `authz.Enforcer`, `bus.Bus`). Never for a module's own repository |
| Integration | `testcontainers-go` | Real Postgres + Redis. One container per package, `t.Cleanup` teardown |
| Load | `k6` | R17 |

**Mock ports, fake or containerize everything else.** Mockery pointed at your own repository interface produces a test asserting that you called the method you wrote — it passes when the SQL is wrong, which is the only thing that was worth testing. Repositories get testcontainers. Ports get mocks, because the point is verifying the *contract* survives extraction.

```yaml
# .mockery.yaml
with-expecter: true
dir: "{{.InterfaceDir}}/mocks"
outpkg: mocks
packages:
  myapp/internal/modules/wallet:
    interfaces:
      UserReader:
  myapp/internal/platform/authz:
    interfaces:
      Enforcer:
```

Restricting the `packages` list is itself an enforcement mechanism: if someone adds a repository interface to `.mockery.yaml`, that is a visible line in a diff to argue about.

---

### 7.8 Redis — five jobs, two instances

**Governing rule: nothing may exist only in Redis**, with two deliberate exceptions — cache (cheap to lose) and sessions (§7.8.2, accepted). Everything else is derived or ephemeral. Reaching for Redis because Postgres feels slow is almost always an index problem wearing a caching costume.

| Pattern | Instance | TTL | Owner |
|---|---|---|---|
| `cache:{module}:{entity}:v{n}:{id}` | `redis-cache` | 1–15m + jitter | module |
| `rt:{userID}:{tokenID}` | `redis-core` | refresh TTL | authn |
| `rt:idx:{userID}` (SET of tokenIDs) | `redis-core` | refresh TTL | authn |
| `jwt:deny:{jti}` | `redis-core` | remaining access TTL | authn |
| `rl:{scope}:{principal}` | `redis-core` | window | httpx |
| `lock:cron:{job}` | `redis-core` | > max job runtime | scheduler |
| *(pub/sub channel)* | `redis-core` | — | casbin watcher (§7.6) |

The `v{n}` segment in cache keys is the invalidation escape hatch: changing a cached struct's shape is a version bump, not a flush.

**Two instances, not one, and this is not premature optimization.** A cache wants `maxmemory-policy allkeys-lru`. Put sessions, locks and rate-limit counters on that same instance and Redis evicts them under memory pressure: random logouts, two schedulers holding the same lock, rate limits silently resetting. Cache eviction is a performance event; core eviction is a correctness incident with no obvious cause. `redis-core` runs `noeviction` with persistence and explicit TTLs on every key; `redis-cache` needs neither.

#### 7.8.1 Module-scoped cache clients

Each module receives a prefix-bound cache handle from `DepsFor()`, the same containment mechanism as GORM's `TablePrefix` in §7.1. Without it, nothing stops `wallet` reading `cache:user:*`.

```go
// internal/platform/redis/module.go
type ModuleCache struct {
	rdb    *redis.Client
	prefix string          // "cache:wallet:"
	sf     singleflight.Group
	ttl    time.Duration
	log    *zap.Logger
}

func (c *ModuleCache) key(parts ...string) string {
	return c.prefix + strings.Join(parts, ":")
}
```

Three behaviours are mandatory in the read path, because they are the three ways a cache layer kills the database it was added to protect:

```go
const tombstone = "\x00nil"

func (c *ModuleCache) GetOrLoad(ctx context.Context, key string, dst any,
	load func(context.Context) (any, error)) error {

	full := c.key(key)

	switch b, err := c.rdb.Get(ctx, full).Bytes(); {
	case err == nil && string(b) == tombstone:
		return ErrNotFound                       // 3. negative cache
	case err == nil:
		if json.Unmarshal(b, dst) == nil {
			return nil
		}
	case !errors.Is(err, redis.Nil):
		c.log.Warn("cache read failed", zap.Error(err)) // degrade, never fail
	}

	v, err, _ := c.sf.Do(full, func() (any, error) {   // 1. singleflight
		val, err := load(ctx)
		if errors.Is(err, ErrNotFound) {
			c.rdb.Set(ctx, full, tombstone, 30*time.Second)
			return nil, err
		}
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(val)
		c.rdb.Set(ctx, full, b, jitter(c.ttl))          // 2. TTL jitter
		return val, nil
	})
	if err != nil {
		return err
	}
	return assign(dst, v)
}

func jitter(d time.Duration) time.Duration {
	return d + time.Duration(rand.Int63n(int64(d/5))) - d/10 // ±10%
}
```

Singleflight collapses 5,000 concurrent misses on a hot key into one query. Jitter stops keys written together from expiring together and stampeding. Negative caching stops lookups for nonexistent IDs from reaching Postgres every time. **A cache read failure must never fail the request** — Redis being down is a latency event, not an availability event.

Batch reads go through `MGET` or a pipeline. The `ByIDs` batch method on every port (§6) exists for this; a 50-item list served as 50 round trips is slower than the join it replaced.

**Invalidation happens after commit, never inside the transaction:**

```go
err := postgres.InTx(ctx, s.db, func(tx *gorm.DB) error {
	return s.repo.Rename(ctx, tx, id, name)
})
if err != nil {
	return err
}
s.cache.Del(ctx, "wallet:v1:"+id)   // AFTER commit
```

Deleting inside the transaction opens a window where a concurrent reader misses, reads the pre-commit row, and writes it back with a full fresh TTL. You then serve stale data for the whole TTL with nothing to alert on. Delete-after-commit can leave a cold cache if the process dies between the two, which is harmless.

#### 7.8.2 Sessions and refresh tokens

An index set makes logout-everywhere possible without `SCAN`:

```go
func (s *Store) Issue(ctx context.Context, userID, tokenID string, meta Meta) error {
	pipe := s.core.TxPipeline()
	pipe.Set(ctx, "rt:"+userID+":"+tokenID, mustJSON(meta), s.ttl)
	pipe.SAdd(ctx, "rt:idx:"+userID, tokenID)
	pipe.Expire(ctx, "rt:idx:"+userID, s.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// GETDEL makes rotation atomic: a replayed refresh token finds nothing.
func (s *Store) Rotate(ctx context.Context, userID, oldID string) (Meta, error) {
	b, err := s.core.GetDel(ctx, "rt:"+userID+":"+oldID).Bytes()
	if errors.Is(err, redis.Nil) {
		return Meta{}, ErrTokenReused // attack signal — revoke the whole family
	}
	...
}
```

`ErrTokenReused` is not noise. A refresh token used twice means it leaked; the correct response is revoking every token for that user, which is what the index set enables. Never `SCAN rt:{user}:*` in a request path.

**This resolves the old Q2.** Accepted trade: a `FLUSHALL` or unreplicated failover logs everyone out. An inconvenience, not data loss — but a decision, not a 3am surprise, and the reason `redis-core` carries persistence.

#### 7.8.3 Rate limiting and locks — opposite failure directions

**Rate limiting fails open. Locks fail closed.** This split has to be deliberate; whoever writes the error branch first will otherwise decide it by accident.

```go
res, err := limiter.Allow(ctx, "rl:api:"+principalOrIP(r), lim)
if err != nil {
	log.Warn("rate limiter unavailable", zap.Error(err))
	next.ServeHTTP(w, r)   // FAIL OPEN — a Redis outage must not 429 everyone
	return
}
```

A dead Redis returning 429 to every caller converts a cache outage into a full outage. A dead Redis letting two schedulers run the same job is a correctness failure, so `Acquire` returning an error means the caller does not run.

`redis_rate` uses GCRA rather than a fixed window: a fixed window lets a client spend its full budget in the last 100ms of one window and again in the first 100ms of the next, so a "120/min" limit is really 240 in a 200ms burst.

Lock release must be a compare-and-delete, not a `DEL`:

```go
var releaseScript = redis.NewScript(`
	if redis.call("GET", KEYS[1]) == ARGV[1] then
		return redis.call("DEL", KEYS[1])
	end
	return 0`)
```

If a job overran its TTL the lock already expired and another instance holds it. A plain `DEL` releases *theirs*, and now two jobs run — the exact failure the lock existed to prevent. Set the lock TTL comfortably above maximum job runtime and give the job a `context.WithTimeout` shorter than the TTL; that fails in the safe direction and is more predictable than a lease-extending watchdog.

Single-instance `SETNX` is correct here. **Redlock is rejected**: it needs five independent nodes, its safety guarantees are actively disputed, and the thing being protected is a cron job.

#### 7.8.4 Client and operational settings

```go
core := redis.NewClient(&redis.Options{
	Addr:            cfg.Redis.CoreAddr,
	PoolSize:        10 * runtime.GOMAXPROCS(0),
	MinIdleConns:    10,                        // no latency cliff on cold start
	ReadTimeout:     200 * time.Millisecond,
	WriteTimeout:    200 * time.Millisecond,
	MaxRetries:      2,
	ConnMaxIdleTime: 5 * time.Minute,
})
redisotel.InstrumentTracing(core)   // spans join the trace from §7.11
redisotel.InstrumentMetrics(core)
```

Short timeouts are the entire point. A slow Redis behind a 3-second default is worse than no Redis: every request waits, goroutines accumulate, the process OOMs. 200ms and fall through.

- **`KEYS` is banned.** O(n) on a single-threaded server. `SCAN` in admin tooling only, never in a handler.
- **Every key gets a TTL, including "permanent" ones.** `redis-core` runs `noeviction`, so untracked keys with no expiry are a memory leak that eventually returns `OOM command not allowed` on every write — meaning nobody can log in.
- **JSON values.** msgpack or protobuf only when a profile names serialization as the bottleneck. It will not.
- **No Cluster at v1.** One primary plus a replica handles far more than 7k RPS. If sharding ever happens, hash tags (`{user:123}`) become mandatory for multi-key operations — worth knowing now so key names do not have to change then.

**Explicitly not used for:** job queues (RabbitMQ owns that — two brokers means two retry models, two DLQ stories, two dashboards), the outbox (it must be transactional with the write, and Redis has no transaction to join), or caching by default (every cached read is a chance to serve stale data; cache what a profile shows is hot and expensive, not what is convenient).

---

### 7.9 RabbitMQ — one broker, two patterns, kept apart

Events and jobs are not the same shape and must not share an abstraction:

| | Events (`bus.Bus`) | Jobs (`jobs.Queue`) |
|---|---|---|
| Exchange | topic `events` | direct `jobs` |
| Routing key | `user.registered.v1` | job name |
| Consumers | many, independent groups | one group |
| Semantics | "this happened" | "do this" |
| Priority | none | `x-max-priority` |
| Producer | **outbox dispatcher only** | any module, plus the scheduler |

Collapsing these into one interface produces an API where half the parameters are meaningless for whichever pattern you are using.

**Publisher confirms are mandatory.** Without them, `basic.publish` is fire-and-forget: the dispatcher marks `published_at` on a row RabbitMQ never accepted and the event is silently lost forever. The dispatcher must await the confirm before the update, in the same loop:

```go
// platform/outbox/dispatcher.go — sketch
for _, ev := range batch {
	dc, err := ch.PublishWithDeferredConfirmWithContext(ctx, "events", ev.Topic, ...)
	if err != nil { return err }                  // leave published_at NULL, retry next tick
	if ok, err := dc.WaitContext(ctx); err != nil || !ok {
		return fmt.Errorf("nack for %d: %w", ev.ID, err)
	}
	confirmed = append(confirmed, ev.ID)
}
markPublished(ctx, confirmed)
```

At-least-once, so publishing twice on a crash between confirm and update is expected and correct. Consumers dedupe on `event_id`.

**Every queue in this system is a quorum queue. Classic queues are banned — including durable ones, including DLQs and retry queues.** This is not a default to be overridden per queue; it is a property of the topology, declared once and asserted at boot.

The reason is that "durable" does not mean what people assume. A durable *classic* queue has exactly one leader on exactly one node, and no replicas. When that node dies the queue is unavailable until it returns, and any message not yet fsynced is gone. Classic mirrored queues — the old workaround — are **deprecated and removed in RabbitMQ 4.x**, and they could lose confirmed messages during a partition because mirror promotion is not consensus-based. Quorum queues replicate via Raft: a publish is confirmed only once a majority of replicas has it, so losing a node loses nothing and a new leader is elected in seconds.

Retry and dead-letter queues are quorum queues too. A DLQ holds precisely the messages you could not afford to lose the first time; storing them on a single-node classic queue inverts the entire point of having one.

**Topology, declared once in `platform/rabbit/topology.go`:**

```go
// Every queue declaration goes through this helper. There is no other path.
func (t *Topology) declareQueue(ch *amqp.Channel, name string, args amqp.Table) error {
	if args == nil {
		args = amqp.Table{}
	}
	args["x-queue-type"] = "quorum"           // MANDATORY. No override parameter exists.
	args["x-quorum-initial-group-size"] = 3
	args["x-delivery-limit"] = 5              // broker-side poison-message cap → DLX
	args["x-max-length"] = 1_000_000
	args["x-overflow"] = "reject-publish"     // nack → outbox row stays NULL (§7.9.2)

	_, err := ch.QueueDeclare(name, true /*durable*/, false /*autoDelete*/, false /*exclusive*/, false, args)
	return err
}
```

`declareQueue` taking no queue-type argument is deliberate. A helper with a `quorumMode bool` parameter is a helper someone eventually passes `false` to at 2am.

Note `x-delivery-limit`: quorum queues enforce the redelivery cap **in the broker**, so a poison message reaches the DLX even if a consumer's own retry accounting is buggy. Classic queues have no equivalent — this is a second, independent reason for the choice.

- Topic exchange `events`, durable; one quorum queue per (module, topic) pair.
- Direct exchange `jobs`, durable; one quorum queue per job type.
- Per-queue DLX to `events.dlx` / `jobs.dlx`, plus TTL retry queues for bounded backoff — all quorum.
- **Retries are always bounded.** Unbounded `nack(requeue=true)` on a poison message is the classic RabbitMQ outage: one malformed payload spins a consumer at full CPU and starves every other message. `x-delivery-limit` backstops the consumer's own cap of ~5 attempts, then DLQ, then alert.
- `basic.qos` prefetch set explicitly on every consumer. The default is unlimited, which means one consumer grabs the whole queue while the others idle.

**Asserted at boot, not trusted.** Topology drifts — someone declares a queue by hand from the management UI to debug something and it is classic forever after. The dispatcher and worker both verify on startup and refuse to run otherwise:

```go
func (t *Topology) Verify(ctx context.Context) error {
	qs, err := t.mgmt.ListQueues(ctx, t.vhost)
	if err != nil {
		return err
	}
	for _, q := range qs {
		if q.Type != "quorum" {
			return fmt.Errorf("queue %q is type %q, expected quorum", q.Name, q.Type)
		}
	}
	return nil
}
```

Failing to boot is the correct response. A service silently consuming from a classic queue is a service that will lose messages on the next node failure, and nothing will tell you it happened.

**One trade-off to know:** quorum queues hold more in memory and write more to disk than classic queues, and they do not support per-message TTL, priorities, or non-durable modes. Two consequences for this design — priority job queues (§7.9) must use `x-max-priority` at declare time, which quorum queues **do not support**, so priority queues are the single documented exception and run as classic queues with the durability trade made explicit and confined to fire-and-forget work; and TTL-based retry backoff must be per-queue (a queue per retry tier), not per-message. Both are noted in the runbook.

**Two properties Rabbit does not give you, worth knowing before you rely on them:**

1. **No ordering.** Two consumers on one queue process out of order, and a redelivery after a nack lands behind newer messages. If a projection needs per-user ordering, the answer is the consistent-hash exchange plugin — and even then, ordering holds per routing key, not globally. This is Q2 in §13, and it should be settled before the topology ships.
2. **Priority queues only order what is already in the queue.** A high prefetch defeats them entirely: the consumer has already pulled 100 low-priority messages into its local buffer before the urgent one arrives. Priority queues need `prefetch` in the 1–10 range, which costs throughput. Use priority only where it earns that cost.

---

#### 7.9.1 What RabbitMQ is for, and what it is not

**Postgres is the source of truth. RabbitMQ is a delivery mechanism for facts already committed.** Every durability decision below follows from that one sentence, and getting it backwards is how teams end up unable to answer "did that actually happen?" during an incident.

The broker's job is decoupling **in time**: `user.registered` commits now, `wallet` reacts in 50ms or in 5 minutes, and the user-facing request does not wait for either. What it is *not*:

- not the record of what happened — that is the outbox table
- not a database — a message is a notification, never the only copy of a fact
- not on any synchronous request path — an HTTP handler that blocks on a publish has made the broker's availability its own

That last one is an arch-test assertion, not a convention.

#### 7.9.2 Not losing work when the broker fails — four layers

Losing messages is a solved problem, but only if all four layers are in place. Any one of them alone leaks.

**Layer 1 — Broker durability. Necessary, not sufficient.**

| Setting | Why |
|---|---|
| **Quorum queues**, 3 or 5 nodes | Raft-replicated. A durable *classic* queue lives on exactly one node: if that node dies the queue is unavailable until it returns, and classic mirrored queues (deprecated) drop messages during partitions. Odd node count so Raft can form a majority. |
| Durable exchanges and queues | Survive broker restart. |
| `DeliveryMode: 2` (persistent) | Non-persistent messages are discarded on restart even from a durable queue. |
| **Publisher confirms** | The only signal that a message is on disk and replicated. Without them `basic.publish` is fire-and-forget. |
| `x-max-length` + `overflow: reject-publish` | A runaway queue otherwise OOMs the node and takes down every other queue with it. Rejecting the publish is the correct behaviour — see Layer 2. |
| `cluster_partition_handling: pause_minority` | Prevents split-brain. The minority side stops rather than accepting writes it will lose. |
| Nodes across availability zones | A quorum in one rack is a quorum until the rack goes. |

A persistent message on a durable queue can still be lost if the node dies before fsync. **The confirm is what tells you it is safe** — which is why Layer 2 waits for it.

**Layer 2 — The outbox. This is the actual guarantee.**

Because the outbox dispatcher is the only publisher and it only marks `published_at` after a confirm, a total broker outage loses exactly nothing:

```go
dc, err := ch.PublishWithDeferredConfirmWithContext(ctx, "events", ev.Topic, ...)
if err != nil {
	return err                                   // published_at stays NULL
}
if ok, err := dc.WaitContext(ctx); err != nil || !ok {
	return fmt.Errorf("nack for event %d: %w", ev.ID, err)  // stays NULL
}
```

Rabbit unreachable, disk alarm, `reject-publish` on a full queue, network partition — every path leaves the row unpublished and it drains on reconnect. The failure is visible as `outbox_unpublished_age_seconds` climbing (R40, alert at 60s), not as silence.

Two supporting rules: keep published outbox rows for **7 days** before pruning, so a consumer bug is recoverable by replay rather than by archaeology; and never let the dispatcher `UPDATE` a batch it has not fully confirmed.

**Layer 3 — Consumers.**

- **Manual ack, after the work succeeds.** `autoAck: true` acknowledges on delivery, so a consumer crash mid-processing loses the message permanently. This is the single most common way messages disappear.
- **Prefetch set explicitly.** Unlimited (the default) means one consumer buffers the queue; if it dies, everything it buffered is redelivered at once while the other consumers sat idle.
- **Idempotent handlers, deduped on `event_id`.** Delivery is at-least-once by design: the dispatcher can crash between confirm and `UPDATE`, and a redelivery follows any nack.
- **Bounded retries → DLQ.** Never `nack(requeue=true)` unbounded. One malformed payload spins a consumer at full CPU and starves the queue — the classic RabbitMQ outage. Cap around 5 attempts with growing TTL, then DLQ, then alert.

**Layer 4 — The gap nobody expects: jobs enqueued outside a transaction.**

Events are safe by construction. **Jobs are not.** `jobs.Enqueue()` called directly writes to Rabbit with no Postgres row behind it, so if the broker is down the work is simply gone. Three cases, three answers:

| Job origin | Answer |
|---|---|
| Triggered by a state change ("send receipt after payment") | **Route through the outbox.** A job is just a topic; the dispatcher publishes it to the `jobs` exchange. It inherits every guarantee above. |
| Triggered by the scheduler (§7.10) | **Make it reconciling, not incremental.** "Sweep everything unprocessed since the last successful run", never "process what happened in the last 5 minutes". A missed tick then self-heals on the next one and losing a message costs nothing. |
| Genuinely fire-and-forget (cache warm, metrics roll-up) | Direct enqueue is fine. Losing one is acceptable by definition — and if it is not, it belongs in row one. |

**Design every scheduled job to be reconciling.** It is the difference between a broker outage being a non-event and being a data-integrity investigation.

#### 7.9.3 Failure-mode reference

| Scenario | Events | Jobs | Recovery |
|---|---|---|---|
| One node dies (3-node quorum) | none lost | none lost | automatic, leader re-elects in seconds |
| Whole cluster down | none lost — outbox accumulates | lost **unless** outbox-routed or reconciling | drains on reconnect |
| Network partition | none lost | none lost | `pause_minority`; minority stops accepting |
| Disk or memory alarm | publishes blocked → nack → rows stay `NULL` | enqueue errors | free resources; outbox drains |
| Consumer crashes mid-work | redelivered (manual ack) | redelivered | automatic |
| Dispatcher crashes after confirm, before `UPDATE` | delivered twice | — | consumer dedupes on `event_id` |
| Poison message | DLQ after ~5 attempts | DLQ | `task rabbit:replay QUEUE=x` after fixing |
| Consumer bug corrupts a projection | replay from outbox (7-day window) | — | `task outbox:replay FROM=<ts> TOPIC=x` |

**Handle blocked connections explicitly.** On a disk or memory alarm RabbitMQ blocks publishers without closing the connection — the dispatcher hangs silently with no error. Wire `Connection.NotifyBlocked` to a log line, a metric, and a bounded publish timeout so the condition surfaces as an alert instead of a stall.

#### 7.9.4 Deployment shape

Three nodes minimum, one per AZ, quorum queues everywhere, `pause_minority`, disk alarm threshold well above the largest expected backlog. Two nodes is worse than one: a two-node Raft cluster cannot form a majority when either node fails.

Sizing follows from the outbox: the broker only needs to hold what consumers are behind by, because everything else is still in Postgres. The queue depth alert (R40) matters more than the disk size.

---

### 7.10 Scheduling — the singleton problem

`robfig/cron/v3` runs in-process, which means **twenty replicas fire every job twenty times.** This is the most common way in-process cron causes an incident, and it does not announce itself: the reconciliation job just runs 20× and mostly appears to work.

Two defences, both required:

**1. A dedicated `cmd/scheduler` deployed at `replicas: 1`.** Not a flag on the API pods. A separate deployment makes the constraint visible in the manifest rather than buried in config.

**2. A Redis lock around every entry**, because `replicas: 1` is a lie during a rolling deploy — old and new pods overlap for seconds:

```go
func (s *Scheduler) Register(name, spec string, fn func(context.Context) error) error {
	_, err := s.cron.AddFunc(spec, func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.maxRun)
		defer cancel()

		lock, err := s.locks.Acquire(ctx, "lock:cron:"+name, 5*time.Minute)
		if err != nil {
			return // another instance holds it. Not an error.
		}
		defer lock.Release(ctx)

		if err := fn(ctx); err != nil {
			s.log.Error("cron job failed", zap.String("job", name), zap.Error(err))
		}
	})
	return err
}
```

**The scheduler enqueues; it never executes.** A cron entry publishes to `jobs` and returns in milliseconds. The actual work happens on `cmd/worker`, which scales horizontally. This keeps the singleton stateless and tiny, means a long job cannot block the next tick, and means job execution gets the same retry, DLQ and priority handling as everything else. A scheduler that does the work is a scheduler that becomes a single point of throughput failure.

Two smaller details that bite: wrap every job in panic recovery (`cron.WithChain(cron.Recover(logger))`) — an unrecovered panic in a cron goroutine takes the process down. And pin `cron.WithLocation(time.UTC)`; "3am daily" silently moves twice a year otherwise.

---

### 7.11 Tracing and metrics

OTel for traces, `prometheus/client_golang` for metrics, both wired in `platform/telemetry` and injected via `Deps`.

**The part that is easy to get wrong: the trace must survive the broker.** A trace that ends at the outbox insert and restarts at the consumer is two disconnected traces, which is exactly no better than no tracing at the moment you need it. The envelope carries the propagated context, and the dispatcher and consumer both handle it:

```go
// platform/bus/envelope.go
type Envelope struct {
	EventID    string            `json:"event_id"`
	Topic      string            `json:"topic"`
	OccurredAt time.Time         `json:"occurred_at"`
	TraceCtx   map[string]string `json:"trace_ctx"` // W3C traceparent
	Payload    json.RawMessage   `json:"payload"`
}

// publish side
otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(env.TraceCtx))

// consume side
ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(env.TraceCtx))
```

The same `trace_id` lands in every zap record (§7 stack table), so a trace ID from a 500 response pulls the full log set across API, dispatcher and worker.

**Metrics: RED per module, plus four that specifically predict this stack's failure modes:**

| Metric | Why it is the one that pages you |
|---|---|
| `outbox_unpublished_age_seconds` | The single best health signal in the system. Rising = dispatcher stuck, Rabbit unreachable, or confirms failing. Alert at 60s. |
| `rabbitmq_queue_depth{queue}` | Consumer death shows up here before it shows up in user complaints. |
| `rabbitmq_dlq_total{queue}` | Any non-zero rate means poison messages. Should alert on the *derivative*, not the value. |
| `pgxpool_acquire_wait_seconds` | Pool exhaustion (§15) presents as latency everywhere and a cause nowhere. |

`/metrics` and pprof bind to a separate admin port, never the public listener.

---

### 7.12 API documentation — swag, with a drift guard

`swaggo/swag` scans annotations across every module's `internal/transport/` and emits one merged spec into `api/docs`. Bruno collections live in `api/bruno/` and are committed.

```go
// modules/wallet/internal/transport/http.go

// @Summary      Withdraw from a wallet
// @Tags         wallet
// @Param        id   path  string        true "Wallet ID" format(uuid)
// @Param        body body  WithdrawReq   true "Withdrawal"
// @Success      200  {object} WalletResp
// @Failure      403  {object} httpx.Problem
// @Router       /wallets/{id}/withdraw [post]
func (h *handler) withdraw(w http.ResponseWriter, r *http.Request) { ... }
```

**Be clear-eyed about what this buys and what it does not.** Annotations are comments. Nothing fails when one is wrong, missing, or describes a response shape the handler stopped returning three sprints ago. Code-first API docs rot silently, and a confidently wrong spec is worse than no spec because clients generate against it.

So the generation is paired with a test that makes drift a build failure:

```go
// test/arch/openapi_test.go
func TestEveryRouteIsDocumented(t *testing.T) {
	r := buildRouterWithAllModules(t)
	spec := loadGeneratedSpec(t, "api/docs/swagger.json")

	chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if isInternal(route) { // /health, /metrics
			return nil
		}
		require.Truef(t, spec.Has(method, normalize(route)),
			"route %s %s has no swagger annotation", method, route)
		return nil
	})
}
```

That catches undocumented and deleted routes. It does not catch a wrong `@Success` body — nothing cheap does. If the spec ever becomes a real contract for external consumers, the answer is spec-first with `oapi-codegen` generating handler interfaces, so the compiler enforces what the comment currently only asserts. That is a P1 conversation, not a v1 one.

---

## 8. Cross-Module Communication Rules

| Need | Mechanism | Notes |
|---|---|---|
| Read another module's data | Consumer-defined port + local adapter | Adapter maps provider types → consumer's own snapshot type |
| Write in reaction to another module | Subscribe to a versioned event | Handler must be idempotent — outbox is at-least-once |
| Atomic multi-module write | Not permitted | Owning module writes + publishes in one tx; peer reacts async |
| Query spanning two modules | Not permitted | Denormalize the needed field and keep it fresh via events, or make two calls |

### Event schema rules

- Topic names carry version: `user.registered.v1`.
- Additive changes only. New fields must be optional.
- Never repurpose a field's meaning. Breaking change → `.v2` topic, dual-publish, migrate consumers, retire v1.
- Envelope carries `event_id`, `occurred_at`, `trace_id`. Consumers dedupe on `event_id`.

### Publish inside the owning transaction

```go
func (s *Service) Register(ctx context.Context, cmd RegisterCmd) (*domain.User, error) {
	return postgres.InTx(ctx, s.db, func(tx pgx.Tx) (*domain.User, error) {
		u, err := s.repo.Create(ctx, tx, domain.NewUser(cmd))
		if err != nil {
			return nil, err
		}
		// Same tx → atomic with the insert. No dual-write problem.
		return u, s.outbox.Publish(ctx, tx, contracts.TopicUserRegistered,
			contracts.UserRegisteredV1{
				UserID:   string(u.ID),
				Currency: u.Currency,
				At:       s.clock.Now(),
			})
	})
}
```

Dispatcher, safe to run N replicas with no coordination:

```sql
SELECT id, topic, payload, headers
FROM outbox_events
WHERE published_at IS NULL
ORDER BY id
LIMIT $1
FOR UPDATE SKIP LOCKED;
```

---

## 9. Enforcement

Boundaries that depend on discipline decay. All three layers below run in CI.

**Layer 1 — Compiler.** `modules/user/internal/**` is unreachable from `modules/wallet/**`. Free, absolute, no configuration.

**Layer 2 — depguard.** Blocks importing another module's *root* package (the part `internal/` cannot protect).

```yaml
# .golangci.yml
linters-settings:
  depguard:
    rules:
      module-isolation:
        files: ["**/internal/modules/**"]
        deny:
          - pkg: "myapp/internal/modules"
            desc: "import <module>/contracts only; sync deps go through your own ports.go"
        allow:
          - "myapp/internal/modules/$1/contracts"   # per-module allowlist, generated
      platform-purity:
        files: ["**/internal/platform/**"]
        deny:
          - pkg: "myapp/internal/modules"
            desc: "platform must never depend on business modules"
```

**Layer 3 — architecture test.** Catches what lint misses and produces the dependency graph in `docs/modules.md`.

```go
// test/arch/arch_test.go
func TestModuleBoundaries(t *testing.T) {
	pkgs := loadPackages(t, "myapp/internal/...")

	for _, p := range pkgs {
		owner, ok := moduleOf(p.PkgPath) // "" if not in a module
		if !ok {
			continue
		}
		for _, imp := range p.Imports {
			target, ok := moduleOf(imp)
			if !ok || target == owner {
				continue
			}
			// cross-module import: only contracts/ is legal,
			// and only from an adapters/ or ports file.
			if !strings.HasSuffix(imp, "/contracts") && !isAdapter(p.PkgPath) {
				t.Errorf("%s imports %s — cross-module import outside contracts/adapters", p.PkgPath, imp)
			}
		}
	}
}

func TestNoCrossModuleForeignKeys(t *testing.T) { /* parse migrations, assert FK targets stay in-schema */ }
func TestPlatformHasNoModuleImports(t *testing.T) { /* ... */ }
```

---

## 10. Requirements

### P0 — blueprint is not usable without these

> Requirement IDs are stable identifiers, not an ordering. New requirements take the next free number regardless of which table they land in, so a reference in a commit message or ADR never goes stale.

| ID | Requirement | Acceptance criteria |
|---|---|---|
| R1 | `modkit.Module` interface + `Deps` bundle | A module implementing it is discovered, migrated, routed, subscribed and health-checked with no changes outside `registry.go` |
| R2 | Composition root with explicit registry and `MODULES_ENABLED` | Booting with a subset starts cleanly; `/health` lists only enabled modules; unknown name fails fast at boot |
| R3 | Per-module goose migrations into own schema | `cmd/migrate` applies all enabled modules in registry order into `<name>_svc.goose_db_version`; each schema owned by a role with no rights to other schemas; at least one Go migration seeds the permission catalogue from typed constants (§7.4) |
| R4 | Outbox store + `SKIP LOCKED` dispatcher + in-process bus | Publishing inside a tx and rolling back leaves no event; two dispatcher replicas never double-deliver the same row |
| R5 | Two reference modules demonstrating both dependency directions | `wallet` reads `user` via port+adapter AND reacts to `user.registered.v1` |
| R6 | Remote adapter for the same port | `adapters/user_remote.go` compiles, has timeout + breaker + batch method, covered by a test with a stub server |
| R7 | Three-layer enforcement | A deliberately-violating branch fails CI at compiler, lint, and arch-test level |
| R8 | Postgres platform: Writer/Reader split, `InTx`, per-module `*gorm.DB`, PgBouncer-safe config | Reader routing verified in integration test; `PrepareStmt: false` and pgx simple protocol; integration test proves a cross-module `Preload` fails at runtime (§7.1); arch test asserts `AutoMigrate` appears nowhere |
| R9 | Cached repository decorator | singleflight + TTL jitter + negative cache; benchmark shows 5k concurrent misses on one key produce exactly 1 DB query |
| R10 | httpx: timeouts, recover, request ID, problem+json errors, graceful shutdown | `ReadHeaderTimeout` set; no handler can run without a context deadline; `errs.Kind` maps to status without HTTP knowledge in domain |
| R11 | Idempotency middleware | `Idempotency-Key` on non-GET stores request hash + response; replay returns cached response, conflicting body returns 422 |
| R12 | `task new-module NAME=x` scaffolder, built on go-stub (§6.3) | Generated module compiles, passes `task lint` and `task arch` with no edits, appears in `/health`, requires only a registry line. Stubs embedded via `embed.FS`. A missing placeholder key fails the run with `ErrMissingKeys` naming the key. Running twice exits non-zero without touching existing files. go-stub does not appear in the root `go.mod`. |
| R13 | Test layering: testify + mockery + testcontainers | Unit tests need no Docker; `task test` runs unit only, `task test:all` runs everything; `.mockery.yaml` lists ports only — a repository interface appearing there fails review by being a visible diff (§7.7) |
| R14 | Observability: zap + OTel + Prometheus | OTel trace context propagates through the outbox envelope into subscriber spans; every zap record carries `trace_id` and `module`; GORM slow queries (>200ms) land in the same stream; per-module RED metrics; pprof on the admin port |
| R15 | Typed config via caarlos0/env, validated at boot | Missing required var fails before the listener binds with the env key named; each module owns its `Config` struct; `modkit` contains no module-specific key |
| R16 | Wire builds the platform graph; registry stays hand-written | `task wire` regenerates cleanly; `wire_gen.go` is committed and CI fails if regenerating produces a diff; cleanup funcs compose in reverse boot order |
| R17 | Request validation at the transport boundary | `validator/v10` on all DTOs; arch test asserts no `validate:` tag appears under `internal/domain/`; field errors render as problem+json without domain importing the validator |
| R18 | Authn: JWT access + refresh, `Principal` in context | Expired, malformed, and wrong-signature tokens each return 401 with distinct log reasons; refresh rotation invalidates the used token |
| R19 | Authz: casbin behind `authz.Enforcer`, module-owned permissions | No module imports casbin; boot fails if `casbin_rule` grants a permission no enabled module declares; redis-watcher propagates a policy change to all replicas within 5s, proven by an integration test with two enforcer instances |
| R20 | Resource-scoped authorization in the service layer | A user cannot withdraw from a wallet they do not own even when their role holds `wallet:withdraw`; covered by a test that passes middleware and fails at the service |
| R34 | Outbox → RabbitMQ with publisher confirms | Dispatcher never marks `published_at` before a confirm; killing the broker mid-batch leaves rows unpublished and they drain on reconnect; no module imports `platform/rabbit` (depguard) |
| R35 | Separate `bus.Bus` (events) and `jobs.Queue` (work) | Quorum queues, per-queue DLX, bounded retry with growing TTL, explicit prefetch on every consumer; a poison message reaches the DLQ within 5 attempts and does not starve the queue |
| R36 | Singleton scheduler that enqueues, never executes | `cmd/scheduler` at `replicas: 1` plus a Redis lock per entry; integration test with two scheduler instances proves one execution per tick; every entry wrapped in panic recovery and pinned to UTC |
| R37 | Two Redis instances with distinct eviction policies | `redis-core` runs `noeviction` with persistence; `redis-cache` runs `allkeys-lru`. Sessions, locks and rate-limit counters never land on the cache instance |
| R38 | Distributed rate limiting | `redis_rate` GCRA in `httpx` middleware, keyed by principal when authenticated and IP otherwise; limit exceeded returns 429 with `Retry-After` |
| R39 | Trace context survives the broker | One trace spans HTTP request → outbox → dispatcher → consumer, proven by an integration test asserting a single trace ID across all four spans; every zap record carries that `trace_id` |
| R40 | Operational metrics with alert thresholds | `outbox_unpublished_age_seconds`, `rabbitmq_queue_depth`, `rabbitmq_dlq_total`, `pgxpool_acquire_wait_seconds` exported with documented thresholds in the runbook; `/metrics` on the admin port only |
| R41 | Generated OpenAPI with a drift guard | `task docs` regenerates into `api/docs`; `task check` fails on an uncommitted diff; the route-coverage test fails when a chi route has no annotation (§7.12) |
| R42 | Module-scoped cache handles | `Deps.Cache` is prefix-bound; a module cannot read or evict another module's keys. Read path implements singleflight + TTL jitter + negative caching; a cache read failure falls through to Postgres and never fails the request |
| R43 | Fail-open / fail-closed split is explicit | Rate limiting serves the request when Redis is unavailable; lock acquisition failure prevents execution. Both covered by tests that kill the Redis container mid-run |
| R44 | Cache invalidation after commit | Arch test asserts no `cache.Del` call inside an `InTx` closure; integration test proves a concurrent reader cannot repopulate from a pre-commit snapshot |
| R45 | **Quorum queues everywhere**, on a 3-node cluster with `pause_minority` | Every queue — including DLQs and retry queues — declares `x-queue-type: quorum` via the single `declareQueue` helper, which exposes no override. `Topology.Verify()` refuses to boot if any queue in the vhost is not quorum type. Killing one node loses no messages and re-elects within seconds; killing two leaves the minority refusing writes rather than accepting divergent ones. Any classic-queue exception (priority queues only) is named in the runbook with its durability trade stated |
| R46 | State-change-triggered jobs route through the outbox | Stopping RabbitMQ entirely, performing a state change, then restarting it results in the job running exactly once. Direct `jobs.Enqueue` is permitted only for fire-and-forget work, asserted by review checklist in the runbook |
| R47 | Scheduled jobs are reconciling, not incremental | Skipping a tick (broker down, lock contention) leaves no permanent gap; each scheduled job sweeps from last-successful-run rather than a fixed window |
| R48 | Blocked-connection handling on the dispatcher | `Connection.NotifyBlocked` raises a metric and a log line; a disk alarm surfaces as an alert within 30s rather than a silent publish stall |
| R49 | Replay tooling | `task outbox:replay FROM=<ts> TOPIC=x` and `task rabbit:replay QUEUE=x`; published outbox rows retained 7 days before pruning |

### P1 — significantly better, not blocking

| ID | Requirement |
|---|---|
| R21 | Auto-generated `docs/modules.md` dependency graph from the arch test |
| R22 | k6 load profile + committed baseline results proving G4 |
| R23 | Outbox dead-letter table + replay command |
| R24 | Per-module rate limiting config |
| R25 | Contract tests: consumer asserts against the producer's published event schema |
| R26 | Admin UI / API for role and policy editing on top of gorm-adapter |
| R27 | Atlas for migration linting — flags destructive DDL in CI before review does |

### P2 — design for, do not build

| ID | Consideration | Design implication now |
|---|---|---|
| R28 | Kafka/NATS if per-partition ordering or event replay is ever needed | `bus.Bus` is the seam; RabbitMQ is one implementation. Trigger: a consumer that provably requires total ordering per key (§7.9) |
| R29 | gRPC transport | `Module` gains `RegisterGRPC`; keep transport code out of `app/` |
| R30 | Separate DB instance per module | Schema-per-module + no cross-schema FK already makes this a connection-string change |
| R31 | Multi-tenancy | Repositories take tenant from context, never from a global. Casbin model already carries a tenant field |
| R32 | Read models / CQRS projections | Subscribers already exist; a projection is just another subscriber writing a denormalized table |
| R33 | OpenFGA / SpiceDB if relationship-based authz is ever needed | `authz.Enforcer` is the seam; casbin is one implementation of it |

---

## 11. Success Metrics

**Leading (measurable at blueprint completion)**
- Time to scaffold + wire a working module: **< 15 min**
- Boundary violations reachable on `main`: **0**
- Unit test suite runtime (no Docker): **< 10s**
- Full suite including integration: **< 3 min**
- Cold boot to serving: **< 2s**

**Lagging (measure at 3 months of use)**
- Cross-module coupling incidents found in code review that CI missed: **0**
- Modules added since blueprint adoption without touching `platform/`: **> 90%**
- Measured extraction cost, if ever exercised: **< 1 sprint per module**
- p99 latency on cached read path at 7k RPS: **< 200ms**

---

## 12. Delivery Phases

**Phase 1 — Skeleton (week 1).**
`config` (caarlos0/env), `platform/postgres`, `platform/httpx` (chi), `platform/telemetry` (zap + otel), `modkit.Module`, Wire platform graph, `cmd/api`, `cmd/migrate` (goose). One trivial module proving the wiring. Exit: binary boots, serves `/health`, applies migrations, missing required env kills the boot with the key named.

**Phase 2 — Persistence and caching (week 2).**
Writer/Reader split, `InTx`, `NewModuleDB` table-prefix isolation, GORM models + mappers, cached decorator with singleflight, testcontainers harness. Exit: R9 benchmark passes and the cross-module `Preload` test fails as designed (R8).

**Phase 3 — Async spine (week 3).**
Outbox store, `cmd/dispatcher` with publisher confirms, RabbitMQ topology, `bus.Bus` + `jobs.Queue`, `cmd/worker`, `cmd/scheduler`, trace propagation through the envelope. Exit: R4, R34, R35, R36, R45, R46, R48. A chaos step is part of the exit criteria: stop the broker, perform a state change, restart, assert exactly-once effect.

**Phase 4 — Two real modules + authz (week 4–5).**
`user` and `wallet` with both dependency directions, local + remote adapters, idempotency middleware, JWT authn, casbin enforcer with gorm-adapter and redis-watcher, module-owned permission catalogues with boot-time drift check. Exit: R5, R6, R11, R18, R19, R20. This phase is the widest in the plan — split it if week 4 slips.

**Phase 5 — Enforcement and ergonomics (week 6).**
depguard config, arch tests, `modgen` scaffolder, Taskfile, CI pipeline, ADRs. Exit: violating branch fails CI at all three layers. CI invokes the same `task` targets developers run locally — no duplicated command definitions in workflow YAML.

**Phase 6 — Proof (week 7).**
k6 load test, committed results, runbook, `docs/modules.md`. Exit: G4 demonstrated with numbers.

---

## 13. Open Questions

**Blocking**
1. **Casbin model shape:** plain RBAC, RBAC with domains (tenant as first-class), or RBAC + ABAC matcher for ownership? Ownership checks currently live in the service layer (§7.6, constraint 3) — decide whether any of that moves into the matcher, because moving it later means rewriting `model.conf` and every stored policy row. *(engineering — before Phase 4)*
2. **Per-key event ordering:** does any consumer actually require it? RabbitMQ gives none across a queue's consumers (§7.9). If wallet balance projections turn out to need per-user ordering, the answer is either a consistent-hash exchange or a different broker — and that is cheaper to decide now than after the topology is deployed. *(engineering — before Phase 3)*
3. **GORM transaction propagation:** does `InTx` pass `*gorm.DB` through context, or does every repository method take a `tx` argument explicitly? Context-passing is less noisy and hides where transactions begin; explicit is verbose and greppable. Whichever is chosen must be uniform — mixing produces silent non-transactional writes. *(engineering — Phase 2)*

**Resolved — do not re-open without an ADR amendment**

| Decision | Outcome | Record |
|---|---|---|
| Scaffolder template engine | `github.com/binafy/go-stub`, isolated in `tools/go.mod`, strict mode, `<< >>` delimiters | ADR-0007 |
| Task runner | go-task (`Taskfile.yml`), not GNU Make. CI invokes the same targets. | ADR-0008 |
| Full stack lock-in | chi · gorm · wire · casbin(+gorm-adapter) · jwt · goose · validator · caarlos0/env · zap · testify/mockery/testcontainers/k6 · air · golangci-lint | §7, ADR-0009 |
| Router (was Q1) | chi — `Mount` and grouped middleware are load-bearing in the module contract | §7 |
| Module config shape (was Q3) | Typed struct owned by each module, embedded in `config.Config` | §7.3 |
| Query layer (was Q2) | GORM for CRUD and associations; raw pgx (`Deps.SQL`) for the outbox poller and hot read paths. sqlc dropped. | §7.1 |
| Broker | RabbitMQ. Outbox is the only publisher. Events and jobs kept as separate abstractions. | §7.9, ADR-0010 |
| Scheduling | robfig/cron in a singleton `cmd/scheduler` that enqueues to RabbitMQ; Redis lock per entry | §7.10 |
| Refresh-token storage (was Q2) | Redis `redis-core`, `noeviction` + persistence. Flush logs everyone out; accepted. | §7.8 |
| API docs | swag code-first, with a route-coverage test making drift a build failure | §7.12 |

**Non-blocking**
4. Does `platform/` become a versioned Go module if a second repo ever needs it, or does the monorepo hold? *(revisit only if a split happens)*
5. Cursor pagination in `shared/pagination` or per-module? Cross-module consistency argues shared; module autonomy argues local.
6. Dedupe store for at-least-once delivery — Redis with TTL, or a Postgres table per consumer group? Redis is faster; Postgres is correct across Redis restarts.

---

## 14. Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Blueprint becomes over-abstracted, slows real feature work | High | Every abstraction must be exercised by two real modules before it ships. Delete anything with one caller. |
| `shared/` becomes a dumping ground | Medium | Hard rule: `shared/` packages have zero internal imports. Arch test asserts it. |
| Eventual consistency surprises product | Medium | Document the window per event flow in `docs/modules.md`. Where sync is genuinely required, keep it in one module. |
| Team routes around boundaries under deadline pressure | High | Enforcement is CI, not review. The compiler layer cannot be argued with. |
| Premature extraction | High | ADR required before any split, naming the measured driver (resource profile, availability class, or team ownership). "Scales better" is explicitly rejected as a driver. |
| Dev-tooling dependency (go-stub) goes unmaintained | Low | Confined to `tools/go.mod`, never in the serving path. Stdlib replacement is ~80 lines and scoped in §6.3. Zero-dependency, so vendoring is a fallback. |
| **GORM makes cross-module joins one line of idiomatic code** | **High** | Three mechanisms in §7.1, all mandatory: per-module table prefix (fails at runtime), unexported models confined to `internal/repo`, `AutoMigrate` banned by arch test. This is the most likely way this blueprint's boundaries get violated. |
| Casbin policy cache goes stale across replicas | High | redis-watcher wired at boot (§7.6). Without it, revoking access appears to work and does not. Integration test with two enforcer instances is a P0 acceptance criterion (R19). |
| Wire's generated file drifts from the providers | Medium | `wire_gen.go` committed; CI regenerates and fails on any diff (R16). If Wire causes more friction than it removes, deleting it is contained — `wire_gen.go` becomes a hand-written `platform.go`. |
| Domain entities acquire `gorm` / `validate` tags under deadline pressure | Medium | Arch test asserts neither tag appears under `internal/domain/`. Once the domain depends on the ORM or the validator, invalid entities become constructible and the mapping cost returns as a bug class. |
| **RabbitMQ becomes a second source of truth** | **High** | Only the outbox dispatcher publishes; depguard blocks `platform/rabbit` from modules. A service calling the broker directly re-creates the dual-write problem the outbox exists to prevent. |
| Unbounded retries on a poison message | High | Bounded attempts + DLX (R35). One malformed payload on an infinite requeue loop saturates a consumer and starves the queue — the classic RabbitMQ outage. |
| Cron fires N times on N replicas | High | Singleton deployment + Redis lock (R36). Fails silently: the job runs 20× and mostly appears to work. |
| Sessions and locks evicted from a shared LRU Redis | High | Two instances with different policies (R37). Presents as random logouts and duplicate scheduled work, with no obvious cause. |
| Swagger annotations drift from handlers | Medium | Route-coverage test (R41) catches missing and deleted routes. It does **not** catch a wrong response schema — accepted limitation of code-first docs; spec-first is the escape hatch if the spec becomes an external contract. |
| Broker adds a failure domain the monolith did not have | Medium | RabbitMQ down means events queue in the outbox rather than being lost — degraded, not broken. Synchronous request paths must never depend on the broker; the arch test asserts no HTTP handler calls `bus.Publish` outside a transaction. |
| **Direct `jobs.Enqueue` loses work when the broker is down** | **High** | The one real gap in the outbox design (§7.9.2, Layer 4). State-change-triggered jobs route through the outbox; scheduled jobs are reconciling; only genuinely fire-and-forget work enqueues directly. |
| Classic queues used instead of quorum queues | High | `declareQueue` hardcodes `x-queue-type: quorum` and takes no override parameter; `Topology.Verify()` fails the boot if any queue in the vhost is classic. Covers hand-declared queues from the management UI, which is how this normally happens. |
| `autoAck: true` on a consumer | High | Acknowledges on delivery, so a crash mid-processing loses the message silently. The most common cause of "messages vanished" reports. Lint rule + review checklist. |
| Publisher blocked by a disk alarm goes unnoticed | Medium | RabbitMQ blocks publishers without closing the connection; the dispatcher stalls with no error. `NotifyBlocked` wired to metric and alert (R48). |
| Cache invalidation inside the transaction | Medium | Serves stale data for a full TTL with nothing to alert on. Arch test (R44). |

---

## 15. Appendix — Reference Snippets

### Task runner — `Taskfile.yml`

**Decision: [go-task](https://taskfile.dev) instead of GNU Make.** Rationale in ADR-0008.

Make's tab-sensitive syntax, recursive variable expansion, and `.PHONY` bookkeeping are pure tax on a repo where nothing is actually a file-dependency graph in the C sense. Task gives typed variables, `requires` for mandatory args, `preconditions` with readable failure messages, `sources`/`generates` for real incremental skipping, and `dotenv` loading — all in YAML that a new engineer can read without knowing Make.

```yaml
version: '3'

dotenv: ['.env']

vars:
  MODULE: {sh: go list -m}
  PKG: ./...

tasks:
  default:
    cmds: [task: --list]

  run:
    desc: Hot-reload API server
    deps: [tools:check]
    cmd: air -c .air.toml

  worker:
    desc: Run event + job consumers
    cmd: go run ./cmd/worker

  dispatcher:
    desc: Run the outbox → RabbitMQ publisher
    cmd: go run ./cmd/dispatcher

  scheduler:
    desc: Run cron (singleton — do not scale this)
    cmd: go run ./cmd/scheduler

  docs:
    desc: Regenerate OpenAPI from swag annotations
    sources: ['internal/modules/*/internal/transport/*.go']
    generates: ['api/docs/swagger.json', 'api/docs/docs.go']
    cmd: swag init -g cmd/api/main.go -d ./ --parseDependency --parseInternal -o api/docs

  test:
    desc: Unit tests only — no Docker required
    cmd: go test {{.PKG}} -short -race

  test:all:
    desc: Unit + integration + e2e (needs Docker)
    deps: [docker:up]
    cmd: go test {{.PKG}} -race -count=1

  test:integration:
    cmd: go test ./test/integration/... -race -count=1

  lint:
    desc: golangci-lint incl. depguard module-isolation rules
    sources: ['**/*.go', .golangci.yml]
    cmd: golangci-lint run

  arch:
    desc: Architecture tests — cross-module import graph
    sources: ['internal/**/*.go', 'test/arch/*.go']
    cmd: go test ./test/arch/... -v

  check:
    desc: Everything CI runs. Use this before pushing.
    deps: [lint, arch, test, generate:verify]

  generate:verify:
    desc: Fails if committed generated code is stale (R16)
    cmds:
      - task: generate
      - git diff --exit-code -- internal/app/wire_gen.go api/docs '**/mocks/**'

  wire:
    desc: Regenerate the platform DI graph
    sources: ['internal/app/wire.go', 'internal/platform/**/provider.go']
    generates: ['internal/app/wire_gen.go']
    cmd: wire ./internal/app

  mocks:
    desc: Regenerate port mocks (ports only — see §7.7)
    sources: ['.mockery.yaml', 'internal/modules/*/ports.go', 'internal/platform/authz/authz.go']
    cmd: mockery

  generate:
    desc: All codegen
    deps: [wire, mocks, docs]

  outbox:replay:
    desc: 'Republish outbox events. Usage: task outbox:replay FROM=2026-09-01T00:00Z TOPIC=user.registered.v1'
    requires:
      vars: [FROM]
    cmd: go run ./cmd/dispatcher replay --from={{.FROM}} --topic={{.TOPIC}}

  rabbit:replay:
    desc: 'Move DLQ messages back to their source queue. Usage: task rabbit:replay QUEUE=wallet.user-registered'
    requires:
      vars: [QUEUE]
    cmd: go run ./cmd/worker replay-dlq --queue={{.QUEUE}}

  migrate:new:
    desc: 'New goose migration. Usage: task migrate:new MODULE=wallet NAME=add_limits'
    requires:
      vars: [MODULE, NAME]
    cmd: goose -dir internal/modules/{{.MODULE}}/migrations create {{.NAME}} sql

  migrate:
    desc: Apply all enabled modules' migrations
    cmd: go run ./cmd/migrate up

  migrate:down:
    prompt: Roll back the last migration — are you sure?
    cmd: go run ./cmd/migrate down 1

  new-module:
    desc: Scaffold a module. Usage: task new-module NAME=wallet
    requires:
      vars: [NAME]
    preconditions:
      - sh: '[ ! -d internal/modules/{{.NAME}} ]'
        msg: 'module {{.NAME}} already exists'
    cmds:
      - cd tools && go run ../cmd/modgen -name={{.NAME}}
      - task: lint
      - task: arch
      - echo '→ add {{.NAME}} to internal/app/registry.go'

  tools:check:
    internal: true
    cmd: cd tools && go mod download

  docker:up:
    desc: postgres + pgbouncer + redis-core + redis-cache + rabbitmq
    cmd: docker compose -f deploy/compose.yaml up -d --wait

  load:
    desc: k6 load profile against a local stack
    deps: [docker:up]
    cmd: k6 run test/load/read_path.js
```

Three details worth keeping:

- **`requires: vars: [NAME]`** — omitting `NAME` fails with a named error instead of scaffolding a module called empty-string. Make's `$(NAME)` expands silently to nothing.
- **`preconditions`** — the "already exists" guard lives here with a readable message, not buried in `modgen`'s Go code. Duplicating it in both places is fine; the fast one should fail first.
- **`sources`/`generates`** — `task lint`, `task wire` and `task mocks` no-op when nothing changed. This is the one Make feature people actually want, and Task's version does not require declaring every output file by hand.

**CI calls the same targets.** A GitHub Actions step is `task check`, not a re-listed set of `go test` flags. Divergence between local and CI commands is a recurring source of "works on my machine" and the fix is having exactly one definition.

### macOS toolchain

```bash
brew install go go-task golangci-lint k6
brew install --cask orbstack

go install github.com/air-verse/air@latest
go install github.com/google/wire/cmd/wire@latest
go install github.com/vektra/mockery/v2@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/swaggo/swag/cmd/swag@latest

# dev-time only — isolated module, never in the service dependency graph
cd tools && go get github.com/binafy/go-stub
```

### PgBouncer-safe pgx config

```go
cfg, _ := pgxpool.ParseConfig(dsn)
cfg.MaxConns = 8                                   // per pod. 20 pods × 8 = 160 < PgBouncer pool
cfg.MaxConnLifetime = 30 * time.Minute
cfg.MaxConnIdleTime = 5 * time.Minute
cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec // no prepared stmt cache
```

GORM sits on the same pool via `stdlib.OpenDBFromPool` and **must** have its own cache disabled too:

```go
sqlDB := stdlib.OpenDBFromPool(pool)
gormDB, _ := gorm.Open(gormpg.New(gormpg.Config{Conn: sqlDB}), &gorm.Config{
	PrepareStmt:            false, // ← "prepared statement already exists" under PgBouncer
	SkipDefaultTransaction: true,  // GORM wraps every write in a tx by default; we do it explicitly
})
```

`SkipDefaultTransaction: true` is a throughput decision as much as a correctness one — an implicit `BEGIN`/`COMMIT` around every single-row insert is wasted round trips at 7k RPS, and it obscures where transaction boundaries actually are.

Pool math is the most common cause of Go-service outages at this scale. `pods × MaxConns` must stay under the PgBouncer pool size, and the PgBouncer pool must stay under Postgres `max_connections`.
