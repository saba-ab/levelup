# Architecture Reference: Go Modular Monolith Blueprint

This document describes the architecture of the blueprint repository so that a product requirements document (PRD) for a new system can be written on top of it without reading the code. It is self-contained. Everything in it is what the repository actually does today, not a proposal.

How to read it:

- Sections 1 to 19 describe the substrate: what is fixed, what every module gets, and which rules are enforced by the compiler, the linter, and the architecture tests.
- Section 20 lists the requirement identifiers the blueprint uses (`R1..R49`) so a PRD can cite them.
- Section 21 is a checklist of what a PRD built on this blueprint must specify.
- Section 22 maps common iGaming concerns onto the mechanisms above. It is guidance for the PRD author, not a design decision.

Vocabulary used throughout: `PRD §N` cites a section of the blueprint's own PRD, `P1..P6` are the six core principles, `R<n>` are requirement identifiers, and `ADR-000n` are architecture decision records.

---

## 1. Summary

The blueprint is a Go modular monolith: one repository, one Go module, one set of binaries deployed together, and business modules whose implementation is private to them at the compiler level. It is built so that a module can later be extracted into its own service by changing one adapter line and one configuration value, without paying the distributed-systems cost up front.

Load-bearing facts:

| Fact | Value |
| --- | --- |
| Language and module | Go 1.26, module path `myapp` (renameable) |
| Binaries | `api`, `worker`, `dispatcher`, `scheduler`, `migrate` |
| Database | PostgreSQL 17, one schema per module (`<name>_svc`), PgBouncer in transaction pooling in front of the writer |
| Cache and ephemera | Two Redis instances: `redis-core` (durable, `noeviction`) and `redis-cache` (`allkeys-lru`) |
| Broker | RabbitMQ 4, quorum queues everywhere, fed only by a Postgres transactional outbox |
| Cross-module writes | Never in one transaction. Owning module writes and publishes an event in one transaction; peers react asynchronously |
| Cross-module reads | Consumer-defined port interface plus an adapter (in-process today, HTTP after extraction) |
| Boundaries | Enforced by the Go compiler (`internal/`), by `depguard` lint rules, and by architecture tests. Zero violations reachable on the main branch |
| Design target | ~7,000 requests per second peak, p99 under 200 ms on cached reads |
| Measured on the reference modules | 12,132 RPS at p99 2.81 ms on the cached read path; cold boot to serving 0.68 s; unit suite 9.6 s; full suite with containers 38 s |

The repository ships with no business modules and an empty module registry. Three reference modules (`user`, `wallet`, `notification`) were built to prove every pattern and were then removed; their code survives as excerpts in the cookbook (`docs/examples.md`). A scaffolder generates a new module skeleton that compiles and passes every check untouched.

---

## 2. Core principles (P1 to P6)

These are the decisions everything else follows from. They are not negotiable within a project built on the blueprint; changing one is an ADR, not a pull request.

**P1. Module privacy is compiler-enforced.** Each module's implementation lives under `internal/modules/<name>/internal/`. Go's `internal/` rule means no package outside `internal/modules/<name>/` can import it. It does not compile.

**P2. Producers own event schemas; consumers own synchronous interfaces.** Outbound events are defined in the producer's `contracts/` package: one authoritative schema, data only. Inbound synchronous dependencies are declared by the consumer as its own port interface with its own snapshot type, bridged by an adapter. The consumer never imports the provider's service or entity types.

**P3. One transaction never spans two modules.** Cross-module effects go through the outbox, which is the only publisher to RabbitMQ. A module cannot import the broker package. This removes the dual-write problem by construction.

**P4. Schema per module.** Module `wallet` owns schema `wallet_svc` and a database role scoped to it. A cross-module join fails at runtime. `pg_dump --schema=wallet_svc` is the extraction path for data.

**P5. No foreign keys across module boundaries.** A `user_id` column in another module is a bare `uuid`. Migrations that reference another module's schema fail the architecture test.

**P6. Boring by default; the stack is locked.** Every dependency in section 18 is chosen and pinned. Adding one requires a new ADR. Where a chosen tool makes a principle easy to violate (GORM most of all), containment is mechanical, not left to review.

---

## 3. Runtime topology

### 3.1 The five binaries

All five binaries share the same composition root and build the full module registry. Module constructors do no I/O, so `migrate` does not need a broker and `api` does not need to consume queues.

| Binary | Role | Replicas | Listens on |
| --- | --- | --- | --- |
| `api` | HTTP server. Mounts every enabled module under `/api/v1`, serves `/health` | horizontal | `HTTP_ADDR` (:8080) public, `HTTP_ADMIN_ADDR` (:8081) metrics and pprof |
| `dispatcher` | Polls the outbox table and publishes to RabbitMQ with publisher confirms. The only process that ever publishes to the broker | horizontal, safe without coordination (`FOR UPDATE SKIP LOCKED`) | admin port only |
| `worker` | Runs every module's event subscriptions and job consumers. Wraps subscriptions with Redis dedupe and the retry ladder | horizontal | admin port only |
| `scheduler` | Cron. Fires each scheduled job by enqueueing a message; never executes work itself | exactly one, plus a per-tick Redis lock for rolling-deploy overlap | admin port only |
| `migrate` | Applies platform schemas, then each enabled module's migrations in registry order, then provisions module DB roles. Runs as a job before serving binaries roll out. Idempotent | one-shot | none |

The serving binaries (`api`, `worker`, `scheduler`) refuse to boot if the database grants a permission that no enabled module declares. The `dispatcher`, `worker`, and `scheduler` refuse to boot if the RabbitMQ virtual host contains any non-quorum queue other than the single sanctioned priority queue.

### 3.2 Infrastructure

| Component | Local (compose) | Production assumptions |
| --- | --- | --- |
| PostgreSQL 17 | direct on 15432 | Writer traffic through PgBouncer in transaction pooling mode. Reader and migration connections go direct because `search_path` is a session setting. `replicas × DB_MAX_CONNS` must stay under the PgBouncer pool, which stays under `max_connections` |
| PgBouncer | 6432 | transaction pooling; pool default 40 |
| redis-core | 6380, `noeviction`, AOF persistence | refresh tokens, JWT deny list, rate-limit counters, scheduler locks, event dedupe keys, casbin watcher channel. Every key has a TTL |
| redis-cache | 6381, `allkeys-lru`, 256 MB locally | module-scoped read caches only. Losing it is a latency event, never a correctness event |
| RabbitMQ 4 | 5672, management on 15672 | 3 nodes minimum, one per availability zone, `pause_minority`, quorum queues everywhere. The management API is used at boot to verify queue types, so credentials are needed in every environment |

### 3.3 Request and event flow

```text
client ──HTTP──▶ api ──▶ module service ──┐
                                          │ one Postgres transaction:
                                          │   write to <module>_svc.*
                                          │   INSERT into outbox_svc.outbox_events
                                          ▼
                                     Postgres ◀── dispatcher polls (SKIP LOCKED, batch 100)
                                                    │ publish + wait for broker confirm
                                                    │ mark published_at only after confirm
                                                    ▼
                                                RabbitMQ (exchange `events`, topic routing)
                                                    │ queue evt.<group>.<topic> per subscriber
                                                    ▼
                                                 worker ──▶ dedupe on event_id (redis-core)
                                                        ──▶ module handler (idempotent)
                                                        ──▶ ack, or retry ladder, or DLQ
scheduler ──tick──▶ Redis lock ──▶ enqueue to exchange `jobs` ──▶ worker executes job
```

