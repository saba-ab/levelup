# Implementation guide: LevelUpOS Go modules

This guide is binding for everyone implementing a module in `backend/internal/modules/<name>`, human or agent.
The architecture and the reasoning behind it are in `00-target-architecture.md`. The as-is Laravel
behaviour and the per-domain Go design are in docs 01–06. The blueprint rules are in `backend/CLAUDE.md`,
and the worked patterns are in `backend/docs/examples.md`, which is the cookbook. Copy its shapes.

## 0. Ground rules

- **Contracts are already written** (`internal/modules/*/contracts/contracts.go`) and are the API between
  modules.
  - In your own module's contracts you may add fields, types and constants. Never rename or remove anything
    that is already there.
  - Never edit another module's contracts. If you need a change there, put it in your final report.
- **Edit only `backend/internal/modules/<your module>/**`.**
  - Do not touch `internal/app`, `internal/config`, `internal/platform`, `internal/shared`, `.mockery.yaml`,
    `go.mod`, `Taskfile.yml`, or other modules.
  - The integrator wires the registry and config and adds mockery entries afterwards.
  - Adding a dependency needs an ADR (ADR-0009), so don't.
- **Several modules are written in parallel in the same tree.** Build, lint and test only your own paths:
  ```bash
  cd backend
  go build ./internal/modules/<name>/...
  go vet ./internal/modules/<name>/...
  go test ./internal/modules/<name>/... -short -race
  golangci-lint run ./internal/modules/<name>/...
  go test ./test/arch/...   # failures in OTHER modules are not yours; fix only your own
  ```
  Never run `go mod tidy`, `task generate`, or `gofmt -w` outside your module directory.
- Docker may not be running. Repository tests must use `pgtest.DSN(t)` and skip themselves under `-short`,
  like the platform's own tests do. Write them anyway; the integrator runs them.

## 1. Module shape

Follow the rich shape in `docs/examples.md` §1. The `eventcatalog` and `activity` modules may use the thin
shape in §14.

```
internal/modules/<name>/
  module.go        New(d modkit.Deps, cfg Config, <ports...>) *Module; implements modkit.Module
                   + postgres.GoMigrator (GoMigrations) for seeds
                   + provider modules expose  func (m *Module) Reader() contracts.Reader
                     (identity: TenantReader())
  config.go        type Config struct { ... `env:"X" envDefault:"..."` }  (integrator embeds it with envPrefix "<NAME>_")
  ports.go         type aliases of internal/ports interfaces, so the registry can name them
  adapters/        local adapters that wrap another module's contracts.Reader into your port
                   e.g. adapters.NewLocalPlayers(playercontracts.Reader) ports.PlayerReader
  contracts/       (exists)
  internal/domain  entities, invariants, errors.go (package-level error vars), no tags
  internal/app     services: tx boundaries, authz, outbox publishes, job/subscription handlers
  internal/ports   consumer-defined ports + snapshot types (your own projection, never the provider's type)
  internal/repo    unexported GORM models (table name derived from struct name; NO TableName()), toDomain/fromDomain
  internal/transport  chi handlers, DTOs with validate tags, swag annotations
  migrations/      0001_init.sql (goose), embed.go, seeds.go (Go migrations: permissions + default grants)
```

`Deps.DB` is already prefixed with `<name>_svc.`. A model struct named `wallet` maps to the table
`points_svc.wallets`. Make the SQL table names match GORM's pluralisation.

## 2. Tenancy (ADR-0015)

- Every tenant-owned table has `tenant_id UUID NOT NULL`. Global rows (`tenant_id IS NULL`) exist only for
  eventcatalog types and categories.
- Every HTTP-facing service method starts with `p, err := authz.RequireTenant(ctx)`.
  - Repositories take `tenantID` explicitly: `ByID(ctx, tenantID, id)`.
  - Another tenant's row returns `errs.NotFound`.
- Async handlers take the tenant from the payload. Never from anything else.
- `tenant_id`, `created_by` and similar are stamped by the service from the principal. Never from the request
  body.
- Every tenant-owned module subscribes to `tenant.deleted.v1` (identity contracts) and deletes its rows for
  that tenant idempotently. The consumer group is the module name.
- Uniqueness is per tenant: `UNIQUE (tenant_id, slug) WHERE deleted_at IS NULL`.

## 3. Authorization

- Permissions are the `Perm*` vars in your contracts. Check them in the service with
  `s.authz.Authorize(ctx, p, contracts.PermX, entity)`.
- Seed them in a Go migration, as in `docs/examples.md` §9:
  - insert into `authz_svc.permissions`;
  - grant the permissions commented `// admin roles` to `identitycontracts.AdminRoles`, and all the others to
    `identitycontracts.MemberRoles`;
  - permissions marked "platform" go only to `RolePlatformAdmin`.
