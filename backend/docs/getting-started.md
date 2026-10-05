# Getting started: using this blueprint for your own service

This repository is a template, not a product. Everything in it runs as-is so you can see the patterns working before you change anything. This page is the ordered list of what to install, what to run first, what to rename, what to configure, what to delete, and what to add. Follow it top to bottom once; after that `CLAUDE.md` and `docs/runbook.md` are the day-to-day references.

## 1. Install the toolchain

| Tool | Version | Why |
| --- | --- | --- |
| Go | 1.26 or newer (`go.mod` pins 1.26) | build and test |
| Docker with Compose v2 | any current | local stack via `deploy/compose.yaml`, and testcontainers for integration tests. OrbStack or Docker Desktop on macOS |
| go-task | 3.x | every command is a task target; CI runs the same targets |
| golangci-lint | v2.13 (config is the v2 format) | lint plus the depguard boundary rules |
| wire | latest | regenerates `internal/app/wire_gen.go` |
| mockery | v2.53.6 (pinned in CI) | port mocks under `**/mocks/` |
| swag | latest | OpenAPI under `api/docs` |
| goose CLI | v3 | `task migrate:new` shells out to it; applying migrations uses `cmd/migrate`, not the CLI |
| air | any current | hot reload for `task run` |
| k6 | any current | `task load` |

macOS:

```bash
brew install go go-task golangci-lint k6 goose
brew install --cask orbstack
go install github.com/google/wire/cmd/wire@latest
go install github.com/vektra/mockery/v2@v2.53.6
go install github.com/swaggo/swag/cmd/swag@latest
go install github.com/air-verse/air@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`, otherwise `task generate` and `task run` cannot find the Go-installed tools.

Ports the local stack binds on the host. Free them or change the mappings in `deploy/compose.yaml` and `.env` together:

| Port | Service |
| --- | --- |
| 8080 | API |
| 8081 | admin: `/metrics`, pprof |
| 6432 | PgBouncer (writer DSN) |
| 15432 | Postgres direct (reader and migrate DSNs) |
| 6380 | redis-core |
| 6381 | redis-cache |
| 5672, 15672 | RabbitMQ AMQP and management UI |

## 2. Run the blueprint unchanged

Prove the stack works on your machine before you touch it.

```bash
cp .env.example .env
task docker:up        # waits for health checks
task migrate          # platform schemas, then each enabled module's schema, then module DB roles
task run              # API, hot reload
```

In separate terminals:

```bash
task dispatcher       # outbox → RabbitMQ; nothing reaches the broker without it
task worker           # event subscribers and job consumers
task scheduler        # cron enqueuer; optional locally
```

Then:

```bash
curl -s localhost:8080/health | jq        # {"status":"ok","modules":{}} until you enable a module
task check                                # lint + arch + unit tests + generated-code drift
E2E=1 go test ./test/e2e/ -count=1        # boots migrate, api, dispatcher, worker and checks /health
```

The Bruno collection under `api/bruno` has a health request against `http://localhost:8080`; add your module's requests next to it. Swagger JSON is served from `api/docs`.

If `task load` reports mostly 429 responses, raise `HTTP_RATE_LIMIT_PER_MINUTE` in `.env` for the run; the default of 600 per minute is per principal and the load profile exceeds it on purpose. If `task migrate` fails to connect, check that `DB_MIGRATE_DSN` points at port 15432, not PgBouncer: goose sets `search_path`, which a transaction pooler does not preserve.

## 3. Rename the Go module

The module path is `myapp`. Keeping it is a legitimate choice for an internal service. If you rename it, do it now, before any of your own code exists, because the name appears in 109 Go files and in several non-Go places that a plain import rewrite misses.

Occurrences with a slash (`levelup/...`) are import paths and tool paths. Occurrences of the bare word are display names.

```bash
NEW=github.com/acme/shop        # your module path

# 1. Everything of the form levelup/... : imports, go.mod for tools, Taskfile,
#    .golangci.yml, .mockery.yaml, test/arch/helpers.go, tools/modgen/main.go, docs.
grep -rl 'levelup/' --include='*.go' --include='*.mod' --include='*.yml' --include='*.yaml' --include='*.md' . \
  | grep -v -e '^./go.sum' -e '^./tools/go.sum' -e '^./docs/getting-started.md' \
  | xargs sed -i '' "s#levelup/#${NEW}/#g"          # GNU sed: drop the '' after -i

# 2. The root go.mod module line has no slash.
sed -i '' "s#^module myapp\$#module ${NEW}#" go.mod

# 3. Bare display names. Pick whatever short name you like for these.
SHORT=shop
sed -i '' "s#\"myapp\"#\"${SHORT}\"#g" internal/platform/telemetry/telemetry.go   # OTel service name and tracer
sed -i '' "s#myapp API#${SHORT} API#" cmd/api/main.go                             # swagger title
sed -i '' "s#^name: myapp#name: ${SHORT}#" deploy/compose.yaml                     # compose project name
sed -i '' "s#\"name\": \"myapp\"#\"name\": \"${SHORT}\"#" api/bruno/bruno.json

# 4. Regenerate and verify.
go mod tidy && (cd tools && go mod tidy)
task generate
task check
```

