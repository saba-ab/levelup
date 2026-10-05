# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go modular monolith blueprint: one deployable set of binaries, modules with compiler-private internals, a Postgres transactional outbox as the only path to RabbitMQ, schema-per-module, and a locked stack. The PRD in `go-modular-monolith-blueprint-prd.md` is the source of truth; code comments cite it as `PRD §N`, principles as `P1..P6`, and requirements as `R1..R49`. `docs/acceptance.md` maps every R-number to the test that proves it. `docs/adr/` records every decision; adding a dependency needs a new ADR (ADR-0009).

Module `myapp`, Go 1.26. The template ships with no business modules and an empty registry. `docs/examples.md` is the cookbook: real excerpts of the three reference modules that used to live in the tree, `user` (rich, fully layered, owned auth), `wallet` (both dependency directions plus resource-scoped authz), and `notification` (thin, flat `internal/`). Every pattern named below has a worked example there. The async-spine integration proofs run against a test-only fixture module in `test/integration`.

## Commands

Everything goes through `Taskfile.yml`; CI runs the same targets (ADR-0008). `.env` is loaded automatically by task.

```bash
task check                 # what CI runs: lint + arch + unit tests + generated-code drift. Run before pushing.
task test                  # unit tests only, -short -race, no Docker
task test:all              # unit + integration + e2e via testcontainers (needs Docker)
task lint                  # golangci-lint incl. depguard boundary rules
task arch                  # architecture tests; also regenerates docs/modules.md
task generate              # wire + mocks + swagger. Commit the output; generate:verify fails on drift.
task docker:up             # postgres + pgbouncer + redis-core + redis-cache + rabbitmq
task migrate               # apply platform + enabled module migrations (direct DSN, not PgBouncer)
task run                   # hot-reload API on :8080, admin (metrics/pprof) on :8081
task worker | task dispatcher | task scheduler
task new-module NAME=x     # scaffold, then add one line to internal/app/registry.go
task migrate:new MODULE=wallet NAME=add_limits
task outbox:replay FROM=2026-09-01T00:00:00Z TOPIC=user.registered.v1
task rabbit:replay QUEUE=evt.wallet.user.registered.v1
task outbox:deadletter:list          # rows parked in outbox_svc.dead_letters, both sides of the broker
task outbox:deadletter:replay ID=42  # or ALL=true TOPIC=t; dispatcher rows re-enter the outbox
```

Single tests:

```bash
go test ./internal/modules/wallet/internal/app/ -run TestWithdrawFromForeignWalletDeniedDespiteRole -short -race
go test ./internal/platform/outbox/ -run TestTwoDispatchersDeliverExactlyOnce -race -count=1   # container test: drop -short
E2E=1 go test ./test/e2e/ -run TestFullUserJourney -count=1                                      # needs task docker:up
```

Container-backed tests call `pgtest.DSN(t)` / `rabbittest.Get(t)` / `redistest.Addr(t)` and skip themselves under `-short`. There are no build tags. E2E skips unless `E2E=1` and drives the real binaries over HTTP only.

## Layout and the dependency rules

```text
cmd/            api · worker · dispatcher · scheduler · migrate  (thin mains)
internal/app/   composition root: wire.go builds Platform, registry.go lists modules, lifecycle.go boots
internal/platform/  infrastructure only, knows no module: postgres, outbox, bus, jobs, rabbit, redis, authn, authz, httpx, idempotency, scheduler, telemetry, modkit
internal/shared/    dependency-free leaf packages: errs, id, money, pagination, validate
internal/modules/<name>/
  module.go       implements modkit.Module; the ONLY importable Go file besides contracts/ and config.go
  config.go       module-owned Config struct, embedded into config.Config with envPrefix <NAME>_
  contracts/      public surface: topics, event payload structs, permission catalogue, offered Reader interfaces. Data only.
  ports.go        type aliases re-exporting internal/ports for the registry (consumer-defined ports)
  adapters/       bridges this module's ports to a provider's contracts (local in-process, or remote HTTP)
  internal/domain entities + invariants, no gorm/validate/json tags
  internal/app    services: transaction boundaries, resource-scoped authz, every outbox publish
  internal/ports  port interfaces + the module's own snapshot types
  internal/repo   unexported GORM models, toDomain/fromDomain mapping
  internal/transport  chi handlers, DTOs, swag annotations, httpx.Decode validation
  migrations/     goose SQL + typed Go seeds, embedded, applied into schema <name>_svc
```

Import rules, enforced in three layers (`docs/enforcement-proof.md`):

- **Compiler**: nothing outside `modules/<name>/` can import `modules/<name>/internal/...`.
- **depguard** (`.golangci.yml`): modules never import `platform/rabbit`, `amqp091-go`, `internal/config`, or `internal/app`; platform never imports modules, config, or app; `shared/` imports nothing internal; casbin appears only in `platform/authz`.
- **arch tests** (`test/arch`): a cross-module import is legal only if the target is `.../contracts` and it is fine from anywhere, or the importer is under `adapters/`. Also: no `AutoMigrate`, no gorm/validate tags under `internal/domain`, no `REFERENCES other_svc.` in migrations, no `.Del`/`.Evict` inside an `InTx` or `s.tx` closure, transport never imports bus or outbox, `jobs.Enqueue` only from the scheduler, no `TableName()` overrides in modules.

