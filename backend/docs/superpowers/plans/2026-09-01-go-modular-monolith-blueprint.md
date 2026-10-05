# Go Modular Monolith Blueprint — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the reusable Go modular-monolith template repo defined by the PRD: compiler-enforced module boundaries, outbox-only eventing over RabbitMQ, schema-per-module Postgres, dual Redis, casbin authz, and a scaffolder — all P0 requirements (R1–R49) demonstrably met.

**Architecture:** Single Go module (`myapp`) with five binaries (`api`, `worker`, `dispatcher`, `scheduler`, `migrate`) sharing a composition root. Modules live under `internal/modules/<name>` with compiler-private `internal/`, public `contracts/` + `module.go` only. Platform (DB, Redis, Rabbit, authz, httpx, telemetry) under `internal/platform`, zero business logic. All cross-module writes go through the Postgres outbox; the dispatcher is the only Rabbit publisher.

**Tech Stack:** chi v5 · GORM+pgx · wire · goose v3 · validator/v10 · caarlos0/env/v11 · zap · golang-jwt/v5 · casbin v2 (+gorm-adapter, redis-watcher) · go-redis v9 + redis_rate v10 · amqp091-go · robfig/cron v3 · OTel + prometheus · swaggo/swag · testify/mockery/testcontainers-go/k6 · golangci-lint (depguard) · go-task · air · binafy/go-stub (tools module only).

**Spec:** `go-modular-monolith-blueprint-prd.md` (repo root). This plan argues from it; every task cites the governing section. Where the PRD contains the code verbatim, the task says so instead of duplicating it — the PRD travels with this plan.

## Global Constraints

- Module path is exactly `myapp` (matches depguard/mockery configs in PRD §9/§7.7). Go `1.26`.
- P1: module implementations under `modules/<name>/internal/` — nothing else may import them (PRD §4).
- P2: events defined in producer's `contracts/`; sync deps as consumer-owned ports + adapters (PRD §4).
- P3: one tx never spans two modules; **only** the outbox publishes to Rabbit; no module imports `platform/rabbit` (PRD §4, §7.9).
- P4/P5: schema `<name>_svc` per module; no cross-schema FKs; `wallets.user_id` is a bare uuid (PRD §4).
- P6: stack is locked; any addition needs an ADR in `docs/adr/` (PRD §4). Additions made by this plan, each with an ADR: `sony/gobreaker` (R6 breaker), `golang.org/x/tools` (arch tests, test-only).
- GORM: `PrepareStmt: false`, `SkipDefaultTransaction: true`, per-module `TablePrefix`, unexported models confined to `internal/repo/`, `AutoMigrate` banned (PRD §7.1, §15).
- Every Rabbit queue is quorum via the single `declareQueue` helper (no override param); the only classic exception is the priority jobs queue, documented in the runbook (PRD §7.9).
- Tests: `require` not `assert`; mockery for **ports only**; integration tests skip under `-short` so `task test` needs no Docker (PRD §7.7, R13).
- Rate limiting fails open; locks fail closed (PRD §7.8.3).
- Cache invalidation only after commit (PRD §7.8.1, R44).
- Redis: every key gets a TTL; `KEYS` banned; JSON values (PRD §7.8.4).
- Commit after every task; conventional-commit messages.

## Resolved open questions (PRD §13, decided by ADR in Task 2)

- **Q1 casbin model:** plain RBAC, subject `role:{id}`, no domains; ownership checks stay in the service layer (§7.6 constraint 3 already mandates this; multi-tenancy is a §3 non-goal). → ADR-0011.
- **Q2 per-key ordering:** not required by any v1 consumer; all subscribers are idempotent + order-independent. Consistent-hash exchange named as the escape hatch. → ADR-0012.
- **Q3 tx propagation:** explicit handle, uniformly: `postgres.InTx(ctx, db, func(tx *gorm.DB) error)`; write-path repo methods take `tx *gorm.DB` after `ctx`; `outbox.Store.Publish(ctx, tx, topic, payload)` joins the same tx. The PRD's own §7.8.1/§8 examples pass tx explicitly. → ADR-0013.

---

## Phase 0 — Bootstrap

### Task 1: Repo skeleton, toolchain, Taskfile

**Files:**
- Create: `go.mod` (`module myapp`, `go 1.26`), `.gitignore`, `.env.example`, `Taskfile.yml` (from PRD §15, verbatim minus not-yet-existing targets), `.air.toml`, `tools/go.mod` (`module levelup/tools`), `deploy/compose.yaml`
- Create: `README.md` (one paragraph: what this blueprint is, pointer to PRD)

**Steps:**
- [ ] Install missing tools: `brew install golangci-lint k6`; `go install github.com/vektra/mockery/v2@latest`; `go install github.com/swaggo/swag/cmd/swag@latest`. Verify each with `--version`.
- [ ] `go mod init myapp`; write `.gitignore` (bin/, .env, tmp/, .task/).
- [ ] `deploy/compose.yaml`: services `postgres` (postgres:17-alpine, healthcheck `pg_isready`), `pgbouncer` (edoburu/pgbouncer, transaction pooling, points at postgres), `redis-core` (redis:7-alpine, `--maxmemory-policy noeviction --appendonly yes`), `redis-cache` (redis:7-alpine, `--maxmemory 256mb --maxmemory-policy allkeys-lru`), `rabbitmq` (rabbitmq:4-management-alpine, mgmt port 15672). Distinct host ports; healthchecks so `--wait` works (PRD §7.8, §15).
- [ ] `Taskfile.yml` from PRD §15; targets referencing not-yet-written code (`wire`, `mocks`, `docs`, replays, `load`) may exist but will only be *run* once their inputs exist.
- [ ] `task docker:up` → all five containers healthy.
- [ ] Commit: `chore: repo skeleton, toolchain, compose stack`

### Task 2: ADRs