Two places are load-bearing rather than cosmetic and are covered by step 1, but check them if anything fails afterwards: `tools/modgen/main.go` finds the repository root by looking for a `go.mod` that is not the tools module, and `test/arch/helpers.go` loads packages with the module prefix. Both must carry the new path or `task new-module` and `task arch` stop working.

## 4. Configure environment and secrets

`.env` is gitignored. `.env.example` is the documented contract: when you add a config key, add it there too. Config is typed and validated at boot, and a missing key fails naming the environment variable, so misconfiguration never reaches a listener.

Change before any shared environment:

- **`JWT_SECRET`**. Minimum 32 bytes, enforced at boot. Generate with `openssl rand -base64 48`.
- **Database credentials.** Compose uses `app`/`app` for Postgres and PgBouncer. Per-module Postgres roles (`user_svc`, `wallet_svc`, ...) are created by `task migrate` with password equal to the role name. They exist so integration tests can prove schema isolation; runtime traffic uses the DSN user. Either give them real passwords in your provisioning or drop `CreateModuleRole` from `Migrate` in `internal/app/lifecycle.go`.
- **RabbitMQ credentials.** `guest`/`guest` for AMQP and the management API. The management URL and credentials are used by `Topology.Verify` to refuse classic queues at boot, so they are needed in every environment, not only locally.
- **`APP_ENV`**. `prod` switches zap to the JSON production encoder. Anything else is the development console encoder.
- **`OTEL_EXPORTER_OTLP_ENDPOINT`**. Unset means spans are created with real trace ids that appear in logs but are never exported. Set it to export over OTLP HTTP.
- **`MODULES_ENABLED`**. Comma-separated, in dependency order. Empty is a valid, empty registry, which is how the template ships. A consumer listed before its provider panics at boot with a message that says so.
- **`HTTP_RATE_LIMIT_PER_MINUTE`**. Per principal, or per IP when anonymous. 600 is a placeholder.
- **`USER_DEFAULT_ROLE_ID`**. Every new user gets this role id in their token, and the permission seeds grant to it. Role administration is not built.
- **Three DSNs.** Writer through PgBouncer in transaction pooling mode. Reader and migrate direct to Postgres. Keep `DB_MAX_CONNS × replicas` under the PgBouncer pool, and that under Postgres `max_connections`.

## 5. Learn the module shapes from the cookbook

The template ships with no business modules and an empty registry, so there is nothing to delete before you start. [`examples.md`](examples.md) is the cookbook: real excerpts of the three reference modules that demonstrated the patterns.

- **`user`** was register, login, refresh with rotation and reuse detection, logout, a cached read repository, and the permission catalogue seed. Most services need this module first; sections 6, 9, 12, and 13 of the cookbook are its shape.
- **`wallet`** was the example of every cross-module mechanism at once: a consumer-defined port with local and remote adapters, an event subscription, a cron job with a reconcile marker, resource-scoped authorization, row locking, and optimistic versioning. Sections 2, 5, 6, 7, 10, and 11.
- **`notification`** was the thin-module shape: flat `internal/`, GORM model as the type, append-only subscriber. Section 14.

When you later remove a module of your own, in this order:

1. Delete `internal/modules/<name>`, its registry line, its `Config` embed, and any `.mockery.yaml` entry for its ports.
2. Remove it from `MODULES_ENABLED` in `.env` and `.env.example`.
3. Fix any tests, Bruno requests, or load profile that called its routes.
4. `task generate && task check`.

Its schema stays in any database that already ran the migrations. Drop `<name>_svc` and the `<name>_svc` role by hand, and delete its rows in `authz_svc.permissions` and `authz_svc.casbin_rule`, or write a platform migration that does. Boot refuses to start while the database grants a permission no enabled module declares.

## 6. Add your first module

```bash
task new-module NAME=order
```

The scaffolder writes `internal/modules/order` with `module.go`, `contracts/`, `internal/app`, `internal/repo`, `internal/transport`, and `migrations/0001_init.sql`, then runs lint and arch. After that:

1. Add `order.New(p.DepsFor("order"))` to `buildModules` in `internal/app/registry.go`. The scaffolded constructor takes only `Deps`.
2. When the module needs settings, create `internal/modules/order/config.go` with a `Config` struct, embed it as `Order order.Config` with `envPrefix:"ORDER_"` in `internal/config/config.go`, and extend `New` to take it, as `user` and `wallet` do.
3. Append `order` to `MODULES_ENABLED`, after any module it reads from.
4. Write the schema with `task migrate:new MODULE=order NAME=init`, then `task migrate`.
5. Declare permissions in `contracts/permissions.go` and seed them with a Go migration, copying `wallet/migrations/0002_seed_permissions.go`. Boot fails if the database grants a permission no enabled module declares.
6. If the module reads another module's data, add `internal/ports`, the aliases in `ports.go`, an adapter under `adapters/`, and a `.mockery.yaml` entry for the port. Mocks are for ports only.
7. Publish events only from the service inside `InTx` via `Deps.Outbox`. React to other modules only through `Subscriptions()`.

`task arch` regenerates `docs/modules.md` with the dependency graph, so the new edge shows up on the next run.

## 7. Repository housekeeping

- **CI branch filter.** `.github/workflows/ci.yml` runs on pushes to `main`; this repository's default branch is `master`. Set the filter to your default branch or the push job never runs. Pull requests trigger regardless.
- **Pinned tool versions in CI.** The workflow installs `wire@latest` and `swag@latest`. Pin them once `generate:verify` has passed with a known version, or a tool release will fail the drift check on an unrelated day.
- **The PRD.** `go-modular-monolith-blueprint-prd.md` is cited from code comments as `PRD §N`. Keep it, or move it to `docs/` and update the README link. Deleting it turns hundreds of comments into dangling references.
- **Blueprint-only documents.** `docs/superpowers/plans/` is the build log for this template. `docs/acceptance.md` and `docs/enforcement-proof.md` are proofs the blueprint met its own requirements, measured while the reference modules were in the tree. Delete or re-measure against your own modules.
- **README.** Rewrite it for your service. The quick-start block is still accurate.
- **`api/bruno`.** Rename the collection and add environments for anything beyond local.
- **Committed generated code.** `internal/app/wire_gen.go`, `api/docs`, and every `mocks/` directory are committed and checked for drift. Never edit them; run `task generate`.
- **Go module for tools.** `tools/go.mod` isolates the scaffolder's template engine from the service dependency graph. Keep the split; do not merge it into the root module.

## 8. Before the first real deployment

No Dockerfile or Kubernetes manifests ship with the blueprint. `deploy/compose.yaml` is the local stack only. What the code assumes about production, drawn from `docs/runbook.md` and the boot checks:

- **Five binaries** from `cmd/`: `api`, `worker`, `dispatcher`, `scheduler`, `migrate`. One multi-stage image with `go build ./cmd/...` and a different entrypoint per deployment is enough.
- **Migrate runs first**, as a job, against the direct DSN, before the new version of the serving binaries rolls out. It is idempotent.
- **Replicas.** `api`, `worker`, and `dispatcher` scale horizontally. `scheduler` runs at exactly one replica; its per-tick Redis lock covers the overlap during a rolling deploy.
- **RabbitMQ.** Three nodes minimum, `pause_minority`, quorum queues. The dispatcher and worker call `Topology.Verify` at boot and refuse to start if a classic queue exists other than `job.priority`.
- **Two Redis instances with different eviction policies.** Core: `noeviction` with AOF. Cache: `allkeys-lru`. The Go types are distinct so they cannot be swapped in code, but the policies live in your infrastructure config.
- **PgBouncer in transaction pooling** in front of the writer. `PrepareStmt: false` and the pgx simple protocol are set for that reason; do not revert them.
- **The admin port stays private.** `/metrics` and pprof are served on `HTTP_ADMIN_ADDR` only.
- **Alerts.** `outbox_unpublished_age_seconds` above 60s, any positive rate on `rabbitmq_dlq_total`, `pgxpool_acquire_wait_seconds` p99 above 50ms, `rabbitmq_connection_blocked` held for 30s. Thresholds and the response to each are in the runbook.

## 9. Done when

- `task check` passes on a clean clone.
- `E2E=1 go test ./test/e2e/` passes against `task docker:up` with your module list.
- `curl localhost:8080/health` lists exactly the modules you enabled, and `{}` before you enable any.
- `docs/modules.md` shows only edges you intended.
- `.env.example` documents every key your `.env` contains.