Postgres is the source of truth. RabbitMQ is a delivery mechanism for facts already committed. A total broker outage loses nothing: outbox rows accumulate, the `outbox_unpublished_age_seconds` metric climbs, and rows drain on reconnect.

---

## 4. Repository layout

```text
cmd/
  api · worker · dispatcher · scheduler · migrate    thin mains, each < 100 lines
internal/
  app/                 composition root: the only package that knows every module
    wire.go            google/wire graph for the platform (infrastructure only)
    wire_gen.go        generated, committed, drift-checked
    platform.go        Platform struct + DepsFor(module): mints per-module DB session, cache, logger
    registry.go        hand-written map of module constructors, filtered by MODULES_ENABLED
    lifecycle.go       boot order, router assembly, /health, graceful shutdown, Migrate
    topology.go        RabbitMQ topology declaration for enabled modules
  config/              environment → typed struct, validated at boot; embeds each module's Config
  platform/            infrastructure only, knows no module
    modkit/            Module interface + Deps bundle
    postgres/          pgx pools (writer/reader), GORM base, per-module session, InTx, goose runner, role provisioning
    outbox/            Store (Publish inside a tx), Dispatcher, dead-letter table, replay
    bus/               Envelope, Bus interface, RabbitMQ implementation, dedupe wrapper, in-process test double
    jobs/              Job, Queue interface, RabbitMQ implementation, worker loop with panic recovery
    rabbit/            connection with reconnect, topology (quorum queues), shared consume loop, DLQ replay
    scheduler/         robfig/cron wrapper with Redis lock per tick; enqueues only
    redis/             Core and Cache client types, ModuleCache, Locker, GCRA limiter
    authn/             JWT issue/verify, refresh token store, Principal into context, middleware
    authz/             Permission, Principal, Enforcer interface, casbin implementation, catalogue check, migrations
    httpx/             server with timeouts, base middleware, RequireAuth, rate limit, Decode, problem+json
    idempotency/       Idempotency-Key store and middleware, migrations
    telemetry/         zap, OpenTelemetry tracer, Prometheus registry, admin handler
    clock/             injectable time source
  shared/              dependency-free leaf packages: errs, id (UUIDv7), money (int64 minor units), pagination (cursor), validate
  modules/             business modules (empty in the template)
test/
  arch/                import-graph and AST assertions; regenerates docs/modules.md
  integration/         testcontainers flows: async spine, failure modes, GORM isolation, OpenAPI drift
  e2e/                 boots the real binaries, black-box HTTP
  load/                k6 scripts
tools/
  modgen/              scaffolder, isolated in tools/go.mod so its template engine never enters the service graph
api/
  docs/                generated OpenAPI (swag), committed, drift-checked
  bruno/               request collections
deploy/compose.yaml    local stack only; no Dockerfile or Kubernetes manifests ship with the blueprint
docs/
  adr/                 every decision; adding a dependency needs a new ADR
  examples.md          cookbook: real excerpts of the reference modules
  acceptance.md        every R-number mapped to the test that proves it
  runbook.md           alert thresholds and replay procedures
  modules.md           generated module dependency graph
Taskfile.yml           every command; CI runs the same targets
.golangci.yml          depguard boundary rules
.mockery.yaml          mocks for ports only
```

Import direction is strictly downward: `cmd` → `app` → `modules` and `platform` → `shared`. `platform` never imports `modules`, `config`, or `app`. `shared` imports nothing internal. Modules never import `config`, `app`, `platform/rabbit`, or the AMQP client.

---

## 5. The module contract

### 5.1 The interface

Every module implements one interface. The composition root knows nothing else about it.

```go
type Module interface {
    Name() string                          // stable id: config prefix, schema name, metrics label, health key
    Migrations() fs.FS                     // embedded goose SQL, applied into schema <Name>_svc; nil = no storage
    RegisterHTTP(r chi.Router)             // router already scoped under /api/v1 with platform middleware applied
    Subscriptions() []bus.Subscription     // events this module consumes: {Topic, Group, Handler}
    Jobs() []jobs.Job                      // background work: {Name, Schedule, Priority, Run}
    Health(ctx context.Context) error      // readiness of this module's own dependencies
    Permissions() []authz.Permission       // every permission this module defines
}

// Optional, type-asserted by the composition root:
type Starter interface{ Start(ctx context.Context) error }
type Stopper interface{ Stop(ctx context.Context) error }
type GoMigrator interface{ GoMigrations() []*goose.Migration }   // typed seeds, e.g. the permission catalogue
```

### 5.2 What every module receives

```go
type Deps struct {
    DB       *gorm.DB                // session pinned to <module>_svc. via TablePrefix; cannot address other schemas
    SQL      *postgres.DB            // raw pgx pools: Writer() / Reader() for hot paths
    Cache    *redis.ModuleCache      // prefix-bound to cache:<module>: on redis-cache
    Redis    goredis.UniversalClient // redis-core, for module-owned ephemera
    Bus      bus.Bus
    Outbox   outbox.Store            // Publish(ctx, tx, topic, payload)
    Clock    clock.Clock
    Log      *zap.Logger             // pre-tagged with module=<name>
    Validate *validate.Validator
    Authz    authz.Enforcer
    Metrics  *prometheus.Registry
    Tracer   trace.Tracer
}
```

`Deps` is identical for every module. Nothing module-specific is in it. Module settings live in a separate `Config` struct owned by the module (5.4).

### 5.3 Registry and runtime selection

`internal/app/registry.go` is hand-written and is the one place modules are listed. Adding a module is one line:

```go
"billing": func() modkit.Module { return billing.New(p.DepsFor("billing"), cfg.Billing) },
```

`MODULES_ENABLED` (comma-separated) selects and orders the registry at runtime. Rules:

- Empty is a valid, empty registry. The template boots that way.
- An unknown name fails the boot naming it. A duplicate fails the boot.
- Order is dependency order. A consumer listed before its provider panics at boot with a message saying so.
- Disabling a module must not break the others. If it does, the coupling was real and a test found it.
- Module-specific wiring stays visible in the registry on purpose: a module that owns credentials takes the platform's `authn` handle as an explicit extra argument; a consumer chooses between a local and a remote adapter here (section 8.1).

`/health` lists exactly the enabled modules with per-module status; any failing module returns 503 with the whole map.

### 5.4 Configuration

Configuration is environment variables parsed once into a typed struct and validated before any listener binds. A missing required key or a malformed value kills the process naming the environment key.

Global keys:

| Key | Default | Meaning |
| --- | --- | --- |
| `APP_ENV` | `local` | `prod` switches logging to the JSON encoder |
| `MODULES_ENABLED` | empty | enabled modules, in dependency order |
| `HTTP_ADDR`, `HTTP_ADMIN_ADDR` | `:8080`, `:8081` | public and admin listeners |
| `HTTP_RATE_LIMIT_PER_MINUTE` | 600 | per principal, or per IP when anonymous |
| `DB_WRITER_DSN` | required | through PgBouncer |
| `DB_READER_DSN` | empty → writer | direct to Postgres or a replica |
| `DB_MIGRATE_DSN` | empty → writer | must be direct, never PgBouncer |
| `DB_MAX_CONNS` | 8 | per process |
| `JWT_SECRET` | required, min 32 bytes | HMAC secret |
| `JWT_ACCESS_TTL`, `JWT_REFRESH_TTL` | 15m, 720h | token lifetimes |
| `REDIS_CORE_ADDR`, `REDIS_CACHE_ADDR` | required | the two instances |
| `RABBIT_URL`, `RABBIT_MGMT_URL`, `RABBIT_MGMT_USER`, `RABBIT_MGMT_PASS` | required / guest | AMQP and management API |
| `OUTBOX_MAX_PUBLISH_ATTEMPTS` | 10 | broker nacks per row before dead-lettering |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset | unset means trace ids are created and logged but never exported |