Run `task lint` and `task arch` after any change that touches imports; they are fast.

## Cross-module communication (PRD §8)

| Need | Mechanism |
| --- | --- |
| Read another module's data synchronously | Consumer declares its own port in `internal/ports`, adapter under `adapters/` wraps the provider's `contracts.Reader`, maps into the consumer's own snapshot type. Registry chooses local vs remote adapter (`docs/examples.md` §3 and §10). Ship a batch method (`ByIDs`) from day one. |
| Write in reaction to another module | Subscribe to a versioned event in `Subscriptions()`. Handler must be idempotent: delivery is at-least-once and unordered (ADR-0012). |
| Atomic multi-module write | Not permitted. Owning module writes and publishes in one transaction; peers react asynchronously. |
| Cross-module SQL join or FK | Not permitted. `user_id` columns are bare uuids. Denormalize via events or make two calls. |

Events: topic names carry the version (`user.registered.v1`), producers own the payload struct in `contracts/`, changes are additive only, breaking changes mean a `.v2` topic with dual publish. The envelope carries `event_id`, `occurred_at`, and W3C trace context.

Publishing: only inside the owning transaction, via `s.outbox.Publish(ctx, tx, topic, payload)` in the service. The dispatcher (`cmd/dispatcher`) is the only broker publisher; it marks rows published only after a broker confirm, drains with `FOR UPDATE SKIP LOCKED`, and retains published rows 7 days for replay. A row the broker nacks backs off and is retried without blocking other topics; after `OUTBOX_MAX_PUBLISH_ATTEMPTS` it moves to `outbox_svc.dead_letters` and comes back through `task outbox:deadletter:replay`. Nothing is ever dropped.

Consuming: `cmd/worker` wraps every subscription with Redis `event_id` dedupe (fails open) and runs the shared consumer loop in `platform/rabbit/consume.go`. Return an `errs.Invalid` error (or `rabbit.ErrDeadLetter`) for payloads that can never succeed: they park in the DLQ immediately. Any other error climbs the retry ladder (5s, 30s, 2m) and then parks in `<queue>.dlq`. Every park is also mirrored into `outbox_svc.dead_letters` with the handler's error, so both sides of the broker are visible in one SQL table. All queues are quorum; the only classic queue is `job.priority`.

Events vs jobs: `bus.Bus` fans facts out to many consumer groups; `jobs.Queue` delivers work to exactly one consumer. State-change-triggered jobs go through the outbox as topic `job.<name>` (R46), never via direct `Enqueue`. Cron jobs declare `Schedule` in `Jobs()`; the singleton scheduler only enqueues, the worker executes. Scheduled jobs must be reconciling, sweeping from a last-successful-run marker, never incremental (R47, see `wallet.reconcile`).

## Multi-step workflows across modules (sagas)

There is no distributed transaction. A workflow that touches several modules or an external provider (payment, inventory, email) is a chain of local transactions linked by an id and a state machine on the owning row. Design rules that follow from the sections above:

- **Order steps from most reversible to least reversible**, and make the irreversible step last. Prefer a hold with an expiry (inventory reservation with TTL, a Stripe authorization) over decrement-then-compensate: a failed hold expires on its own, a failed compensation leaves the system wrong.
- **Never call another module or an external API inside an open transaction.** Commit the pending state first, call outside, then settle in a second transaction. The reasoning is the same as R44 for cache eviction.
- **Derive every external idempotency key from your own ids** (order id + attempt), never random. A retry after a timeout must reuse the key so the provider returns the original outcome.
- **A business outcome is a result, not an error.** A declined card returns a value and settles the row as failed. Only unknown outcomes (timeout, 5xx, breaker open) return `errs.Unavailable`, so the ladder retries instead of dead-lettering.
- **The event path is the default. Always.** A module writes its own row and publishes; the provider module reacts to the event, does its work, and publishes the outcome; the first module subscribes and settles. Callers get a pending state and poll or receive a notification. Do not reach for a synchronous cross-module command because it feels simpler or because a client wants an immediate answer.
- **A synchronous call to another module is the exception, allowed only when the feature cannot work any other way**, for example a checkout that must show a decline before the customer leaves the page. When it is genuinely required, it is a fast path layered on top of the event path, never a replacement: keep the subscription to the provider's outcome events, settle on a definitive result, and on an unknown result respond 202 with the pending state. Make the settle transition idempotent so whichever arrives first wins and the other is a no-op, and guard transitions by attempt id because outcomes can arrive out of order. Say in the PR why the async path alone could not deliver the feature.
- **Every pending state needs a sweep.** A reconciling job resolves rows stuck in a pending state by asking the provider by idempotency key and releasing expired holds. The DLQ is the human path; the sweep is the automatic one.
- **A provider module records its own attempt row before calling out, then records the outcome and publishes in one transaction.** That row is what the sweep and the caller's retry both rely on.