**Files:** `docs/adr/0001-modular-monolith.md`, `0007-go-stub-scaffolder.md`, `0008-go-task.md`, `0009-stack-lock.md`, `0010-rabbitmq-outbox.md`, `0011-casbin-plain-rbac.md`, `0012-no-per-key-ordering.md`, `0013-explicit-tx-propagation.md`, `0014-gobreaker.md`

- [ ] Each ADR ≤ 1 page: Context / Decision / Consequences, content lifted from the PRD section that resolved it (§13 table, §6.3, §15, §7.9, plus the three Q-resolutions above and gobreaker for R6).
- [ ] Commit: `docs: seed ADRs 0001–0014`

---

## Phase 1 — Skeleton (PRD §12 Phase 1; exit: binary boots, /health lists modules, migrations apply, missing env kills boot naming the key)

### Task 3: `shared/errs`

**Files:** `internal/shared/errs/errs.go`, `errs_test.go`

**Produces:**
```go
type Kind uint8
const (Unknown Kind = iota; Invalid; NotFound; AlreadyExists; Conflict; PermissionDenied; Unauthenticated; Unavailable; Internal)
func New(k Kind, msg string) error
func Wrap(k Kind, msg string, err error) error   // nil err → same as New
func KindOf(err error) Kind                      // walks Unwrap chain; Unknown if none
func WithFields(err error, f map[string]string) error
func FieldsOf(err error) map[string]string
```
No HTTP knowledge (PRD §5 tree note).

- [ ] Write table-driven tests first: KindOf on wrapped chains (incl. `fmt.Errorf("%w")` around an errs error), FieldsOf round-trip, errors.Is/As behavior. Run → fail. Implement. Run → pass.
- [ ] Commit: `feat(shared): errs kind taxonomy`

### Task 4: `shared/validate` + `shared/id` + `shared/money` + `shared/pagination`

**Files:** `internal/shared/validate/validate.go` (+test), `internal/shared/id/id.go` (+test), `internal/shared/money/money.go` (+test), `internal/shared/pagination/cursor.go` (+test)

**Produces:**
```go
// validate
func New() *Validator                       // registers iso4217, custom rules
func (v *Validator) Struct(s any) error     // → errs.Invalid with field map
// id
func NewID() string                          // UUIDv7 (github.com/google/uuid)
// money
type Amount int64                            // minor units; Add/Sub with overflow check
// pagination
func EncodeCursor(t time.Time, id string) string
func DecodeCursor(s string) (time.Time, string, error)  // errs.Invalid on garbage
```
Rule (PRD §14): `shared/` has **zero** `levelup/internal` imports except `shared/errs`.

- [ ] Tests first (validator maps `validator.ValidationErrors` → `errs.Invalid` + FieldsOf populated; cursor round-trip; money overflow). Implement. Pass.
- [ ] Commit: `feat(shared): validate, id, money, pagination`

### Task 5: config

**Files:** `internal/config/config.go`, `config_test.go`

Per PRD §7.3 verbatim shape (Env, ModulesEnabled required, DB{WriterDSN required, ReaderDSN, MaxConns}, JWT{Secret min=32, TTLs}, plus `Redis{CoreAddr, CacheAddr}`, `Rabbit{URL, MgmtURL, MgmtUser, MgmtPass}`, `HTTP{Addr, AdminAddr}`; `Wallet wallet.Config`/`User user.Config` embedded **later** when those modules exist — leave a comment slot now).

- [ ] Test first: `Load()` with missing `MODULES_ENABLED` fails naming the key; malformed duration fails; happy path parses (use `t.Setenv`). Implement with `env.Parse` + `validate.Struct`. Pass. (R15)
- [ ] Commit: `feat(config): typed env config validated at boot`

### Task 6: platform primitives — clock, telemetry

**Files:** `internal/platform/clock/clock.go`, `internal/platform/telemetry/telemetry.go`, `log.go`, `admin.go`

**Produces:**
```go
// clock
type Clock interface{ Now() time.Time }; func System() Clock; type Fake struct{...}
// telemetry
type Telemetry struct{ Log *zap.Logger; Tracer trace.Tracer; Registry *prometheus.Registry; TP *sdktrace.TracerProvider }
func New(cfg config.Config) (*Telemetry, func(), error)   // zap prod/dev by cfg.Env; OTLP exporter optional (no-op if unset)
func (t *Telemetry) AdminHandler() http.Handler            // /metrics + /debug/pprof/*
func ModuleLogger(l *zap.Logger, module string) *zap.Logger // pre-tagged zap.String("module", name)
```
- [ ] Small tests: Fake clock; AdminHandler serves /metrics. Implement. Pass. (R14 partial, §7.11: metrics/pprof on admin port only)
- [ ] Commit: `feat(platform): clock + telemetry (zap, otel, prom, pprof admin)`

### Task 7: bus/jobs/outbox interfaces + envelope + in-process bus

**Files:** `internal/platform/bus/bus.go`, `envelope.go`, `inprocess.go` (+tests), `internal/platform/jobs/queue.go`, `internal/platform/outbox/store.go` (interface only this phase)

**Produces:**
```go
// bus
type Envelope struct{ EventID, Topic string; OccurredAt time.Time; TraceCtx map[string]string; Payload json.RawMessage } // PRD §7.11
func NewEnvelope(ctx context.Context, clock clock.Clock, topic string, payload any) (Envelope, error) // sets EventID (id.NewID), injects W3C traceparent
type Handler func(ctx context.Context, e Envelope) error
type Subscription struct{ Topic, Group string; Handler Handler }
type Bus interface { Publish(ctx context.Context, e Envelope) error; Subscribe(ctx context.Context, s Subscription) error }
// jobs
type Job struct{ Name, Schedule string; Priority uint8; Run func(ctx context.Context, payload []byte) error } // Schedule=="" → queue-consumer only
type Queue interface { Enqueue(ctx context.Context, name string, payload []byte, opts ...EnqueueOpt) error }
// outbox
type Store interface { Publish(ctx context.Context, tx *gorm.DB, topic string, payload any) error }
```
`inprocess.Bus` is the test double (PRD §5: "NOT a prod path"): synchronous fan-out, per-Group delivery.