Module keys: each module defines its own `Config` struct with `env` tags in `config.go` at the module root. The composition root embeds it in the global config with `envPrefix:"<NAME>_"`. Nothing in the platform ever learns a module-specific key. A module's extraction switch (for example `WALLET_USER_URL`) is one such key.

---

## 6. Module anatomy

### 6.1 The rich shape

```text
internal/modules/<name>/
  module.go          implements modkit.Module; constructor; the only importable Go file besides contracts/, config.go, ports.go
  config.go          module-owned Config struct with env tags
  ports.go           type aliases re-exporting internal/ports so the registry can name the port
  contracts/         PUBLIC surface: topic constants, event payload structs, permission catalogue,
                     offered Reader interfaces and snapshot types. Data only. Zero dependencies.
  adapters/          bridges this module's ports to a provider's contracts: local (in-process) and remote (HTTP)
  internal/domain    entities, value objects, invariants, package-level error vars. No gorm, validate, or json tags.
  internal/app       services: transaction boundaries, resource-scoped authorization, every outbox publish
  internal/ports     port interfaces the module needs, plus its own snapshot types
  internal/repo      unexported GORM models, toDomain/fromDomain mapping, row locks
  internal/transport chi handlers, request/response DTOs with validate tags, swagger annotations
  migrations/        goose SQL files plus typed Go seeds, embedded, applied into <name>_svc
```

Responsibilities by layer:

- **domain** holds invariants. Constructors are the only way to build a valid entity. Business rules ("a withdrawal may not exceed the balance") return typed domain errors.
- **app** (the service) opens transactions, loads entities, performs the ownership check where the entity is in hand, calls repositories with the transaction, publishes to the outbox inside the same transaction, and invalidates cache after commit. It holds an injectable transaction runner so unit tests pass a no-op.
- **repo** maps between domain types and unexported GORM models. Write methods take `tx *gorm.DB` immediately after `ctx`. Read methods use the repository's own handle. Repositories are tested against a real Postgres container, never mocked.
- **transport** decodes and validates request shape, calls the service, renders responses. It never imports the bus or the outbox (architecture test).

### 6.2 The thin shape

A thin CRUD module collapses `internal/` into three flat files (`repo.go`, `service.go`, `http.go`) and may let the unexported GORM model be the module's type. The rule is about what escapes the package, not how many packages there are. Use the layered shape only when there are invariants worth isolating.

### 6.3 What is public

Importable from outside the module: `module.go` (constructor and the `Module` type), `config.go`, `ports.go` (aliases), everything under `contracts/`, and `adapters/`. Everything under `internal/` is unreachable at compile time.

`contracts/` contains:

- Topic constants with the version in the name: `TopicUserRegistered = "user.registered.v1"`.
- Event payload structs, one per topic version, plain data.
- The permission catalogue: `PermWithdraw = authz.Permission{Module: "wallet", Action: "withdraw"}` and `AllPermissions`.
- Offered read interfaces (`contracts.Reader` with `Snapshot` and `SnapshotByIDs`) and the snapshot types they return, for other modules' local adapters to wrap.

---

## 7. Persistence rules

### 7.1 Schema and role per module

`migrate` creates schema `<name>_svc` and a role `<name>_svc` whose `search_path` is its own schema. Module roles additionally get `INSERT` on `outbox_svc.outbox_events`, the one deliberate exception to schema privacy, because writing the event inside the owning transaction is the whole design. Migrations run with `search_path = <name>_svc,public`, so unqualified DDL lands in the module schema.

Platform-owned schemas: `outbox_svc` (events, dead letters), `authz_svc` (casbin policy rows and the permission catalogue), `idempotency_svc` (stored responses). Each has its own goose version table.

No migration may contain `REFERENCES <other>_svc.` (architecture test). Identifiers referring to another module's entities are bare `uuid` columns.

### 7.2 GORM containment (the highest-risk tool in the stack)

Three mechanisms, all mandatory:

1. **Per-module table prefix.** `Deps.DB` is a GORM session whose naming strategy is pinned to `<name>_svc.`. `Preload("User")` from `wallet` resolves to `wallet_svc.users`, which does not exist, and fails in the first integration test. `TableName()` overrides are banned in modules because the prefix ignores them.
2. **Models never leave `internal/repo`.** GORM models are unexported and mapped to domain types. Domain structs carry no `gorm`, `validate`, or `json` tags (architecture test).
3. **`AutoMigrate` is banned.** goose owns DDL. The string may not appear under `internal/` (architecture test).

Behind PgBouncer transaction pooling, `PrepareStmt: false`, `SkipDefaultTransaction: true`, and the pgx simple protocol are mandatory. Reverting them produces "prepared statement already exists" errors in production.

Where GORM is the wrong tool, the raw pgx pools in `Deps.SQL` are used: the outbox poller, cached hot read paths, and any query whose SQL must be read before it ships.

### 7.3 Explicit transactions

```go
err := postgres.InTx(ctx, db, func(tx *gorm.DB) error {
    w, err := s.repo.ByIDForUpdate(ctx, tx, id)      // SELECT ... FOR UPDATE
    ...
    if err := s.repo.Save(ctx, tx, w); err != nil { return err }
    return s.outbox.Publish(ctx, tx, contracts.TopicWalletDebited, payload)
})
if err != nil { return err }
s.cache.Del(ctx, "wallet:v1:"+id)                    // AFTER commit, never inside
```

Transactions are never smuggled through context. Every write-path repository method takes `tx *gorm.DB`. Money movements take a row lock (`clause.Locking{Strength: "UPDATE"}`) and carry an optimistic `version` column. Cache eviction inside an `InTx` closure fails the architecture test.

### 7.4 Migrations and seeds

goose, SQL files for DDL, Go migrations for typed seeding. The canonical Go migration seeds the module's permission catalogue into `authz_svc.permissions` from the same constants the code enforces, so a permission can never exist in the database under a name the code does not define. `task migrate:new MODULE=x NAME=y` creates a new SQL file.

### 7.5 Shared value types

| Package | Content |
| --- | --- |
| `shared/errs` | error kinds and wrapping (section 10.3) |
| `shared/id` | UUIDv7 string ids: time-ordered, index-friendly, safe in URLs and envelopes |
| `shared/money` | `Amount int64` in minor units, overflow-checked `Add`/`Sub`; sign rules are domain invariants |
| `shared/pagination` | opaque cursor encoding `(timestamp, id)` |
| `shared/validate` | validator singleton, custom rules, mapping to `errs.Invalid` |

---

## 8. Cross-module communication

| Need | Mechanism |
| --- | --- |
| Read another module's data synchronously | Consumer-defined port and adapter (8.1) |
| Write in reaction to another module | Subscribe to a versioned event (8.2, 8.4) |
| Atomic write across two modules | Not permitted. Owning module writes and publishes in one transaction; peers react asynchronously |
| SQL join or foreign key across modules | Not permitted. Denormalize via events, or make two calls |
| Trigger work because state changed | Publish topic `job.<name>` through the outbox (8.5) |
| Periodic work | Cron job declared in `Jobs()`, executed by the worker, reconciling by design (8.6) |