The wallet subscriber in `docs/examples.md` §6 is the reference for the simple case: create-on-event is an upsert on a unique `(user_id, currency)` index, so it is idempotent by construction and needs no attempt guard.

## Persistence rules (PRD §7.1, ADR-0013)

- Transactions are explicit: `postgres.InTx(ctx, db, func(tx *gorm.DB) error)`. Every write-path repository method takes `tx *gorm.DB` right after `ctx`; read methods use the repo's own handle. Services hold `s.tx` as an injectable runner so unit tests pass a no-op.
- `Deps.DB` is already pinned to `<module>_svc.` via `TablePrefix`. Do not add `TableName()` methods; the prefix ignores them. Derive tables from struct names.
- GORM models stay unexported in `internal/repo` and map to domain types. The thin-module carve-out (notification) lets the model be the type, still unexported, never in `contracts/`.
- `PrepareStmt: false` and `SkipDefaultTransaction: true` are mandatory behind PgBouncer. Writer goes through PgBouncer, reader and migrations connect direct.
- goose owns DDL. `task migrate:new` for SQL; typed seeds (permission catalogues) are Go migrations in `migrations/`.
- Money movements take a row lock (`clause.Locking{Strength: "UPDATE"}`) and also carry an optimistic `version`.
- Cache invalidation happens in the service after commit, never inside the transaction. Module caches are prefix-bound (`Deps.Cache`); `redis.Core` (durable, noeviction) and `redis.Cache` (LRU) are distinct Go types. Rate limiting fails open, locks fail closed (R43).

## Errors, HTTP, auth

- Errors are `errs.New(kind, msg)` / `errs.Wrap(kind, msg, err)` with kinds `Invalid, NotFound, AlreadyExists, Conflict, PermissionDenied, Unauthenticated, Unavailable, Internal`. `httpx.Error` maps kind to status and renders RFC 9457 problem+json; 5xx detail is never echoed. The consumer loop also reads the kind (see above), so choose it deliberately.
- Domain errors are package-level vars in `internal/domain/errors.go`. Validation of request shape happens only in transport via `httpx.Decode` with `validate` tags on DTOs, never on domain structs.
- Auth: token parsing is global; rejection is per route group with `r.Use(httpx.RequireAuth)`. Role checks go through `authz.Enforcer.Authorize(ctx, principal, perm, resource)`; ownership checks live in the service where the entity is loaded (R20, ADR-0011). Permissions are declared per module in `contracts/permissions.go` and seeded by a Go migration; boot fails if the DB grants a permission no enabled module declares.
- `Idempotency-Key` on non-GET requests is handled by platform middleware: same key and body replays the stored response, same key with a different body returns 422.
- Routes are mounted under `/api/v1` by the composition root; `/metrics` and pprof live on the admin port only.

## Adding a module

1. `task new-module NAME=x`, then add the constructor line in `internal/app/registry.go` and, when the module has settings, the `X x.Config` field with `envPrefix:"X_"` in `internal/config/config.go`.
2. Constructors do no I/O; every binary builds the full registry. Module-specific wiring (a module that owns credentials taking `*authn.Auth`, as the reference `user` module did) is a visible extra argument in the registry, on purpose.
3. If the module needs another module's data, write the port in `internal/ports`, the alias in `ports.go`, the adapter in `adapters/`, and add the port to `.mockery.yaml`. Mocks are generated for ports only, never for repositories (PRD §7.7).
4. Enable it in `MODULES_ENABLED` in dependency order. An empty list is a valid empty registry; an unknown name fails the boot naming it.
5. Removing a module: delete its directory, registry line, config embed, and mock entries, then drop its `<name>_svc` schema and role and its rows in `authz_svc` by hand. Boot refuses grants that no enabled module declares.

## Testing conventions

- `require`, never `assert`.
- Service unit tests use a hand-written fake repo, a fake outbox, `clock.NewFake`, and mockery mocks for ports (`internal/ports/mocks`). See `docs/examples.md` §15.
- Repositories are tested against testcontainers Postgres, never mocked. Platform packages own their container helpers (`pgtest`, `rabbittest`, `redistest`).
- `test/integration` covers cross-package flows (async flow through the fixture module, failure modes, GORM isolation, OpenAPI drift). `test/e2e` boots the real binaries as a smoke; extend it with your own journey. `test/load` is k6, parameterised by `READ_PATH`.
- Generated files (`wire_gen.go`, `api/docs`, `**/mocks/**`) are committed and drift-checked; regenerate with `task generate`, never edit by hand.

## Operations pointers

`docs/runbook.md` has alert thresholds and replay procedures. The single best health signal is `outbox_unpublished_age_seconds` (alert at 60s). A positive rate on `rabbitmq_dlq_total` means a consumer is parking messages: inspect, fix the consumer, then `task rabbit:replay`. Pool math: `pods × DB_MAX_CONNS` must stay under the PgBouncer pool.