- [ ] Tests first for inprocess bus (two groups both receive; handler error surfaces) and NewEnvelope (trace ctx injected when span present). Implement. Pass.
- [ ] Commit: `feat(platform): bus/jobs/outbox contracts, envelope, in-process bus`

### Task 8: modkit — the module contract

**Files:** `internal/platform/modkit/module.go`, `deps.go`

Verbatim PRD §6: `Module` interface (Name, Migrations, RegisterHTTP, Subscriptions, Jobs, Health, Permissions), `Starter`/`Stopper`, and `Deps` (DB, SQL, Cache, Redis, Bus, Outbox, Clock, Log, Validate, Authz, Metrics, Tracer). `authz.Permission`/`Enforcer` must exist first → define `internal/platform/authz/authz.go` **now** with the PRD §7.6 types (Permission, Principal, Enforcer, `From(ctx)`, `Into(ctx, p)`); the casbin impl arrives in Phase 4.

- [ ] Compile-only task (interfaces). `go build ./...` green.
- [ ] Commit: `feat(modkit): Module interface + Deps bundle; authz contract types`

### Task 9: platform/postgres — pools, gorm base, goose runner

**Files:** `internal/platform/postgres/db.go`, `gorm.go`, `migrate.go` (+`postgres_test.go` integration, skipped with `-short`)

**Produces:**
```go
type DB struct{ ... }; func New(ctx, cfg config.Config) (*DB, func(), error)  // writer pool required, reader falls back to writer
func (d *DB) Writer() *pgxpool.Pool; func (d *DB) Reader() *pgxpool.Pool
func (d *DB) GormBase() *gorm.DB          // stdlib.OpenDBFromPool(writer), PrepareStmt:false, SkipDefaultTransaction:true (PRD §15)
func NewModuleDB(base *gorm.DB, module string) *gorm.DB   // PRD §7.1 Mechanism 1 (session w/ TablePrefix "<module>_svc.")
func Apply(ctx context.Context, db *sql.DB, name string, fsys fs.FS) error   // PRD §7.4: CREATE SCHEMA IF NOT EXISTS <name>_svc + goose provider w/ table <name>_svc.goose_db_version
func CreateModuleRole(ctx, pool, name string) error       // role <name>_svc, search_path pinned, no rights elsewhere (R3)
```
`pgxpool` config per PRD §15 (QueryExecModeExec, MaxConns from cfg).

- [ ] Integration test (testcontainers postgres:17): Apply creates schema + version table; second Apply is a no-op; CreateModuleRole → connecting as that role cannot `SELECT` from another schema. Guard: `if testing.Short() { t.Skip }`. Run with `-short` (skips) and without (passes).
- [ ] Commit: `feat(platform): postgres pools, module-scoped gorm, goose runner, module roles`

### Task 10: platform/httpx

**Files:** `internal/platform/httpx/server.go`, `middleware.go`, `render.go`, `errors.go` (+ `httpx_test.go`)

**Produces:**
```go
func NewServer(addr string, h http.Handler) *http.Server   // ReadHeaderTimeout 5s, Read 15s, Write 30s, Idle 60s
func Shutdown(ctx, srv) error
func BaseMiddleware(log *zap.Logger, tracer trace.Tracer, reg *prometheus.Registry) []func(http.Handler) http.Handler // RequestID→ctx+header, Recover→500 problem, zap access log w/ trace_id, otelhttp, RED metrics per route pattern, 30s context timeout
func JSON(w, status, v); func Error(w http.ResponseWriter, r *http.Request, err error)   // problem+json; errs.Kind→status map: Invalid→422 NotFound→404 AlreadyExists/Conflict→409 PermissionDenied→403 Unauthenticated→401 Unavailable→503 else→500; FieldsOf → "errors" member
func Decode(r *http.Request, dst any, v *validate.Validator) error  // json decode (→400) + validate (→errs.Invalid)
type Problem struct{ Type, Title string; Status int; Detail string; Errors map[string]string; TraceID string }
```
- [ ] Tests first: Error renders each Kind to the right status with `application/problem+json`; Recover turns panic into 500 problem + log; request without deadline impossible (assert ctx has deadline inside a probe handler). Implement. Pass. (R10)
- [ ] Commit: `feat(platform): httpx server, middleware, problem+json rendering`

### Task 11: composition root + cmd/api + cmd/migrate + trivial module

**Files:**
- Create: `internal/app/platform.go` (Platform struct + `DepsFor(name)`), `internal/app/wire.go` + generated `wire_gen.go`, `internal/app/registry.go`, `internal/app/lifecycle.go`, `internal/app/app.go`
- Create: `cmd/api/main.go`, `cmd/migrate/main.go`
- Create: `internal/modules/notification/` — thin shape (PRD §5): `module.go`, `contracts/events.go` (empty placeholder types ok), `internal/service.go`, `internal/repo.go`, `internal/http.go`, `migrations/0001_init.up.sql` (+`.down`), `migrations/embed.go`
- Test: `internal/app/registry_test.go`, `internal/modules/notification/internal/service_test.go`