**The event path is the default, always.** A synchronous cross-module call is the exception and needs a written reason (section 9).

### 8.1 Synchronous reads: ports and adapters

The consumer declares what it needs in `internal/ports`, in its own vocabulary, with a batch method from day one:

```go
type UserReader interface {
    ByID(ctx context.Context, id string) (UserSnapshot, error)
    ByIDs(ctx context.Context, ids []string) (map[string]UserSnapshot, error)
}
type UserSnapshot struct{ ID, Status, Currency string }   // the consumer's projection, never the provider's entity
```

Two adapters implement the port under `adapters/`:

- **Local**: wraps the provider's `contracts.Reader` in-process and maps into the snapshot type.
- **Remote**: HTTP client against the provider's internal batch endpoint, with a per-call timeout (2 s in the reference), a circuit breaker (`sony/gobreaker`, opens after 5 consecutive failures, half-open probe after 10 s), and one round trip for any batch size. Breaker-open and transport failures map to `errs.Unavailable`.

The registry picks the adapter: if the consumer's `<NAME>_<PROVIDER>_URL` config key is set, remote; else if the provider module is enabled earlier in the list, local; else fail the boot with an actionable message. **Extraction is that one line plus that one config value.** Nothing inside the consumer changes.

Port mocks are generated with mockery for unit tests. Mocks are for ports only, never for repositories.

### 8.2 Events: contracts and versioning

- Topic names carry the version: `user.registered.v1`. The producer owns the payload struct in its `contracts/`.
- Changes are additive only; new fields are optional. A breaking change is a `.v2` topic with dual publishing, consumer migration, then retirement of v1.
- Envelope: `event_id` (UUIDv7), `topic`, `occurred_at`, `trace_ctx` (W3C traceparent), `payload` (JSON).
- Delivery is at-least-once and unordered (ADR-0012). There is no per-key ordering guarantee. Consumers are idempotent and commutative by construction; the Redis dedupe on `event_id` is a second layer, not the guarantee.

### 8.3 The outbox and the dispatcher

Publishing happens only inside the owning transaction, only from the service layer: `s.outbox.Publish(ctx, tx, topic, payload)` inserts into `outbox_svc.outbox_events` (`event_id`, `topic`, `payload jsonb`, `headers jsonb`, `occurred_at`, `published_at`, `attempts`, `next_attempt_at`). A rollback leaves no event.

The dispatcher:

- Claims batches of 100 unpublished rows in id order with `FOR UPDATE SKIP LOCKED`. Any number of replicas run without coordination; two replicas never double-deliver a row (proven by test with 500 events).
- Publishes each row as a persistent message to exchange `events` with routing key = topic, and waits for the publisher confirm. Only confirmed rows get `published_at`. A crash between confirm and update yields a duplicate delivery, which consumers dedupe.
- A row the broker nacks (full queue with `reject-publish`, alarm) backs off from 1 s doubling to 60 s without blocking other rows; after `OUTBOX_MAX_PUBLISH_ATTEMPTS` (10, about four minutes) it moves to `outbox_svc.dead_letters` with `source='dispatcher'`.
- Idles with exponential sleep from 50 ms to 1 s when the table is empty, and prunes published rows older than 7 days once an hour. The 7-day window is the replay horizon.
- Wires `Connection.NotifyBlocked` to a gauge and a bounded 5 s publish timeout so a broker alarm surfaces as an alert rather than a silent stall.

Replay tooling: `task outbox:replay FROM=<rfc3339> TOPIC=<topic>` clears `published_at` on matching rows; `task outbox:deadletter:list` and `task outbox:deadletter:replay ID=<id>` (or `ALL=true TOPIC=<topic>`) move dead letters back into the outbox with a fresh attempt count. Nothing is ever dropped.

### 8.4 Consuming: the worker

For each enabled module and each `Subscription{Topic, Group, Handler}` the worker consumes queue `evt.<group>.<topic>`. Group is normally the module name, so every module gets its own copy of every event it subscribes to.

- Handler is wrapped with dedupe: `SET NX EX 24h` on the event id in redis-core. Dedupe fails open (a dead Redis does not stop consumption).
- Prefetch is explicit (32 for events). Acknowledgement is manual, after the handler returns nil.
- A handler returning an `errs.Invalid` error (or the `rabbit.ErrDeadLetter` sentinel) parks the message immediately: the payload will be exactly as invalid in five seconds.
- Any other error climbs the retry ladder: three quorum tier queues with TTLs of 5 s, 30 s, and 2 m, each dead-lettering back to the source queue. After the ladder, the message parks in `<queue>.dlq`. The broker also enforces `x-delivery-limit = 5`.
- Every park is mirrored into `outbox_svc.dead_letters` with `source='consumer'` and the handler's error, so both sides of the broker are visible in one SQL table.
- `task rabbit:replay QUEUE=evt.<group>.<topic>` shovels a DLQ back onto its source queue with a fresh ladder.

Trace context is extracted from the envelope, so one trace id spans HTTP request, outbox insert, dispatch, and consumption (proven by test).

### 8.5 Jobs versus events

| | Events (`bus.Bus`) | Jobs (`jobs.Queue`) |
| --- | --- | --- |
| Exchange | topic `events` | direct `jobs` |
| Routing key | topic (`user.registered.v1`) | job name |
| Consumers | many independent groups | exactly one group |
| Semantics | "this happened" | "do this" |
| Priority | none | optional, via the one classic queue `job.priority` |
| Producer | outbox dispatcher only | outbox (topic `job.<name>`), scheduler, or direct `Enqueue` for fire-and-forget work only |

The one gap in the outbox design: a direct `jobs.Enqueue` writes to the broker with no Postgres row behind it, so it is lost if the broker is down. Rules:

- State-change-triggered work ("send receipt after payment") publishes topic `job.<name>` through the outbox and inherits every guarantee. Proven by stopping the broker, performing the state change, restarting, and asserting exactly-once execution.
- Direct `Enqueue` is allowed only from an allowlist (the scheduler) enforced by architecture test.
- Job handlers get the same retry ladder, DLQ, dead-letter mirror, and panic recovery as event handlers.
- `job.priority` is the single classic queue in the system (quorum queues do not support `x-max-priority`). The durability trade is confined to work that may be lost. Priority queues need prefetch in the 1 to 10 range to mean anything.

### 8.6 Scheduling

`cmd/scheduler` runs at one replica and still takes a Redis lock per job per tick (`lock:cron:<job>:<tick>`, 5 m TTL, 30 s tick timeout) because one replica is a lie during a rolling deploy. Two scheduler instances enqueue exactly once per tick (proven by test). Cron runs in UTC with panic recovery.

The scheduler only enqueues. The worker executes. Every scheduled job must be **reconciling**: sweep everything unprocessed since a last-successful-run marker, never "process the last five minutes". A missed tick then self-heals on the next one.

### 8.7 RabbitMQ topology

Declared once at boot by `dispatcher`, `worker`, and `scheduler` for the enabled modules:

| Queue | Type | Purpose |
| --- | --- | --- |
| `evt.<group>.<topic>` | quorum | one consumer queue per (module, topic) |
| `evt.<group>.<topic>.retry.<tier>` | quorum | TTL tier, dead-letters back to the source |
| `evt.<group>.<topic>.dlq` | quorum | parked poison messages |
| `job.<name>`, `.retry.<tier>`, `.dlq` | quorum | one queue per job type |
| `job.priority` | classic | the single sanctioned exception |

