# Go Modular Monolith Blueprint

A reusable template for a single-deployable Go service with compiler-enforced module
boundaries, designed so any module can be extracted to its own binary later by swapping
one adapter and one config value — without paying the distributed-systems tax up front.

The template ships with **no business modules**: [`docs/examples.md`](docs/examples.md) is the cookbook, with real excerpts of the three reference modules (user, wallet, notification) that demonstrated every pattern, and `task new-module` scaffolds a new one.

Everything here follows the PRD in [`go-modular-monolith-blueprint-prd.md`](go-modular-monolith-blueprint-prd.md).
The load-bearing decisions are its §4 principles: compiler-private module internals,
producer-owned event contracts, consumer-owned ports, one-transaction-one-module via a
Postgres outbox, schema-per-module, and a locked boring stack (chi · GORM/pgx · wire ·
goose · casbin · RabbitMQ · dual Redis · zap · OTel).

## Quick start

```bash
brew install go go-task golangci-lint k6 && brew install --cask orbstack
cp .env.example .env
task docker:up      # postgres + pgbouncer + redis-core + redis-cache + rabbitmq
task migrate        # per-module schemas via goose
task run            # API on :8080, admin (metrics/pprof) on :8081
task new-module NAME=billing   # scaffold a module in <15 min end-to-end
```

`task check` runs exactly what CI runs: lint (depguard boundaries), arch tests
(import-graph assertions), unit tests, and generated-code drift checks.

Architecture decisions live in `docs/adr/`. Operational thresholds and failure
playbooks live in `docs/runbook.md`.

Taking this repository as the starting point for your own service? Follow [`docs/getting-started.md`](docs/getting-started.md): toolchain, first run, renaming the module, configuration, what to keep or delete, and the production checklist.