**Interfaces:**
- `Platform` holds: cfg, `*postgres.DB`, gorm base, redis core/cache clients (nil until Task 14 — wire providers added then; for now construct in `platform.go` manually where wire would), Bus (inprocess for api in Phase 1), Outbox (noop stub logging a warning — replaced Phase 3), Telemetry, Clock, Validator, Enforcer (allow-all stub until Phase 4, clearly named `authz.AllowAll` and only referenced from the composition root).
- `DepsFor(module string) modkit.Deps` — mints NewModuleDB session, ModuleLogger, prefix-bound cache (nil-safe stub until Task 14), pre-tagged tracer/metrics (PRD §7.2: "worth more than any Wire provider").
- `registry.go` per PRD §6/§7.2: `buildModules(p *Platform, cfg config.Config) ([]modkit.Module, error)` — map of ctors, filter by `cfg.ModulesEnabled`, unknown name → error.
- `lifecycle.go`: `Run(ctx)` boot order: config → platform → modules → aggregate `Permissions()` → mount `/health` + module routes under `/api/v1` → serve public + admin listeners → on signal: drain HTTP, run Stoppers reverse, close platform (cleanup funcs reverse boot order, R16).
- `/health`: JSON `{status, modules: {name: ok|error}}` from each module's `Health(ctx)` (R2).
- `cmd/migrate`: loads config, for each **platform schema** then each enabled module in registry order: `postgres.Apply` (R3). Subcommands `up` (default) and `down 1`.
- notification module: `POST /notifications` stores a row (id, kind, body, created_at) via plain GORM model (thin carve-out §7.1), `GET /notifications` lists w/ cursor pagination; `Health` pings DB.

**Steps:**
- [ ] Registry test first: enabled subset boots only those; unknown name errors (R2). Implement registry.
- [ ] Wire: providers for telemetry/postgres/clock/validate + `wire.Struct(Platform)`; `task wire` generates `wire_gen.go`; commit generated file (R16).
- [ ] notification service test (fake repo): create/list. Implement module.
- [ ] Boot proof: `task docker:up`; `.env` from `.env.example`; `go run ./cmd/migrate up` applies `notification_svc.0001`; `go run ./cmd/api` boots; `curl /health` → `{"notification":"ok"}`; `MODULES_ENABLED=nope go run ./cmd/api` fails fast naming module; unset `DB_WRITER_DSN` → boot fails naming the key. Capture outputs in the task log.
- [ ] Commit: `feat(app): composition root, api+migrate binaries, notification module — phase 1 exit`

---

## Phase 2 — Persistence & caching (exit: R9 benchmark passes; cross-module Preload fails as designed, R8)

### Task 12: InTx + tx-aware repos

**Files:** `internal/platform/postgres/tx.go` (+ integration test)

```go
func InTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error // BEGIN/COMMIT/ROLLBACK; wraps errs
```
- [ ] Integration test: rollback on error leaves no row; nested use returns error (no silent savepoints). Implement. Pass. (ADR-0013)
- [ ] Commit: `feat(platform): explicit InTx`

### Task 13: user module (rich shape) — domain, repo, service, transport

**Files:** `internal/modules/user/` per PRD §5 tree: `contracts/{events.go,topics.go,types.go,permissions.go}`, `internal/domain/{user.go,errors.go}`, `internal/app/{service.go,service_test.go}`, `internal/repo/{repository.go,model.go,gorm.go}`, `internal/transport/{http.go,dto.go}`, `migrations/{0001_init.up.sql,...,embed.go}`, `config.go`, `module.go`; register in registry + embed `user.Config` in config.

**Interfaces (consumed by wallet later — exact):**
```go
// contracts/types.go
type UserID string; type Status string (StatusActive/StatusBlocked)
// contracts/topics.go
const TopicUserRegistered = "user.registered.v1"
// contracts/events.go
type UserRegisteredV1 struct{ UserID, Email, Currency string; At time.Time }
// module root (public surface, PRD §5 note): 
func New(d modkit.Deps, cfg Config) *Module
func (m *Module) Reader() Reader   // Reader iface: ByID/ByIDs returning contracts-level snapshots — the thing wallet's local adapter bridges to
```
- Domain: `NewUser(email, currency)` validates invariants, typed errors; **no gorm/validate tags** (§7.1/§7.5).
- Repo: unexported `userModel` + mappers (§7.1 Mechanism 2); `Create(ctx, tx, u)` / `ByID(ctx, id)` / `ByIDs(ctx, ids)` / `ByEmail`.
- Service: `Register(ctx, cmd)` inside `InTx`: repo.Create + `outbox.Publish(TopicUserRegistered, UserRegisteredV1{...})` (§8 pattern — with the Phase-1 outbox stub this logs; becomes real in Phase 3); `Get(ctx, id)`.
- Transport: `POST /users` (register), `GET /users/{id}`; swag annotations from day one.
- Migration 0001: `users(id uuid pk, email text unique, status text, currency text, created_at, updated_at)` — in `user_svc` schema implicitly (search_path via goose runner)…tables created unqualified; goose runs with `search_path=user_svc` — set in Apply via connection param.

- [ ] Service unit tests first w/ fake repo + fake outbox (no DB, R13). Then repo integration test (testcontainers): create/byID round-trip preserves domain values. Implement. Pass.
- [ ] Commit: `feat(user): rich module — domain, repo, service, http`

### Task 14: platform/redis + module cache + wire into Deps

**Files:** `internal/platform/redis/client.go`, `module.go`, `lock.go`, `ratelimit.go` (+ integration tests), wire providers updated, `DepsFor` now hands real `*redis.ModuleCache`

- `client.go`: core (short timeouts per PRD §7.8.4, redisotel instrumented) + cache clients from cfg.
- `module.go`: `ModuleCache` verbatim PRD §7.8.1 — `GetOrLoad` (singleflight, ±10% jitter, `"\x00nil"` tombstone 30s, degrade on read error), `Del`, `MGetOrLoad` batch via MGET.
- `lock.go`: `Acquire(ctx, key, ttl) (Lock, error)` SETNX + value token; `Release` via compare-and-delete Lua (PRD §7.8.3 verbatim). Acquire error/held → caller does not run (fail closed).
- `ratelimit.go`: thin wrapper over `redis_rate.NewLimiter` (GCRA).