Every queue is declared through one helper that hardcodes `x-queue-type: quorum`, `x-quorum-initial-group-size: 3`, `x-delivery-limit: 5`, `x-max-length: 1,000,000`, and `x-overflow: reject-publish`. There is no override parameter. `Topology.Verify` lists the vhost through the management API and refuses to boot on any non-quorum queue, which catches queues hand-declared in the UI.

### 8.8 Eventual consistency

Reactions to an event normally land in under a second (dispatcher poll plus queue depth) but are unbounded during a broker outage. Where product needs read-your-own-write, the read and the write live in one module. The PRD must state the acceptable window per cross-module flow.

---

## 9. Multi-step workflows (sagas)

There is no distributed transaction. A workflow touching several modules or an external provider is a chain of local transactions linked by an id and a state machine on the owning row.

- **Order steps from most reversible to least reversible**, irreversible step last. Prefer a hold with an expiry (a reservation with TTL, a payment authorization) over decrement-then-compensate: a failed hold expires on its own; a failed compensation leaves the system wrong.
- **Never call another module or an external API inside an open transaction.** Commit the pending state, call outside, settle in a second transaction.
- **Derive every external idempotency key from your own ids** (row id plus attempt number), never random, so a retry after a timeout returns the original outcome.
- **A business outcome is a result, not an error.** A declined payment returns a value and settles the row as failed. Only unknown outcomes (timeout, 5xx, breaker open) return `errs.Unavailable`, so the ladder retries instead of dead-lettering.
- **The event path is the default.** Module A writes its row and publishes; provider module B reacts, does its work, publishes the outcome; A subscribes and settles. Callers get a pending state and poll or receive a notification.
- **A synchronous call to another module is allowed only when the feature cannot work otherwise** (a checkout that must show a decline before the customer leaves the page). It is a fast path layered on the event path, never a replacement: keep the outcome subscription, settle on a definitive result, respond 202 with the pending state on an unknown result, make the settle transition idempotent and guarded by attempt id because outcomes arrive out of order. The PR says why the async path alone could not deliver the feature.
- **Every pending state needs a sweep.** A reconciling job resolves rows stuck in pending by asking the provider by idempotency key and releasing expired holds. The DLQ is the human path; the sweep is the automatic one.
- **A provider module records its own attempt row before calling out**, then records the outcome and publishes in one transaction. That row is what the sweep and the caller's retry rely on.

The simplest case, create-on-event, is an upsert on a unique natural key and needs no attempt guard.

---

## 10. HTTP layer

### 10.1 Router and middleware

chi v5. The composition root mounts every enabled module under `/api/v1`; `/health` is at the root; `/metrics` and pprof are on the admin port only. Middleware order on every request:

1. Base: request id, panic recovery (renders a 500 problem with no detail), structured access log, OpenTelemetry span, RED metrics per route, a 30 s context deadline on every handler.
2. Token parsing (global, never rejects): a valid bearer token puts a `Principal` in the context.
3. Rate limiting: GCRA via `redis_rate`, keyed by principal when authenticated and by IP otherwise, 429 with `Retry-After`. Fails open when Redis is unavailable.
4. Idempotency: on non-GET requests with an `Idempotency-Key` header, the request hash and the response are stored in `idempotency_svc.keys`; the same key and body replay the stored response, the same key with a different body returns 422.

Rejection of unauthenticated requests is per route group inside the module: `r.Use(httpx.RequireAuth)`. Server timeouts: `ReadHeaderTimeout` 5 s, `ReadTimeout` 15 s, `WriteTimeout` 35 s, `IdleTimeout` 60 s, 15 s drain on shutdown. Shutdown order: listeners, then module `Stop` hooks in reverse order, then platform cleanup in reverse boot order.

### 10.2 Validation

Request shape is validated only in transport, via `httpx.Decode(r, &dto, validator)` with `validate` tags on DTOs. Invariants are validated by domain constructors. Field errors render as problem+json without the domain importing the validator.

### 10.3 Errors

Errors are `errs.New(kind, msg)` or `errs.Wrap(kind, msg, err)`. The kind is chosen deliberately because both the HTTP layer and the consumer loop read it.

| Kind | HTTP status | Consumer loop |
| --- | --- | --- |
| `Invalid` | 422 | park immediately (DLQ) |
| `NotFound` | 404 | retry ladder |
| `AlreadyExists`, `Conflict` | 409 | retry ladder |
| `PermissionDenied` | 403 | retry ladder |
| `Unauthenticated` | 401 | retry ladder |
| `Unavailable` | 503 | retry ladder |
| `Internal`, unknown | 500 | retry ladder |

Responses are RFC 9457 `application/problem+json`. 5xx detail is never echoed to the client. Domain errors are package-level variables in `internal/domain/errors.go`.

### 10.4 API documentation

`swaggo/swag` scans annotations in every module's transport package into one spec under `api/docs`, committed and drift-checked. A test walks the real chi router and fails on any route without an annotation. Wrong response schemas are not caught; if the spec ever becomes an external contract, the escape hatch is spec-first generation. Bruno collections live in `api/bruno`.

---

## 11. Authentication and authorization

### 11.1 Authentication (platform `authn`)

- JWT access tokens (15 m default) and refresh tokens (30 d default), HMAC-signed with `JWT_SECRET`.
- Refresh tokens live in redis-core under `rt:{userID}:{tokenID}` with an index set `rt:idx:{userID}` so logout-everywhere needs no `SCAN`. Rotation is an atomic `GETDEL`; a refresh token used twice is treated as a leak and revokes the whole family. A deny list `jwt:deny:{jti}` covers explicit logout for the remaining access TTL.
- Expired, malformed, and wrong-signature tokens each produce 401 with distinct log reasons.
- A module that owns credentials (registration, login) takes the platform `authn` handle as an explicit constructor argument in the registry. The blueprint's reference `user` module did this; the template ships no such module, so the PRD must specify one.
- Accepted trade: a `FLUSHALL` on redis-core logs everyone out. This is why redis-core is persistent and separate from the cache.

### 11.2 Authorization (platform `authz`)

- Casbin v3 behind an `Enforcer` interface. No module imports casbin (lint rule).
- Plain RBAC (ADR-0011). Subjects are ids, never names: `user:{uuid}` and `role:{int}`. Objects are permission keys `module:action`. Renaming a role never changes who can move money.
- Policy rows live in `authz_svc.casbin_rule`; the permission catalogue in `authz_svc.permissions`. A Redis watcher propagates policy changes to every replica within seconds (proven with two enforcer instances).
- Permissions are declared per module in `contracts/permissions.go` and seeded by that module's Go migration. At boot, the serving binaries aggregate every enabled module's catalogue and refuse to start if the database grants a permission no enabled module declares.
- Two checks, two places. The role gate (`Authorize(ctx, principal, perm, resource)`) may run in a handler or at the top of the service. The ownership check ("may this user withdraw from this wallet") runs in the service where the entity is loaded, and returns `errs.PermissionDenied`. Holding `wallet:withdraw` does not let a user withdraw from someone else's wallet (proven by test).
- `Principal{UserID, RoleIDs []int64, TenantID}` carries a tenant field so a tenant-scoped model remains adoptable without touching call sites. Multi-tenancy itself is not implemented.
- Not built: role administration API or UI (R26). The reference module shipped one default role id via config.