- Go migration version numbers must not collide with your SQL files. Use `0001_init.sql`, then Go versions
  2 and 3.
- Route groups use `r.Use(httpx.RequireAuth)`.

## 4. HTTP conventions (ADR-0016)

- Mounted under `/api/v1` by the composition root. You mount your own prefix: `r.Route("/players", ...)`.
- JSON is snake_case. Ids are UUID strings. Times are RFC 3339 UTC.
- Create returns 201 with the resource. Update is `PATCH` with partial fields; use pointer DTO fields so
  omitted fields stay untouched (the Laravel `PUT` nulled them). Delete returns 204. Async acceptance returns
  202.
- Lists take `?limit=` (default 25, max 100) and `?cursor=`. They respond with
  `{"data":[...],"next_cursor":"..."}` (empty string when done), using `shared/pagination` over
  `(created_at, id)` DESC. Filters are query params.
- Validate request shape only in DTOs, via `httpx.Decode` and `validate` tags. Business rules belong in the
  domain or service.
- Errors are `errs.New` / `errs.Wrap` with the right kind, plus `errs.WithCode(err, "snake_case_code")` for
  every business error a client may branch on (`insufficient_balance`, `badge_already_earned`,
  `invalid_status_transition`, ...). Render with `httpx.Error`.
- Every handler gets swag annotations, as in the scaffold stub.
- Money-like and award endpoints should accept the `Idempotency-Key` header. Platform middleware already
  handles it.

## 5. Events, commands, idempotency

- **Publish** only inside the owning transaction: `s.outbox.Publish(ctx, tx, topic, payload)`.
- **Commands to another module** use the target's contracts:
  `s.outbox.Publish(ctx, tx, pointscontracts.Topic(pointscontracts.JobCredit), pointscontracts.CreditCmdV1{...})`.
  - Derive the `IdempotencyKey` deterministically from your own ids: `id.Derive("mission_completion", attemptID, "points")`.
  - Set `Source` (`shared/effect`).
  - Issue the command in the same tx that records the fact which justifies it.
- **Consuming a command.** Declare it in `Jobs()` as `jobs.Job{Name: contracts.JobX, Run: ...}` with no
  Schedule.
  - Decode with `jobs.DecodeCommand(body, &cmd)`.
  - Malformed payload or invalid values → `errs.Invalid` (immediate DLQ).
  - Apply in one tx with `UNIQUE (tenant_id, idempotency_key)` on your ledger or award row
    (`ON CONFLICT DO NOTHING`). On conflict, do nothing and publish nothing.
  - Business rejections (player inactive, already earned, insufficient balance) are recorded and published as
    `<module>.<verb>_rejected.v1`, then you return nil.
  - Transient errors → return them; the retry ladder handles them.
- **Subscriptions** (`Subscriptions()`, `Group: "<module>"`) must be idempotent and tolerate redelivery and
  reordering (ADR-0012). Prefer upserts keyed by natural keys, or an `applied_events(event_id)` table.
- **Money and award rows** take a row lock (`clause.Locking{Strength: "UPDATE"}`) and carry a `version`
  column.
- **Cache eviction** happens after commit, never inside `InTx`.
- **Cron jobs** (`Schedule` set) must be reconciling and sweep from a last-successful-run marker (R47).

## 6. Player checks

Mechanics that apply commands to a player read the player through a `PlayerReader` port wrapping
`playercontracts.Reader`:
- player not found → reject with `effect.ReasonPlayerNotFound`;
- player inactive → reject with `effect.ReasonPlayerInactive`.

HTTP endpoints that take a `player_id` validate it the same way (404 `player_not_found`).

## 7. Tests

- Use `require`, never `assert`.
- Domain: table-driven invariant tests.
- Service: hand-written fake repo, fake outbox recording topics and payloads, `clock.NewFake`, fakes for
  ports. Cover:
  - happy paths;
  - authz denial;
  - cross-tenant 404;
  - idempotent redelivery of every command and subscription (applying twice gives one row and one publish);
  - rejection paths;
  - every bug listed in your domain doc's "do not port blindly" section that your design fixes.
- Repository: `pgtest.DSN(t)` tests for the locking or idempotency SQL, skipped under `-short`.

## 8. Final report (what you return)

1. Files created.
2. The exact registry line: the constructor call, which ports it needs, and which provider `Reader()`
   satisfies each.
3. Config fields and defaults.
4. Every subscription and every job, with schedules.
5. The endpoint list.
6. Deviations from the docs, and contract additions.
7. Anything you could not finish, and the command output proving build, vet, test and lint pass for your module.