- [ ] Integration tests (testcontainers redis): GetOrLoad caches; tombstone returns ErrNotFound without hitting loader; **R9 benchmark test**: 5,000 goroutines miss one key → loader counter == 1; lock: second Acquire fails while held; Release only releases own token; **R42**: cache handle for module A cannot read module B's key (prefix proof); **R43 seed**: stop container mid-test → GetOrLoad falls through to loader without error.
- [ ] Commit: `feat(platform): dual redis clients, module cache (singleflight+jitter+negative), locks, gcra limiter`

### Task 15: cached repo decorator + Preload-fails proof

**Files:** `internal/modules/user/internal/repo/cached.go` (+ integration test), `test/integration/gorm_isolation_test.go`

- `cached.go`: decorator over user repository using `Deps.Cache` (`GetOrLoad` keyed `user:v1:{id}`, `ByIDs` via batch, `Del` after commit on mutation — invalidation lives in service *after* InTx returns, R44).
- Isolation test (R8): a deliberately-wrong GORM query from a wallet-prefixed session `Preload`ing users resolves to `wallet_svc.users` and errors at runtime (PRD §7.1). Also assert Writer/Reader routing: reads go to reader pool (verify via `pg_stat_activity` application_name or pool counters).

- [ ] Tests first, implement, pass. Tag phase-2 exit in commit message.
- [ ] Commit: `feat(user): cached repository decorator; prove cross-module preload fails — phase 2 exit`

---

## Phase 3 — Async spine (exit: R4, R34, R35, R36, R45, R46, R48 + broker chaos test)

### Task 16: rabbit conn + topology

**Files:** `internal/platform/rabbit/conn.go`, `topology.go` (+ integration test w/ testcontainers rabbitmq:4-management)

- `conn.go`: dial from cfg, auto-reconnect loop w/ backoff, channel factory, `NotifyBlocked` → `rabbitmq_connection_blocked` gauge + zap warn (R48).
- `topology.go` verbatim PRD §7.9: `declareQueue` (quorum hardcoded, delivery-limit 5, max-length 1M, reject-publish), exchanges `events` (topic) + `jobs` (direct) + `events.dlx`/`jobs.dlx` + per-tier retry queues (TTL 5s/30s/2m dead-lettering back), `EventQueue(module, topic)` = `<module>.<topic>`, `JobQueue(name)`; **the one classic exception**: `jobs.priority` with `x-max-priority=10` (PRD §7.9 trade-off), named in runbook. `Verify(ctx)` via mgmt HTTP API — every queue in vhost is quorum except the documented priority queue → else error (R45).

- [ ] Integration test: declared queues report `"type":"quorum"` via mgmt API; Verify passes; hand-declare a classic queue → Verify fails naming it. Implement. Pass.
- [ ] Commit: `feat(platform): rabbit connection + quorum-only topology with boot verification`

### Task 17: outbox store + platform migrations

**Files:** `internal/platform/outbox/store.go` (real impl replacing stub), `internal/platform/outbox/migrations/{0001_outbox.up.sql,embed.go}` (+ integration test); `cmd/migrate` extended to apply platform schemas (`outbox`) before modules.

```sql
CREATE TABLE outbox.outbox_events (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE, topic TEXT NOT NULL,
  payload JSONB NOT NULL, headers JSONB NOT NULL,      -- carries TraceCtx (R39)
  occurred_at TIMESTAMPTZ NOT NULL, published_at TIMESTAMPTZ);
CREATE INDEX ON outbox.outbox_events (id) WHERE published_at IS NULL;
```
`Publish(ctx, tx, topic, payload)`: build Envelope via `bus.NewEnvelope` (captures trace ctx **at publish time**), `tx.Exec("INSERT INTO outbox.outbox_events ...")`.

- [ ] Integration test (R4): publish inside InTx + rollback → zero rows; commit → one row with headers containing traceparent when a span is active. Implement. Pass.
- [ ] Commit: `feat(outbox): transactional store + outbox schema`

### Task 18: dispatcher

**Files:** `internal/platform/outbox/dispatcher.go`, `cmd/dispatcher/main.go` (+ integration tests)

- Poll loop on writer pool: PRD §8 `FOR UPDATE SKIP LOCKED` batch (100), publish with **deferred confirms** per PRD §7.9.2 Layer 2 verbatim, `markPublished` only confirmed IDs, exponential idle backoff 50ms→1s.
- Metrics: `outbox_unpublished_age_seconds` gauge (oldest NULL row), `outbox_published_total`, `outbox_publish_failures_total` (R40).
- `replay` subcommand: `--from --topic` clears `published_at` for matching rows (R49); pruning: `DELETE ... WHERE published_at < now()-'7 days'` on a ticker (PRD §7.9.2).
- [ ] Integration tests: two dispatcher instances against one table, 1k rows → each event published exactly once (SKIP LOCKED, R4); **kill broker container mid-batch** → rows stay NULL, restart broker → drained (R34). Implement. Pass.
- [ ] Commit: `feat(dispatcher): skip-locked poller with publisher confirms, replay, pruning`

### Task 19: rabbit bus (consume side) + dedupe

**Files:** `internal/platform/bus/rabbit.go`, `dedupe.go` (+ integration tests)

- `rabbit.Bus`: `Publish` (used only by dispatcher; depguard keeps modules out), `Subscribe`: ensure queue `<group>.<topic>` bound to `events`, manual ack after handler success, nack w/o requeue → retry tier queues → DLX after cap (broker `x-delivery-limit` backstop), explicit prefetch 32 (PRD §7.9.2 Layer 3).
- `dedupe.go`: `WithDedupe(rdb, group) func(Handler) Handler` — `SET dedupe:{group}:{event_id} NX EX 86400` on redis-core; duplicate → ack + skip (Q13.6: Redis chosen; handlers stay idempotent by construction as the real guarantee).
- Consumer extracts `TraceCtx` → child span (R39 consume side).
- [ ] Integration tests: happy path delivery; poison message (handler always errors) lands in DLQ within 5 attempts and queue keeps flowing (R35); duplicate event_id delivered once.
- [ ] Commit: `feat(bus): rabbit consumer with bounded retry, dlq, dedupe, trace extraction`