---

## 12. Redis

Governing rule: nothing exists only in Redis, with two accepted exceptions: cache (cheap to lose) and refresh tokens (accepted trade, persisted).

| Key pattern | Instance | TTL | Owner |
| --- | --- | --- | --- |
| `cache:{module}:{entity}:v{n}:{id}` | redis-cache | 5 m default, ±10% jitter | module |
| `rt:{userID}:{tokenID}`, `rt:idx:{userID}` | redis-core | refresh TTL | authn |
| `jwt:deny:{jti}` | redis-core | remaining access TTL | authn |
| `rl:{scope}:{principal}` | redis-core | window | httpx |
| `lock:cron:{job}:{tick}` | redis-core | 5 m | scheduler |
| `dedupe:{group}:{event_id}` | redis-core | 24 h | worker |
| pub/sub channel | redis-core | none | casbin watcher |

`ModuleCache` is prefix-bound per module; a module cannot read or evict another module's keys (proven by test). Its `GetOrLoad` read path implements singleflight (5,000 concurrent misses on one key produce exactly one database query), TTL jitter, negative caching with a 30 s tombstone, and fail-open on read errors. The `v{n}` segment is the invalidation escape hatch: a shape change is a version bump, not a flush. Invalidation runs after commit, never inside the transaction.

Failure directions are deliberate and opposite: **rate limiting fails open, locks fail closed.** A dead Redis must not 429 the API and must never let two schedulers run one job (proven by killing the container mid-test). Lock release is compare-and-delete, never a plain `DEL`.

Client settings: 200 ms read and write timeouts, 2 retries, pool sized to CPUs. `KEYS` is banned. Every key has a TTL. Values are JSON. No Redis Cluster at v1. Redis is explicitly not used for job queues or for the outbox.

---

## 13. Observability

- **Logs**: zap, JSON encoder in `prod`, every record carries `module` and `trace_id`. GORM queries slower than 200 ms land in the same stream.
- **Traces**: OpenTelemetry, exported over OTLP HTTP when the endpoint is set. The trace survives the broker: one trace id across HTTP request, outbox publish, dispatch, and consume (proven by test).
- **Metrics**: Prometheus on the admin port only. RED per route, plus the metrics that predict this stack's failure modes:

| Metric | Alert | Meaning |
| --- | --- | --- |
| `outbox_unpublished_age_seconds` | > 60 s | the single best health signal: dispatcher stuck, broker unreachable, confirms failing. Delay, not loss |
| `rabbitmq_queue_depth{queue}` | sustained growth | consumers dying or behind |
| `rabbitmq_dlq_total{queue}` | any positive rate | poison messages parking |
| `outbox_dead_letters_total{source}` | any positive rate | dispatcher gave up on a row, or a consumer park was mirrored |
| `pgxpool_acquire_wait_seconds` | p99 > 50 ms | pool exhaustion; re-check pool math before scaling |
| `rabbitmq_connection_blocked` | == 1 for > 30 s | broker disk or memory alarm |

---

## 14. Boundary enforcement

Three layers, all in CI, each demonstrated with a deliberate violation that only that layer catches:

1. **Compiler.** Go's `internal/` rule. `wallet/internal/app` importing `user/internal/domain` does not compile.
2. **depguard** (`.golangci.yml`). Platform never imports modules, config, or the composition root. Modules never import `platform/rabbit`, the AMQP client, `internal/config`, or `internal/app`. `shared/` imports nothing internal. casbin appears only in `platform/authz`.
3. **Architecture tests** (`test/arch`, run by `task arch`):
   - A cross-module import is legal only if the target is that module's `contracts/` package, or the importer is under `adapters/`.
   - Platform has no module imports; shared has zero internal imports.
   - `AutoMigrate` appears nowhere under `internal/`.
   - No `gorm` or `validate` tags under any `internal/domain`.
   - No `REFERENCES <other>_svc.` in any migration.
   - No cache `.Del`/`.Evict` inside an `InTx` or `s.tx` closure.
   - Transport packages never import the bus or the outbox.
   - `jobs.Enqueue` callers are allowlisted.
   - No `TableName()` overrides in modules.
   - Generates `docs/modules.md`, the module dependency graph, from the real import graph.

---

## 15. Testing

| Layer | Tool | Rule |
| --- | --- | --- |
| Unit (`task test`, `-short -race`, no Docker) | testify `require` (never `assert`), hand-written fake repository, fake outbox, fake clock, mockery mocks for ports | services are tested without a database |
| Repository | testcontainers Postgres | repositories are never mocked |
| Platform | each package owns its container helper (`pgtest`, `rabbittest`, `redistest`); container tests skip under `-short` | no build tags |
| Integration (`test/integration`) | testcontainers | async flow through a test-only fixture module, broker outage loses nothing, trace survives the broker, Redis outage failure directions, GORM cross-module isolation, OpenAPI drift |
| End-to-end (`test/e2e`, needs `E2E=1` and the compose stack) | real binaries over HTTP | boots migrate, api, dispatcher, worker and checks `/health`; extend with product journeys |
| Load (`test/load`) | k6, parameterised by `READ_PATH` | baseline committed |

`.mockery.yaml` lists ports only; a repository interface appearing there is a visible diff to argue about. Generated files (`wire_gen.go`, `api/docs`, every `mocks/` directory) are committed and drift-checked; they are regenerated with `task generate`, never edited.

---

## 16. Tooling and workflow

Everything runs through `Taskfile.yml`; CI runs the same targets, so local and CI commands cannot diverge (ADR-0008).

| Target | Purpose |
| --- | --- |
| `task check` | what CI runs: lint, arch, unit tests, generated-code drift. Run before pushing |
| `task test` / `task test:all` | unit only / unit plus integration and e2e via testcontainers |
| `task lint` / `task arch` | depguard rules / architecture tests and `docs/modules.md` |
| `task generate` | wire, mocks, swagger; `generate:verify` fails on drift |
| `task docker:up` | postgres, pgbouncer, redis-core, redis-cache, rabbitmq |
| `task migrate` | platform plus enabled module migrations over the direct DSN |
| `task run` / `worker` / `dispatcher` / `scheduler` | run each binary, API with hot reload |
| `task new-module NAME=x` | scaffold a module that compiles and passes lint and arch untouched |
| `task migrate:new MODULE=x NAME=y` | new goose SQL file |
| `task outbox:replay`, `task rabbit:replay`, `task outbox:deadletter:list`, `task outbox:deadletter:replay` | replay tooling |
| `task load` | k6 |

Adding a module:

1. `task new-module NAME=x`. The scaffolder (`tools/modgen`, template engine isolated in `tools/go.mod`) writes `module.go`, `contracts/`, `internal/app`, `internal/repo`, `internal/transport`, `migrations/0001_init.sql`, refuses to overwrite, and fails on any unresolved placeholder.
2. Add the constructor line to the registry. When the module has settings, add its `Config` embed with `envPrefix:"X_"`.
3. If it reads another module: port in `internal/ports`, aliases in `ports.go`, adapters in `adapters/`, port entry in `.mockery.yaml`.
4. Declare permissions in `contracts/permissions.go` and seed them with a Go migration.
5. Append the name to `MODULES_ENABLED` after any module it reads from. Add every new key to `.env.example`.
6. Publish only from the service inside a transaction via the outbox. React to others only through `Subscriptions()`.

