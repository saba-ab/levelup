# P0 acceptance matrix (PRD §10)

Every P0 requirement with the artifact that proves it. "Proof" means a test
that fails if the property breaks — not a file that exists.

The reference modules (`user`, `wallet`, `notification`) were removed on
2026-09-06 when the repository became a template. Rows that cited their
tests now cite [`examples.md`](examples.md), where that code lives as
excerpts, and the async-spine proofs run against a test-only fixture module
in [`test/integration`](../test/integration/fixture_test.go). Rows citing
platform tests are unchanged.

| ID | Requirement | Proof |
| --- | --- | --- |
| R1 | `modkit.Module` + `Deps` bundle | [`modkit/module.go`](../internal/platform/modkit/module.go), [`deps.go`](../internal/platform/modkit/deps.go); every scaffold and the integration fixture module implement it with no platform changes; the three reference shapes are in [`examples.md`](examples.md) §2 and §14 |
| R2 | Composition root, explicit registry, `MODULES_ENABLED` | `TestSelectModules*` in [`registry_test.go`](../internal/app/registry_test.go); `TestEmptyModulesEnabledIsAnEmptyRegistry` ([`config_test.go`](../internal/config/config_test.go)); boot proof: unknown module → `fatal: unknown module "nope"`; `/health` lists only enabled modules ([`e2e_test.go`](../test/e2e/e2e_test.go)) |
| R3 | Per-module goose migrations into own schema | `TestApplyCreatesSchemaAndIsIdempotent`, `TestModuleRoleCannotTouchOtherSchemas` ([`postgres_test.go`](../internal/platform/postgres/postgres_test.go)); typed permission seeding shape in [`examples.md`](examples.md) §9 |
| R4 | Outbox + `SKIP LOCKED` dispatcher | `TestPublishRollsBackWithTransaction`, `TestTwoDispatchersDeliverExactlyOnce` ([`outbox_test.go`](../internal/platform/outbox/outbox_test.go)) — 500 events, two dispatchers, every event delivered exactly once |
| R5 | Two modules, both dependency directions | Synchronous read via consumer-defined port + adapter and asynchronous reaction to a versioned event, both in [`examples.md`](examples.md) §2 and §10; the async direction runs live through the fixture module in [`async_flow_test.go`](../test/integration/async_flow_test.go) |
| R6 | Remote adapter for the same port | Remote adapter with timeout, breaker and batch, plus its tests (`TestRemoteTimeoutHonored`, `TestRemoteBreakerOpensAndShortCircuits`), in [`examples.md`](examples.md) §10 and §15 |
| R7 | Three-layer enforcement | [`enforcement-proof.md`](enforcement-proof.md) — one probe per layer, each rejected by the layer designed to catch it |
| R8 | Postgres platform, PgBouncer-safe, prefix isolation | `TestCrossModulePreloadFailsAtRuntime` ([`gorm_isolation_test.go`](../test/integration/gorm_isolation_test.go)); `TestNoAutoMigrate` (arch); `PrepareStmt:false` + `QueryExecModeExec` in [`db.go`](../internal/platform/postgres/db.go) |
| R9 | Cached repository decorator | `TestSingleflightCollapsesConcurrentMisses` — 5,000 concurrent misses on one key → exactly 1 load ([`redis_test.go`](../internal/platform/redis/redis_test.go)); the decorator shape in [`examples.md`](examples.md) §13 |
| R10 | httpx: timeouts, recover, request ID, problem+json | [`httpx_test.go`](../internal/platform/httpx/httpx_test.go): every `errs.Kind` → status, panic → 500 problem, `TestEveryHandlerGetsADeadline`, `ReadHeaderTimeout` asserted |
| R11 | Idempotency middleware | `TestReplayReturnsStoredResponse`, `TestConflictingBodyRejected` (422) ([`idempotency_test.go`](../internal/platform/idempotency/idempotency_test.go)) |
| R12 | `task new-module` scaffolder | Acceptance run recorded in [ADR-0007](adr/0007-go-stub-scaffolder.md): generated module compiles, passes lint+arch untouched, appears in `/health` via one registry line; double run refuses; missing key → `unresolved placeholders: NAEM`; go-stub absent from root `go.mod` |
| R13 | Test layering | `task test` (`-short`) needs no Docker — every container-backed test skips; `task test:all` runs everything; [`.mockery.yaml`](../.mockery.yaml) lists ports only |
| R14 | Observability | `TestTraceSurvivesTheBroker` (R39); zap carries `trace_id` + `module`; GORM slow-query logging in [`gorm.go`](../internal/platform/postgres/gorm.go); pprof + `/metrics` on the admin port only |
| R15 | Typed config, validated at boot | [`config_test.go`](../internal/config/config_test.go): missing key fails naming it; malformed duration names the env key (not the Go field) |
| R16 | Wire builds the platform, registry hand-written | `wire_gen.go` committed; `task check` → `generate:verify` fails on any diff; cleanup funcs compose in reverse boot order |
| R17 | Validation at the transport boundary | `TestDomainCarriesNoFrameworkTags` (arch); `httpx.Decode` renders field errors as problem+json without the domain importing the validator |
| R18 | JWT access + refresh, Principal in context | `TestVerifyDistinguishesFailureModes` (expired / malformed / bad-signature are distinct), `TestRefreshRotation`, `TestReuseRevokesWholeFamily` ([`authn_test.go`](../internal/platform/authn/authn_test.go)); the login/refresh/logout flow in [`examples.md`](examples.md) §12 |
| R19 | Casbin behind the interface, module-owned permissions | `TestVerifyCatalogueCatchesOrphans` (boot fails on a grant no enabled module declares), `TestWatcherPropagatesAcrossReplicas` (two enforcers, < 5s) ([`casbin_test.go`](../internal/platform/authz/casbin_test.go)); depguard confines casbin |
| R20 | Resource-scoped authorization in the service | `TestWithdrawFromForeignWalletDeniedDespiteRole` — principal HOLDS `wallet:withdraw`, still refused — in [`examples.md`](examples.md) §6 and §15 |
| R34 | Outbox → RabbitMQ with publisher confirms | `TestTwoDispatchersDeliverExactlyOnce`; `TestBrokerOutageLosesNothing` — broker stopped, state change succeeds, rows stay NULL, drain on restart ([`async_flow_test.go`](../test/integration/async_flow_test.go)) |
| R35 | Separate bus and jobs, quorum, DLQ, bounded retry | `TestPoisonMessageParksInDLQAndQueueKeepsFlowing` (bus), `TestPanickingJobIsRecoveredRetriedThenParked` (jobs), `TestDedupeSkipsSecondDelivery` |
| R36 | Singleton scheduler that enqueues, never executes | `TestTwoSchedulersEnqueueOncePerTick` — two instances, one enqueue per tick ([`cron_test.go`](../internal/platform/scheduler/cron_test.go)) |
| R37 | Two Redis instances, distinct eviction policies | [`compose.yaml`](../deploy/compose.yaml): redis-core `noeviction`+`appendonly`, redis-cache `allkeys-lru`; distinct Go types (`redis.Core` / `redis.Cache`) make swapping them a compile error |
| R38 | Distributed rate limiting | GCRA keyed by principal-or-IP with `Retry-After` ([`ratelimit.go`](../internal/platform/httpx/ratelimit.go)); the load test's first run proved it enforces (98% 429s at 600/min) |
| R39 | Trace context survives the broker | `TestTraceSurvivesTheBroker`: one trace ID across `POST /api/v1/internal/fixture` → `outbox.publish` → `outbox.dispatch` → `consume` |
| R40 | Operational metrics with thresholds | `outbox_unpublished_age_seconds`, `rabbitmq_connection_blocked`, RED per route; thresholds documented in [`runbook.md`](runbook.md) |
| R41 | Generated OpenAPI with drift guard | `TestEveryRouteIsDocumented` walks the real chi router against `api/docs/swagger.json` |
| R42 | Module-scoped cache handles | `TestModulePrefixIsolation` (module A cannot read or evict module B's key), `TestCacheReadFailureDegradesToLoader` |
| R43 | Fail-open / fail-closed split | `TestFailureDirectionsUnderRedisOutage` — same dead Redis: limiter serves 204, lock acquisition errors ([`failmodes_test.go`](../test/integration/failmodes_test.go)) |
| R44 | Cache invalidation after commit | `TestNoCacheEvictionInsideTransactions` (AST: no `.Del`/`.Evict` inside an `InTx` closure) |
| R45 | Quorum queues everywhere | `TestDeclareCreatesQuorumQueuesEverywhere` (incl. retry tiers and DLQs), `TestVerifyRefusesClassicQueues` — hand-declared classic queue fails the boot |
| R46 | State-change jobs route through the outbox | `TestBrokerOutageLosesNothing` (exactly-once across an outage); `TestEnqueueCallersAllowlisted` (arch) keeps direct `Enqueue` to the allowlist |
| R47 | Scheduled jobs are reconciling | The reconciling sweep with a last-successful-run marker, and its repository test, in [`examples.md`](examples.md) §6 and §7 |
| R48 | Blocked-connection handling | `Connection.NotifyBlocked` → `rabbitmq_connection_blocked` gauge + warn ([`conn.go`](../internal/platform/rabbit/conn.go)); bounded 5s publish timeout so a blocked broker cannot stall silently |
| R49 | Replay tooling | `task outbox:replay FROM=… TOPIC=…` (`TestReplayClearsPublishedAt`), `task rabbit:replay QUEUE=…` (`TestReplayDLQ`); 7-day retention before pruning |

## P1 picked up along the way

- **R21** dependency graph: generated into [`modules.md`](modules.md) by the arch test.
- **R22** load profile: [`read_path.js`](../test/load/read_path.js), parameterised by `READ_PATH`. The baseline measured on the reference read path is in the metrics table below; the results file left with the modules.
- **R23** outbox dead-letter table + replay: `TestNackedRowBacksOffWithoutStallingOtherTopics`, `TestRowExhaustingPublishAttemptsIsDeadLettered` ([`dispatcher_deadletter_test.go`](../internal/platform/outbox/dispatcher_deadletter_test.go)), store round-trip in [`deadletter_test.go`](../internal/platform/outbox/deadletter_test.go); consumer mirror `TestDeadLetteredDeliveryIsReportedToSink` (bus) and `TestJobExhaustingLadderIsReportedToSink` (jobs); `task outbox:deadletter:list` / `task outbox:deadletter:replay`.

Deliberately not built (P1/P2, per §10): R24 per-module rate limits (the
limit is global config today), R25 contract tests, R26 role administration
UI/API — the reference user module shipped one default role and said so,
R27 Atlas migration linting, R28–R33 design-for-only.

## Success metrics (PRD §11, leading)

Measured 2026-09-01 on Apple `Mac17,7` (18 cores, 64 GB), with the three
reference modules in the tree.

| Metric | Target | Actual |
| --- | --- | --- |
| Scaffold + wire a module | < 15 min | `task new-module` + one registry line; the acceptance run took under a minute |
| Boundary violations reachable on `main` | 0 | 0 — three layers, each demonstrated |
| Unit suite runtime (no Docker) | < 10s | **9.6s** warm build cache, 13.5s cold. Test *execution* is ~2s; the rest is compiling. Watch this one: it is the metric most likely to slip first |
| Full suite incl. integration | < 3 min | **38s** (`go test ./... -race -count=1`, testcontainers, cold test cache; e2e adds ~10s with `E2E=1`) |
| Cold boot to serving | < 2s | 0.68s from exec to `/health` 200 |
| p99 on the cached read path | < 200ms at 7k RPS | 2.81ms at 12,132 RPS against `GET /users/{id}` through the cached repository decorator |