### Task 20: jobs queue + worker binary

**Files:** `internal/platform/jobs/rabbit.go`, `retry.go`, `worker.go`, `cmd/worker/main.go` (+ integration tests)

- `Enqueue(ctx, name, payload, WithPriority(n))` → direct exchange `jobs`; priority routes to the classic `jobs.priority` queue, prefetch 5 (PRD §7.9 note).
- `worker.go`: for each module Job (Schedule=="" too): consume `JobQueue(name)`, panic recovery, per-job timeout, bounded retry → DLQ; `replay-dlq --queue` subcommand shovels DLQ → source (R49).
- `cmd/worker`: builds modules via registry, starts bus Subscriptions **and** job consumers (PRD §5: "rabbit consumers: events + jobs").
- [ ] Integration tests: enqueue→run; panic recovered, message retried then DLQ'd; priority message overtakes backlog on priority queue.
- [ ] Commit: `feat(jobs): rabbit queue, resilient worker, dlq replay`

### Task 21: scheduler binary

**Files:** `internal/platform/scheduler/cron.go`, `cmd/scheduler/main.go` (+ integration test)

- PRD §7.10 verbatim: robfig/cron v3, `cron.WithLocation(time.UTC)`, `cron.Recover`, Redis lock `lock:cron:{name}` (fail closed), **enqueues only** — tick handler is `queue.Enqueue(ctx, job.Name, nil)`; scans registry modules' `Jobs()` where `Schedule != ""`.
- [ ] Integration test (R36): two scheduler instances, 1s-cron test job, 3+ ticks → exactly one enqueue per tick (count via queue depth or a counting consumer).
- [ ] Commit: `feat(scheduler): singleton enqueue-only cron with per-entry locks`

### Task 22: end-to-end async proof — trace + chaos

**Files:** `test/integration/async_flow_test.go`, `test/integration/chaos_test.go`; notification module gains a real subscription (`user.registered.v1` → store notification row) making it the second consumer example.

- [ ] R39 test: HTTP `POST /users` (httptest server w/ real platform against containers) → outbox → real dispatcher → rabbit → consumer; assert **one trace ID** across the four spans (in-memory span exporter) and zap records carry it.
- [ ] R46 chaos test: stop rabbit container → `POST /users` succeeds (outbox row NULL) → start rabbit → event consumed exactly once (dedupe + confirm path); assert notification row count == 1.
- [ ] Commit: `test: async spine e2e — single trace through broker, broker-outage chaos — phase 3 exit`

---

## Phase 4 — Real modules + auth (exit: R5, R6, R11, R18, R19, R20)

### Task 23: authn — JWT + refresh rotation

**Files:** `internal/platform/authn/{jwt.go,store.go,middleware.go}` (+ tests), user module gains `POST /auth/login`, `POST /auth/refresh`; config JWT section already present.

- `jwt.go`: HS256 access (15m, claims: sub, rids, jti) + opaque refresh (uuid) — verify returns distinct errors: expired / malformed / bad signature → 401 w/ distinct zap reasons (R18).
- `store.go` verbatim PRD §7.8.2: Issue (pipeline SET + SAdd + Expire), `Rotate` via `GETDEL`, `ErrTokenReused` → `RevokeAll(userID)` (SMEMBERS+DEL), deny-list `jwt:deny:{jti}` honored by middleware.
- `middleware.go`: parse bearer → `authz.Principal` → ctx (never rejects); `httpx.RequireAuth` (401 problem if no principal).
- [ ] Unit tests: three bad-token classes; rotation invalidates old (miniature: use testcontainers redis); reuse triggers family revocation. Implement. Pass.
- [ ] Commit: `feat(authn): jwt access/refresh with rotation + reuse detection`

### Task 24: authz — casbin behind the interface

**Files:** `internal/platform/authz/{casbin.go,model.conf,watcher.go,migrations/0001_authz.up.sql,catalogue.go}` (+ integration tests); composition root: aggregate `Permissions()`, boot-time drift check; replace AllowAll stub.

- `model.conf`: plain RBAC (ADR-0011), request `sub, obj, act`, policy `p, sub, obj, act`, `g` role graph; subjects `role:{id}`, `user:{id}` (PRD §7.6).
- `casbin.go`: gorm-adapter on `authz_svc.casbin_rule` (own schema, platform-owned exception); `Enforcer.Authorize(ctx, principal, perm, resource)`: role check via casbin; `resource any` reserved for service-layer ownership (constraint 3 keeps ownership out of the matcher).
- `catalogue.go`: `VerifyCatalogue(ctx, enforcer, declared []Permission) error` — every `p`-rule object exists in declared set, else boot failure (R19; PRD §7.6 constraint 2).
- `watcher.go`: `casbin/redis-watcher/v2` on redis-core → `LoadPolicy` on change.
- user module migration `0003_seed_permissions.go` (goose Go migration) seeds from `contracts.AllPermissions` (PRD §7.4 verbatim; requires `goose.NewProvider` w/ go migrations registered per module — extend `postgres.Apply` signature to accept them).
- [ ] Integration tests: grant `role:1 user:view` → allow; undeclared permission row → boot check fails; **two enforcer instances**: policy change through one visible in the other <5s via watcher (R19). Implement. Pass.
- [ ] Commit: `feat(authz): casbin enforcer, module-owned catalogue with boot drift check, redis watcher`

### Task 25: wallet module — both dependency directions