Removing a module: delete its directory, registry line, config embed, and mock entries; drop `<name>_svc` schema and role and its rows in `authz_svc` by hand. Boot refuses grants that no enabled module declares.

Any new dependency needs a new ADR. Existing ADRs: 0001 modular monolith, 0007 scaffolder engine, 0008 go-task, 0009 stack lock, 0010 RabbitMQ behind the outbox, 0011 plain RBAC with ownership in the service, 0012 no per-key ordering, 0013 explicit transaction propagation, 0014 circuit breaker.

---

## 17. Operations

- **Deploy shape**: `api`, `worker`, `dispatcher` scale horizontally; `scheduler` at one replica; `migrate` as a pre-rollout job against the direct DSN. One multi-stage image with a different entrypoint per deployment is sufficient. No manifests ship with the blueprint.
- **Pool math**: `replicas × DB_MAX_CONNS` under the PgBouncer pool, under `max_connections`. Defaults: 8 per process, pool 40.
- **RabbitMQ**: 3 nodes minimum, `pause_minority`, quorum everywhere. Two nodes is worse than one.
- **Redis**: two instances with the eviction policies above; the Go types `redis.Core` and `redis.Cache` are distinct so swapping them is a compile error, but the policies live in infrastructure config.
- **Deliberate exceptions**: `job.priority` is the only classic queue; rate limiting fails open while locks fail closed; the outbox table and `authz_svc` are the only cross-module storage exceptions; a redis-core flush logs everyone out.
- **Consumer review checklist**: manual ack only; handler idempotent by construction; retries bounded (the platform ladder, never `requeue=true` in a loop); scheduled jobs reconciling; state-change work through the outbox.
- **Failure reference**: one broker node dies, nothing lost; whole cluster down, events accumulate in the outbox, only directly enqueued jobs are lost; dispatcher crash after confirm, duplicate delivery deduped; consumer bug corrupts a projection, replay from the outbox within 7 days; poison message, DLQ after the ladder, replay after the fix.

---

## 18. Locked stack

| Concern | Library | Version in `go.mod` |
| --- | --- | --- |
| HTTP router | `go-chi/chi/v5` | 5.3 |
| ORM and driver | `gorm.io/gorm`, `gorm.io/driver/postgres`, `jackc/pgx/v5` | 1.31, 1.6, 5.10 |
| Dependency injection (platform only) | `google/wire` | 0.7 |
| Migrations | `pressly/goose/v3` | 3.27 |
| Validation | `go-playground/validator/v10` | 10.30 |
| Config | `caarlos0/env/v11` | 11.4 |
| Logging | `uber-go/zap` | 1.28 |
| Authentication | `golang-jwt/jwt/v5` | 5.3 |
| Authorization | `casbin/casbin/v3`, `casbin/gorm-adapter/v3`, `casbin/redis-watcher/v2` | 3.11, 3.41, 2.8 |
| Redis | `redis/go-redis/v9`, `go-redis/redis_rate/v10` | 9.22, 10.0 |
| Broker | `rabbitmq/amqp091-go` | 1.14 |
| Scheduling | `robfig/cron/v3` | 3.0 |
| Circuit breaker | `sony/gobreaker/v2` (ADR-0014) | 2.4 |
| Tracing and metrics | OpenTelemetry SDK and OTLP HTTP exporter, `prometheus/client_golang` | 1.46, 1.24 |
| Identifiers | `google/uuid` (v7) | 1.6 |
| API docs | `swaggo/swag`, Bruno collections | 1.16 |
| Testing | `stretchr/testify`, `vektra/mockery` v2.53, `testcontainers-go` (postgres, rabbitmq, redis modules), k6 | 1.12, 0.44 |
| Lint | `golangci-lint` v2.13 with depguard | |
| Task runner | go-task 3.x | |
| Hot reload | air | |
| Scaffolding | `binafy/go-stub`, confined to `tools/go.mod` | |

Rejected and why: Viper (multi-source precedence, untyped), Redlock (disputed guarantees, protects a cron job), Kafka (ordering and replay machinery unjustified at this load), Redis as a job queue (two brokers means two retry models), a DI framework for modules (the registry must stay greppable), classic mirrored queues (deprecated, lossy in partitions).

---

## 19. Non-goals and deferred work

Non-goals of the substrate: microservice deployment topology, gRPC or GraphQL transports at v1 (the contract has a slot for `RegisterGRPC`), Kafka and CQRS read models, multi-tenancy (not prevented, not implemented).

Deferred and designed for, not built:

| ID | Item | Design implication already in place |
| --- | --- | --- |
| R24 | per-module rate limits | limit is global config today |
| R25 | contract tests against producers' event schemas | |
| R26 | role and policy administration API or UI | one default role id via config |
| R27 | migration linting for destructive DDL | |
| R28 | log-structured broker if per-key ordering is ever proven necessary | `bus.Bus` is the seam; consistent-hash exchange first |
| R29 | gRPC transport | keep transport code out of `app/` |
| R30 | separate database instance per module | schema-per-module and no cross-schema FK already make it a connection-string change |
| R31 | multi-tenancy | repositories take tenant from context; `Principal` carries `TenantID` |
| R32 | read models and projections | a projection is another subscriber writing a denormalized table |
| R33 | relationship-based authorization (OpenFGA, SpiceDB) | `authz.Enforcer` is the seam |

Extraction of a module to its own service, when a measured driver exists (resource profile, availability class, team ownership; "scales better" is rejected): switch the consumer's registry line to the remote adapter via its URL config key, remove the provider from `MODULES_ENABLED` in the consumer deployment, `pg_dump --schema=<name>_svc` for the data. An ADR naming the driver is required first.

---

## 20. Requirement identifiers

The blueprint tracks requirements as stable ids. `docs/acceptance.md` maps each to the test that proves it. A PRD built on the blueprint can cite them as "inherited" and take the next free number (R50 onward) for its own requirements.

| ID | Requirement |
| --- | --- |
| R1 | `modkit.Module` interface and `Deps` bundle |
| R2 | Composition root, explicit registry, `MODULES_ENABLED`, `/health` lists enabled modules |
| R3 | Per-module goose migrations into own schema and role; Go migrations seed the permission catalogue |
| R4 | Outbox store plus `SKIP LOCKED` dispatcher; rollback leaves no event; two dispatchers never double-deliver |
| R5 | Two modules demonstrating both dependency directions (sync port, async event) |
| R6 | Remote adapter for the same port with timeout, breaker, batch |
| R7 | Three-layer boundary enforcement |
| R8 | Postgres platform: writer/reader split, `InTx`, per-module GORM session, PgBouncer-safe, no `AutoMigrate` |
| R9 | Cached repository decorator: singleflight, TTL jitter, negative cache |
| R10 | httpx: timeouts, recovery, request id, problem+json, graceful shutdown |
| R11 | Idempotency-Key middleware |
| R12 | `task new-module` scaffolder |
| R13 | Test layering: unit without Docker, containers for repositories, mocks for ports only |
| R14 | Observability: zap, OpenTelemetry, Prometheus, pprof on the admin port |
| R15 | Typed config validated at boot, module-owned Config structs |
| R16 | Wire builds the platform graph; registry hand-written; generated code drift-checked |
| R17 | Request validation at the transport boundary only |
| R18 | JWT access and refresh with rotation and reuse detection |
| R19 | casbin behind `authz.Enforcer`, module-owned permissions, boot-time catalogue check, watcher |
| R20 | Resource-scoped authorization in the service layer |
| R21 | Generated module dependency graph |
| R22 | k6 load profile and baseline |
| R23 | Outbox dead-letter table with backoff, consumer mirror, replay |
| R34 | Outbox to RabbitMQ with publisher confirms; broker outage loses nothing |
| R35 | Separate bus and jobs; quorum queues, per-queue DLX, bounded retry, explicit prefetch |
| R36 | Singleton scheduler with Redis lock that enqueues, never executes |
| R37 | Two Redis instances with distinct eviction policies |
| R38 | Distributed GCRA rate limiting keyed by principal or IP |
| R39 | Trace context survives the broker |
| R40 | Operational metrics with documented thresholds |
| R41 | Generated OpenAPI with a route-coverage drift guard |
| R42 | Module-scoped cache handles |
| R43 | Rate limiting fails open, locks fail closed |
| R44 | Cache invalidation after commit, enforced by architecture test |
| R45 | Quorum queues everywhere; boot refuses classic queues |
| R46 | State-change-triggered jobs route through the outbox |
| R47 | Scheduled jobs are reconciling, not incremental |
| R48 | Blocked-connection handling on the dispatcher |
| R49 | Replay tooling; 7-day outbox retention |