**Files:** `internal/modules/wallet/` full tree per PRD §5: `contracts/`, `ports.go` (verbatim §6 UserReader+UserSnapshot+ByIDs), `adapters/user_local.go`, `internal/domain/{wallet.go,errors.go}` (money.Amount, optimistic `version`), `internal/app/service.go` (+tests), `internal/repo/{repository.go,model.go,gorm.go,cached.go}`, `internal/transport/{http.go,dto.go}`, `migrations/0001_init.up.sql` (`wallets: id uuid pk, user_id uuid **bare, no FK** (P5), balance_minor bigint, currency, version int`), `config.go`, `module.go`; registry + config wiring.

- Endpoints: `POST /wallets` (from principal), `GET /wallets/{id}`, `POST /wallets/{id}/deposit`, `POST /wallets/{id}/withdraw` (pessimistic lock `FOR UPDATE` via GORM clause §7.1; publishes `wallet.debited.v1` through outbox).
- Subscription: `user.registered.v1` → create wallet, **idempotent** via `INSERT ... ON CONFLICT (user_id, currency) DO NOTHING` (R5 second direction).
- Job: `wallet.reconcile` `*/15 * * * *` — *reconciling* sweep (recompute balances from ledger since last successful run marker, R47).
- Service `Withdraw` does resource-scoped authz per PRD §7.6 constraint 3: role check `PermWithdraw` **and** ownership (`w.UserID == principal.UserID` unless `PermViewAny`); covered by R20 test: passes middleware, fails in service.
- [ ] Unit tests first (fake repo, mock UserReader — first real mockery target in `.mockery.yaml`): withdraw happy, insufficient funds, not-owner → PermissionDenied (R20), currency mismatch vs user snapshot. Integration: subscription idempotency (deliver twice → one wallet).
- [ ] Commit: `feat(wallet): module with port+adapter read of user, event reaction, resource-scoped authz`

### Task 26: remote adapter + breaker (R6, G3)

**Files:** `internal/modules/wallet/adapters/user_remote.go` (+ test w/ `httptest` stub server), `docs/adr/0014-gobreaker.md` already exists.

- `NewRemoteUserReader(baseURL string, timeout time.Duration)`: GET `/api/v1/internal/users?ids=...` batch endpoint (add internal handler to user transport), 2s timeout, gobreaker (open after 5 consecutive failures, half-open 10s), per-call ctx deadline, maps 404→errs.NotFound; **same interface as local** — swapping is the one-line change PRD §6 promises.
- [ ] Test w/ stub server: batch returns map; breaker opens after N failures (stub returns 500s) and short-circuits; timeout honored. Implement. Pass.
- [ ] Commit: `feat(wallet): remote user adapter — timeout, breaker, batch`

### Task 27: idempotency middleware (R11)

**Files:** `internal/platform/idempotency/{store.go,middleware.go,migrations/0001_idempotency.up.sql}` (+ integration test); mounted in `BaseMiddleware` chain for non-GET.

- Table `idempotency.keys(key text pk, request_hash bytea, status int, response bytea, created_at)`; Postgres for durability (money paths).
- Middleware: `Idempotency-Key` header on POST/PUT/PATCH/DELETE → first request: run + store status/body (tee via recorder) atomically (`INSERT ... ON CONFLICT DO NOTHING` claim-first: loser waits/polls short); replay same key+hash → stored response; same key different hash → 422 problem (R11).
- [ ] Integration test: duplicate POST /wallets/{id}/deposit with same key charges once, second response identical; different body → 422. Implement. Pass.
- [ ] Commit: `feat(platform): idempotency-key middleware backed by postgres`

### Task 28: rate limiting + failure-direction proof (R38, R43)

**Files:** `internal/platform/httpx/ratelimit.go` (uses redis.Limiter) (+ integration test `test/integration/failmodes_test.go`)

- Middleware keyed `rl:api:{principal|ip}` GCRA from per-module/default config; 429 + `Retry-After` (R38); Redis error → log warn + **serve** (fail open, PRD §7.8.3 verbatim).
- [ ] Integration test R43: kill redis-core container → limited endpoint still serves 200; lock `Acquire` in same window returns error and guarded fn does **not** run. Restart container. Pass.
- [ ] Commit: `feat(httpx): gcra rate limiting failing open; prove lock fails closed — phase 4 exit`

---

## Phase 5 — Enforcement & ergonomics (exit: violating branch fails all three layers; R7, R12, R41; CI = task check)

### Task 29: depguard + full golangci config