R24 to R33 are listed in section 19.

---

## 21. What a PRD built on this blueprint must specify

The substrate above is fixed. The PRD's job is to fill in the parts the blueprint deliberately leaves empty. A complete PRD on this architecture contains:

**Module list, in dependency order.** For each module: name (becomes schema `<name>_svc`, env prefix `<NAME>_`, metrics label, consumer group), shape (rich or thin), and a one-line ownership statement of which entities it is the source of truth for. Two modules must never own the same fact.

**Per module:**

- Entities and invariants (what the domain constructors enforce).
- HTTP routes with method, path under `/api/v1`, whether the route group requires authentication, which permission gates it, and which ownership check the service performs.
- Events published: topic with version suffix, payload fields, and the transaction that emits each one.
- Subscriptions: topic, what the handler does, and the natural key or upsert that makes it idempotent. State explicitly that redelivery and reordering are tolerated.
- Ports needed from other modules: the snapshot fields, and the batch method. Note the adapter's remote endpoint shape if extraction is foreseeable.
- Jobs: cron jobs with schedule and the last-successful-run marker they sweep from; outbox-triggered jobs as `job.<name>` topics.
- Config keys under the module prefix, with defaults.
- Cache keys, TTLs, and the invalidation points.
- Permissions in `module:action` form.

**Cross-module flows as event chains.** Every workflow that touches more than one module is written as: trigger, owning row and its state machine, the events in order, who settles, the eventual-consistency window the product accepts, and the sweep that resolves stuck rows. If any flow needs a synchronous cross-module call, the PRD states why the event path alone cannot deliver the feature.

**External integrations as provider modules.** For each provider (payments, game aggregators, KYC vendors, messaging): the attempt row, how idempotency keys derive from internal ids, which outcomes are results versus retryable unknowns, callback and webhook handling, and the reconciling sweep.

**Identity.** The template ships no user module. The PRD must specify the module that owns registration, login, refresh, logout, roles, and the default role assignment, or an external identity provider and how the platform's JWT layer relates to it.

**Non-functional targets** in the blueprint's terms: peak RPS, p99 per path class, acceptable async windows, alert thresholds where they differ from the runbook, retention of outbox rows if longer than 7 days is needed for audit (it should not be; audit is its own table).

**Stack additions**, each with an ADR: any new library, any new infrastructure component.

**Requirements** numbered from R50, each with an acceptance criterion that a test can fail.

Things the PRD must not specify, because the blueprint forbids them: microservice topology at v1, cross-module joins or foreign keys, shared tables between modules, a module publishing to the broker directly, a transaction spanning two modules, per-key event ordering, business logic in the scheduler, business validation in HTTP DTOs, classic queues, KEYS or SCAN on a request path, a third Redis role on either instance.

---

## 22. Notes for an iGaming PRD

Guidance only. These are the places where iGaming concerns land on the mechanisms above; the PRD decides the actual decomposition.

**Money.** Amounts are `int64` minor units with a currency per balance, never floats. Balance changes take a row lock on the balance row and bump a `version`. A ledger is an append-only table of entries in the module that owns balances; the balance row is a materialization guarded by the lock, not something recomputed from event order. Idempotency for every money movement comes from a unique natural key on the entry (provider transaction id, or round id plus action plus attempt), so a redelivered event or a retried callback inserts nothing.

**Bets, rounds, and settlement.** A wager and its settlement are two local transactions on one round row with a state machine (placed, settled, voided, rolled back). Provider callbacks from a game aggregator arrive at-least-once and out of order; the settle transition must be idempotent and guarded by the round or transaction id. Debit before credit is the natural reversible-to-irreversible order; a hold with an expiry is preferable to compensating a completed debit where the provider supports it. Every pending round needs a sweep that asks the provider by idempotency key.

**Payments.** Deposits and withdrawals are the canonical provider-module saga: record the attempt row, commit, call the PSP with an idempotency key derived from the attempt id, settle in a second transaction, publish `payment.completed.v1` or a failed outcome. A declined payment is a result, not an error. The wallet module credits on the event with an upsert keyed by the payment id. Withdrawals typically need a hold on the balance first and a manual-review state; both are states on the row, not branches in a handler.

**Bonuses and promotions.** Reactions to `bet.placed.v1`, `payment.completed.v1`, `player.registered.v1`; wagering progress is a projection updated by commutative increments keyed by event id, never by assumed order. Bonus balances are separate ledger entries, not a second balance type with its own arithmetic.

**Player, KYC, and compliance.** A player snapshot (status, KYC level, self-exclusion, limits) is what other modules read through a port. Limits enforced at bet or deposit time are checked in the service of the module performing the money movement, using the snapshot, and the check is repeated inside the transaction if the limit is a hard regulatory bound.

**Game catalogue and provider integration.** The catalogue is a module of its own; per-provider integrations are adapters inside it or provider modules, each with its own attempt rows and breaker. Session tokens for game launches are ephemera in redis-core with a TTL, never the only record of a session.

**Real-time and load.** The bet and balance paths are the hot paths: use the raw pgx pools for the tight queries, cached read decorators for catalogue and player snapshots, and state the per-path RPS and p99 targets separately from the rest of the API. Per-module rate limits (R24) are not built; if a game provider needs a different limit than end users, that is one of the first requirements to add.

**Audit and reporting.** The outbox's 7-day retention is a replay window, not an audit log. Regulatory audit needs append-only tables owned by the modules that produce the facts, and reporting is a subscriber projection (R32) writing denormalized tables in its own schema, rebuildable by replay.

**Ordering.** The blueprint gives no per-key ordering. Anything that would need "process events for one player in order" must instead be commutative or must read the current state from the owning module under a lock. If a projection provably cannot be made commutative, R28 is the trigger and the decision belongs in an ADR before the topology ships.

Illustrative module candidates, in a plausible dependency order: `player` (identity, auth, profile, limits), `wallet` (balances and ledger), `payment` (PSP integration), `game` (catalogue, provider integration, sessions), `bet` (rounds and settlement), `bonus`, `kyc` or `compliance`, `notification`, `reporting`. The PRD should confirm or replace this list, and for every edge between two modules name whether it is a port (synchronous read) or an event (asynchronous reaction).