**Files:** `.golangci.yml` (final): per-module `module-isolation` blocks (files `**/internal/modules/<m>/**` deny `levelup/internal/modules` allow own subtree + every other module's `/contracts`), `platform-purity` (platform never imports modules), `no-direct-rabbit` (modules deny `levelup/internal/platform/rabbit`, P3), `casbin-confined` (only platform/authz imports casbin). Plus baseline linters (govet, errcheck, staticcheck, revive).

- [ ] `task lint` green on the real tree; temporarily add `import user internal` into wallet → lint fails; revert (transcript into `docs/enforcement-proof.md`).
- [ ] Commit: `chore(lint): depguard boundary rules — layer 2 enforcement`

### Task 30: arch tests (R7 layer 3, R8/R17/R44 assertions, R21 graph)

**Files:** `test/arch/arch_test.go`, `openapi_test.go` (Task 32 fills), `helpers.go`

Using `golang.org/x/tools/go/packages` (+ go/ast where needed):
- [ ] `TestModuleBoundaries` (PRD §9 verbatim: cross-module imports only `/contracts`, only from adapters/ports files) — also **writes `docs/modules.md`** dependency graph (R21).
- [ ] `TestPlatformHasNoModuleImports`; `TestSharedHasNoInternalImports` (§14); `TestNoAutoMigrate` (string absent under internal/, §7.1); `TestDomainHasNoTags` (no `gorm:`/`validate:` struct tags under `internal/domain`, R17); `TestNoCrossModuleForeignKeys` (parse `migrations/*.sql` for `REFERENCES <other>_svc`, P5); `TestNoCacheDelInsideTx` (AST: no `.Del(` call inside func literal passed to `InTx`, R44); `TestNoBusPublishInHandlers` (no `bus.Publish` under `internal/transport`, §14); `TestNoDirectEnqueueOutsideAllowlist` (R46: `jobs.Enqueue` callers limited to scheduler + documented fire-and-forget list in `test/arch/allowlist.go`).
- [ ] Violation branch demo: `git checkout -b demo/boundary-violation`, add all three layer-breaking edits (import internal → compiler; import module root → depguard; contracts import from non-adapter → arch test), record the three failures in `docs/enforcement-proof.md`, checkout main, cherry-pick the doc. (R7)
- [ ] Commit: `test(arch): boundary, tag, fk, cache-del, enqueue assertions + module graph — layer 3`

### Task 31: modgen scaffolder (R12, G1)

**Files:** `cmd/modgen/main.go` (PRD §6.3 verbatim), `cmd/modgen/stubs/module/**` (nine stub files per PRD §5 tree: module.go.stub, ports.go.stub, contracts/{events,topics}.go.stub, internal/app/service{,_test}.go.stub, internal/repo/postgres.go.stub, internal/transport/http.go.stub, migrations/{0001_init.up.sql.stub, embed.go.stub}), `tools/go.mod` gains `binafy/go-stub`.

- Stub content: thin-leaning module that compiles: Module impl w/ all seven methods, one `<<NAME>>CreatedV1` event, one HTTP GET, one table. Placeholders `<< PACKAGE >> << NAME >> << SCHEMA >> << TOPIC >> << MODULE >>`.
- **Risk gate:** if `go get github.com/binafy/go-stub` fails or its API ≠ PRD sketch, implement the sanctioned stdlib fallback (~80 lines: WalkDir + text/template `<< >>` delims + go/format + case helpers) in `tools/stubgen/` — PRD §6.3 explicitly scopes this; note it in ADR-0007.
- [ ] Acceptance run (R12): `task new-module NAME=demo` → compiles, `task lint` + `task arch` pass untouched, add registry line → appears in `/health`; run twice → non-zero, files untouched; remove a key from one stub → `ErrMissingKeys` names it; `grep go-stub go.mod` → absent. Then delete demo module.
- [ ] Commit: `feat(modgen): embedded-stub scaffolder on go-stub in tools module`

### Task 32: OpenAPI + drift guard (R41)

**Files:** swag annotations across all module transports (verbatim style PRD §7.12), `api/docs/` generated, `api/bruno/` seed collection, `test/arch/openapi_test.go` (PRD §7.12 verbatim route-coverage walk).

- [ ] `task docs` generates; route-coverage test green; delete one annotation → test names the route; restore.
- [ ] Commit: `feat(api): swagger generation with route-coverage drift guard`

### Task 33: CI + mockery + check green

**Files:** `.github/workflows/ci.yml` (setup-go + go-task + docker; steps: `task check`, `task test:all`), `.mockery.yaml` final (ports only: `wallet.UserReader`, `authz.Enforcer`, `bus.Bus` — PRD §7.7), `docs/runbook.md` (alert thresholds R40 table, classic-priority exception, autoAck checklist, replay procedures R49, eventual-consistency windows).

- [ ] `task check` (lint+arch+test+generate:verify) fully green locally; `task test:all` green.
- [ ] Commit: `chore(ci): single-source pipeline via task check; runbook — phase 5 exit`

---

## Phase 6 — Proof (exit: G4 numbers committed)

### Task 34: e2e black-box + fixtures

**Files:** `test/e2e/e2e_test.go`, `test/fixtures/`

- [ ] Compile `cmd/api` + `cmd/migrate` + `cmd/dispatcher` + `cmd/worker`, run against compose stack, black-box HTTP: register → login → refresh → create wallet (event-driven) → deposit w/ idempotency-key → withdraw → 403 on foreign wallet. Assert via public API only.
- [ ] Commit: `test(e2e): full-binary black-box flow`

### Task 35: k6 load + results (G4, R22)

**Files:** `test/load/read_path.js` (ramp to target RPS on `GET /users/{id}` cached path + `GET /wallets/{id}`), `test/load/RESULTS.md`

- [ ] Run `task load` against local stack; commit actual numbers + hardware note (M-series laptop ≠ commodity server; note deviation from the 7k/200ms target env). If local peak < 7k RPS, record what was reached and the bottleneck observed — the committed methodology is the deliverable.
- [ ] Commit: `test(load): k6 read-path profile + committed baseline`

### Task 36: final acceptance sweep

- [ ] Walk PRD §10 P0 table R1→R49: for each, name the test/artifact proving it; fix any gap found; write the matrix into `docs/acceptance.md`.
- [ ] `task check && task test:all` one last time, green.
- [ ] Commit: `docs: P0 acceptance matrix — blueprint complete`

---

## Self-review notes (spec coverage)

- R1–R5→Tasks 8,11,13,17,18,25 · R6→26 · R7→29,30 · R8→9,15 · R9→14 · R10→10 · R11→27 · R12→31 · R13→throughout (+33) · R14→6,22 · R15→5 · R16→11 · R17→10,30 · R18→23 · R19–R20→24,25 · R34→18 · R35→19,20 · R36→21 · R37→1,14 · R38→28 · R39→22 · R40→18,33 · R41→32 · R42→14 · R43→28 · R44→15,30 · R45→16 · R46→22,30 · R47→25 · R48→16 · R49→18,20.
- P1 picked up cheaply where adjacent: R21 (Task 30), R22 (Task 35), R23 partial (DLQ replay Task 20). R24–R27 deliberately deferred (P1, non-blocking).
- Types cross-checked: `modkit.Deps` fields match Task 14/24 providers; `bus.Handler` signature consistent across 7/19/22; `Job.Run(ctx, []byte)` consistent across 7/20/21.
